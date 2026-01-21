// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// SpawnedPolecatInfo contains info about a spawned polecat session.
type SpawnedPolecatInfo struct {
	RigName     string // Rig name (e.g., "gastown")
	PolecatName string // Polecat name (e.g., "Toast")
	ClonePath   string // Path to polecat's git worktree
	SessionName string // Tmux session name (e.g., "gt-gastown-p-Toast")
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
//
// This is a convenience wrapper that allocates a name and delegates to SpawnPolecatForSlingWithName.
// For batch operations, use SpawnPolecatForSlingWithName directly with pre-allocated names to avoid races.
func SpawnPolecatForSling(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Allocate a name using the common function
	polecatName, err := allocatePolecatName(townRoot, rigName)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	return SpawnPolecatForSlingWithName(rigName, polecatName, opts)
}

// allocatePolecatName allocates a single polecat name for a rig.
// This is used by SpawnPolecatForSling for single spawns.
func allocatePolecatName(townRoot, rigName string) (string, error) {
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
		return "", fmt.Errorf("rig '%s' not found", rigName)
	}

	// Get polecat manager (with agents for session-aware allocation)
	polecatGit := git.NewGit(r.Path)
	agents := agent.Default()
	polecatMgr := polecat.NewManager(agents, r, polecatGit)

	// Allocate a new polecat name
	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return "", fmt.Errorf("allocating polecat name: %w", err)
	}

	return polecatName, nil
}

// SpawnPolecatForSlingWithName creates a polecat with a pre-allocated name.
// This is used by gt queue run to avoid race conditions when spawning in parallel.
// The name should have been allocated upfront by the caller using a shared NamePool.
func SpawnPolecatForSlingWithName(rigName, polecatName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
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

	// Get polecat manager (name already allocated, no need for session-aware allocation)
	polecatGit := git.NewGit(r.Path)
	agents := agent.Default()
	polecatMgr := polecat.NewManager(agents, r, polecatGit)

	fmt.Printf("Using pre-allocated polecat: %s\n", polecatName)

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

	// Check if already running using agents interface (agent auto-resolved)
	id := agent.PolecatAddress(rigName, polecatName)
	factoryAgents := factory.Agents()
	if !factoryAgents.Exists(id) {
		fmt.Printf("Starting session for %s/%s...\n", rigName, polecatName)
		if _, err := factory.Start(townRoot, id, factory.WithAgent(opts.Agent)); err != nil {
			return nil, fmt.Errorf("starting session: %w", err)
		}
	}

	// Get session name using session manager
	polecatSessMgr := factory.New(townRoot).PolecatSessionManager(r, opts.Agent)
	sessionName := polecatSessMgr.SessionName(polecatName)

	fmt.Printf("%s Polecat %s spawned\n", style.Bold.Render("✓"), polecatName)

	// Log spawn event to activity feed
	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	return &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   polecatObj.ClonePath,
		SessionName: sessionName,
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

// SpawnAndHookOptions contains options for spawning and hooking a bead.
type SpawnAndHookOptions struct {
	Force    bool   // Force spawn even if polecat has uncommitted work
	Account  string // Claude Code account handle to use
	Create   bool   // Create polecat if it doesn't exist
	Agent    string // Agent override for this spawn
	Subject  string // Subject for the nudge prompt
	Args     string // Args to pass to the polecat
	LogEvent bool   // Whether to log a sling event
}

// HookOptions contains options for hooking a bead to an agent.
type HookOptions struct {
	Subject  string // Subject for the nudge prompt
	Args     string // Args to pass to the agent
	LogEvent bool   // Whether to log a sling event
}

// HookBeadToAgent hooks a bead to an existing agent (polecat).
// This does the hooking, logging, and nudging - everything except spawning.
func HookBeadToAgent(townRoot, beadID, targetAgent, hookWorkDir string, opts HookOptions) error {
	townBeadsDir := townRoot + "/.beads"

	// Hook the bead using bd update
	hookCmd := exec.Command("bd", "--no-daemon", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
	hookCmd.Dir = hookWorkDir
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking bead: %w", err)
	}

	// Log sling event and store dispatcher
	actor := detectActor()
	if opts.LogEvent {
		_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadID, targetAgent))
	}

	// Store dispatcher in bead (enables completion notification)
	_ = storeDispatcherInBead(beadID, actor)

	// Store args in bead (durable for no-tmux mode)
	if opts.Args != "" {
		_ = storeArgsInBead(beadID, opts.Args)
	}

	// Update agent bead state
	updateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)

	// Auto-attach work molecule
	_ = attachPolecatWorkMolecule(targetAgent, hookWorkDir, townRoot)

	// Nudge the agent
	agentID, err := addressToAgentID(targetAgent)
	if err == nil {
		_ = ensureAgentReady(townRoot, agentID)
		_ = injectStartPrompt(townRoot, agentID, beadID, opts.Subject, opts.Args)
	}

	return nil
}

// SpawnAndHookBead spawns a polecat and hooks a bead to it.
// This is the common function used by single slinging operations.
//
// This is a convenience wrapper that allocates a name and delegates to SpawnAndHookBeadWithName.
// For batch operations, use SpawnAndHookBeadWithName directly with pre-allocated names to avoid races.
func SpawnAndHookBead(townRoot, rigName, beadID string, opts SpawnAndHookOptions) (*SpawnedPolecatInfo, error) {
	// Allocate a name
	polecatName, err := allocatePolecatName(townRoot, rigName)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	return SpawnAndHookBeadWithName(townRoot, rigName, polecatName, beadID, opts)
}

// SpawnAndHookBeadWithName spawns a polecat with a pre-allocated name and hooks a bead to it.
// This is used by batch operations to avoid race conditions when spawning in parallel.
func SpawnAndHookBeadWithName(townRoot, rigName, polecatName, beadID string, opts SpawnAndHookOptions) (*SpawnedPolecatInfo, error) {
	// Spawn polecat with pre-allocated name and hook_bead set atomically
	spawnOpts := SlingSpawnOptions{
		Force:    opts.Force,
		Account:  opts.Account,
		Create:   opts.Create,
		HookBead: beadID,
		Agent:    opts.Agent,
	}
	spawnInfo, err := SpawnPolecatForSlingWithName(rigName, polecatName, spawnOpts)
	if err != nil {
		return nil, fmt.Errorf("spawning polecat: %w", err)
	}

	// Hook the bead to the spawned polecat
	hookOpts := HookOptions{
		Subject:  opts.Subject,
		Args:     opts.Args,
		LogEvent: opts.LogEvent,
	}
	if err := HookBeadToAgent(townRoot, beadID, spawnInfo.AgentID(), spawnInfo.ClonePath, hookOpts); err != nil {
		return nil, err
	}

	return spawnInfo, nil
}
