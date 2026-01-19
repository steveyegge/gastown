// Liftoff test: 2026-01-09T14:30:00

package polecat

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Common errors
var (
	ErrPolecatExists     = errors.New("polecat already exists")
	ErrPolecatNotFound   = errors.New("polecat not found")
	ErrHasChanges        = errors.New("polecat has uncommitted changes")
	ErrHasUncommittedWork = errors.New("polecat has uncommitted work")
)

// UncommittedWorkError provides details about uncommitted work.
type UncommittedWorkError struct {
	PolecatName string
	Status      *git.UncommittedWorkStatus
}

func (e *UncommittedWorkError) Error() string {
	return fmt.Sprintf("polecat %s has uncommitted work: %s", e.PolecatName, e.Status.String())
}

func (e *UncommittedWorkError) Unwrap() error {
	return ErrHasUncommittedWork
}

// Manager handles polecat lifecycle.
type Manager struct {
	rig       *rig.Rig
	git       *git.Git
	beads     *beads.Beads // Rig-level beads for issue operations
	townBeads *beads.Beads // Town-level beads for agent bead operations (hq- prefix)
	namePool  *NamePool
	tmux      *tmux.Tmux

	// allocMu protects name allocation within a single process.
	// File locking (in AllocateName) handles cross-process synchronization.
	allocMu sync.Mutex
}

// NewManager creates a new polecat manager.
func NewManager(r *rig.Rig, g *git.Git, t *tmux.Tmux) *Manager {
	// Use the resolved beads directory to find where bd commands should run.
	// For tracked beads: rig/.beads/redirect -> mayor/rig/.beads, so use mayor/rig
	// For local beads: rig/.beads is the database, so use rig root
	resolvedBeads := beads.ResolveBeadsDir(r.Path)
	beadsPath := filepath.Dir(resolvedBeads) // Get the directory containing .beads

	// Town beads for agent bead operations (hq- prefix).
	// Agent beads are coordination artifacts stored at town level to avoid
	// prefix/database mismatch issues (fix for loc-1augh).
	townRoot := filepath.Dir(r.Path)
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Try to load rig settings for namepool config
	settingsPath := filepath.Join(r.Path, "settings", "config.json")
	var pool *NamePool

	settings, err := config.LoadRigSettings(settingsPath)
	if err == nil && settings.Namepool != nil {
		// Use configured namepool settings
		pool = NewNamePoolWithConfig(
			r.Path,
			r.Name,
			settings.Namepool.Style,
			settings.Namepool.Names,
			settings.Namepool.MaxBeforeNumbering,
		)
	} else {
		// Use defaults
		pool = NewNamePool(r.Path, r.Name)
	}
	_ = pool.Load() // non-fatal: state file may not exist for new rigs

	return &Manager{
		rig:       r,
		git:       g,
		beads:     beads.NewWithBeadsDir(beadsPath, resolvedBeads),
		townBeads: beads.NewWithBeadsDir(townRoot, townBeadsDir),
		namePool:  pool,
		tmux:      t,
	}
}

// assigneeID returns the beads assignee identifier for a polecat.
// Format: "rig/polecatName" (e.g., "gastown/Toast")
func (m *Manager) assigneeID(name string) string {
	return fmt.Sprintf("%s/%s", m.rig.Name, name)
}

// agentBeadID returns the agent bead ID for a polecat.
// Format: "hq-<rig>-polecat-<name>" (e.g., "hq-gastown-polecat-Toast")
// Polecat agent beads are stored at town-level with hq- prefix to avoid
// prefix/database mismatch issues (fix for loc-1augh).
func (m *Manager) agentBeadID(name string) string {
	return beads.PolecatBeadIDTown(m.rig.Name, name)
}

// getCleanupStatusFromBead reads the cleanup_status from the polecat's agent bead.
// Returns CleanupUnknown if the bead doesn't exist or has no cleanup_status.
// ZFC #10: This is the ZFC-compliant way to check if removal is safe.
func (m *Manager) getCleanupStatusFromBead(name string) CleanupStatus {
	agentID := m.agentBeadID(name)
	_, fields, err := m.townBeads.GetAgentBead(agentID)
	if err != nil || fields == nil {
		return CleanupUnknown
	}
	if fields.CleanupStatus == "" {
		return CleanupUnknown
	}
	return CleanupStatus(fields.CleanupStatus)
}

// checkCleanupStatus validates the cleanup status against removal safety rules.
// Returns an error if removal should be blocked based on the status.
// force=true: allow has_uncommitted, block has_stash and has_unpushed
// force=false: block all non-clean statuses
func (m *Manager) checkCleanupStatus(name string, status CleanupStatus, force bool) error {
	// Clean status is always safe
	if status.IsSafe() {
		return nil
	}

	// With force, uncommitted changes can be bypassed
	if force && status.CanForceRemove() {
		return nil
	}

	// Map status to appropriate error
	switch status {
	case CleanupUncommitted:
		return &UncommittedWorkError{
			PolecatName: name,
			Status:      &git.UncommittedWorkStatus{HasUncommittedChanges: true},
		}
	case CleanupStash:
		return &UncommittedWorkError{
			PolecatName: name,
			Status:      &git.UncommittedWorkStatus{StashCount: 1},
		}
	case CleanupUnpushed:
		return &UncommittedWorkError{
			PolecatName: name,
			Status:      &git.UncommittedWorkStatus{UnpushedCommits: 1},
		}
	default:
		// Unknown status - be conservative and block
		return &UncommittedWorkError{
			PolecatName: name,
			Status:      &git.UncommittedWorkStatus{HasUncommittedChanges: true},
		}
	}
}

