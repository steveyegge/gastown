// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/ratelimit"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/terminal"
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
	K8sSpawn    bool   // True when dispatched to K8s (no local worktree/session)

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
	Force           bool   // Force spawn even if polecat has uncommitted work
	Account         string // Claude Code account handle to use
	Create          bool   // Create polecat if it doesn't exist (currently always true for sling)
	HookBead        string // Bead ID to set as hook_bead at spawn time (atomic assignment)
	Agent           string // Agent override for this spawn (e.g., "gemini", "codex", "claude-haiku")
	ExecutionTarget string // "local" (default) or "k8s" — overrides rig config
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

	// Load rig config (beads-first, filesystem fallback).
	rigsConfig, err := loadRigsConfigBeadsFirst(townRoot)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	// Resolve execution target: explicit override > rig settings > "local"
	execTarget := resolveExecutionTarget(r.Path, opts.ExecutionTarget)
	if execTarget == config.ExecutionTargetK8s {
		return spawnPolecatForK8sCMD(townRoot, rigName, r, opts)
	}

	// Check for rate limit backoff before spawning
	tracker := ratelimit.NewTracker(r.Path)
	if loadErr := tracker.Load(); loadErr == nil {
		if tracker.ShouldDefer() {
			waitTime := tracker.TimeUntilReady()
			return nil, fmt.Errorf("%w: rate limit backoff active for %s, retry in %v",
				polecat.ErrRateLimited, rigName, waitTime.Round(1e9))
		}
	}

	// Get polecat manager (with tmux for session-aware allocation)
	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	polecatMgr := polecat.NewManager(r, polecatGit)

	// Allocate a new polecat name
	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return nil, fmt.Errorf("allocating polecat name: %w", err)
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	// Clean up orphaned state for this polecat before creation/repair
	// This prevents contamination from previous failed spawns (hq-gsk9g, hq-cv-bn5ug)
	cleanupOrphanPolecatState(rigName, polecatName, r.Path, tmux.NewTmux())

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

	// NOTE: Agent bead is already created by AddWithOptions/RepairWorktreeWithOptions
	// with hook_bead set atomically. No need to call createPolecatAgentBead here.

	// Verify worktree was actually created (fixes #1070)
	// The identity bead may exist but worktree creation can fail silently
	if err := verifyWorktreeExists(polecatObj.ClonePath); err != nil {
		// Clean up the partial state before returning error
		_ = polecatMgr.Remove(polecatName, true) // force=true to clean up partial state
		return nil, fmt.Errorf("worktree verification failed for %s: %w\nHint: try 'gt polecat nuke %s/%s --force' to clean up",
			polecatName, err, rigName, polecatName)
	}

	// Verify hook_bead is set before starting session (fix for bd-3q6.8-1).
	// The slot set in CreateOrReopenAgentBead may fail silently. If HookBead was
	// specified but not set, retry the slot set here before the session starts.
	if opts.HookBead != "" {
		if err := verifyAndSetHookBead(townRoot, rigName, polecatName, opts.HookBead); err != nil {
			// Non-fatal warning - session will start but polecat may need to discover work via gt prime
			fmt.Printf("Warning: could not verify hook_bead: %v\n", err)
		}
	}

	// Resolve account for runtime config
	accountsPath := constants.MayorAccountsPath(townRoot)
	resolvedAccount, err := config.ResolveAccount(accountsPath, opts.Account)
	if err != nil {
		return nil, fmt.Errorf("resolving account: %w", err)
	}

	// Validate that the account has credentials before spawning
	// This prevents OAuth prompts from appearing in polecat sessions
	if err := config.ValidateAccountAuth(resolvedAccount); err != nil {
		return nil, err
	}

	// Extract account fields (handle nil account)
	var claudeConfigDir, accountHandle, authToken, baseURL string
	if resolvedAccount != nil {
		claudeConfigDir = resolvedAccount.ConfigDir
		accountHandle = resolvedAccount.Handle
		authToken = resolvedAccount.AuthToken
		baseURL = resolvedAccount.BaseURL
	}

	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Materialize MCP config from config beads before session start.
	// This writes .mcp.json to the polecat home dir; the session manager
	// will skip writing the embedded template since the file already exists.
	polecatHomeDir := filepath.Dir(polecatObj.ClonePath)
	townBeadsDir := beads.ResolveBeadsDir(townRoot)
	beadsForMCP := beads.NewWithBeadsDir(polecatHomeDir, townBeadsDir)
	townName := filepath.Base(townRoot)
	mcpLayers, _ := beadsForMCP.ResolveConfigMetadata(beads.ConfigCategoryMCP, townName, rigName, "polecat", polecatName)
	if len(mcpLayers) > 0 {
		if err := claude.MaterializeMCPConfig(polecatHomeDir, mcpLayers); err != nil {
			fmt.Printf("Warning: could not materialize MCP config from beads: %v\n", err)
		}
	}

	// Get session manager for session name (session start is deferred)
	polecatSessMgr := polecat.NewSessionManager(r)

	// Check if already running
	running, _ := polecatSessMgr.IsRunning(polecatName)

	// FIX (gt-e15cu): If session is running, check if polecat has hooked work (busy).
	// Polecats are single-task workers - we must not queue work behind existing work.
	// If busy, kill the existing session and start fresh.
	if running {
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
		beadsClient := beads.New(townRoot)
		if agentBead, showErr := beadsClient.Show(agentBeadID); showErr == nil {
			if agentBead.HookBead != "" && agentBead.HookBead != opts.HookBead {
				// Polecat has different hooked work - it's busy
				fmt.Printf("  Polecat %s has hooked work (%s), killing stale session...\n",
					polecatName, agentBead.HookBead)
				sessionName := polecatSessMgr.SessionName(polecatName)
				_ = backend.KillSession(sessionName)
				running = false // Will start fresh session below
			}
		}
	}

	if !running {
		fmt.Printf("Starting session for %s/%s...\n", rigName, polecatName)
		startOpts := polecat.SessionStartOptions{
			RuntimeConfigDir: claudeConfigDir,
			AuthToken:        authToken,
			BaseURL:          baseURL,
		}
		if opts.Agent != "" {
			cmd, err := config.BuildPolecatStartupCommandWithAgentOverride(rigName, polecatName, r.Path, "", opts.Agent)
			if err != nil {
				return nil, err
			}
			startOpts.Command = cmd
		}
		if err := polecatSessMgr.Start(polecatName, startOpts); err != nil {
			// Check if failure was due to rate limiting
			if errors.Is(err, polecat.ErrRateLimited) {
				// Rate limit detected - state already recorded by session manager
				fmt.Printf("⚠️  Rate limit detected during spawn\n")
				// Notify witness about rate limit (best effort)
				notifyWitnessRateLimit(rigName, polecatName, accountHandle)
			}
			return nil, fmt.Errorf("starting session: %w", err)
		}
	}

	// Get session name and pane
	sessionName := polecatSessMgr.SessionName(polecatName)

	// Debug: verify session exists before pane lookup
	debug := os.Getenv("GT_DEBUG_SLING") != ""
	if debug {
		fmt.Fprintf(os.Stderr, "[sling-debug] SpawnPolecatForSling: session started, verifying session %q exists...\n", sessionName)
		hasSession, hasErr := backend.HasSession(sessionName)
		if hasErr != nil || !hasSession {
			fmt.Fprintf(os.Stderr, "[sling-debug] WARNING: session %q does NOT exist after Start() returned!\n", sessionName)
		} else {
			fmt.Fprintf(os.Stderr, "[sling-debug] session %q exists, checking if pane is alive...\n", sessionName)
			dead, deadErr := backend.IsPaneDead(sessionName)
			if deadErr != nil {
				fmt.Fprintf(os.Stderr, "[sling-debug] IsPaneDead check failed: %v\n", deadErr)
			} else if dead {
				fmt.Fprintf(os.Stderr, "[sling-debug] pane is dead\n")
			} else {
				fmt.Fprintf(os.Stderr, "[sling-debug] pane is alive\n")
			}
		}
	}

	// Final verification: confirm worktree and session both still exist.
	// Issue: gt sling reports success but worktree never created (hq-yh8icr).
	// This catches any race conditions or cleanup that might have occurred.
	if err := verifySpawnedPolecat(polecatObj.ClonePath, sessionName, t, backend); err != nil {
		return nil, fmt.Errorf("spawn verification failed for %s: %w", polecatName, err)
	}

	fmt.Printf("%s Polecat %s spawned\n", style.Bold.Render("✓"), polecatName)

	// Log spawn event to activity feed
	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	return &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   polecatObj.ClonePath,
		SessionName: sessionName,
		Pane:        "", // Empty until StartSession is called
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

	// Load rig config (beads-first, filesystem fallback).
	rigsConfig, err := loadRigsConfigBeadsFirst(townRoot)
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
	polecatSessMgr := polecat.NewSessionManager(r)

	fmt.Printf("Starting session for %s/%s...\n", s.RigName, s.PolecatName)
	startOpts := polecat.SessionStartOptions{
		RuntimeConfigDir: claudeConfigDir,
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
	runtimeConfig := config.LoadRuntimeConfig(r.Path)
	if err := t.WaitForRuntimeReady(s.SessionName, runtimeConfig, 30*time.Second); err != nil {
		fmt.Printf("Warning: runtime may not be fully ready: %v\n", err)
	}

	// Update agent state
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit)
	if err := polecatMgr.SetAgentState(s.PolecatName, "working"); err != nil {
		fmt.Printf("Warning: could not update agent state: %v\n", err)
	}

	// Get pane
	pane, err := getSessionPane(s.SessionName)
	if err != nil {
		return "", fmt.Errorf("getting pane for %s: %w", s.SessionName, err)
	}

	s.Pane = pane
	return pane, nil
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

	rigsConfig, err := loadRigsConfigBeadsFirst(townRoot)
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

