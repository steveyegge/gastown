package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getMayorSessionName returns the Mayor session name.
func getMayorSessionName() string {
	return session.MayorSessionName()
}

var mayorCmd = &cobra.Command{
	Use:     "mayor",
	Aliases: []string{"may"},
	GroupID: GroupAgents,
	Short:   "Manage the Mayor session",
	RunE:    requireSubcommand,
	Long: `Manage the Mayor tmux session.

The Mayor is the global coordinator for Gas Town, running as a persistent
tmux session. Use the subcommands to start, stop, attach, and check status.`,
}

var mayorStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Mayor session",
	Long: `Start the Mayor tmux session.

Creates a new detached tmux session for the Mayor and launches Claude.
The session runs in the workspace root directory.`,
	RunE: runMayorStart,
}

var mayorStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Mayor session",
	Long: `Stop the Mayor tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runMayorStop,
}

var mayorAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Mayor session",
	Long: `Attach to the running Mayor tmux session.

Attaches the current terminal to the Mayor's tmux session.
Detach with Ctrl-B D.`,
	RunE: runMayorAttach,
}

var mayorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Mayor session status",
	Long:  `Check if the Mayor tmux session is currently running.`,
	RunE:  runMayorStatus,
}

var mayorRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Mayor session",
	Long: `Restart the Mayor tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runMayorRestart,
}

func init() {
	mayorCmd.AddCommand(mayorStartCmd)
	mayorCmd.AddCommand(mayorStopCmd)
	mayorCmd.AddCommand(mayorAttachCmd)
	mayorCmd.AddCommand(mayorStatusCmd)
	mayorCmd.AddCommand(mayorRestartCmd)

	rootCmd.AddCommand(mayorCmd)
}

func runMayorStart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getMayorSessionName()

	// Check if session already exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		return fmt.Errorf("Mayor session already running. Attach with: gt mayor attach")
	}

	if err := startMayorSession(t, sessionName); err != nil {
		return err
	}

	fmt.Printf("%s Mayor session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt mayor attach"))

	return nil
}

// startMayorSession creates and initializes the Mayor tmux session.
func startMayorSession(t *tmux.Tmux, sessionName string) error {
	// Find workspace root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create session in workspace root
	fmt.Println("Starting Mayor session...")
	if err := t.NewSession(sessionName, townRoot); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "mayor")
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", "mayor")

	// Apply Mayor theme (non-fatal: theming failure doesn't affect operation)
	// Note: ConfigureGasTownSession includes cycle bindings
	theme := tmux.MayorTheme()
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Mayor", "coordinator")

	// Launch Claude - the startup hook handles 'gt prime' automatically
	// Use SendKeysDelayed to allow shell initialization after NewSession
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	// Mayor uses default runtime config (empty rigPath) since it's not rig-specific
	claudeCmd := config.BuildAgentStartupCommand("mayor", "mayor", "", "")
	if err := t.SendKeysDelayed(sessionName, claudeCmd, 200); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}
	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: "mayor",
		Sender:    "human",
		Topic:     "cold-start",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous coordination.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionName, session.PropulsionNudgeForRole("mayor", townRoot)) // Non-fatal

	return nil
}

func runMayorStop(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getMayorSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return errors.New("Mayor session is not running")
	}

	fmt.Println("Stopping Mayor session...")

	// Try graceful shutdown first (best-effort interrupt)
	_ = t.SendKeysRaw(sessionName, "C-c")
	time.Sleep(100 * time.Millisecond)

	// Kill the session
	if err := t.KillSession(sessionName); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	fmt.Printf("%s Mayor session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runMayorAttach(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getMayorSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Mayor session not running, starting...")
		if err := startMayorSession(t, sessionName); err != nil {
			return err
		}
	}

	// Use shared attach helper (smart: links if inside tmux, attaches if outside)
	return attachToTmuxSession(sessionName)
}

func runMayorStatus(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getMayorSessionName()

	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if running {
		// Get session info for more details
		info, err := t.GetSessionInfo(sessionName)
		if err == nil {
			status := "detached"
			if info.Attached {
				status = "attached"
			}
			fmt.Printf("%s Mayor session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
			fmt.Printf("  Status: %s\n", status)
			fmt.Printf("  Created: %s\n", info.Created)
			fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt mayor attach"))
		} else {
			fmt.Printf("%s Mayor session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
		}
	} else {
		fmt.Printf("%s Mayor session is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt mayor start"))
	}

	return nil
}

func runMayorRestart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getMayorSessionName()

	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if running {
		// Stop the current session (best-effort interrupt before kill)
		fmt.Println("Stopping Mayor session...")
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
		if err := t.KillSession(sessionName); err != nil {
			return fmt.Errorf("killing session: %w", err)
		}
	}

	// Start fresh
	return runMayorStart(cmd, args)
}
