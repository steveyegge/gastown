// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
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
	Pane        string // Tmux pane ID
}

// AgentID returns the agent identifier (e.g., "gastown/polecats/Toast")
func (s *SpawnedPolecatInfo) AgentID() string {
	return fmt.Sprintf("%s/polecats/%s", s.RigName, s.PolecatName)
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

	// Start session (reuse tmux from manager)
	polecatSessMgr := polecat.NewSessionManager(t, r)

	// Check if already running
	running, _ := polecatSessMgr.IsRunning(polecatName)
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
			return nil, fmt.Errorf("starting session: %w", err)
		}
	}

	// Get session name and pane
	sessionName := polecatSessMgr.SessionName(polecatName)

	// Debug: verify session exists before pane lookup
	debug := os.Getenv("GT_DEBUG_SLING") != ""
	if debug {
		fmt.Fprintf(os.Stderr, "[sling-debug] SpawnPolecatForSling: session started, verifying session %q exists...\n", sessionName)
		hasCmd := exec.Command("tmux", "has-session", "-t", "="+sessionName)
		if hasErr := hasCmd.Run(); hasErr != nil {
			fmt.Fprintf(os.Stderr, "[sling-debug] WARNING: session %q does NOT exist after Start() returned!\n", sessionName)
			// Try to get more info about what happened
			listCmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
			if listOut, listErr := listCmd.Output(); listErr == nil {
				fmt.Fprintf(os.Stderr, "[sling-debug] Current sessions: %s\n", strings.TrimSpace(string(listOut)))
			}
		} else {
			fmt.Fprintf(os.Stderr, "[sling-debug] session %q exists, checking if pane is alive...\n", sessionName)
			// Check if Claude is running
			paneCmd := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_current_command}")
			if paneOut, paneErr := paneCmd.Output(); paneErr == nil {
				fmt.Fprintf(os.Stderr, "[sling-debug] pane command: %s\n", strings.TrimSpace(string(paneOut)))
			} else {
				fmt.Fprintf(os.Stderr, "[sling-debug] list-panes failed: %v\n", paneErr)
			}
		}
	}

	pane, err := getSessionPane(sessionName)
	if err != nil {
		return nil, fmt.Errorf("getting pane for %s: %w", sessionName, err)
	}

	// Final verification: confirm worktree and session both still exist.
	// Issue: gt sling reports success but worktree never created (hq-yh8icr).
	// This catches any race conditions or cleanup that might have occurred.
	if err := verifySpawnedPolecat(polecatObj.ClonePath, sessionName, t); err != nil {
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
		Pane:        pane,
	}, nil
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

// verifySpawnedPolecat performs final verification that a spawned polecat is valid.
// This catches race conditions where worktree or session disappear during spawn.
//
// Checks:
// 1. Worktree directory exists and has .git
// 2. Tmux session exists
//
// Issue: gt sling reports success but worktree never created (hq-yh8icr).
func verifySpawnedPolecat(clonePath, sessionName string, t *tmux.Tmux) error {
	// Check 1: Worktree exists and has .git
	gitPath := filepath.Join(clonePath, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree disappeared: %s (missing .git)", clonePath)
		}
		return fmt.Errorf("checking worktree: %w", err)
	}

	// Check 2: Tmux session exists
	hasSession, err := t.HasSession(sessionName)
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
	// Agent bead uses hq- prefix and is stored in town beads
	agentBeadID := beads.PolecatBeadIDTown(rigName, polecatName)

	// Read the agent bead from town beads
	townBeadsClient := beads.New(townRoot)
	agentIssue, err := townBeadsClient.Show(agentBeadID)
	if err != nil {
		return fmt.Errorf("reading agent bead %s: %w", agentBeadID, err)
	}

	// Check if hook_bead is already set correctly
	if agentIssue.HookBead == hookBead {
		return nil // Already set correctly
	}

	// Hook not set or set to wrong value - retry setting it
	fmt.Printf("  Retrying hook_bead set for %s...\n", agentBeadID)
	if err := townBeadsClient.SetHookBead(agentBeadID, hookBead); err != nil {
		return fmt.Errorf("retrying hook_bead set: %w", err)
	}

	return nil
}
