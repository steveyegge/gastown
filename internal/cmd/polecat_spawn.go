// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// SpawnedPolecatInfo contains info about a spawned polecat session.
type SpawnedPolecatInfo struct {
	RigName     string // Rig name (e.g., "gastown")
	PolecatName string // Polecat name (e.g., "Toast")
	ClonePath   string // Path to polecat's git worktree
	SessionName string // Tmux session name (e.g., "gt-gastown-p-Toast")
	Pane        string // Tmux pane ID (empty until StartSession is called)
	DoltBranch  string // Dolt branch for write isolation (empty if not created)

	// Internal fields for deferred session start
	account string
	agent   string
}

// AgentID returns the agent identifier (e.g., "gastown/polecats/Toast")
func (s *SpawnedPolecatInfo) AgentID() string {
	return fmt.Sprintf("%s/polecats/%s", s.RigName, s.PolecatName)
}

// SessionStarted returns true if the tmux session has been started.
func (s *SpawnedPolecatInfo) SessionStarted() bool {
	return s.Pane != ""
}

// SlingSpawnOptions contains options for spawning a polecat via sling.
type SlingSpawnOptions struct {
	Force    bool   // Force spawn even if polecat has uncommitted work
	Account  string // Claude Code account handle to use
	Create   bool   // Create polecat if it doesn't exist (currently always true for sling)
	HookBead string // Bead ID to set as hook_bead at spawn time (atomic assignment)
	Agent    string // Agent override for this spawn (e.g., "gemini", "codex", "claude-haiku")
}

// SpawnPolecatForSling creates a fresh polecat and optionally starts its session.
// This is used by gt sling when the target is a rig name.
// The caller (sling) handles hook attachment and nudging.
func SpawnPolecatForSling(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	// Get polecat manager (with tmux for session-aware allocation)
	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	polecatMgr := polecat.NewManager(r, polecatGit, t)

	// Pre-spawn Dolt health check (gt-94llt7): verify Dolt is reachable before
	// allocating a polecat. Prevents orphaned polecats when Dolt is down.
	if err := polecatMgr.CheckDoltHealth(); err != nil {
		return nil, fmt.Errorf("pre-spawn health check failed: %w", err)
	}

	// Pre-spawn admission control (gt-1obzke): verify Dolt server has connection
	// capacity before spawning. Prevents connection storms during mass sling.
	if err := polecatMgr.CheckDoltServerCapacity(); err != nil {
		return nil, fmt.Errorf("admission control: %w", err)
	}

	// Allocate a new polecat name
	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return nil, fmt.Errorf("allocating polecat name: %w", err)
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	// Check if polecat already exists (shouldn't happen - indicates stale state needing repair)
	existingPolecat, err := polecatMgr.Get(polecatName)

	// Build add options with hook_bead set atomically at spawn time
	addOpts := polecat.AddOptions{
		HookBead: opts.HookBead,
	}

	if err == nil {
		// Stale state: polecat exists despite fresh name allocation - repair it
		// Check for uncommitted work first
		if !opts.Force {
			pGit := git.NewGit(existingPolecat.ClonePath)
			workStatus, checkErr := pGit.CheckUncommittedWork()
			if checkErr == nil && !workStatus.Clean() {
				return nil, fmt.Errorf("polecat '%s' has uncommitted work: %s\nUse --force to proceed anyway",
					polecatName, workStatus.String())
			}
		}

		// Check for unmerged MRs - destroying a polecat with pending MR loses work (ne-rn24b)
		if existingPolecat.Branch != "" {
			bd := beads.New(r.Path)
			mr, mrErr := bd.FindMRForBranch(existingPolecat.Branch)
			if mrErr == nil && mr != nil {
				return nil, fmt.Errorf("polecat '%s' has unmerged MR: %s\n"+
					"Wait for MR to merge before respawning, or use:\n"+
					"  gt polecat nuke --force %s/%s  # to abandon the MR",
					polecatName, mr.ID, rigName, polecatName)
			}
		}

		fmt.Printf("Repairing stale polecat %s with fresh worktree...\n", polecatName)
		if _, err = polecatMgr.RepairWorktreeWithOptions(polecatName, opts.Force, addOpts); err != nil {
			return nil, fmt.Errorf("repairing stale polecat: %w", err)
		}
	} else if err == polecat.ErrPolecatNotFound {
		// Create new polecat
		fmt.Printf("Creating polecat %s...\n", polecatName)
		if _, err = polecatMgr.AddWithOptions(polecatName, addOpts); err != nil {
			return nil, fmt.Errorf("creating polecat: %w", err)
		}
	} else {
		return nil, fmt.Errorf("getting polecat: %w", err)
	}

	// Get polecat object for path info
	polecatObj, err := polecatMgr.Get(polecatName)
	if err != nil {
		return nil, fmt.Errorf("getting polecat after creation: %w", err)
	}

	// Verify worktree was actually created (fixes #1070)
	// The identity bead may exist but worktree creation can fail silently
	if err := verifyWorktreeExists(polecatObj.ClonePath); err != nil {
		// Clean up the partial state before returning error
		_ = polecatMgr.Remove(polecatName, true) // force=true to clean up partial state
		return nil, fmt.Errorf("worktree verification failed for %s: %w\nHint: try 'gt polecat nuke %s/%s --force' to clean up",
			polecatName, err, rigName, polecatName)
	}

	// Branch-per-polecat: generate name but DEFER creation to after sling writes.
	// DOLT_BRANCH forks from HEAD, but BD_DOLT_AUTO_COMMIT=off means writes
	// stay in working set. Caller must call CreateDoltBranch() after all writes
	// are complete to flush the working set and create the branch.
	doltBranch := doltserver.PolecatBranchName(polecatName)

	// Get session manager for session name (session start is deferred)
	polecatSessMgr := polecat.NewSessionManager(t, r)
	sessionName := polecatSessMgr.SessionName(polecatName)

	fmt.Printf("%s Polecat %s spawned (session start deferred)\n", style.Bold.Render("✓"), polecatName)

	// Log spawn event to activity feed
	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	return &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   polecatObj.ClonePath,
		SessionName: sessionName,
		Pane:        "", // Empty until StartSession is called
		DoltBranch:  doltBranch,
		account:     opts.Account,
		agent:       opts.Agent,
	}, nil
}