// repoBase returns the git directory and Git object to use for worktree operations.
// Prefers the shared bare repo (.repo.git) if it exists, otherwise falls back to mayor/rig.
// The bare repo architecture allows all worktrees (refinery, polecats) to share branch visibility.
func (m *Manager) repoBase() (*git.Git, error) {
	// First check for shared bare repo (new architecture)
	bareRepoPath := filepath.Join(m.rig.Path, ".repo.git")
	if info, err := os.Stat(bareRepoPath); err == nil && info.IsDir() {
		// Bare repo exists - use it
		return git.NewGitWithDir(bareRepoPath, ""), nil
	}

	// Fall back to mayor/rig (legacy architecture)
	mayorPath := filepath.Join(m.rig.Path, "mayor", "rig")
	if _, err := os.Stat(mayorPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no repo base found (neither .repo.git nor mayor/rig exists)")
	}
	return git.NewGit(mayorPath), nil
}

// polecatDir returns the parent directory for a polecat.
// This is polecats/<name>/ - the polecat's home directory.
func (m *Manager) polecatDir(name string) string {
	return filepath.Join(m.rig.Path, "polecats", name)
}

// clonePath returns the path where the git worktree lives.
// New structure: polecats/<name>/<rigname>/ - gives LLMs recognizable repo context.
// Falls back to old structure: polecats/<name>/ for backward compatibility.
func (m *Manager) clonePath(name string) string {
	// New structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(m.rig.Path, "polecats", name, m.rig.Name)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		return newPath
	}

	// Old structure: polecats/<name>/ (backward compat)
	oldPath := filepath.Join(m.rig.Path, "polecats", name)
	if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
		// Check if this is actually a git worktree (has .git file or dir)
		gitPath := filepath.Join(oldPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return oldPath
		}
	}

	// Default to new structure for new polecats
	return newPath
}

// exists checks if a polecat exists.
func (m *Manager) exists(name string) bool {
	_, err := os.Stat(m.polecatDir(name))
	return err == nil
}

// AddOptions configures polecat creation.
type AddOptions struct {
	HookBead string // Bead ID to set as hook_bead at spawn time (atomic assignment)
}

// Add creates a new polecat as a git worktree from the repo base.
// Uses the shared bare repo (.repo.git) if available, otherwise mayor/rig.
// This is much faster than a full clone and shares objects with all worktrees.
// Polecat state is derived from beads assignee field, not state.json.
//
// Branch naming: Each polecat run gets a unique branch (polecat/<name>-<timestamp>).
// This prevents drift issues from stale branches and ensures a clean starting state.
// Old branches are ephemeral and never pushed to origin.
func (m *Manager) Add(name string) (*Polecat, error) {
	return m.AddWithOptions(name, AddOptions{})
}