// cleanupOrphanPolecatState removes orphaned tmux sessions and stale git worktrees
// for a polecat that's being allocated. This prevents contamination from failed
// spawns where the directory was created but the worktree wasn't.
//
// This implements the fix from investigation hq-gsk9g (polecat worktree hygiene):
// - Kills orphan tmux sessions without corresponding directories
// - Prunes stale git worktree registrations
// - Clears hook_bead on respawn (via fresh agent bead creation)
func cleanupOrphanPolecatState(rigName, polecatName, rigPath string, tm *tmux.Tmux) {
	polecatDir := filepath.Join(rigPath, "polecats", polecatName)
	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)

	// Step 1: Kill orphan tmux session if it exists
	if err := tm.KillSession(sessionName); err == nil {
		fmt.Printf("  Cleaned up orphan tmux session: %s\n", sessionName)
	}

	// Step 2: Remove empty polecat directory (failed worktree creation)
	// This handles the race condition where RepairWorktreeWithOptions steps 1-2
	// succeed but step 3 (worktree creation) fails, leaving an empty directory.
	if entries, err := filepath.Glob(polecatDir + "/*"); err == nil && len(entries) == 0 {
		if rmErr := os.RemoveAll(polecatDir); rmErr == nil {
			fmt.Printf("  Cleaned up empty polecat directory: %s\n", polecatDir)
		}
	}

	// Step 3: Prune stale git worktree entries (non-fatal cleanup)
	repoGit := git.NewGit(rigPath)
	_ = repoGit.WorktreePrune()
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

