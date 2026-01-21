// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

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

	// Resolve account for runtime config
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, opts.Account)
	if err != nil {
		return nil, fmt.Errorf("resolving account: %w", err)
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
	pane, err := getSessionPane(sessionName)
	if err != nil {
		return nil, fmt.Errorf("getting pane for %s: %w", sessionName, err)
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