// AddWithOptions creates a new polecat with the specified options.
// This allows setting hook_bead atomically at creation time, avoiding
// cross-beads routing issues when slinging work to new polecats.
func (m *Manager) AddWithOptions(name string, opts AddOptions) (*Polecat, error) {
	if m.exists(name) {
		return nil, ErrPolecatExists
	}

	// New structure: polecats/<name>/<rigname>/ for LLM ergonomics
	// The polecat's home dir is polecats/<name>/, worktree is polecats/<name>/<rigname>/
	polecatDir := m.polecatDir(name)
	clonePath := filepath.Join(polecatDir, m.rig.Name)

	// Branch naming: include issue ID when available for better traceability.
	// Format: polecat/<worker>/<issue>@<timestamp> when HookBead is set
	// The @timestamp suffix ensures uniqueness if the same issue is re-slung.
	// parseBranchName strips the @suffix to extract the issue ID.
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)
	var branchName string
	if opts.HookBead != "" {
		branchName = fmt.Sprintf("polecat/%s/%s@%s", name, opts.HookBead, timestamp)
	} else {
		// Fallback to timestamp format when no issue is known at spawn time
		branchName = fmt.Sprintf("polecat/%s-%s", name, timestamp)
	}

	// Create polecat directory (polecats/<name>/)
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		return nil, fmt.Errorf("creating polecat dir: %w", err)
	}

	// Get the repo base (bare repo or mayor/rig)
	repoGit, err := m.repoBase()
	if err != nil {
		return nil, fmt.Errorf("finding repo base: %w", err)
	}

	// Fetch latest from origin to ensure worktree starts from up-to-date code
	if err := repoGit.Fetch("origin"); err != nil {
		// Non-fatal - proceed with potentially stale code
		fmt.Printf("Warning: could not fetch origin: %v\n", err)
	}

	// Determine the start point for the new worktree
	// Use origin/<default-branch> to ensure we start from the rig's configured branch
	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(m.rig.Path); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}
	startPoint := fmt.Sprintf("origin/%s", defaultBranch)

	// Always create fresh branch - unique name guarantees no collision
	// git worktree add -b polecat/<name>-<timestamp> <path> <startpoint>
	// Worktree goes in polecats/<name>/<rigname>/ for LLM ergonomics
	if err := repoGit.WorktreeAddFromRef(clonePath, branchName, startPoint); err != nil {
		return nil, fmt.Errorf("creating worktree from %s: %w", startPoint, err)
	}

	// Verify worktree was actually created (guards against silent failures).
	// Issue: gt sling reports success but worktree never created.
	if err := verifyWorktree(clonePath); err != nil {
		// Clean up the parent directory we created
		_ = os.RemoveAll(polecatDir)
		return nil, fmt.Errorf("verifying worktree at %s: %w", clonePath, err)
	}

	// Ensure AGENTS.md exists - critical for polecats to "land the plane"
	// Fall back to copy from mayor/rig if not in git (e.g., stale fetch, local-only file)
	agentsMDPath := filepath.Join(clonePath, "AGENTS.md")
	if _, err := os.Stat(agentsMDPath); os.IsNotExist(err) {
		srcPath := filepath.Join(m.rig.Path, "mayor", "rig", "AGENTS.md")
		if srcData, readErr := os.ReadFile(srcPath); readErr == nil {
			if writeErr := os.WriteFile(agentsMDPath, srcData, 0644); writeErr != nil {
				fmt.Printf("Warning: could not copy AGENTS.md: %v\n", writeErr)
			}
		}
	}

	// NOTE: We intentionally do NOT write to CLAUDE.md here.
	// Gas Town context is injected ephemerally via SessionStart hook (gt prime).
	// Writing to CLAUDE.md would overwrite project instructions and could leak
	// Gas Town internals into the project repo if merged.

	// Set up shared beads: polecat uses rig's .beads via redirect file.
	// This eliminates git sync overhead - all polecats share one database.
	if err := m.setupSharedBeads(clonePath); err != nil {
		// Non-fatal - polecat can still work with local beads
		// Log warning but don't fail the spawn
		fmt.Printf("Warning: could not set up shared beads: %v\n", err)
	}

	// Provision PRIME.md with Gas Town context for this worker.
	// This is the fallback if SessionStart hook fails - ensures polecats
	// always have GUPP and essential Gas Town context.
	if err := beads.ProvisionPrimeMDForWorktree(clonePath); err != nil {
		// Non-fatal - polecat can still work via hook, warn but don't fail
		fmt.Printf("Warning: could not provision PRIME.md: %v\n", err)
	}

	// Copy overlay files from .runtime/overlay/ to polecat root.
	// This allows services to have .env and other config files at their root.
	if err := rig.CopyOverlay(m.rig.Path, clonePath); err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("Warning: could not copy overlay files: %v\n", err)
	}

	// Run setup hooks from .runtime/setup-hooks/.
	// These hooks can inject local git config, copy secrets, or perform other setup tasks.
	if err := rig.RunSetupHooks(m.rig.Path, clonePath); err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("Warning: could not run setup hooks: %v\n", err)
	}

	// NOTE: Slash commands (.claude/commands/) are provisioned at town level by gt install.
	// All agents inherit them via Claude's directory traversal - no per-workspace copies needed.

	// Create or reopen agent bead for ZFC compliance (self-report state).
	// State starts as "spawning" - will be updated to "working" when Claude starts.
	// HookBead is set atomically at creation time if provided (avoids cross-beads routing issues).
	// Uses CreateOrReopenAgentBead to handle re-spawning with same name (GH #332).
	agentID := m.agentBeadID(name)
	_, err = m.townBeads.CreateOrReopenAgentBead(agentID, agentID, &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        m.rig.Name,
		AgentState: "spawning",
		RoleBead:   beads.RoleBeadIDTown("polecat"),
		HookBead:   opts.HookBead, // Set atomically at spawn time
	})
	if err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("Warning: could not create agent bead: %v\n", err)
	}

	// Return polecat with working state (transient model: polecats are spawned with work)
	// State is derived from beads, not stored in state.json
	now := time.Now()
	polecat := &Polecat{
		Name:      name,
		Rig:       m.rig.Name,
		State:     StateWorking, // Transient model: polecat spawns with work
		ClonePath: clonePath,
		Branch:    branchName,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return polecat, nil
}

// Remove deletes a polecat worktree.
// If force is true, removes even with uncommitted changes (but not stashes/unpushed).
// Use nuclear=true to bypass ALL safety checks.
func (m *Manager) Remove(name string, force bool) error {
	return m.RemoveWithOptions(name, force, false)
}

