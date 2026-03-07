// Package cmd provides polecat spawning utilities for gt sling.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/daytona"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/proxy"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// SpawnedPolecatInfo contains info about a spawned polecat session.
type SpawnedPolecatInfo struct {
	RigName     string // Rig name (e.g., "gastown")
	PolecatName string // Polecat name (e.g., "Toast")
	ClonePath   string // Path to polecat's git worktree
	SessionName string // Tmux session name (e.g., "gt-gastown-p-Toast")
	Pane        string // Tmux pane ID (empty until StartSession is called)
	BaseBranch  string // Effective base branch (e.g., "main", "integration/epic-id")
	Branch      string // Git branch name (for cleanup on rollback)

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
	Force      bool   // Force spawn even if polecat has uncommitted work
	Account    string // Claude Code account handle to use
	Create     bool   // Create polecat if it doesn't exist (currently always true for sling)
	HookBead   string // Bead ID to set as hook_bead at spawn time (atomic assignment)
	Agent      string // Agent override for this spawn (e.g., "gemini", "codex", "claude-haiku")
	BaseBranch string // Override base branch for polecat worktree (e.g., "develop", "release/v2")
	Daytona    bool   // Force daytona remote mode (overrides rig config auto-detection)
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

	// Daytona mode detection: explicit --daytona flag or auto-detect from rig config.
	// When active, configure the polecat manager for remote mode and run preflight checks.
	settingsPath := filepath.Join(r.Path, "settings", "config.json")
	rigSettings, _ := config.LoadRigSettings(settingsPath)
	useDaytona := shouldUseDaytona(opts.Daytona, rigSettings)
	var daytonaClient *daytona.Client
	if useDaytona {
		if err := runDaytonaPreflightChecks(townRoot, rigSettings); err != nil {
			return nil, fmt.Errorf("daytona preflight failed: %w", err)
		}
		// Load town config for installation prefix
		townConfigPath := filepath.Join(townRoot, "mayor", "town.json")
		townConfig, err := config.LoadTownConfig(townConfigPath)
		if err != nil {
			return nil, fmt.Errorf("loading town config for daytona: %w", err)
		}
		shortID := townConfig.ShortInstallationID()
		if shortID == "" {
			return nil, fmt.Errorf("empty InstallationID in town config — cannot scope daytona workspaces. Run 'gt install' to initialize")
		}
		installPrefix := constants.InstallPrefix(shortID)
		daytonaClient = daytona.NewClient(installPrefix)

		// Load proxy CA (required for mTLS cert issuance to remote polecats)
		caDir := filepath.Join(townRoot, ".runtime", "ca")
		ca, err := proxy.LoadOrGenerateCA(caDir)
		if err != nil {
			return nil, fmt.Errorf("loading proxy CA for daytona: %w", err)
		}

		// Ensure rig settings has RemoteBackend when --daytona flag forces it
		if rigSettings == nil {
			rigSettings = &config.RigSettings{}
		}
		if rigSettings.RemoteBackend == nil {
			rigSettings.RemoteBackend = &config.RemoteBackend{Provider: "daytona"}
		}

		polecatMgr.SetDaytona(daytonaClient, ca, rigSettings)
		fmt.Printf("  Daytona remote mode enabled (prefix: %s)\n", installPrefix)
	}

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

	// Polecat count cap (clown show #22): refuse to spawn if there are already
	// too many active polecats. This is a last-resort safety net for the direct-dispatch
	// path. For configurable capacity gating, use scheduler.max_polecats in town settings
	// (see internal/scheduler/capacity/).
	const defaultMaxActivePolecats = 25
	activeCount := countActivePolecats()
	if activeCount >= defaultMaxActivePolecats {
		return nil, fmt.Errorf("polecat cap reached: %d active polecats (max %d). "+
			"This is a safety limit to prevent spawn storms. "+
			"Investigate why polecats are accumulating before spawning more",
			activeCount, defaultMaxActivePolecats)
	}

	// Per-bead respawn circuit breaker (clown show #22):
	// Track how many times this bead has been slung. Block after N attempts
	// to prevent witness→deacon→sling feedback loops.
	if opts.HookBead != "" && !opts.Force {
		if witness.ShouldBlockRespawn(townRoot, opts.HookBead) {
			maxRespawns := config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV()
			return nil, fmt.Errorf("respawn limit reached for %s (%d attempts). "+
				"This bead keeps failing — investigate before re-dispatching.\n"+
				"Override: gt sling %s %s --force\n"+
				"Reset:    gt sling respawn-reset %s",
				opts.HookBead, maxRespawns,
				opts.HookBead, rigName, opts.HookBead)
		}
		witness.RecordBeadRespawn(townRoot, opts.HookBead)
	}

	// Per-rig directory cap: prevent unbounded worktree accumulation even when
	// polecats die quickly (tmux session count stays low).
	const maxPolecatDirsPerRig = 30
	rigPolecatDir := filepath.Join(townRoot, rigName, "polecats")
	if entries, err := os.ReadDir(rigPolecatDir); err == nil {
		dirCount := 0
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirCount++
			}
		}
		if dirCount >= maxPolecatDirsPerRig {
			return nil, fmt.Errorf("rig %s has %d polecat directories (max %d). "+
				"Nuke idle polecats first: gt polecat nuke %s/<name> --force",
				rigName, dirCount, maxPolecatDirsPerRig, rigName)
		}
	}

	// Persistent polecat model (gt-4ac): try to reuse an idle polecat first.
	// Idle polecats have completed their work but kept their sandbox (worktree).
	// Reusing avoids the overhead of creating a new worktree.
	//
	// Use FindAndReuseIdlePolecat for atomic find+claim under pool lock (gtd-frf).
	// This prevents concurrent gt sling processes from racing on the same idle polecat.
	{
		// Determine base branch before atomic find+reuse
		reuseBaseBranch := opts.BaseBranch
		if reuseBaseBranch == "" && opts.HookBead != "" {
			settingsPath := filepath.Join(r.Path, "settings", "config.json")
			polecatIntegrationEnabled := true
			if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
				polecatIntegrationEnabled = settings.MergeQueue.IsPolecatIntegrationEnabled()
			}
			if polecatIntegrationEnabled {
				repoGit, repoErr := getRigGit(r.Path)
				if repoErr == nil {
					bd := beads.New(r.Path)
					detected, detectErr := beads.DetectIntegrationBranch(bd, repoGit, opts.HookBead)
					if detectErr == nil && detected != "" {
						reuseBaseBranch = "origin/" + detected
						fmt.Printf("  Auto-detected integration branch: %s\n", detected)
					}
				}
			}
		}
		if reuseBaseBranch != "" && !strings.HasPrefix(reuseBaseBranch, "origin/") {
			reuseBaseBranch = "origin/" + reuseBaseBranch
		}

		addOpts := polecat.AddOptions{
			HookBead:   opts.HookBead,
			BaseBranch: reuseBaseBranch,
		}

		// Build Daytona workspace filter: skip idle polecats whose workspace
		// was auto-deleted.
		var filter polecat.IdlePolecatFilter
		if useDaytona && daytonaClient != nil {
			filter = func(p *polecat.Polecat) bool {
				wsName := p.DaytonaWorkspaceName
				if wsName == "" {
					wsName = daytonaClient.WorkspaceName(rigName, p.Name)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				_, wsErr := daytonaClient.Info(ctx, wsName)
				cancel()
				if wsErr != nil {
					fmt.Printf("Idle polecat %s workspace %s gone (auto-deleted?), skipping reuse\n",
						p.Name, wsName)
					return false
				}
				return true
			}
		}

		reusedPolecat, reuseErr := polecatMgr.FindAndReuseIdlePolecat(addOpts, filter)
		if reuseErr != nil {
			// Reuse failed (e.g., branch-only reuse failed) — fall through to new allocation.
			fmt.Printf("  Idle polecat reuse failed: %v, allocating new...\n", reuseErr)
		}
		if reusedPolecat != nil {
			polecatName := reusedPolecat.Name
			fmt.Printf("Reusing idle polecat: %s\n", polecatName)

			polecatObj, err := polecatMgr.Get(polecatName)
			if err != nil {
				return nil, fmt.Errorf("getting idle polecat after reuse: %w", err)
			}
			// Skip worktree verification for Daytona polecats — no local worktree.
			// The workspace will be restarted (if stopped) by StartSession.
			if !useDaytona {
				if err := verifyWorktreeExists(polecatObj.ClonePath); err != nil {
					return nil, fmt.Errorf("worktree verification failed for reused %s: %w", polecatName, err)
				}
			}

			polecatSessMgr := polecat.NewSessionManager(t, r)
			sessionName := polecatSessMgr.SessionName(polecatName)

			fmt.Printf("%s Polecat %s reused (idle → working, session start deferred)\n", style.Bold.Render("✓"), polecatName)
			_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

			effectiveBranch := strings.TrimPrefix(reuseBaseBranch, "origin/")
			if effectiveBranch == "" {
				effectiveBranch = r.DefaultBranch()
			}

			return &SpawnedPolecatInfo{
				RigName:     rigName,
				PolecatName: polecatName,
				ClonePath:   polecatObj.ClonePath,
				SessionName: sessionName,
				Pane:        "",
				BaseBranch:  effectiveBranch,
				Branch:      polecatObj.Branch,
				account:     opts.Account,
				agent:       opts.Agent,
			}, nil
		}
	}

	// Determine base branch for polecat worktree
	baseBranch := opts.BaseBranch
	if baseBranch == "" && opts.HookBead != "" {
		settingsPath := filepath.Join(r.Path, "settings", "config.json")
		polecatIntegrationEnabled := true
		if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
			polecatIntegrationEnabled = settings.MergeQueue.IsPolecatIntegrationEnabled()
		}
		if polecatIntegrationEnabled {
			repoGit, repoErr := getRigGit(r.Path)
			if repoErr == nil {
				bd := beads.New(r.Path)
				detected, detectErr := beads.DetectIntegrationBranch(bd, repoGit, opts.HookBead)
				if detectErr == nil && detected != "" {
					baseBranch = "origin/" + detected
					fmt.Printf("  Auto-detected integration branch: %s\n", detected)
				}
			}
		}
	}
	if baseBranch != "" && !strings.HasPrefix(baseBranch, "origin/") {
		baseBranch = "origin/" + baseBranch
	}

	addOpts := polecat.AddOptions{
		HookBead:   opts.HookBead,
		BaseBranch: baseBranch,
	}

	// Build filter for Daytona workspace existence check (runs under pool lock).
	var idleFilter polecat.IdlePolecatFilter
	if useDaytona && daytonaClient != nil {
		idleFilter = func(p *polecat.Polecat) bool {
			wsName := p.DaytonaWorkspaceName
			if wsName == "" {
				wsName = daytonaClient.WorkspaceName(rigName, p.Name)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_, wsErr := daytonaClient.Info(ctx, wsName)
			cancel()
			if wsErr != nil {
				fmt.Printf("Idle polecat %s workspace %s gone (auto-deleted?), skipping reuse\n",
					p.Name, wsName)
				return false
			}
			return true
		}
	}

	reusedPolecat, reuseErr := polecatMgr.FindAndReuseIdlePolecat(addOpts, idleFilter)
	if reuseErr != nil {
		// Atomic reuse failed — try full worktree repair as fallback (local mode only).
		// For Daytona mode, fall through to new allocation since there's no local worktree to repair.
		if !useDaytona {
			fmt.Printf("  Idle polecat reuse failed: %v, trying repair...\n", reuseErr)
			// Re-find without lock to get the name for repair attempt
			if idleP, _ := polecatMgr.FindIdlePolecat(); idleP != nil {
				if _, repairErr := polecatMgr.RepairWorktreeWithOptions(idleP.Name, true, addOpts); repairErr == nil {
					reusedPolecat, reuseErr = polecatMgr.Get(idleP.Name)
					if reuseErr == nil {
						reusedPolecat.State = polecat.StateWorking
					}
				}
			}
		}
	}
	if reuseErr == nil && reusedPolecat != nil {
		polecatName := reusedPolecat.Name
		fmt.Printf("Reusing idle polecat: %s\n", polecatName)

		// Skip worktree verification for Daytona polecats — no local worktree.
		// The workspace will be restarted (if stopped) by StartSession.
		if !useDaytona {
			if err := verifyWorktreeExists(reusedPolecat.ClonePath); err != nil {
				return nil, fmt.Errorf("worktree verification failed for reused %s: %w", polecatName, err)
			}
		}

		polecatSessMgr := polecat.NewSessionManager(t, r)
		sessionName := polecatSessMgr.SessionName(polecatName)

		fmt.Printf("%s Polecat %s reused (idle → working, session start deferred)\n", style.Bold.Render("✓"), polecatName)
		_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

		effectiveBranch := strings.TrimPrefix(baseBranch, "origin/")
		if effectiveBranch == "" {
			effectiveBranch = r.DefaultBranch()
		}

		return &SpawnedPolecatInfo{
			RigName:     rigName,
			PolecatName: polecatName,
			ClonePath:   reusedPolecat.ClonePath,
			SessionName: sessionName,
			Pane:        "",
			BaseBranch:  effectiveBranch,
			Branch:      reusedPolecat.Branch,
			account:     opts.Account,
			agent:       opts.Agent,
		}, nil
	}

	// No idle polecat available — allocate and create atomically (GH#2215).
	// baseBranch and addOpts were computed above (shared by reuse and fresh paths).
	// AllocateAndAdd holds the pool lock through directory creation, preventing
	// concurrent processes from allocating the same name.
	polecatName, _, err := polecatMgr.AllocateAndAdd(addOpts)
	if err != nil {
		return nil, fmt.Errorf("allocating and creating polecat: %w", err)
	}
	fmt.Printf("Created polecat: %s\n", polecatName)

	// Get polecat object for path info
	polecatObj, err := polecatMgr.Get(polecatName)
	if err != nil {
		// Clean up partial state: AllocateAndAdd succeeded, so the polecat directory,
		// agent bead, branch, workspace, and cert are all allocated. Without removal
		// they leak with no cleanup path.
		_ = polecatMgr.Remove(polecatName, true)
		return nil, fmt.Errorf("getting polecat after creation: %w", err)
	}

	// Verify worktree was actually created (fixes #1070)
	// The identity bead may exist but worktree creation can fail silently.
	// Skip for daytona polecats — their worktree is remote, not local.
	if !useDaytona {
		if err := verifyWorktreeExists(polecatObj.ClonePath); err != nil {
			// Clean up the partial state before returning error
			_ = polecatMgr.Remove(polecatName, true) // force=true to clean up partial state
			return nil, fmt.Errorf("worktree verification failed for %s: %w\nHint: try 'gt polecat nuke %s/%s --force' to clean up",
				polecatName, err, rigName, polecatName)
		}
	}

	// Get session manager for session name (session start is deferred)
	polecatSessMgr := polecat.NewSessionManager(t, r)
	sessionName := polecatSessMgr.SessionName(polecatName)

	fmt.Printf("%s Polecat %s spawned (session start deferred)\n", style.Bold.Render("✓"), polecatName)

	// Log spawn event to activity feed
	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, polecatName))

	// Compute effective base branch (strip origin/ prefix since formula prepends it)
	effectiveBranch := strings.TrimPrefix(baseBranch, "origin/")
	if effectiveBranch == "" {
		effectiveBranch = r.DefaultBranch()
	}

	return &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		ClonePath:   polecatObj.ClonePath,
		SessionName: sessionName,
		Pane:        "", // Empty until StartSession is called
		BaseBranch:  effectiveBranch,
		Branch:      polecatObj.Branch,
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

	// Configure Daytona remote mode if rig has RemoteBackend configured.
	rigSettings, _ := config.LoadRigSettings(config.RigSettingsPath(r.Path))
	if rigSettings != nil && rigSettings.RemoteBackend != nil {
		townConfigPath := filepath.Join(townRoot, "mayor", "town.json")
		townConfig, err := config.LoadTownConfig(townConfigPath)
		if err == nil {
			shortID := townConfig.ShortInstallationID()
			if shortID != "" {
				installPrefix := constants.InstallPrefix(shortID)
				daytonaClient := daytona.NewClient(installPrefix)
				polecatSessMgr.SetDaytona(daytonaClient, rigSettings)
			}
		}
	}

	fmt.Printf("Starting session for %s/%s...\n", s.RigName, s.PolecatName)
	startOpts := polecat.SessionStartOptions{
		RuntimeConfigDir: claudeConfigDir,
		Agent:            s.agent,
		Branch:           s.Branch,
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
	// When an agent override is specified (e.g., --agent codex), resolve the runtime
	// config from the override so WaitForRuntimeReady uses the correct readiness
	// strategy (delay-based for Codex vs prompt-polling for Claude). Without this,
	// ResolveRoleAgentConfig returns the default agent (Claude) and polls for "❯ "
	// in a Codex session, always timing out after 30 seconds (gt-1j3m).
	spawnTownRoot := filepath.Dir(r.Path)
	var runtimeConfig *config.RuntimeConfig
	if s.agent != "" {
		rc, _, err := config.ResolveAgentConfigWithOverride(spawnTownRoot, r.Path, s.agent)
		if err != nil {
			style.PrintWarning("resolving agent config for %s: %v (using default)", s.agent, err)
			runtimeConfig = config.ResolveRoleAgentConfig("polecat", spawnTownRoot, r.Path)
		} else {
			runtimeConfig = rc
		}
	} else {
		runtimeConfig = config.ResolveRoleAgentConfig("polecat", spawnTownRoot, r.Path)
	}
	if err := t.WaitForRuntimeReady(s.SessionName, runtimeConfig, 30*time.Second); err != nil {
		style.PrintWarning("runtime may not be fully ready: %v", err)
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
		style.PrintWarning("could not update agent state after retries: %v", err)
	}

	// Update issue status from hooked to in_progress.
	// Also warn-only for the same reason: session is already running.
	if err := polecatMgr.SetState(s.PolecatName, polecat.StateWorking); err != nil {
		style.PrintWarning("could not update issue status to in_progress: %v", err)
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

// IsRigName checks if a target string is a rig name (not a role or path).
// Returns the rig name and true if it's a valid rig.
func IsRigName(target string) (string, bool) {
	// If it contains a slash, it's a path format (rig/role or rig/crew/name)
	if strings.Contains(target, "/") {
		return "", false
	}

	// Check known non-rig role names
	switch strings.ToLower(target) {
	case constants.RoleMayor, "may", constants.RoleDeacon, "dea", constants.RoleCrew, constants.RoleWitness, "wit", constants.RoleRefinery, "ref":
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

// shouldUseDaytona returns true if daytona remote mode should be activated,
// either from an explicit flag or auto-detected from rig settings.
func shouldUseDaytona(explicitFlag bool, settings *config.RigSettings) bool {
	if explicitFlag {
		return true
	}
	return settings != nil && settings.RemoteBackend != nil && settings.RemoteBackend.Provider == "daytona"
}

// checkCAFiles verifies that the proxy CA certificate and key exist at the
// expected paths under townRoot. Returns nil if both files are present.
func checkCAFiles(townRoot string) error {
	caDir := filepath.Join(townRoot, ".runtime", "ca")
	certPath := filepath.Join(caDir, "ca.crt")
	keyPath := filepath.Join(caDir, "ca.key")
	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("proxy CA certificate not found at %s\n"+
			"The proxy server creates the CA on startup. Start the proxy first: gt-proxy-server", certPath)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return fmt.Errorf("proxy CA key not found at %s\n"+
			"The proxy server creates the CA on startup. Start the proxy first: gt-proxy-server", keyPath)
	}
	return nil
}

// runDaytonaPreflightChecks verifies the prerequisites for daytona remote mode:
// 1. daytona CLI is installed and accessible
// 2. proxy server is running (admin API reachable)
// 3. proxy CA exists (required for mTLS cert issuance)
// 4. daytona CLI is authenticated (has active profile)
func runDaytonaPreflightChecks(townRoot string, settings *config.RigSettings) error {
	// 1. Verify daytona CLI is installed
	if _, err := exec.LookPath("daytona"); err != nil {
		return fmt.Errorf("daytona CLI not found in PATH\n" +
			"Install: https://www.daytona.io/docs/installation/installation/\n" +
			"Or remove remote_backend from rig settings to use local mode")
	}

	// 2. Verify proxy server is running (check admin API reachability)
	adminAddr := constants.DefaultProxyAdminAddr
	if settings != nil && settings.RemoteBackend != nil && settings.RemoteBackend.ProxyAdminAddr != "" {
		adminAddr = settings.RemoteBackend.ProxyAdminAddr
	}
	adminClient := proxy.NewAdminClient(adminAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminClient.Ping(ctx); err != nil {
		return fmt.Errorf("proxy server not reachable at %s: %w\n"+
			"Start the proxy: gt-proxy-server --town-root %s", adminAddr, err, townRoot)
	}

	// 3. Verify proxy CA exists
	if err := checkCAFiles(townRoot); err != nil {
		return err
	}

	// 4. Verify daytona is authenticated
	// An unauthenticated CLI passes all other checks but causes confusing errors
	// later during sandbox creation. Use "daytona list" as a lightweight auth check
	// (v0.149+ removed the "profile" subcommand).
	listCmd := exec.Command("daytona", "list")
	if output, err := listCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daytona CLI is not authenticated: %s\n"+
			"Run 'daytona login' to authenticate", strings.TrimSpace(string(output)))
	}

	return nil
}

// verifyWorktreeExists checks that a git worktree was actually created at the given path
// and that it is a functional git repository. Returns an error if the worktree is missing,
// has a broken .git reference, or fails basic git validation. (GH#2056)
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
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree missing .git file (not a valid git worktree): %s", clonePath)
		}
		return fmt.Errorf("checking .git: %w", err)
	}

	// For worktree .git files, verify the gitdir reference points to a valid path.
	// A broken reference (e.g., from os.Rename instead of git worktree move) causes
	// "fatal: not a git repository" for every git operation.
	gitContent, err := os.ReadFile(gitPath)
	if err == nil {
		content := strings.TrimSpace(string(gitContent))
		if strings.HasPrefix(content, "gitdir: ") {
			gitdirPath := strings.TrimPrefix(content, "gitdir: ")
			if !filepath.IsAbs(gitdirPath) {
				gitdirPath = filepath.Join(clonePath, gitdirPath)
			}
			if _, err := os.Stat(gitdirPath); err != nil {
				return fmt.Errorf("worktree .git references nonexistent gitdir %s: %w", gitdirPath, err)
			}
		}
	}

	// Final validation: run git rev-parse to confirm the worktree is functional
	cmd := exec.Command("git", "-C", clonePath, "rev-parse", "--git-dir")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree at %s is not a valid git repository: %s", clonePath, strings.TrimSpace(string(output)))
	}

	return nil
}
