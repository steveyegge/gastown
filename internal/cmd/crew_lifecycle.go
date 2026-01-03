package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCrewRemove(cmd *cobra.Command, args []string) error {
	var lastErr error

	for _, arg := range args {
		name := arg
		rigOverride := crewRig

		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if rigOverride == "" {
				rigOverride = rig
			}
			name = crewName
		}

		crewMgr, r, err := getCrewManager(rigOverride)
		if err != nil {
			fmt.Printf("Error removing %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Check for running session (unless forced)
		if !crewForce {
			t := tmux.NewTmux()
			sessionID := crewSessionName(r.Name, name)
			hasSession, _ := t.HasSession(sessionID)
			if hasSession {
				fmt.Printf("Error removing %s: session '%s' is running (use --force to kill and remove)\n", arg, sessionID)
				lastErr = fmt.Errorf("session running")
				continue
			}
		}

		// Kill session if it exists
		t := tmux.NewTmux()
		sessionID := crewSessionName(r.Name, name)
		if hasSession, _ := t.HasSession(sessionID); hasSession {
			if err := t.KillSession(sessionID); err != nil {
				fmt.Printf("Error killing session for %s: %v\n", arg, err)
				lastErr = err
				continue
			}
			fmt.Printf("Killed session %s\n", sessionID)
		}

		// Remove the crew workspace
		if err := crewMgr.Remove(name, crewForce); err != nil {
			if err == crew.ErrCrewNotFound {
				fmt.Printf("Error removing %s: crew workspace not found\n", arg)
			} else if err == crew.ErrHasChanges {
				fmt.Printf("Error removing %s: uncommitted changes (use --force)\n", arg)
			} else {
				fmt.Printf("Error removing %s: %v\n", arg, err)
			}
			lastErr = err
			continue
		}

		fmt.Printf("%s Removed crew workspace: %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)

		// Close the agent bead if it exists
		// Use the rig's configured prefix (e.g., "gt" for gastown, "bd" for beads)
		townRoot, _ := workspace.Find(r.Path)
		if townRoot == "" {
			townRoot = r.Path
		}
		prefix := beads.GetPrefixForRig(townRoot, r.Name)
		agentBeadID := beads.CrewBeadIDWithPrefix(prefix, r.Name, name)
		closeArgs := []string{"close", agentBeadID, "--reason=Crew workspace removed"}
		if sessionID := os.Getenv("CLAUDE_SESSION_ID"); sessionID != "" {
			closeArgs = append(closeArgs, "--session="+sessionID)
		}
		closeCmd := exec.Command("bd", closeArgs...)
		closeCmd.Dir = r.Path // Run from rig directory for proper beads resolution
		if output, err := closeCmd.CombinedOutput(); err != nil {
			// Non-fatal: bead might not exist or already be closed
			if !strings.Contains(string(output), "no issue found") &&
				!strings.Contains(string(output), "already closed") {
				style.PrintWarning("could not close agent bead %s: %v", agentBeadID, err)
			}
		} else {
			fmt.Printf("Closed agent bead: %s\n", agentBeadID)
		}
	}

	return lastErr
}

func runCrewRefresh(cmd *cobra.Command, args []string) error {
	name := args[0]
	// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
	if rig, crewName, ok := parseRigSlashName(name); ok {
		if crewRig == "" {
			crewRig = rig
		}
		name = crewName
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

	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)

	// Check if session exists
	hasSession, _ := t.HasSession(sessionID)

	// Create handoff message
	handoffMsg := crewMessage
	if handoffMsg == "" {
		handoffMsg = fmt.Sprintf("Context refresh for %s. Check mail and beads for current work state.", name)
	}

	// Send handoff mail to self
	mailDir := filepath.Join(worker.ClonePath, "mail")
	if _, err := os.Stat(mailDir); os.IsNotExist(err) {
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			return fmt.Errorf("creating mail dir: %w", err)
		}
	}

	// Create and send mail
	mailbox := mail.NewMailbox(mailDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/%s", r.Name, name),
		To:      fmt.Sprintf("%s/%s", r.Name, name),
		Subject: "🤝 HANDOFF: Context Refresh",
		Body:    handoffMsg,
	}
	if err := mailbox.Append(msg); err != nil {
		return fmt.Errorf("sending handoff mail: %w", err)
	}
	fmt.Printf("Sent handoff mail to %s/%s\n", r.Name, name)

	// Kill existing session if running
	if hasSession {
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing old session: %w", err)
		}
		fmt.Printf("Killed old session %s\n", sessionID)
	}

	// Start new session
	if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Wait for shell to be ready
	if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
		return fmt.Errorf("waiting for shell: %w", err)
	}

	// Build the startup beacon for predecessor discovery via /resume
	// Pass it as Claude's initial prompt - processed when Claude is ready
	address := fmt.Sprintf("%s/crew/%s", r.Name, name)
	beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     "refresh",
	})

	// Start claude with environment exports and beacon as initial prompt
	// Refresh uses regular permissions (no --dangerously-skip-permissions)
	// SessionStart hook handles context loading (gt prime --hook)
	claudeCmd := config.BuildCrewStartupCommand(r.Name, name, r.Path, beacon)
	// Remove --dangerously-skip-permissions for refresh (interactive mode)
	claudeCmd = strings.Replace(claudeCmd, " --dangerously-skip-permissions", "", 1)
	if err := t.SendKeys(sessionID, claudeCmd); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	// Wait for Claude to start (optional, for status feedback)
	shells := constants.SupportedShells
	if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}

	fmt.Printf("%s Refreshed crew workspace: %s/%s\n",
		style.Bold.Render("✓"), r.Name, name)
	fmt.Printf("Attach with: %s\n", style.Dim.Render(fmt.Sprintf("gt crew at %s", name)))

	return nil
}

