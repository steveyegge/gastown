package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// crewAtRetried tracks if we've already retried after stale session cleanup
var crewAtRetried bool

func runCrewAt(cmd *cobra.Command, args []string) error {
	var name string

	// Debug mode: --debug flag or GT_DEBUG env var
	debug := crewDebug || os.Getenv("GT_DEBUG") != ""
	if debug {
		cwd, _ := os.Getwd()
		fmt.Printf("[DEBUG] runCrewAt: args=%v, crewRig=%q, cwd=%q\n", args, crewRig, cwd)
	}

	// Determine crew name: from arg, or auto-detect from cwd
	if len(args) > 0 {
		name = args[0]
		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if crewRig == "" {
				crewRig = rig
			}
			name = crewName
		}
	} else {
		// Try to detect from current directory
		detected, err := detectCrewFromCwd()
		if err != nil {
			// Try to show available crew members if we can detect the rig
			hint := "\n\nUsage: gt crew at <name>"
			if crewRig != "" {
				if mgr, _, mgrErr := getCrewManager(crewRig); mgrErr == nil {
					if members, listErr := mgr.List(); listErr == nil && len(members) > 0 {
						hint = fmt.Sprintf("\n\nAvailable crew in %s:", crewRig)
						for _, m := range members {
							hint += fmt.Sprintf("\n  %s", m.Name)
						}
					}
				}
			}
			return fmt.Errorf("could not detect crew workspace from current directory: %w%s", err, hint)
		}
		name = detected.crewName
		if crewRig == "" {
			crewRig = detected.rigName
		}
		fmt.Printf("Detected crew workspace: %s/%s\n", detected.rigName, name)
	}

	if debug {
		fmt.Printf("[DEBUG] after detection: name=%q, crewRig=%q\n", name, crewRig)
	}

	crewMgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	// Get the crew worker
	worker, err := crewMgr.Get(name)
	if err != nil {
		if err == crew.ErrCrewNotFound {
			return fmt.Errorf("crew workspace '%s' not found", name)
		}
		return fmt.Errorf("getting crew worker: %w", err)
	}

	// Ensure crew workspace is on default branch (persistent roles should not use feature branches)
	ensureDefaultBranch(worker.ClonePath, fmt.Sprintf("Crew workspace %s/%s", r.Name, name), r.Path)

	// If --no-tmux, just print the path
	if crewNoTmux {
		fmt.Println(worker.ClonePath)
		return nil
	}

	// Resolve account for runtime config
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	accountsPath := constants.MayorAccountsPath(townRoot)
	resolvedAccount, err := config.ResolveAccount(accountsPath, crewAccount)
	if err != nil {
		return fmt.Errorf("resolving account: %w", err)
	}

	// Validate that the account has credentials before starting
	// This prevents OAuth prompts from appearing in crew sessions
	if err := config.ValidateAccountAuth(resolvedAccount); err != nil {
		return err
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

	runtimeConfig := config.LoadRuntimeConfig(r.Path)
	if err := runtime.EnsureSettingsForRoleWithAccount(worker.ClonePath, "crew", claudeConfigDir, runtimeConfig); err != nil {
		// Non-fatal but log warning - missing settings can cause agents to start without hooks
		style.PrintWarning("could not ensure settings for %s: %v", name, err)
	}

	// Check if session exists
	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)
	if debug {
		fmt.Printf("[DEBUG] sessionID=%q (r.Name=%q, name=%q)\n", sessionID, r.Name, name)
	}
	hasSession, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if debug {
		fmt.Printf("[DEBUG] hasSession=%v\n", hasSession)
	}

	// Before creating a new session, check if there's already a runtime session
	// running in this crew's directory (might have been started manually or via
	// a different mechanism)
	if !hasSession {
		existingSessions, err := t.FindSessionByWorkDir(worker.ClonePath, runtimeConfig.Tmux.ProcessNames)
		if err == nil && len(existingSessions) > 0 {
			// Found an existing session with runtime running in this directory
			existingSession := existingSessions[0]
			fmt.Printf("%s Found existing runtime session '%s' in crew directory\n",
				style.Warning.Render("⚠"),
				existingSession)
			fmt.Printf("  Attaching to existing session instead of creating a new one\n")

			// If inside tmux (but different session), inform user
			if tmux.IsInsideTmux() {
				fmt.Printf("Use C-b s to switch to '%s'\n", existingSession)
				return nil
			}

			// Outside tmux: attach unless --detached flag is set
			if crewDetached {
				fmt.Printf("Existing session: '%s'. Run 'tmux attach -t %s' to attach.\n",
					existingSession, existingSession)
				return nil
			}

			// Attach to existing session
			return attachToTmuxSession(existingSession)
		}
	}

	if !hasSession {
		// Create new session
		if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
			return fmt.Errorf("creating session: %w", err)
		}

		// Set environment (non-fatal: session works without these)
		// Use centralized AgentEnv for consistency across all role startup paths
		envVars := config.AgentEnv(config.AgentEnvConfig{
			Role:             "crew",
			Rig:              r.Name,
			AgentName:        name,
			TownRoot:         townRoot,
			RuntimeConfigDir: claudeConfigDir,
			BeadsNoDaemon:    true,
			AuthToken:        authToken,
			BaseURL:          baseURL,
		})
		for k, v := range envVars {
			_ = t.SetEnvironment(sessionID, k, v)
		}

		// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
		// Note: ConfigureGasTownSession includes cycle bindings
		theme := getThemeForRig(r.Name)
		_ = t.ConfigureGasTownSession(sessionID, theme, r.Name, name, "crew")

		// Wait for shell to be ready after session creation
		if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
			return fmt.Errorf("waiting for shell: %w", err)
		}

		// Get pane ID for respawn
		paneID, err := t.GetPaneID(sessionID)
		if err != nil {
			return fmt.Errorf("getting pane ID: %w", err)
		}

		// Build startup beacon for predecessor discovery via /resume
		// Use FormatStartupBeacon instead of bare "gt prime" which confuses agents
		// The SessionStart hook handles context injection (gt prime --hook)
		address := fmt.Sprintf("%s/crew/%s", r.Name, name)
		beacon := session.FormatStartupBeacon(session.BeaconConfig{
			Recipient: address,
			Sender:    "human",
			Topic:     "start",
		})

		// Use respawn-pane to replace shell with runtime directly
		// This gives cleaner lifecycle: runtime exits → session ends (no intermediate shell)
		// Export GT_ROLE and BD_ACTOR since tmux SetEnvironment only affects new panes
		startupCmd, err := config.BuildCrewStartupCommandWithAgentOverride(r.Name, name, r.Path, beacon, crewAgentOverride)
		if err != nil {
			return fmt.Errorf("building startup command: %w", err)
		}
		// Prepend config dir env if available
		if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && claudeConfigDir != "" {
			startupCmd = config.PrependEnv(startupCmd, map[string]string{runtimeConfig.Session.ConfigDirEnv: claudeConfigDir})
		}
		// Note: Don't call KillPaneProcesses here - this is a NEW session with just
		// a fresh shell. Killing it would destroy the pane before we can respawn.
		// KillPaneProcesses is only needed when restarting in an EXISTING session
		// where Claude/Node processes might be running and ignoring SIGHUP.
		if err := t.RespawnPane(paneID, startupCmd); err != nil {
			return fmt.Errorf("starting runtime: %w", err)
		}

		fmt.Printf("%s Created session for %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)
	} else {
		// Session exists - check if runtime is still running
		// Uses both pane command check and UI marker detection to avoid
		// restarting when user is in a subshell spawned from the runtime
		agentCfg, _, err := config.ResolveAgentConfigWithOverride(townRoot, r.Path, crewAgentOverride)
		if err != nil {
			return fmt.Errorf("resolving agent: %w", err)
		}
		if !t.IsAgentRunning(sessionID, config.ExpectedPaneCommands(agentCfg)...) {
			// Runtime has exited, restart it using respawn-pane
			fmt.Printf("Runtime exited, restarting...\n")

			// Get pane ID for respawn
			paneID, err := t.GetPaneID(sessionID)
			if err != nil {
				return fmt.Errorf("getting pane ID: %w", err)
			}

			// Build startup beacon for predecessor discovery via /resume
			// Use FormatStartupBeacon instead of bare "gt prime" which confuses agents
			address := fmt.Sprintf("%s/crew/%s", r.Name, name)
			beacon := session.FormatStartupBeacon(session.BeaconConfig{
				Recipient: address,
				Sender:    "human",
				Topic:     "restart",
			})

			// Use respawn-pane to replace shell with runtime directly
			// Export GT_ROLE and BD_ACTOR since tmux SetEnvironment only affects new panes
			startupCmd, err := config.BuildCrewStartupCommandWithAgentOverride(r.Name, name, r.Path, beacon, crewAgentOverride)
			if err != nil {
				return fmt.Errorf("building startup command: %w", err)
			}
			// Prepend config dir env if available
			if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && claudeConfigDir != "" {
				startupCmd = config.PrependEnv(startupCmd, map[string]string{runtimeConfig.Session.ConfigDirEnv: claudeConfigDir})
			}
			// Kill all processes in the pane before respawning to prevent orphan leaks
			// RespawnPane's -k flag only sends SIGHUP which Claude/Node may ignore
			if err := t.KillPaneProcesses(paneID); err != nil {
				// Non-fatal but log the warning
				style.PrintWarning("could not kill pane processes: %v", err)
			}
			if err := t.RespawnPane(paneID, startupCmd); err != nil {
				// If pane is stale (session exists but pane doesn't), recreate the session
				if strings.Contains(err.Error(), "can't find pane") {
					if crewAtRetried {
						return fmt.Errorf("stale session persists after cleanup: %w", err)
					}
					fmt.Printf("Stale session detected, recreating...\n")
					if killErr := t.KillSession(sessionID); killErr != nil {
						return fmt.Errorf("failed to kill stale session: %w", killErr)
					}
					crewAtRetried = true
					defer func() { crewAtRetried = false }()
					return runCrewAt(cmd, args) // Retry with fresh session
				}
				return fmt.Errorf("restarting runtime: %w", err)
			}
		}
	}

	// Check if we're already in the target session
	if isInTmuxSession(sessionID) {
		// Check if agent is already running - don't restart if so
		agentCfg, _, err := config.ResolveAgentConfigWithOverride(townRoot, r.Path, crewAgentOverride)
		if err != nil {
			return fmt.Errorf("resolving agent: %w", err)
		}
		if t.IsAgentRunning(sessionID, config.ExpectedPaneCommands(agentCfg)...) {
			// Agent is already running, nothing to do
			fmt.Printf("Already in %s session with %s running.\n", name, agentCfg.Command)
			return nil
		}

		// We're in the session at a shell prompt - start the agent
		// Build startup beacon for predecessor discovery via /resume
		address := fmt.Sprintf("%s/crew/%s", r.Name, name)
		beacon := session.FormatStartupBeacon(session.BeaconConfig{
			Recipient: address,
			Sender:    "human",
			Topic:     "start",
		})
		fmt.Printf("Starting %s in current session...\n", agentCfg.Command)
		return execAgent(agentCfg, beacon)
	}

	// If inside tmux (but different session), don't switch - just inform user
	insideTmux := tmux.IsInsideTmux()
	if debug {
		fmt.Printf("[DEBUG] tmux.IsInsideTmux()=%v\n", insideTmux)
	}
	if insideTmux {
		fmt.Printf("Session %s ready. Use C-b s to switch.\n", sessionID)
		return nil
	}

	// Outside tmux: attach unless --detached flag is set
	if crewDetached {
		fmt.Printf("Started %s/%s. Run 'gt crew at %s' to attach.\n", r.Name, name, name)
		return nil
	}

	// Attach to session - show which session we're attaching to
	fmt.Printf("Attaching to %s...\n", sessionID)
	if debug {
		fmt.Printf("[DEBUG] calling attachToTmuxSession(%q)\n", sessionID)
	}
	return attachToTmuxSession(sessionID)
}
