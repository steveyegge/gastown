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
	"github.com/steveyegge/gastown/internal/terminal"
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
		// No local rig: try remote attach via coop if connected to a daemon.
		if crewRig != "" && name != "" {
			if attached, remoteErr := tryRemoteCrewAt(crewRig, name); attached {
				return remoteErr
			}
		}
		return err
	}

	// Get the crew worker
	worker, err := crewMgr.Get(name)
	if err != nil {
		if err == crew.ErrCrewNotFound {
			// Might exist remotely: try coop attach.
			if crewRig != "" {
				if attached, remoteErr := tryRemoteCrewAt(crewRig, name); attached {
					return remoteErr
				}
			}
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
	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	sessionID := crewSessionName(r.Name, name)
	if debug {
		fmt.Printf("[DEBUG] sessionID=%q (r.Name=%q, name=%q)\n", sessionID, r.Name, name)
	}
	hasSession, err := backend.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if debug {
		fmt.Printf("[DEBUG] hasSession=%v\n", hasSession)
	}

	if !hasSession {
		// Session doesn't exist — build startup command and start via backend
		address := fmt.Sprintf("%s/crew/%s", r.Name, name)
		beacon := session.FormatStartupBeacon(session.BeaconConfig{
			Recipient: address,
			Sender:    "human",
			Topic:     "start",
		})

		startupCmd, err := config.BuildCrewStartupCommandWithAgentOverride(r.Name, name, r.Path, beacon, crewAgentOverride)
		if err != nil {
			return fmt.Errorf("building startup command: %w", err)
		}
		if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && claudeConfigDir != "" {
			startupCmd = config.PrependEnv(startupCmd, map[string]string{runtimeConfig.Session.ConfigDirEnv: claudeConfigDir})
		}

		// Set environment via backend
		envVars := config.AgentEnv(config.AgentEnvConfig{
			Role:             "crew",
			Rig:              r.Name,
			AgentName:        name,
			TownRoot:         townRoot,
			RuntimeConfigDir: claudeConfigDir,
			BeadsNoDaemon:    true,
			BDDaemonHost:     os.Getenv("BD_DAEMON_HOST"),
			AuthToken:        authToken,
			BaseURL:          baseURL,
		})
		for k, v := range envVars {
			_ = backend.SetEnvironment(sessionID, k, v)
		}

		if err := backend.RespawnPane(sessionID); err != nil {
			return fmt.Errorf("starting runtime: %w", err)
		}

		fmt.Printf("%s Created session for %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)
	} else {
		// Session exists - check if runtime is still running
		if running, _ := backend.IsAgentRunning(sessionID); !running {
			// Runtime has exited, restart it
			fmt.Printf("Runtime exited, restarting...\n")

			if err := backend.RespawnPane(sessionID); err != nil {
				if strings.Contains(err.Error(), "can't find pane") {
					if crewAtRetried {
						return fmt.Errorf("stale session persists after cleanup: %w", err)
					}
					fmt.Printf("Stale session detected, recreating...\n")
					if killErr := backend.KillSession(sessionID); killErr != nil {
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
		if running, _ := backend.IsAgentRunning(sessionID); running {
			// Agent is already running, nothing to do
			fmt.Printf("Already in %s session with %s running.\n", name, agentCfg.Command)
			return nil
		}

		// We're in the session at a shell prompt - start the agent
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
	insideTmux := os.Getenv("TMUX") != ""
	if debug {
		fmt.Printf("[DEBUG] insideTmux=%v\n", insideTmux)
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

	// Attach to session
	fmt.Printf("Attaching to %s...\n", sessionID)
	if debug {
		fmt.Printf("[DEBUG] calling attachToTmuxSession(%q)\n", sessionID)
	}
	return attachToTmuxSession(sessionID)
}

// tryRemoteCrewAt attempts to attach to a remote K8s crew pod via coop.
// Returns (true, err) if a remote backend was found (coop),
// or (false, nil) if no remote backend exists (fall through to local).
func tryRemoteCrewAt(rigName, crewName string) (handled bool, err error) {
	// Check if we're connected to a remote daemon.
	if newConnectedDaemonClient() == nil {
		return false, nil
	}

	agentID := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	backend := terminal.ResolveBackend(agentID)
	switch backend.(type) {
	case *terminal.CoopBackend:
		podName := fmt.Sprintf("gt-%s-crew-%s", rigName, crewName)
		namespace := os.Getenv("NAMESPACE")
		if namespace == "" {
			namespace = "gastown"
		}
		fmt.Printf("Attaching to remote crew %s/%s...\n", rigName, crewName)
		return true, attachToCoopPodWithBrowser(podName, namespace, crewBrowser)
	default:
		// Non-Coop backend (e.g., unconfigured) — fall through to local handling
	}
	return false, nil
}
