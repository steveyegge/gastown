// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Naked    bool   // No-tmux mode: skip session creation
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

	// Get polecat manager
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit)

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

	// Handle naked mode (no-tmux)
	if opts.Naked {
		fmt.Println()
		fmt.Printf("%s\n", style.Bold.Render("🔧 NO-TMUX MODE (--naked)"))
		fmt.Printf("Polecat created. Agent must be started manually.\n\n")
		fmt.Printf("To start the agent:\n")
		fmt.Printf("  cd %s\n", polecatObj.ClonePath)
		// Use rig's configured agent command, unless overridden.
		agentCmd, err := config.GetRuntimeCommandWithAgentOverride(r.Path, opts.Agent)
		if err != nil {
			return nil, err
		}
		fmt.Printf("  %s\n\n", agentCmd)
		fmt.Printf("Agent will discover work via gt prime on startup.\n")

		return &SpawnedPolecatInfo{
			RigName:     rigName,
			PolecatName: polecatName,
			ClonePath:   polecatObj.ClonePath,
			SessionName: "", // No session in naked mode
			Pane:        "", // No pane in naked mode
		}, nil
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

	// Start session
	t := tmux.NewTmux()
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

// IdlePolecatInfo contains information about an idle (existing but not running) polecat.
type IdlePolecatInfo struct {
	Name      string // Polecat name
	ClonePath string // Path to polecat's git worktree
	Session   string // Tmux session name (for starting)
	CreatedAt int64  // Unix timestamp when polecat was created (for sorting)
	Clean     bool   // True if git state is clean (no uncommitted changes)
}

// StartPreference defines how to select from multiple idle polecats.
type StartPreference string

const (
	PreferenceAny      StartPreference = "any"      // First found (default, fastest)
	PreferenceNewest   StartPreference = "newest"   // Most recently created
	PreferenceOldest   StartPreference = "oldest"   // Least recently created
	PreferenceCleanest StartPreference = "cleanest" // Cleanest git state
)

// FindIdlePolecatOptions configures how idle polecats are selected.
type FindIdlePolecatOptions struct {
	SpecificName string          // Exact polecat name to find (empty = any)
	Preference   StartPreference // Selection preference when multiple idle exist
}

// FindIdlePolecat looks for an existing polecat that is not currently running.
// Returns the first idle polecat found matching the criteria, or nil if none exist.
// This enables reusing polecats instead of always spawning new ones.
func FindIdlePolecat(rigName string, opts FindIdlePolecatOptions) (*IdlePolecatInfo, error) {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	// Get polecat manager
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit)

	// List all polecats
	polecats, err := polecatMgr.List()
	if err != nil {
		return nil, fmt.Errorf("listing polecats: %w", err)
	}

	// Collect idle polecats with metadata
	var idlePolecats []*IdlePolecatInfo
	t := tmux.NewTmux()

	for _, p := range polecats {
		// If specific name requested, skip non-matching
		if opts.SpecificName != "" && !strings.EqualFold(p.Name, opts.SpecificName) {
			continue
		}

		sessionName := fmt.Sprintf("gt-%s-%s", rigName, p.Name)
		running, err := t.HasSession(sessionName)
		if err != nil || running {
			continue // Session active or error checking, skip
		}

		// Check git state for cleanliness
		pGit := git.NewGit(p.ClonePath)
		workStatus, checkErr := pGit.CheckUncommittedWork()
		clean := checkErr == nil && workStatus.Clean()

		// Get creation time from worktree metadata
		var createdAt int64
		if stat, statErr := os.Stat(p.ClonePath); statErr == nil {
			createdAt = stat.ModTime().Unix()
		}

		idlePolecats = append(idlePolecats, &IdlePolecatInfo{
			Name:      p.Name,
			ClonePath: p.ClonePath,
			Session:   sessionName,
			CreatedAt: createdAt,
			Clean:     clean,
		})
	}

	// No idle polecats found
	if len(idlePolecats) == 0 {
		// If specific name was requested, provide a helpful error
		if opts.SpecificName != "" {
			return nil, fmt.Errorf("polecat '%s' not found or already running in rig '%s'", opts.SpecificName, rigName)
		}
		return nil, nil
	}

	// Select based on preference
	var selected *IdlePolecatInfo
	switch opts.Preference {
	case PreferenceNewest:
		// Sort by creation time descending (newest first)
		sort.Slice(idlePolecats, func(i, j int) bool {
			return idlePolecats[i].CreatedAt > idlePolecats[j].CreatedAt
		})
		selected = idlePolecats[0]
	case PreferenceOldest:
		// Sort by creation time ascending (oldest first)
		sort.Slice(idlePolecats, func(i, j int) bool {
			return idlePolecats[i].CreatedAt < idlePolecats[j].CreatedAt
		})
		selected = idlePolecats[0]
	case PreferenceCleanest:
		// Prioritize clean git state
		for _, p := range idlePolecats {
			if p.Clean {
				selected = p
				break
			}
		}
		// Fallback to first if none are clean
		if selected == nil {
			selected = idlePolecats[0]
		}
	default: // PreferenceAny
		selected = idlePolecats[0]
	}

	return selected, nil
}

// StartPolecatSession starts an existing polecat's tmux session.
// This is used when --start flag is provided and an idle polecat is found.
// Returns the pane ID for nudging, or an error if startup fails.
func StartPolecatSession(rigName, polecatName, clonePath string, account string, agentOverride string) (string, error) {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rig to get path and config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return "", fmt.Errorf("loading rigs config: %w", err)
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return "", fmt.Errorf("rig '%s' not found", rigName)
	}

	// Resolve account for Claude config
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, account)
	if err != nil {
		return "", fmt.Errorf("resolving account: %w", err)
	}
	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Start session
	t := tmux.NewTmux()
	polecatSessMgr := polecat.NewSessionManager(t, r)

	sessionName := polecatSessMgr.SessionName(polecatName)

	// Check if already running (idempotent)
	running, _ := polecatSessMgr.IsRunning(polecatName)
	if running {
		fmt.Printf("Session already running for %s/%s\n", rigName, polecatName)
		pane, err := getSessionPane(sessionName)
		if err != nil {
			return "", fmt.Errorf("getting pane for %s: %w", sessionName, err)
		}
		return pane, nil
	}

	fmt.Printf("Starting session for %s/%s...\n", rigName, polecatName)
	startOpts := polecat.SessionStartOptions{
		RuntimeConfigDir: claudeConfigDir,
	}
	if agentOverride != "" {
		cmd, err := config.BuildPolecatStartupCommandWithAgentOverride(rigName, polecatName, r.Path, "", agentOverride)
		if err != nil {
			return "", err
		}
		startOpts.Command = cmd
	}
	if err := polecatSessMgr.Start(polecatName, startOpts); err != nil {
		return "", fmt.Errorf("starting session: %w", err)
	}

	// Get pane for nudging
	pane, err := getSessionPane(sessionName)
	if err != nil {
		return "", fmt.Errorf("getting pane for %s: %w", sessionName, err)
	}

	fmt.Printf("%s Polecat %s session started\n", style.Bold.Render("✓"), polecatName)

	// Log spawn event to activity feed (reuse type indicates idle polecat was used)
	payload := events.SpawnPayload(rigName, polecatName)
	payload["reuse"] = "true"
	_ = events.LogFeed(events.TypeSpawn, "gt", payload)

	return pane, nil
}