// verifySpawnedPolecat performs final verification that a spawned polecat is valid.
// This catches race conditions where worktree or session disappear during spawn.
//
// Checks:
// 1. Worktree directory exists and has .git
// 2. Session exists (via backend for coop, or tmux directly)
//
// Issue: gt sling reports success but worktree never created (hq-yh8icr).
func verifySpawnedPolecat(clonePath, sessionName string, t *tmux.Tmux, backend terminal.Backend) error {
	// Check 1: Worktree exists and has .git
	gitPath := filepath.Join(clonePath, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree disappeared: %s (missing .git)", clonePath)
		}
		return fmt.Errorf("checking worktree: %w", err)
	}

	// Check 2: Session exists
	var hasSession bool
	var err error
	if backend != nil {
		hasSession, err = backend.HasSession(sessionName)
	} else {
		hasSession, err = t.HasSession(sessionName)
	}
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !hasSession {
		return fmt.Errorf("session disappeared: %s", sessionName)
	}

	return nil
}

// verifyAndSetHookBead verifies the agent bead has hook_bead set, and retries if not.
// This fixes bd-3q6.8-1 where the slot set in CreateOrReopenAgentBead may fail silently.
func verifyAndSetHookBead(townRoot, rigName, polecatName, hookBead string) error {
	// Agent bead uses rig prefix and is stored in rig beads
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)

	// Read the agent bead
	beadsClient := beads.New(townRoot)
	agentIssue, err := beadsClient.Show(agentBeadID)
	if err != nil {
		return fmt.Errorf("reading agent bead %s: %w", agentBeadID, err)
	}

	// Check if hook_bead is already set correctly
	if agentIssue.HookBead == hookBead {
		return nil // Already set correctly
	}

	// Hook not set or set to wrong value - retry setting it
	fmt.Printf("  Retrying hook_bead set for %s...\n", agentBeadID)
	if err := beadsClient.SetHookBead(agentBeadID, hookBead); err != nil {
		return fmt.Errorf("retrying hook_bead set: %w", err)
	}

	return nil
}