// StartSession starts the tmux session for a spawned polecat.
// This is called after the molecule/bead is attached, so the polecat
// sees its work when gt prime runs on session start.
// Returns the pane ID after session start.
func (s *SpawnedPolecatInfo) StartSession() (string, error) {
	if s.SessionStarted() {
		return s.Pane, nil
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(s.RigName)
	if err != nil {
		return "", fmt.Errorf("rig '%s' not found", s.RigName)
	}

	// Resolve account
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, _, err := config.ResolveAccountConfigDir(accountsPath, s.account)
	if err != nil {
		return "", fmt.Errorf("resolving account: %w", err)
	}

	// Start session
	t := tmux.NewTmux()
	polecatSessMgr := polecat.NewSessionManager(t, r)

	fmt.Printf("Starting session for %s/%s...\n", s.RigName, s.PolecatName)
	startOpts := polecat.SessionStartOptions{
		RuntimeConfigDir: claudeConfigDir,
		DoltBranch:       s.DoltBranch,
	}
	if s.agent != "" {
		cmd, err := config.BuildPolecatStartupCommandWithAgentOverride(s.RigName, s.PolecatName, r.Path, "", s.agent)
		if err != nil {
			return "", err
		}
		startOpts.Command = cmd
	}
	if err := polecatSessMgr.Start(s.PolecatName, startOpts); err != nil {
		return "", fmt.Errorf("starting session: %w", err)
	}

	// Wait for runtime to be fully ready before returning.
	spawnTownRoot := filepath.Dir(r.Path)
	runtimeConfig := config.ResolveRoleAgentConfig("polecat", spawnTownRoot, r.Path)
	if err := t.WaitForRuntimeReady(s.SessionName, runtimeConfig, 30*time.Second); err != nil {
		fmt.Printf("Warning: runtime may not be fully ready: %v\n", err)
	}

	// Update agent state with retry logic (gt-94llt7: fail-safe Dolt writes).
	// Note: warn-only, not fail-hard. The tmux session is already started above,
	// so returning an error here would leave an orphaned session with no cleanup path.
	// The polecat can still function without the agent state update — it only affects
	// monitoring visibility, not correctness. Compare with createAgentBeadWithRetry
	// which fails hard because a polecat without an agent bead is untrackable.
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit, t)
	if err := polecatMgr.SetAgentStateWithRetry(s.PolecatName, "working"); err != nil {
		fmt.Printf("Warning: could not update agent state after retries: %v\n", err)
	}

	// Update issue status from hooked to in_progress.
	// Also warn-only for the same reason: session is already running.
	if err := polecatMgr.SetState(s.PolecatName, polecat.StateWorking); err != nil {
		fmt.Printf("Warning: could not update issue status to in_progress: %v\n", err)
	}

	// Get pane — if this fails, the session may have died during startup.
	// Kill the dead session to prevent "session already running" on next attempt (gt-jn40ft).
	pane, err := getSessionPane(s.SessionName)
	if err != nil {
		// Session likely died — clean up the tmux session so it doesn't block re-sling
		_ = t.KillSession(s.SessionName)
		return "", fmt.Errorf("getting pane for %s (session likely died during startup): %w", s.SessionName, err)
	}

	s.Pane = pane
	return pane, nil
}

