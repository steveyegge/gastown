package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCrewAt(cmd *cobra.Command, args []string) error {
	var name string

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
			return fmt.Errorf("could not detect crew workspace from current directory: %w\n\nUsage: gt crew at <name>", err)
		}
		name = detected.crewName
		if crewRig == "" {
			crewRig = detected.rigName
		}
		fmt.Printf("Detected crew workspace: %s/%s\n", detected.rigName, name)
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

	// Resolve account for Claude config
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, crewAccount)
	if err != nil {
		return fmt.Errorf("resolving account: %w", err)
	}
	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Check if session exists
	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)
	hasSession, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	// Before creating a new session, check if there's already a Claude session
	// running in this crew's directory (might have been started manually or via
	// a different mechanism)
	if !hasSession {
		existingSessions, err := t.FindSessionByWorkDir(worker.ClonePath, true)
		if err == nil && len(existingSessions) > 0 {
			// Found an existing session with Claude running in this directory
			existingSession := existingSessions[0]
			fmt.Printf("%s Found existing Claude session '%s' in crew directory\n",
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
		_ = t.SetEnvironment(sessionID, "GT_ROLE", "crew")
		_ = t.SetEnvironment(sessionID, "GT_RIG", r.Name)
		_ = t.SetEnvironment(sessionID, "GT_CREW", name)

		// Set CLAUDE_CONFIG_DIR for account selection (non-fatal)
		if claudeConfigDir != "" {
			_ = t.SetEnvironment(sessionID, "CLAUDE_CONFIG_DIR", claudeConfigDir)
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

		// Use respawn-pane to replace shell with Claude directly
		// This gives cleaner lifecycle: Claude exits → session ends (no intermediate shell)
		// Pass "gt prime" as initial prompt so Claude loads context immediately
		// Export GT_ROLE and BD_ACTOR since tmux SetEnvironment only affects new panes
		claudeCmd := config.BuildCrewStartupCommand(r.Name, name, r.Path, "gt prime")
		if err := t.RespawnPane(paneID, claudeCmd); err != nil {
			return fmt.Errorf("starting claude: %w", err)
		}

		fmt.Printf("%s Created session for %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)
	} else {
		// Session exists - check if Claude is still running
		// Uses both pane command check and UI marker detection to avoid
		// restarting when user is in a subshell spawned from Claude
		if !t.IsClaudeRunning(sessionID) {
			// Claude has exited, restart it using respawn-pane
			fmt.Printf("Claude exited, restarting...\n")

			// Get pane ID for respawn
			paneID, err := t.GetPaneID(sessionID)
			if err != nil {
				return fmt.Errorf("getting pane ID: %w", err)
			}

			// Use respawn-pane to replace shell with Claude directly
			// Pass "gt prime" as initial prompt so Claude loads context immediately
			// Export GT_ROLE and BD_ACTOR since tmux SetEnvironment only affects new panes
			claudeCmd := config.BuildCrewStartupCommand(r.Name, name, r.Path, "gt prime")
			if err := t.RespawnPane(paneID, claudeCmd); err != nil {
				return fmt.Errorf("restarting claude: %w", err)
			}
		}
	}

	// Check if we're already in the target session
	if isInTmuxSession(sessionID) {
		// We're in the session at a shell prompt - just start the agent directly
		// Pass "gt prime" as initial prompt so it loads context immediately
		agentCfg := config.ResolveAgentConfig(townRoot, r.Path)
		fmt.Printf("Starting %s in current session...\n", agentCfg.Command)
		return execAgent(agentCfg, "gt prime")
	}

	// If inside tmux (but different session), don't switch - just inform user
	if tmux.IsInsideTmux() {
		fmt.Printf("Started %s/%s. Use C-b s to switch.\n", r.Name, name)
		return nil
	}

	// Outside tmux: attach unless --detached flag is set
	if crewDetached {
		fmt.Printf("Started %s/%s. Run 'gt crew at %s' to attach.\n", r.Name, name, name)
		return nil
	}

	// Attach to session
	return attachToTmuxSession(sessionID)
}
