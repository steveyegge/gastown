package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mayor"
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
var mayorPrompt string

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
Detach with Ctrl-B D.

Use --prompt to send a message to the Mayor before attaching:
  gt mayor attach --prompt "Please check the status of all rigs"`,
	RunE: runMayorAttach,
}

var mayorPromptCmd = &cobra.Command{
	Use:   "prompt <message>",
	Short: "Send a prompt to the Mayor without attaching",
	Long: `Send a prompt to the running Mayor session without opening tmux.

This allows you to deliver instructions to the Mayor that will be visible
when you next attach. The prompt is sent via tmux send-keys to the session.

Examples:
  gt mayor prompt "Please summarize the current work status"
  gt mayor prompt "Check mail and respond to any urgent requests"

The Mayor must be running. Use 'gt mayor start' first if needed.`,
	Args: cobra.ExactArgs(1),
	RunE: runMayorPrompt,
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
	mayorCmd.AddCommand(mayorPromptCmd)
	mayorCmd.AddCommand(mayorStatusCmd)
	mayorCmd.AddCommand(mayorRestartCmd)

	mayorStartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorStartCmd.Flags().StringVarP(&mayorPrompt, "prompt", "p", "", "Initial prompt to send after startup")
	mayorAttachCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorAttachCmd.Flags().StringVarP(&mayorPrompt, "prompt", "p", "", "Prompt to send before attaching")
	mayorRestartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorRestartCmd.Flags().StringVarP(&mayorPrompt, "prompt", "p", "", "Initial prompt to send after restart")

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
	if err := mgr.StartWithPrompt(mayorAgentOverride, mayorPrompt); err != nil {
		if err == mayor.ErrAlreadyRunning {
			return fmt.Errorf("Mayor session already running. Attach with: gt mayor attach")
		}
		return err
	}

	if mayorPrompt != "" {
		fmt.Printf("%s Mayor session started with prompt. Attach with: %s\n",
			style.Bold.Render("✓"),
			style.Dim.Render("gt mayor attach"))
	} else {
		fmt.Printf("%s Mayor session started. Attach with: %s\n",
			style.Bold.Render("✓"),
			style.Dim.Render("gt mayor attach"))
	}

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

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Mayor session not running, starting...")
		if err := mgr.StartWithPrompt(mayorAgentOverride, mayorPrompt); err != nil {
			return err
		}
	} else if mayorPrompt != "" {
		// Session already running - send prompt before attaching
		// This is the key feature: inject prompt deterministically before attach
		t := tmux.NewTmux()
		if err := t.NudgeSession(mgr.SessionName(), mayorPrompt); err != nil {
			return fmt.Errorf("sending prompt: %w", err)
		}
		fmt.Printf("%s Prompt sent to Mayor\n", style.Bold.Render("✓"))
	}

	// Use shared attach helper (smart: links if inside tmux, attaches if outside)
	return attachToTmuxSession(mgr.SessionName())
}

// runMayorPrompt sends a prompt to the Mayor without attaching.
// This enables prompt injection without opening tmux - the prompt is visible on attach.
func runMayorPrompt(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return fmt.Errorf("Mayor session is not running. Start with: gt mayor start")
	}

	prompt := args[0]
	t := tmux.NewTmux()
	if err := t.NudgeSession(mgr.SessionName(), prompt); err != nil {
		return fmt.Errorf("sending prompt: %w", err)
	}

	fmt.Printf("%s Prompt sent to Mayor (visible on attach)\n", style.Bold.Render("✓"))
	return nil
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

	// Start fresh with optional prompt
	return runMayorStart(cmd, args)
}