// CreateDoltBranch flushes the main working set to HEAD and creates the polecat's
// Dolt branch. Must be called AFTER all sling writes (hook, formula, fields) so the
// branch fork includes everything. This fixes the visibility gap where DOLT_BRANCH
// forks from HEAD but BD_DOLT_AUTO_COMMIT=off leaves writes in working set only.
//
// On error, callers are responsible for cleaning up the spawned polecat (worktree,
// agent bead) and unhooking any attached beads. See rollbackSlingArtifacts for the
// standard cleanup pattern.
func (s *SpawnedPolecatInfo) CreateDoltBranch() error {
	if s.DoltBranch == "" {
		return nil
	}
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	// Flush main working set to HEAD so DOLT_BRANCH includes all sling writes
	if err := doltserver.CommitServerWorkingSet(townRoot, s.RigName, "sling: flush for "+s.PolecatName); err != nil {
		return fmt.Errorf("flushing working set for %s: %w", s.PolecatName, err)
	}
	// Create branch from now-committed HEAD (includes all writes)
	if err := doltserver.CreatePolecatBranch(townRoot, s.RigName, s.DoltBranch); err != nil {
		return fmt.Errorf("creating Dolt branch %s: %w", s.DoltBranch, err)
	}
	fmt.Printf("%s Dolt branch: %s\n", style.Bold.Render("✓"), s.DoltBranch)
	return nil
}

// IsRigName checks if a target string is a rig name (not a role or path).
// Returns the rig name and true if it's a valid rig.
func IsRigName(target string) (string, bool) {
	// If it contains a slash, it's a path format (rig/role or rig/crew/name)
	if strings.Contains(target, "/") {
		return "", false
	}

	// Check known non-rig role names
	switch strings.ToLower(target) {
	case "mayor", "may", "deacon", "dea", "crew", "witness", "wit", "refinery", "ref":
		return "", false
	}

	// Try to load as a rig
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", false
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return "", false
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	_, err = rigMgr.GetRig(target)
	if err != nil {
		return "", false
	}

	return target, true
}

// verifyWorktreeExists checks that a git worktree was actually created at the given path.
// Returns an error if the worktree is missing or invalid.
func verifyWorktreeExists(clonePath string) error {
	// Check if directory exists
	info, err := os.Stat(clonePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree directory does not exist: %s", clonePath)
		}
		return fmt.Errorf("checking worktree directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path is not a directory: %s", clonePath)
	}

	// Check for .git file (worktrees have a .git file, not a .git directory)
	gitPath := filepath.Join(clonePath, ".git")
	gitInfo, err := os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree missing .git file (not a valid git worktree): %s", clonePath)
		}
		return fmt.Errorf("checking .git: %w", err)
	}

	// .git should be a file for worktrees (contains "gitdir: ..." pointer)
	// or a directory for regular clones - either is valid
	_ = gitInfo // Both file and directory are acceptable

	return nil
}
