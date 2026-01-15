package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

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

var mayorAgentOverride string

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

	mayorStartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorAttachCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorRestartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")

	rootCmd.AddCommand(mayorCmd)
}

// getMayorManager returns a mayor manager for the current workspace.
func getMayorManager() (*mayor.Manager, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	return mayor.NewManager(townRoot), nil
}

// getMayorSessionName returns the Mayor session name.
func getMayorSessionName() string {
	return mayor.SessionName()
}

func runMayorStart(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	fmt.Println("Starting Mayor session...")
	if err := mgr.Start(mayorAgentOverride); err != nil {
		if err == mayor.ErrAlreadyRunning {
			return fmt.Errorf("Mayor session already running. Attach with: gt mayor attach")
		}
		return err
	}

	fmt.Printf("%s Mayor session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt mayor attach"))

	return nil
}

func runMayorStop(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	fmt.Println("Stopping Mayor session...")
	if err := mgr.Stop(); err != nil {
		if err == mayor.ErrNotRunning {
			return fmt.Errorf("Mayor session is not running")
		}
		return err
	}

	fmt.Printf("%s Mayor session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runMayorAttach(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	t := tmux.NewTmux()
	sessionID := mgr.SessionName()

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Mayor session not running, starting...")
		if err := mgr.Start(mayorAgentOverride); err != nil {
			return err
		}
	} else {
		// Session exists - check if runtime is still running (hq-95xfq)
		// If runtime exited or sitting at shell, restart with proper context
		agentCfg, _, err := config.ResolveAgentConfigWithOverride(townRoot, townRoot, mayorAgentOverride)
		if err != nil {
			return fmt.Errorf("resolving agent: %w", err)
		}
		if !t.IsAgentRunning(sessionID, config.ExpectedPaneCommands(agentCfg)...) {
			// Runtime has exited, restart it with proper context
			fmt.Println("Runtime exited, restarting with context...")

			paneID, err := t.GetPaneID(sessionID)
			if err != nil {
				return fmt.Errorf("getting pane ID: %w", err)
			}

			// Build startup beacon for context (like gt handoff does)
			beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
				Recipient: "mayor",
				Sender:    "human",
				Topic:     "attach",
			})

			// Build startup command with beacon
			startupCmd, err := config.BuildAgentStartupCommandWithAgentOverride("mayor", "", townRoot, "", beacon, mayorAgentOverride)
			if err != nil {
				return fmt.Errorf("building startup command: %w", err)
			}

			if err := t.RespawnPane(paneID, startupCmd); err != nil {
				return fmt.Errorf("restarting runtime: %w", err)
			}

			fmt.Printf("%s Mayor restarted with context\n", style.Bold.Render("✓"))
		}
	}

	// Use shared attach helper (smart: links if inside tmux, attaches if outside)
	return attachToTmuxSession(sessionID)
}

func runMayorStatus(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	info, err := mgr.Status()
	if err != nil {
		if err == mayor.ErrNotRunning {
			fmt.Printf("%s Mayor session is %s\n",
				style.Dim.Render("○"),
				"not running")
			fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt mayor start"))
			return nil
		}
		return fmt.Errorf("checking status: %w", err)
	}

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

	return nil
}

func runMayorRestart(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	// Stop if running (ignore not-running error)
	if err := mgr.Stop(); err != nil && err != mayor.ErrNotRunning {
		return fmt.Errorf("stopping session: %w", err)
	}

	// Start fresh
	return runMayorStart(cmd, args)
}