// RemoveWithOptions deletes a polecat worktree with explicit control over safety checks.
// force=true: bypass uncommitted changes check (legacy behavior)
// nuclear=true: bypass ALL safety checks including stashes and unpushed commits
//
// ZFC #10: Uses cleanup_status from agent bead if available (polecat self-report),
// falls back to git check for backward compatibility.
func (m *Manager) RemoveWithOptions(name string, force, nuclear bool) error {
	if !m.exists(name) {
		return ErrPolecatNotFound
	}

	// Clone path is where the git worktree lives (new or old structure)
	clonePath := m.clonePath(name)
	// Polecat dir is the parent directory (polecats/<name>/)
	polecatDir := m.polecatDir(name)

	// Check for uncommitted work unless bypassed
	if !nuclear {
		// ZFC #10: First try to read cleanup_status from agent bead
		// This is the ZFC-compliant path - trust what the polecat reported
		cleanupStatus := m.getCleanupStatusFromBead(name)

		if cleanupStatus != CleanupUnknown {
			// ZFC path: Use polecat's self-reported status
			if err := m.checkCleanupStatus(name, cleanupStatus, force); err != nil {
				return err
			}
		} else {
			// Fallback path: Check git directly (for polecats that haven't reported yet)
			polecatGit := git.NewGit(clonePath)
			status, err := polecatGit.CheckUncommittedWork()
			if err == nil && !status.Clean() {
				// For backward compatibility: force only bypasses uncommitted changes, not stashes/unpushed
				if force {
					// Force mode: allow uncommitted changes but still block on stashes/unpushed
					if status.StashCount > 0 || status.UnpushedCommits > 0 {
						return &UncommittedWorkError{PolecatName: name, Status: status}
					}
				} else {
					return &UncommittedWorkError{PolecatName: name, Status: status}
				}
			}
		}
	}

	// Get repo base to remove the worktree properly
	repoGit, err := m.repoBase()
	if err != nil {
		// Best-effort: try to prune stale worktree entries from both possible repo locations.
		// This handles edge cases where the repo base is corrupted but worktree entries exist.
		bareRepoPath := filepath.Join(m.rig.Path, ".repo.git")
		if info, statErr := os.Stat(bareRepoPath); statErr == nil && info.IsDir() {
			bareGit := git.NewGitWithDir(bareRepoPath, "")
			_ = bareGit.WorktreePrune()
		}
		mayorRigPath := filepath.Join(m.rig.Path, "mayor", "rig")
		if info, statErr := os.Stat(mayorRigPath); statErr == nil && info.IsDir() {
			mayorGit := git.NewGit(mayorRigPath)
			_ = mayorGit.WorktreePrune()
		}
		// Fall back to direct removal if repo base not found
		return os.RemoveAll(polecatDir)
	}

	// Try to remove as a worktree first (use force flag for worktree removal too)
	if err := repoGit.WorktreeRemove(clonePath, force); err != nil {
		// Fall back to direct removal if worktree removal fails
		// (e.g., if this is an old-style clone, not a worktree)
		if removeErr := os.RemoveAll(clonePath); removeErr != nil {
			return fmt.Errorf("removing clone path: %w", removeErr)
		}
	} else {
		// GT-1L3MY9: git worktree remove may leave untracked directories behind.
		// Clean up any leftover files (overlay files, .beads/, setup hook outputs, etc.)
		// Use RemoveAll to handle non-empty directories with untracked files.
		_ = os.RemoveAll(clonePath)
	}

	// Also remove the parent polecat directory
	// (for new structure: polecats/<name>/ contains only polecats/<name>/<rigname>/)
	if polecatDir != clonePath {
		// GT-1L3MY9: Clean up any orphaned files at polecat level.
		// Use RemoveAll to handle non-empty directories with leftover files.
		_ = os.RemoveAll(polecatDir)
	}

	// Prune any stale worktree entries (non-fatal: cleanup only)
	_ = repoGit.WorktreePrune()

	// Release name back to pool if it's a pooled name (non-fatal: state file update)
	m.namePool.Release(name)
	_ = m.namePool.Save()

	// Close agent bead (non-fatal: may not exist or beads may not be available)
	// NOTE: We use CloseAndClearAgentBead instead of DeleteAgentBead because bd delete --hard
	// creates tombstones that cannot be reopened.
	agentID := m.agentBeadID(name)
	if err := m.townBeads.CloseAndClearAgentBead(agentID, "polecat removed"); err != nil {
		// Only log if not "not found" - it's ok if it doesn't exist
		if !errors.Is(err, beads.ErrNotFound) {
			fmt.Printf("Warning: could not close agent bead %s: %v\n", agentID, err)
		}
	}

	return nil
}