// runCrewStart starts crew workers in a rig.
// args[0] is the rig name (required)
// args[1:] are crew member names (optional, or use --all flag)
func runCrewStart(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	crewNames := args[1:]

	// Get the rig manager and rig
	crewMgr, r, err := getCrewManager(rigName)
	if err != nil {
		return err
	}

	// If --all flag, get all crew members
	if crewAll {
		workers, err := crewMgr.List()
		if err != nil {
			return fmt.Errorf("listing crew: %w", err)
		}
		if len(workers) == 0 {
			fmt.Printf("No crew members in rig %s\n", rigName)
			return nil
		}
		for _, w := range workers {
			crewNames = append(crewNames, w.Name)
		}
	}

	// Start each crew member
	var lastErr error
	startedCount := 0
	for _, name := range crewNames {
		// Set the start.go flags before calling runStartCrew
		startCrewRig = rigName
		startCrewAccount = crewAccount

		// Use rig/name format for runStartCrew
		fullName := rigName + "/" + name
		if err := runStartCrew(cmd, []string{fullName}); err != nil {
			fmt.Printf("Error starting %s/%s: %v\n", rigName, name, err)
			lastErr = err
		} else {
			startedCount++
		}
	}

	if startedCount > 0 {
		fmt.Printf("\n%s Started %d crew member(s) in %s\n",
			style.Bold.Render("✓"), startedCount, r.Name)
	}

	return lastErr
}

