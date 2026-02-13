package sling

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/ratelimit"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/terminal"
	"github.com/steveyegge/gastown/internal/workspace"
)

// ResolveExecutionTarget determines the execution target for a rig.
// Priority: explicit override > rig settings > K8s auto-detect > "local".
// When running inside a K8s pod (KUBERNETES_SERVICE_HOST is set), defaults
// to "k8s" instead of "local" so agents don't require tmux.
func ResolveExecutionTarget(rigPath, override string) config.ExecutionTarget {
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

// SpawnPolecatForSling creates a fresh polecat and starts its session.
// This is used by gt sling when the target is a rig name.
// The caller handles hook attachment and nudging.
func SpawnPolecatForSling(rigName string, opts SpawnOptions) (*SpawnResult, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

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

	// Resolve execution target: explicit override > rig settings > "local"
	execTarget := ResolveExecutionTarget(r.Path, opts.ExecutionTarget)
	if execTarget == config.ExecutionTargetK8s {
		return spawnPolecatForK8s(townRoot, rigName, r, opts)
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

	polecatGit := git.NewGit(r.Path)
	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	polecatMgr := polecat.NewManager(r, polecatGit)

	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return nil, fmt.Errorf("allocating polecat name: %w", err)
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	cleanupOrphanPolecatState(rigName, polecatName, r.Path, backend)

	existingPolecat, err := polecatMgr.Get(polecatName)

	addOpts := polecat.AddOptions{
		HookBead: opts.HookBead,
	}

	if err == nil {
		if !opts.Force {
			pGit := git.NewGit(existingPolecat.ClonePath)
			workStatus, checkErr := pGit.CheckUncommittedWork()
			if checkErr == nil && !workStatus.Clean() {
				return nil, fmt.Errorf("polecat '%s' has uncommitted work: %s\nUse --force to proceed anyway",
					polecatName, workStatus.String())
			}
		}

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
		fmt.Printf("Creating polecat %s...\n", polecatName)
		if _, err = polecatMgr.AddWithOptions(polecatName, addOpts); err != nil {
			return nil, fmt.Errorf("creating polecat: %w", err)
		}
	} else {
		return nil, fmt.Errorf("getting polecat: %w", err)
	}

	polecatObj, err := polecatMgr.Get(polecatName)
	if err != nil {
		return nil, fmt.Errorf("getting polecat after creation: %w", err)
	}

	if err := verifyWorktreeExists(polecatObj.ClonePath); err != nil {
		_ = polecatMgr.Remove(polecatName, true)
		return nil, fmt.Errorf("worktree verification failed for %s: %w\nHint: try 'gt polecat nuke %s/%s --force' to clean up",
			polecatName, err, rigName, polecatName)
	}

	if opts.HookBead != "" {
		if err := verifyAndSetHookBead(townRoot, rigName, polecatName, opts.HookBead); err != nil {
			fmt.Printf("Warning: could not verify hook_bead: %v\n", err)
		}
	}

	accountsPath := constants.MayorAccountsPath(townRoot)
	resolvedAccount, err := config.ResolveAccount(accountsPath, opts.Account)
	if err != nil {
		return nil, fmt.Errorf("resolving account: %w", err)
	}

	if err := config.ValidateAccountAuth(resolvedAccount); err != nil {
		return nil, err
	}

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

	// Materialize MCP config from config beads before session start
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

	polecatSessMgr := polecat.NewSessionManager(r)
	running, _ := polecatSessMgr.IsRunning(polecatName)

	if running {
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
		beadsClient := beads.New(townRoot)
		if agentBead, showErr := beadsClient.Show(agentBeadID); showErr == nil {
			if agentBead.HookBead != "" && agentBead.HookBead != opts.HookBead {
				fmt.Printf("  Polecat %s has hooked work (%s), killing stale session...\n",
					polecatName, agentBead.HookBead)
				sessionName := polecatSessMgr.SessionName(polecatName)
				_ = backend.KillSession(sessionName)
				running = false
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
			if errors.Is(err, polecat.ErrRateLimited) {
				fmt.Printf("⚠️  Rate limit detected during spawn\n")
				notifyWitnessRateLimit(rigName, polecatName, accountHandle)
			}
			return nil, fmt.Errorf("starting session: %w", err)
		}
	}

	sessionName := polecatSessMgr.SessionName(polecatName)

	if err := verifySpawnedPolecat(polecatObj.ClonePath, sessionName, backend); err != nil {
		return nil, fmt.Errorf("spawn verification failed for %s: %w", polecatName, err)
	}

	fmt.Printf("✓ Polecat %s spawned\n", polecatName)

	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	return &SpawnResult{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   polecatObj.ClonePath,
		SessionName: sessionName,
		Pane:        "",
		Account:     opts.Account,
		Agent:       opts.Agent,
	}, nil
}

// WakeRigAgents wakes the witness for a rig after polecat dispatch.
func WakeRigAgents(rigName string) {
	bootCmd := exec.Command("gt", "rig", "boot", rigName)
	_ = bootCmd.Run()

	// Nudge witness via backend (coop in K8s mode).
	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
	_ = backend.NudgeSession(witnessSession, "Polecat dispatched - check for work")
}

// OjSlingEnabled returns true when OJ dispatch is active.
func OjSlingEnabled() bool {
	return os.Getenv("GT_SLING_OJ") == "1"
}

func cleanupOrphanPolecatState(rigName, polecatName, rigPath string, backend terminal.Backend) {
	polecatDir := filepath.Join(rigPath, "polecats", polecatName)
	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)

	if err := backend.KillSession(sessionName); err == nil {
		fmt.Printf("  Cleaned up orphan session: %s\n", sessionName)
	}

	if entries, err := filepath.Glob(polecatDir + "/*"); err == nil && len(entries) == 0 {
		if rmErr := os.RemoveAll(polecatDir); rmErr == nil {
			fmt.Printf("  Cleaned up empty polecat directory: %s\n", polecatDir)
		}
	}

	repoGit := git.NewGit(rigPath)
	_ = repoGit.WorktreePrune()
}

func verifyWorktreeExists(clonePath string) error {
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

	gitPath := filepath.Join(clonePath, ".git")
	_, err = os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree missing .git file (not a valid git worktree): %s", clonePath)
		}
		return fmt.Errorf("checking .git: %w", err)
	}
	return nil
}