// AllocateName allocates a name from the name pool.
// Returns a pooled name (polecat-01 through polecat-50) if available,
// otherwise returns an overflow name (rigname-N).
//
// This function uses two levels of locking to ensure safe allocation:
// 1. Process-level mutex for concurrent goroutines within the same process
// 2. File-based locking for concurrent processes (e.g., parallel gt sling commands)
func (m *Manager) AllocateName() (string, error) {
	// First acquire process-level lock for goroutine safety
	m.allocMu.Lock()
	defer m.allocMu.Unlock()

	// Use file lock to synchronize across processes
	// This prevents race conditions when multiple gt sling commands run in parallel
	lockPath := filepath.Join(m.rig.Path, ".runtime", "namepool.lock")
	flock := util.NewFileLock(lockPath)

	var name string
	err := flock.WithLock(func() error {
		// First reconcile pool with existing polecats to handle stale state
		// This must happen inside the lock to get accurate filesystem state
		m.ReconcilePool()

		var allocErr error
		name, allocErr = m.namePool.Allocate()
		if allocErr != nil {
			return allocErr
		}

		if saveErr := m.namePool.Save(); saveErr != nil {
			return fmt.Errorf("saving pool state: %w", saveErr)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return name, nil
}

// ReleaseName releases a name back to the pool.
// This is called when a polecat is removed.
func (m *Manager) ReleaseName(name string) {
	m.namePool.Release(name)
	_ = m.namePool.Save() // non-fatal: state file update
}

// RepairWorktree repairs a stale polecat by removing it and creating a fresh worktree.
// This is NOT for normal operation - it handles reconciliation when AllocateName
// returns a name that unexpectedly already exists (stale state recovery).
//
// The polecat starts with the latest code from origin/<default-branch>.
// The name is preserved (not released to pool) since we're repairing immediately.
// force controls whether to bypass uncommitted changes check.
//
// Branch naming: Each repair gets a unique branch (polecat/<name>-<timestamp>).
// Old branches are left for garbage collection - they're never pushed to origin.
func (m *Manager) RepairWorktree(name string, force bool) (*Polecat, error) {
	return m.RepairWorktreeWithOptions(name, force, AddOptions{})
}

// RepairWorktreeWithOptions repairs a stale polecat and creates a fresh worktree with options.
// This is NOT for normal operation - see RepairWorktree for context.
// Allows setting hook_bead atomically at repair time.
// After repair, uses new structure: polecats/<name>/<rigname>/
func (m *Manager) RepairWorktreeWithOptions(name string, force bool, opts AddOptions) (*Polecat, error) {
	if !m.exists(name) {
		return nil, ErrPolecatNotFound
	}

	// Get the old clone path (may be old or new structure)
	oldClonePath := m.clonePath(name)
	polecatGit := git.NewGit(oldClonePath)

	// New clone path uses new structure
	polecatDir := m.polecatDir(name)
	newClonePath := filepath.Join(polecatDir, m.rig.Name)

	// Get the repo base (bare repo or mayor/rig)
	repoGit, err := m.repoBase()
	if err != nil {
		return nil, fmt.Errorf("finding repo base: %w", err)
	}

	// Check for uncommitted work unless forced
	if !force {
		status, err := polecatGit.CheckUncommittedWork()
		if err == nil && !status.Clean() {
			return nil, &UncommittedWorkError{PolecatName: name, Status: status}
		}
	}

	// Close old agent bead before recreation (non-fatal)
	// NOTE: We use CloseAndClearAgentBead instead of DeleteAgentBead because bd delete --hard
	// creates tombstones that cannot be reopened.
	agentID := m.agentBeadID(name)
	if err := m.townBeads.CloseAndClearAgentBead(agentID, "polecat repair"); err != nil {
		if !errors.Is(err, beads.ErrNotFound) {
			fmt.Printf("Warning: could not close old agent bead %s: %v\n", agentID, err)
		}
	}

	// Remove the old worktree (use force for git worktree removal)
	if err := repoGit.WorktreeRemove(oldClonePath, true); err != nil {
		// Fall back to direct removal
		if removeErr := os.RemoveAll(oldClonePath); removeErr != nil {
			return nil, fmt.Errorf("removing old clone path: %w", removeErr)
		}
	}

	// Prune stale worktree entries (non-fatal: cleanup only)
	_ = repoGit.WorktreePrune()

	// Fetch latest from origin to ensure we have fresh commits (non-fatal: may be offline)
	_ = repoGit.Fetch("origin")

	// Ensure polecat directory exists for new structure
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		return nil, fmt.Errorf("creating polecat dir: %w", err)
	}

	// Determine the start point for the new worktree
	// Use origin/<default-branch> to ensure we start from latest fetched commits
	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(m.rig.Path); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}
	startPoint := fmt.Sprintf("origin/%s", defaultBranch)

	// Create fresh worktree with unique branch name, starting from origin's default branch
	// Old branches are left behind - they're ephemeral (never pushed to origin)
	// and will be cleaned up by garbage collection
	// Branch naming: include issue ID when available for better traceability.
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)
	var branchName string
	if opts.HookBead != "" {
		branchName = fmt.Sprintf("polecat/%s/%s@%s", name, opts.HookBead, timestamp)
	} else {
		branchName = fmt.Sprintf("polecat/%s-%s", name, timestamp)
	}
	if err := repoGit.WorktreeAddFromRef(newClonePath, branchName, startPoint); err != nil {
		return nil, fmt.Errorf("creating fresh worktree from %s: %w", startPoint, err)
	}

	// Verify worktree was actually created (guards against silent failures).
	// Issue: gt sling reports success but worktree never created.
	if err := verifyWorktree(newClonePath); err != nil {
		return nil, fmt.Errorf("verifying worktree at %s: %w", newClonePath, err)
	}

	// Ensure AGENTS.md exists - critical for polecats to "land the plane"
	// Fall back to copy from mayor/rig if not in git (e.g., stale fetch, local-only file)
	agentsMDPath := filepath.Join(newClonePath, "AGENTS.md")
	if _, err := os.Stat(agentsMDPath); os.IsNotExist(err) {
		srcPath := filepath.Join(m.rig.Path, "mayor", "rig", "AGENTS.md")
		if srcData, readErr := os.ReadFile(srcPath); readErr == nil {
			if writeErr := os.WriteFile(agentsMDPath, srcData, 0644); writeErr != nil {
				fmt.Printf("Warning: could not copy AGENTS.md: %v\n", writeErr)
			}
		}
	}

	// NOTE: We intentionally do NOT write to CLAUDE.md here.
	// Gas Town context is injected ephemerally via SessionStart hook (gt prime).

	// Set up shared beads
	if err := m.setupSharedBeads(newClonePath); err != nil {
		fmt.Printf("Warning: could not set up shared beads: %v\n", err)
	}

	// Copy overlay files from .runtime/overlay/ to polecat root.
	if err := rig.CopyOverlay(m.rig.Path, newClonePath); err != nil {
		fmt.Printf("Warning: could not copy overlay files: %v\n", err)
	}

	// NOTE: Slash commands inherited from town level - no per-workspace copies needed.

	// Create or reopen agent bead for ZFC compliance
	// HookBead is set atomically at recreation time if provided.
	// Uses CreateOrReopenAgentBead to handle re-spawning with same name (GH #332).
	_, err = m.townBeads.CreateOrReopenAgentBead(agentID, agentID, &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        m.rig.Name,
		AgentState: "spawning",
		RoleBead:   beads.RoleBeadIDTown("polecat"),
		HookBead:   opts.HookBead, // Set atomically at spawn time
	})
	if err != nil {
		fmt.Printf("Warning: could not create agent bead: %v\n", err)
	}

	// Return fresh polecat in working state (transient model: polecats are spawned with work)
	now := time.Now()
	return &Polecat{
		Name:      name,
		Rig:       m.rig.Name,
		State:     StateWorking,
		ClonePath: newClonePath,
		Branch:    branchName,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ReconcilePool derives pool InUse state from existing polecat directories and active sessions.
// This implements ZFC: InUse is discovered from filesystem and tmux, not tracked separately.
// Called before each allocation to ensure InUse reflects reality.
//
// In addition to directory checks, this also:
// - Kills orphaned tmux sessions (sessions without directories are broken)
func (m *Manager) ReconcilePool() {
	// Get polecats with existing directories
	polecats, err := m.List()
	if err != nil {
		return
	}

	var namesWithDirs []string
	for _, p := range polecats {
		namesWithDirs = append(namesWithDirs, p.Name)
	}

	// Get names with tmux sessions
	var namesWithSessions []string
	if m.tmux != nil {
		poolNames := m.namePool.getNames()
		for _, name := range poolNames {
			sessionName := fmt.Sprintf("gt-%s-%s", m.rig.Name, name)
			hasSession, _ := m.tmux.HasSession(sessionName)
			if hasSession {
				namesWithSessions = append(namesWithSessions, name)
			}
		}
	}

	m.ReconcilePoolWith(namesWithDirs, namesWithSessions)

	// Prune any stale git worktree entries (handles manually deleted directories)
	if repoGit, err := m.repoBase(); err == nil {
		_ = repoGit.WorktreePrune()
	}
}

// ReconcilePoolWith reconciles the name pool given lists of names from different sources.
// This is the testable core of ReconcilePool.
//
// - namesWithDirs: names that have existing worktree directories (in use)
// - namesWithSessions: names that have tmux sessions
//
// Names with sessions but no directories are orphans and their sessions are killed.
// Only namesWithDirs are marked as in-use for allocation.
func (m *Manager) ReconcilePoolWith(namesWithDirs, namesWithSessions []string) {
	dirSet := make(map[string]bool)
	for _, name := range namesWithDirs {
		dirSet[name] = true
	}

	// Kill orphaned sessions (session exists but no directory)
	if m.tmux != nil {
		for _, name := range namesWithSessions {
			if !dirSet[name] {
				sessionName := fmt.Sprintf("gt-%s-%s", m.rig.Name, name)
				_ = m.tmux.KillSessionWithProcesses(sessionName)
			}
		}
	}

	m.namePool.Reconcile(namesWithDirs)
	// Note: No Save() needed - InUse is transient state, only OverflowNext is persisted
}

// PoolStatus returns information about the name pool.
func (m *Manager) PoolStatus() (active int, names []string) {
	return m.namePool.ActiveCount(), m.namePool.ActiveNames()
}

// List returns all polecats in the rig.
func (m *Manager) List() ([]*Polecat, error) {
	polecatsDir := filepath.Join(m.rig.Path, "polecats")

	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading polecats dir: %w", err)
	}

	var polecats []*Polecat
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecat, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid polecats
		}
		polecats = append(polecats, polecat)
	}

	return polecats, nil
}