func runCrewRestart(cmd *cobra.Command, args []string) error {
	// Handle --all flag
	if crewAll {
		return runCrewRestartAll()
	}

	var lastErr error

	for _, arg := range args {
		name := arg
		rigOverride := crewRig

		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if rigOverride == "" {
				rigOverride = rig
			}
			name = crewName
		}

		crewMgr, r, err := getCrewManager(rigOverride)
		if err != nil {
			fmt.Printf("Error restarting %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Get the crew worker, create if not exists (idempotent)
		worker, err := crewMgr.Get(name)
		if err == crew.ErrCrewNotFound {
			fmt.Printf("Creating crew workspace %s in %s...\n", name, r.Name)
			worker, err = crewMgr.Add(name, false) // No feature branch for crew
			if err != nil {
				fmt.Printf("Error creating %s: %v\n", arg, err)
				lastErr = err
				continue
			}
			fmt.Printf("Created crew workspace: %s/%s\n", r.Name, name)
		} else if err != nil {
			fmt.Printf("Error getting %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		t := tmux.NewTmux()
		sessionID := crewSessionName(r.Name, name)

		// Kill existing session if running
		if hasSession, _ := t.HasSession(sessionID); hasSession {
			if err := t.KillSession(sessionID); err != nil {
				fmt.Printf("Error killing session for %s: %v\n", arg, err)
				lastErr = err
				continue
			}
			fmt.Printf("Killed session %s\n", sessionID)
		}

		// Start new session
		if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
			fmt.Printf("Error creating session for %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Set environment
		_ = t.SetEnvironment(sessionID, "GT_ROLE", "crew")
		// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
		theme := getThemeForRig(r.Name)
		_ = t.ConfigureGasTownSession(sessionID, theme, r.Name, name, "crew")

		// Wait for shell to be ready
		if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
			fmt.Printf("Error waiting for shell for %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Build the startup beacon for predecessor discovery via /resume
		// Pass it as Claude's initial prompt - processed when Claude is ready
		address := fmt.Sprintf("%s/crew/%s", r.Name, name)
		beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
			Recipient: address,
			Sender:    "human",
			Topic:     "restart",
		})

		// Start claude with environment exports and beacon as initial prompt
		// SessionStart hook handles context loading (gt prime --hook)
		// The startup protocol tells agent to check mail/hook, no explicit prompt needed
		claudeCmd := config.BuildCrewStartupCommand(r.Name, name, r.Path, beacon)
		if err := t.SendKeys(sessionID, claudeCmd); err != nil {
			fmt.Printf("Error starting claude for %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Wait for Claude to start (optional, for status feedback)
		shells := constants.SupportedShells
		if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
			style.PrintWarning("Timeout waiting for Claude to start for %s: %v", arg, err)
		}

		fmt.Printf("%s Restarted crew workspace: %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)
		fmt.Printf("Attach with: %s\n", style.Dim.Render(fmt.Sprintf("gt crew at %s", name)))
	}

	return lastErr
}

// runCrewRestartAll restarts all running crew sessions.
// If crewRig is set, only restarts crew in that rig.
func runCrewRestartAll() error {
	// Get all agent sessions (including polecats to find crew)
	agents, err := getAgentSessions(true)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Filter to crew agents only
	var targets []*AgentSession
	for _, agent := range agents {
		if agent.Type != AgentCrew {
			continue
		}
		// Filter by rig if specified
		if crewRig != "" && agent.Rig != crewRig {
			continue
		}
		targets = append(targets, agent)
	}

	if len(targets) == 0 {
		fmt.Println("No running crew sessions to restart.")
		if crewRig != "" {
			fmt.Printf("  (filtered by rig: %s)\n", crewRig)
		}
		return nil
	}

	// Dry run - just show what would be restarted
	if crewDryRun {
		fmt.Printf("Would restart %d crew session(s):\n\n", len(targets))
		for _, agent := range targets {
			fmt.Printf("  %s %s/crew/%s\n", AgentTypeIcons[AgentCrew], agent.Rig, agent.AgentName)
		}
		return nil
	}

	fmt.Printf("Restarting %d crew session(s)...\n\n", len(targets))

	var succeeded, failed int
	var failures []string

	for _, agent := range targets {
		agentName := fmt.Sprintf("%s/crew/%s", agent.Rig, agent.AgentName)

		// Use crewRig temporarily to get the right crew manager
		savedRig := crewRig
		crewRig = agent.Rig

		crewMgr, r, err := getCrewManager(crewRig)
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", agentName, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentName)
			crewRig = savedRig
			continue
		}

		worker, err := crewMgr.Get(agent.AgentName)
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", agentName, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentName)
			crewRig = savedRig
			continue
		}

		// Restart the session
		if err := restartCrewSession(r.Name, agent.AgentName, worker.ClonePath); err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", agentName, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentName)
		} else {
			succeeded++
			fmt.Printf("  %s %s\n", style.SuccessPrefix, agentName)
		}

		crewRig = savedRig

		// Small delay between restarts to avoid overwhelming the system
		time.Sleep(constants.ShutdownNotifyDelay)
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("%s Restart complete: %d succeeded, %d failed\n",
			style.WarningPrefix, succeeded, failed)
		for _, f := range failures {
			fmt.Printf("  %s\n", style.Dim.Render(f))
		}
		return fmt.Errorf("%d restart(s) failed", failed)
	}

	fmt.Printf("%s Restart complete: %d crew session(s) restarted\n", style.SuccessPrefix, succeeded)
	return nil
}

// restartCrewSession handles the core restart logic for a single crew session.
func restartCrewSession(rigName, crewName, clonePath string) error {
	t := tmux.NewTmux()
	sessionID := crewSessionName(rigName, crewName)

	// Kill existing session if running
	if hasSession, _ := t.HasSession(sessionID); hasSession {
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing old session: %w", err)
		}
	}

	// Start new session
	if err := t.NewSession(sessionID, clonePath); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Apply rig-based theming
	theme := getThemeForRig(rigName)
	_ = t.ConfigureGasTownSession(sessionID, theme, rigName, crewName, "crew")

	// Wait for shell to be ready
	if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
		return fmt.Errorf("waiting for shell: %w", err)
	}

	// Build the startup beacon for predecessor discovery via /resume
	// Pass it as Claude's initial prompt - processed when Claude is ready
	address := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     "restart",
	})

	// Start claude with environment exports and beacon as initial prompt
	// SessionStart hook handles context loading (gt prime --hook)
	claudeCmd := config.BuildCrewStartupCommand(rigName, crewName, "", beacon)
	if err := t.SendKeys(sessionID, claudeCmd); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	// Wait for Claude to start (optional, for status feedback)
	shells := constants.SupportedShells
	if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal warning
	}

	return nil
}