// resolveExecutionTarget determines the execution target for a rig.
// Priority: explicit override > rig settings > K8s auto-detect > "local".
// When running inside a K8s pod (KUBERNETES_SERVICE_HOST is set), defaults
// to "k8s" instead of "local" so agents spawn as pods, not tmux sessions.
func resolveExecutionTarget(rigPath, override string) config.ExecutionTarget {
	if override != "" {
		return config.ExecutionTarget(override)
	}

	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err == nil && settings.Execution != nil && settings.Execution.Target != "" {
		return settings.Execution.Target
	}

	// Auto-detect K8s: every pod gets KUBERNETES_SERVICE_HOST injected by kubelet.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return config.ExecutionTargetK8s
	}

	return config.ExecutionTargetLocal
}

// spawnPolecatForK8sCMD creates an agent bead for a K8s polecat without creating
// a local worktree or tmux session. The K8s controller watches for agent beads
// with agent_state=spawning and execution_target:k8s label, then creates pods.
func spawnPolecatForK8sCMD(townRoot, rigName string, r *rig.Rig, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
	// Allocate polecat name. Try the full polecat Manager first (works when
	// the rig has a full local clone with .beads/). Fall back to a simple
	// name pool when the rig was created via gt rig register (K8s-only, no clone).
	var polecatName string
	rigBeadsDir := beads.ResolveBeadsDir(r.Path)
	if _, err := os.Stat(rigBeadsDir); err == nil {
		// Full rig: use polecat Manager for proper pool + reconciliation.
		polecatGit := git.NewGit(r.Path)
		polecatMgr := polecat.NewManager(r, polecatGit)
		name, err := polecatMgr.AllocateName()
		if err != nil {
			return nil, fmt.Errorf("allocating polecat name: %w", err)
		}
		polecatName = name
	} else {
		// K8s-only rig (gt rig register): use lightweight name pool.
		pool := polecat.NewNamePool(r.Path, r.Name)
		_ = pool.Load()
		name, err := pool.Allocate()
		if err != nil {
			return nil, fmt.Errorf("allocating polecat name: %w", err)
		}
		_ = pool.Save()
		polecatName = name
	}
	fmt.Printf("Allocated polecat: %s (K8s)\n", polecatName)

	// Create or reopen agent bead with spawning state and hook_bead set atomically.
	// Always use townRoot for the beads client — it has daemon connection via
	// .beads/config.yaml. Using the rig's local .beads/ would bypass the daemon
	// and hit a stale local SQLite (missing columns like auto_close).
	// The daemon handles prefix-based routing to the correct database.
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
	beadsClient := beads.New(townRoot)
	_, err := beadsClient.CreateOrReopenAgentBead(agentBeadID, agentBeadID, &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        rigName,
		AgentState: "spawning",
		HookBead:   opts.HookBead,
	})
	if err != nil {
		return nil, fmt.Errorf("creating agent bead for K8s polecat: %w", err)
	}

	// Label the agent bead so the controller knows this is a K8s polecat.
	if err := beadsClient.AddLabel(agentBeadID, "execution_target:k8s"); err != nil {
		fmt.Printf("Warning: could not add execution_target label: %v\n", err)
	}

	fmt.Printf("✓ Polecat %s dispatched to K8s (agent_state=spawning)\n", polecatName)

	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	return &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   "", // No local worktree for K8s polecats
		SessionName: "", // No local tmux session
		Pane:        "",
		K8sSpawn:    true,
		account:     opts.Account,
		agent:       opts.Agent,
	}, nil
}

// notifyWitnessRateLimit sends a rate limit notification to the rig's witness.
// This is best-effort and failures are silently ignored.
func notifyWitnessRateLimit(rigName, polecatName, account string) {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return
	}

	// Send mail to witness using gt mail send
	subject := fmt.Sprintf("RATE_LIMITED polecat:%s", polecatName)
	body := fmt.Sprintf("Account: %s\nSource: spawn\nPolecat: %s\nRig: %s\n",
		account, polecatName, rigName)

	witnessAddr := fmt.Sprintf("%s/witness", rigName)
	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body)
	cmd.Dir = townRoot
	_ = cmd.Run() // Best effort - ignore errors
}