// Get returns a specific polecat by name.
// State is derived from beads assignee field:
// - If an issue is assigned to this polecat: StateWorking
// - If no issue assigned: StateDone (ready for cleanup - transient polecats should have work)
func (m *Manager) Get(name string) (*Polecat, error) {
	if !m.exists(name) {
		return nil, ErrPolecatNotFound
	}

	return m.loadFromBeads(name)
}

// SetState updates a polecat's state.
// In the beads model, state is derived from issue status:
// - StateWorking/StateActive: issue status set to in_progress
// - StateDone: assignee cleared from issue (polecat ready for cleanup)
// - StateStuck: issue status set to blocked (if supported)
// If beads is not available, this is a no-op.
func (m *Manager) SetState(name string, state State) error {
	if !m.exists(name) {
		return ErrPolecatNotFound
	}

	// Find the issue assigned to this polecat
	assignee := m.assigneeID(name)
	issue, err := m.beads.GetAssignedIssue(assignee)
	if err != nil {
		// If beads is not available, treat as no-op (state can't be changed)
		return nil
	}

	switch state {
	case StateWorking, StateActive:
		// Set issue to in_progress if there is one
		if issue != nil {
			status := "in_progress"
			if err := m.beads.Update(issue.ID, beads.UpdateOptions{Status: &status}); err != nil {
				return fmt.Errorf("setting issue status: %w", err)
			}
		}
	case StateDone:
		// Clear assignment when done (polecat ready for cleanup)
		if issue != nil {
			empty := ""
			if err := m.beads.Update(issue.ID, beads.UpdateOptions{Assignee: &empty}); err != nil {
				return fmt.Errorf("clearing assignee: %w", err)
			}
		}
	case StateStuck:
		// Mark issue as blocked if supported, otherwise just note in issue
		if issue != nil {
			// For now, just keep the assignment - the issue's blocked_by would indicate stuck
			// We could add a status="blocked" here if beads supports it
		}
	}

	return nil
}

// AssignIssue assigns an issue to a polecat by setting the issue's assignee in beads.
func (m *Manager) AssignIssue(name, issue string) error {
	if !m.exists(name) {
		return ErrPolecatNotFound
	}

	// Set the issue's assignee to this polecat
	assignee := m.assigneeID(name)
	status := "in_progress"
	if err := m.beads.Update(issue, beads.UpdateOptions{
		Assignee: &assignee,
		Status:   &status,
	}); err != nil {
		return fmt.Errorf("setting issue assignee: %w", err)
	}

	return nil
}