func verifySpawnedPolecat(clonePath, sessionName string, backend terminal.Backend) error {
	gitPath := filepath.Join(clonePath, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree disappeared: %s (missing .git)", clonePath)
		}
		return fmt.Errorf("checking worktree: %w", err)
	}

	hasSession, err := backend.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !hasSession {
		return fmt.Errorf("session disappeared: %s", sessionName)
	}
	return nil
}

func verifyAndSetHookBead(townRoot, rigName, polecatName, hookBead string) error {
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)

	beadsClient := beads.New(townRoot)
	agentIssue, err := beadsClient.Show(agentBeadID)
	if err != nil {
		return fmt.Errorf("reading agent bead %s: %w", agentBeadID, err)
	}

	if agentIssue.HookBead == hookBead {
		return nil
	}

	fmt.Printf("  Retrying hook_bead set for %s...\n", agentBeadID)
	if err := beadsClient.SetHookBead(agentBeadID, hookBead); err != nil {
		return fmt.Errorf("retrying hook_bead set: %w", err)
	}
	return nil
}

func notifyWitnessRateLimit(rigName, polecatName, account string) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return
	}

	subject := fmt.Sprintf("RATE_LIMITED polecat:%s", polecatName)
	body := fmt.Sprintf("Account: %s\nSource: spawn\nPolecat: %s\nRig: %s\n",
		account, polecatName, rigName)

	witnessAddr := fmt.Sprintf("%s/witness", rigName)
	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body)
	cmd.Dir = townRoot
	_ = cmd.Run()
}

// spawnPolecatForK8s creates an agent bead for a K8s polecat without creating
// a local worktree or tmux session. The K8s controller watches for agent beads
// with agent_state=spawning and execution_target:k8s label, then creates pods.
func spawnPolecatForK8s(townRoot, rigName string, r *rig.Rig, opts SpawnOptions) (*SpawnResult, error) {
	g := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, g)

	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return nil, fmt.Errorf("allocating polecat name: %w", err)
	}
	fmt.Printf("Allocated polecat: %s (K8s)\n", polecatName)

	// Create or reopen agent bead with spawning state and hook_bead set atomically.
	// Use rig beads (mayor/rig) to match where local polecats store their agent beads.
	// The K8s controller detects agent_state=spawning + execution_target:k8s label
	// and creates a pod for this polecat.
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
	rigBeadsPath := filepath.Join(r.Path, "mayor", "rig")
	beadsClient := beads.New(rigBeadsPath)
	_, err = beadsClient.CreateOrReopenAgentBead(agentBeadID, agentBeadID, &beads.AgentFields{
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

	return &SpawnResult{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   "", // No local worktree for K8s polecats
		SessionName: "", // No local tmux session
		Pane:        "",
		Account:     opts.Account,
		Agent:       opts.Agent,
		K8sSpawn:    true,
	}, nil
}

// GetSessionPane returns the pane identifier for a session.
// In K8s mode (no tmux), pane IDs are not applicable.
// Returns the session name as the pane identifier for backend-based sessions.
func GetSessionPane(sessionName string) (string, error) {
	// Pane IDs are a tmux concept. In K8s/coop mode, just return the session name.
	return sessionName, nil
}
