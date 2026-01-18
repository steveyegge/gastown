package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var mayorCmd = &cobra.Command{
	Use:     "mayor",
	Aliases: []string{"may"},
	GroupID: GroupAgents,
	Short:   "Manage the Mayor (Chief of Staff for cross-rig coordination)",
	RunE:    requireSubcommand,
	Long: `Manage the Mayor - the Overseer's Chief of Staff.

The Mayor is the global coordinator for Gas Town:
  - Receives escalations from Witnesses and Deacon
  - Coordinates work across multiple rigs
  - Handles human communication when needed
  - Routes strategic decisions and cross-project issues

The Mayor is the primary interface between the human Overseer and the
automated agents. When in doubt, escalate to the Mayor.

Role shortcuts: "mayor" in mail/nudge addresses resolves to this agent.`,
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

func runMayorStart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	fmt.Println("Starting Mayor session...")
	if _, err := factory.Start(townRoot, agent.MayorAddress, factory.WithAgent(mayorAgentOverride)); err != nil {
		if err == agent.ErrAlreadyRunning {
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
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agents := factory.Agents()
	id := agent.MayorAddress

	fmt.Println("Stopping Mayor session...")
	if !agents.Exists(id) {
		return fmt.Errorf("Mayor session is not running")
	}
	if err := agents.Stop(id, true); err != nil {
		return err
	}

	fmt.Printf("%s Mayor session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runMayorAttach(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agents := factory.Agents()
	id := agent.MayorAddress

	if !agents.Exists(id) {
		// Auto-start if not running
		fmt.Println("Mayor session not running, starting...")
		if _, err := factory.Start(townRoot, agent.MayorAddress, factory.WithAgent(mayorAgentOverride)); err != nil {
			return err
		}
	}

	// Smart attach: switches if inside tmux, attaches if outside
	return agents.Attach(agent.MayorAddress)
}

func runMayorStatus(cmd *cobra.Command, args []string) error {
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agents := factory.Agents()
	id := agent.MayorAddress

	if !agents.Exists(id) {
		fmt.Printf("%s Mayor session is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt mayor start"))
		return nil
	}

	fmt.Printf("%s Mayor session is %s\n",
		style.Bold.Render("●"),
		style.Bold.Render("running"))
	fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt mayor attach"))

	return nil
}

func runMayorRestart(cmd *cobra.Command, args []string) error {
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Stop if running (ignore errors - we'll start fresh anyway)
	agents := factory.Agents()
	id := agent.MayorAddress
	_ = agents.Stop(id, true)

	// Start fresh
	return runMayorStart(cmd, args)
}