// ClearIssue removes the issue assignment from a polecat.
// In the transient model, this transitions to Done state for cleanup.
// This clears the assignee from the currently assigned issue in beads.
// If beads is not available, this is a no-op.
func (m *Manager) ClearIssue(name string) error {
	if !m.exists(name) {
		return ErrPolecatNotFound
	}

	// Find the issue assigned to this polecat
	assignee := m.assigneeID(name)
	issue, err := m.beads.GetAssignedIssue(assignee)
	if err != nil {
		// If beads is not available, treat as no-op
		return nil
	}

	if issue == nil {
		// No issue assigned, nothing to clear
		return nil
	}

	// Clear the assignee from the issue
	empty := ""
	if err := m.beads.Update(issue.ID, beads.UpdateOptions{
		Assignee: &empty,
	}); err != nil {
		return fmt.Errorf("clearing issue assignee: %w", err)
	}

	return nil
}

// loadFromBeads gets polecat info from beads assignee field and agent bead hook_bead.
// State is determined by: has assigned issue OR has hook_bead → working, neither → done.
// This fixes the bug where polecats with newly hooked work show as "done" because
// the issue assignee hasn't been updated yet.
// Transient polecats should always have work; no work means ready for Witness cleanup.
// We don't interpret issue status (ZFC: Go is transport, not decision-maker).
func (m *Manager) loadFromBeads(name string) (*Polecat, error) {
	// Use clonePath which handles both new (polecats/<name>/<rigname>/)
	// and old (polecats/<name>/) structures
	clonePath := m.clonePath(name)

	// Get actual branch from worktree (branches are now timestamped)
	polecatGit := git.NewGit(clonePath)
	branchName, err := polecatGit.CurrentBranch()
	if err != nil {
		// Fall back to old format if we can't read the branch
		branchName = fmt.Sprintf("polecat/%s", name)
	}

	// Query beads for assigned issue
	assignee := m.assigneeID(name)
	issue, beadsErr := m.beads.GetAssignedIssue(assignee)
	if beadsErr != nil {
		// If beads query fails, return basic polecat info as working
		// (assume polecat is doing something if it exists)
		return &Polecat{
			Name:      name,
			Rig:       m.rig.Name,
			State:     StateWorking,
			ClonePath: clonePath,
			Branch:    branchName,
		}, nil
	}

	// Transient model: has issue = working, no issue = done (ready for cleanup)
	// Polecats without work should be nuked by the Witness
	state := StateDone
	issueID := ""
	if issue != nil {
		issueID = issue.ID
		state = StateWorking
	}

	// FIX (hq-50u3h): Also check agent bead's hook_bead field.
	// If hook_bead is set, the polecat has work waiting and should be "working".
	// This prevents polecats from showing as "done" when new work is hooked via gt sling.
	// gt sling sets hook_bead atomically, so this is the reliable signal for pending work.
	agentBeadID := m.agentBeadID(name)
	if _, fields, err := m.townBeads.GetAgentBead(agentBeadID); err == nil && fields != nil {
		if fields.HookBead != "" {
			// Polecat has hooked work - should be working, not done
			state = StateWorking
			// If issueID is empty, use the hook_bead as the issue reference
			// This ensures the hooked work is visible in listings
			if issueID == "" {
				issueID = fields.HookBead
			}
		}
	}

	return &Polecat{
		Name:      name,
		Rig:       m.rig.Name,
		State:     state,
		ClonePath: clonePath,
		Branch:    branchName,
		Issue:     issueID,
	}, nil
}

// setupSharedBeads creates a redirect file so the polecat uses the rig's shared .beads database.
// This eliminates the need for git sync between polecat clones - all polecats share one database.
// Also sets beads.role=maintainer in git config to ensure beads are written to the rig's database
// instead of ~/.beads-planning (which is the default for HTTPS-cloned repos without credentials).
func (m *Manager) setupSharedBeads(clonePath string) error {
	townRoot := filepath.Dir(m.rig.Path)
	if err := beads.SetupRedirect(townRoot, clonePath); err != nil {
		return err
	}

	// Set beads.role=maintainer to prevent beads from routing writes to ~/.beads-planning.
	// Without this, HTTPS-cloned repos are treated as "contributor" and writes go to the
	// wrong database, causing MR beads to be lost. See: gt-3ml66
	polecatGit := git.NewGit(clonePath)
	if err := polecatGit.SetConfig("beads.role", "maintainer"); err != nil {
		return fmt.Errorf("setting beads.role config: %w", err)
	}

	return nil
}

// verifyWorktree checks that a worktree was actually created and is valid.
// This guards against silent failures in git worktree add where git returns
// success but the worktree is incomplete or missing.
//
// Checks:
// 1. Directory exists
// 2. Has a .git file (worktree marker) or .git directory
// 3. Can run git commands in it
//
// Issue: gt sling reports success but worktree never created (hq-yh8icr).
func verifyWorktree(path string) error {
	// Check 1: Directory exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree directory does not exist")
		}
		return fmt.Errorf("checking worktree directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path is not a directory")
	}

	// Check 2: Has .git file or directory (indicates git worktree or repo)
	gitPath := filepath.Join(path, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree missing .git (not a valid worktree)")
		}
		return fmt.Errorf("checking .git: %w", err)
	}

	// Check 3: Can run git rev-parse to verify it's a working git directory
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git rev-parse failed (invalid git worktree): %w", err)
	}

	return nil
}