// runCrewStop stops one or more crew workers.
// Supports: "name", "rig/name" formats, or --all to stop all.
func runCrewStop(cmd *cobra.Command, args []string) error {
	// Handle --all flag
	if crewAll {
		return runCrewStopAll()
	}

	var lastErr error
	t := tmux.NewTmux()

	for _, arg := range args {
		name := arg
		rigOverride := crewRig

		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if rigOverride == "" {
				rigOverride = rig
			}
			name = crewName
		}

		_, r, err := getCrewManager(rigOverride)
		if err != nil {
			fmt.Printf("Error stopping %s: %v\n", arg, err)
			lastErr = err
			continue
		}

		sessionID := crewSessionName(r.Name, name)

		// Check if session exists
		hasSession, _ := t.HasSession(sessionID)
		if !hasSession {
			fmt.Printf("No session found for %s/%s\n", r.Name, name)
			continue
		}

		// Capture output before stopping (best effort)
		var output string
		if !crewForce {
			output, _ = t.CapturePane(sessionID, 50)
		}

		// Kill the session
		if err := t.KillSession(sessionID); err != nil {
			fmt.Printf("  %s [%s] %s: %s\n",
				style.ErrorPrefix,
				r.Name, name,
				style.Dim.Render(err.Error()))
			lastErr = err
			continue
		}

		fmt.Printf("  %s [%s] %s: stopped\n",
			style.SuccessPrefix,
			r.Name, name)

		// Log kill event to town log
		townRoot, _ := workspace.Find(r.Path)
		if townRoot != "" {
			agent := fmt.Sprintf("%s/crew/%s", r.Name, name)
			logger := townlog.NewLogger(townRoot)
			_ = logger.Log(townlog.EventKill, agent, "gt crew stop")
		}

		// Log captured output (truncated)
		if len(output) > 200 {
			output = output[len(output)-200:]
		}
		if output != "" {
			fmt.Printf("      %s\n", style.Dim.Render("(output captured)"))
		}
	}

	return lastErr
}

// runCrewStopAll stops all running crew sessions.
// If crewRig is set, only stops crew in that rig.
func runCrewStopAll() error {
	// Get all agent sessions (including polecats to find crew)
	agents, err := getAgentSessions(true)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Filter to crew agents only
	var targets []*AgentSession
	for _, agent := range agents {
		if agent.Type != AgentCrew {
			continue
		}
		// Filter by rig if specified
		if crewRig != "" && agent.Rig != crewRig {
			continue
		}
		targets = append(targets, agent)
	}

	if len(targets) == 0 {
		fmt.Println("No running crew sessions to stop.")
		if crewRig != "" {
			fmt.Printf("  (filtered by rig: %s)\n", crewRig)
		}
		return nil
	}

	// Dry run - just show what would be stopped
	if crewDryRun {
		fmt.Printf("Would stop %d crew session(s):\n\n", len(targets))
		for _, agent := range targets {
			fmt.Printf("  %s %s/crew/%s\n", AgentTypeIcons[AgentCrew], agent.Rig, agent.AgentName)
		}
		return nil
	}

	fmt.Printf("%s Stopping %d crew session(s)...\n\n",
		style.Bold.Render("🛑"), len(targets))

	t := tmux.NewTmux()
	var succeeded, failed int
	var failures []string

	for _, agent := range targets {
		agentName := fmt.Sprintf("%s/crew/%s", agent.Rig, agent.AgentName)
		sessionID := agent.Name // agent.Name IS the tmux session name

		// Capture output before stopping (best effort)
		var output string
		if !crewForce {
			output, _ = t.CapturePane(sessionID, 50)
		}

		// Kill the session
		if err := t.KillSession(sessionID); err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", agentName, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentName)
			continue
		}

		succeeded++
		fmt.Printf("  %s %s\n", style.SuccessPrefix, agentName)

		// Log kill event to town log
		townRoot, _ := workspace.FindFromCwd()
		if townRoot != "" {
			logger := townlog.NewLogger(townRoot)
			_ = logger.Log(townlog.EventKill, agentName, "gt crew stop --all")
		}

		// Log captured output (truncated)
		if len(output) > 200 {
			output = output[len(output)-200:]
		}
		if output != "" {
			fmt.Printf("      %s\n", style.Dim.Render("(output captured)"))
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("%s Stop complete: %d succeeded, %d failed\n",
			style.WarningPrefix, succeeded, failed)
		for _, f := range failures {
			fmt.Printf("  %s\n", style.Dim.Render(f))
		}
		return fmt.Errorf("%d stop(s) failed", failed)
	}

	fmt.Printf("%s Stop complete: %d crew session(s) stopped\n", style.SuccessPrefix, succeeded)
	return nil
}