// CleanupStaleBranches removes orphaned polecat branches that are no longer in use.
// This includes:
// - Branches for polecats that no longer exist
// - Old timestamped branches (keeps only the most recent per polecat name)
// Returns the number of branches deleted.
func (m *Manager) CleanupStaleBranches() (int, error) {
	repoGit, err := m.repoBase()
	if err != nil {
		return 0, fmt.Errorf("finding repo base: %w", err)
	}

	// List all polecat branches
	branches, err := repoGit.ListBranches("polecat/*")
	if err != nil {
		return 0, fmt.Errorf("listing branches: %w", err)
	}

	if len(branches) == 0 {
		return 0, nil
	}

	// Get list of existing polecats
	polecats, err := m.List()
	if err != nil {
		return 0, fmt.Errorf("listing polecats: %w", err)
	}

	// Build set of current polecat branches (from actual polecat objects)
	currentBranches := make(map[string]bool)
	for _, p := range polecats {
		currentBranches[p.Branch] = true
	}

	// Delete branches not in current set
	deleted := 0
	for _, branch := range branches {
		if currentBranches[branch] {
			continue // This branch is in use
		}
		// Delete orphaned branch
		if err := repoGit.DeleteBranch(branch, true); err != nil {
			// Log but continue - non-fatal
			fmt.Printf("Warning: could not delete branch %s: %v\n", branch, err)
			continue
		}
		deleted++
	}

	return deleted, nil
}

// StalenessInfo contains details about a polecat's staleness.
type StalenessInfo struct {
	Name            string
	CommitsBehind   int  // How many commits behind origin/main
	HasActiveSession bool // Whether tmux session is running
	HasUncommittedWork bool // Whether there's uncommitted or unpushed work
	AgentState      string // From agent bead (empty if no bead)
	IsStale         bool   // Overall assessment: safe to clean up
	Reason          string // Why it's considered stale (or not)
}

// DetectStalePolecats identifies polecats that are candidates for cleanup.
// A polecat is considered stale if:
// - No active tmux session AND
// - Either: way behind main (>threshold commits) OR no agent bead/activity
// - Has no uncommitted work that could be lost
//
// threshold: minimum commits behind main to consider "way behind" (e.g., 20)
func (m *Manager) DetectStalePolecats(threshold int) ([]*StalenessInfo, error) {
	polecats, err := m.List()
	if err != nil {
		return nil, fmt.Errorf("listing polecats: %w", err)
	}

	if len(polecats) == 0 {
		return nil, nil
	}

	// Get default branch from rig config
	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(m.rig.Path); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	var results []*StalenessInfo
	for _, p := range polecats {
		info := &StalenessInfo{
			Name: p.Name,
		}

		// Check for active tmux session
		// Session name follows pattern: gt-<rig>-<polecat>
		sessionName := fmt.Sprintf("gt-%s-%s", m.rig.Name, p.Name)
		info.HasActiveSession = checkTmuxSession(sessionName)

		// Check how far behind main
		polecatGit := git.NewGit(p.ClonePath)
		info.CommitsBehind = countCommitsBehind(polecatGit, defaultBranch)

		// Check for uncommitted work (excluding .beads/ files which are synced across worktrees)
		status, err := polecatGit.CheckUncommittedWork()
		if err == nil && !status.CleanExcludingBeads() {
			info.HasUncommittedWork = true
		}

		// Check agent bead state
		agentID := m.agentBeadID(p.Name)
		_, fields, err := m.townBeads.GetAgentBead(agentID)
		if err == nil && fields != nil {
			info.AgentState = fields.AgentState
		}

		// Determine staleness
		info.IsStale, info.Reason = assessStaleness(info, threshold)
		results = append(results, info)
	}

	return results, nil
}

// checkTmuxSession checks if a tmux session exists.
func checkTmuxSession(sessionName string) bool {
	// Use has-session command which returns 0 if session exists
	cmd := exec.Command("tmux", "has-session", "-t", sessionName) //nolint:gosec // G204: sessionName is constructed internally
	return cmd.Run() == nil
}

// countCommitsBehind counts how many commits a worktree is behind origin/<defaultBranch>.
func countCommitsBehind(g *git.Git, defaultBranch string) int {
	// Use rev-list to count commits: origin/main..HEAD shows commits ahead,
	// HEAD..origin/main shows commits behind
	remoteBranch := "origin/" + defaultBranch
	count, err := g.CountCommitsBehind(remoteBranch)
	if err != nil {
		return 0 // Can't determine, assume not behind
	}
	return count
}

// assessStaleness determines if a polecat should be cleaned up.
// Per gt-zecmc: uses tmux state (HasActiveSession) rather than agent_state
// since observable states (running, done, idle) are no longer recorded in beads.
func assessStaleness(info *StalenessInfo, threshold int) (bool, string) {
	// Never clean up if there's uncommitted work
	if info.HasUncommittedWork {
		return false, "has uncommitted work"
	}

	// If session is active, not stale (tmux is source of truth for liveness)
	if info.HasActiveSession {
		return false, "session active"
	}

	// No active session - this polecat is a cleanup candidate
	// Check for reasons to keep it:

	// Check for non-observable states that indicate intentional pause
	// (stuck, awaiting-gate are still stored in beads per gt-zecmc)
	if info.AgentState == "stuck" || info.AgentState == "awaiting-gate" {
		return false, fmt.Sprintf("agent_state=%s (intentional pause)", info.AgentState)
	}

	// No session and way behind main = stale
	if info.CommitsBehind >= threshold {
		return true, fmt.Sprintf("%d commits behind main, no active session", info.CommitsBehind)
	}

	// No session and no agent bead = abandoned, clean up
	if info.AgentState == "" {
		return true, "no agent bead, no active session"
	}

	// No session but has agent bead without special state = clean up
	// (The session is the source of truth for liveness)
	return true, "no active session"
}
