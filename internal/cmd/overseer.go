package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getOverseerSessionName returns the Overseer session name.
func getOverseerSessionName() string {
	return session.OverseerSessionName()
}

var overseerCmd = &cobra.Command{
	Use:     "overseer",
	Aliases: []string{"ov"},
	GroupID: GroupAgents,
	Short:   "Manage the Overseer (town-level formula scheduler)",
	RunE:    requireSubcommand,
	Long: `Manage the Overseer - the town-level formula scheduler for Gas Town.

The Overseer runs assigned patrol formulas on a schedule:
  - Executes formulas assigned via 'gt patrol add'
  - Runs on a configurable interval (default: 10m)
  - Can be stopped/started independently of other agents

Use 'gt patrol list/add/remove' to manage which formulas the Overseer runs.

Role shortcuts: "overseer" in mail/nudge addresses resolves to this agent.`,
}

var overseerStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"spawn"},
	Short:   "Start the Overseer session",
	Long: `Start the Overseer tmux session.

Creates a new detached tmux session for the Overseer and launches Claude.
The session runs in the workspace overseer/ directory.`,
	RunE: runOverseerStart,
}

var overseerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Overseer session",
	Long: `Stop the Overseer tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runOverseerStop,
}

var overseerAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Overseer session",
	Long: `Attach to the running Overseer tmux session.

Attaches the current terminal to the Overseer's tmux session.
Detach with Ctrl-B D.`,
	RunE: runOverseerAttach,
}

var overseerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Overseer session status",
	Long: `Check if the Overseer tmux session is currently running.

Shows whether the Overseer has an active tmux session and reports
its session name.

Examples:
  gt overseer status`,
	RunE: runOverseerStatus,
}

var overseerRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Overseer session",
	Long: `Restart the Overseer tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runOverseerRestart,
}

var overseerAgentOverride string

func init() {
	overseerCmd.AddCommand(overseerStartCmd)
	overseerCmd.AddCommand(overseerStopCmd)
	overseerCmd.AddCommand(overseerAttachCmd)
	overseerCmd.AddCommand(overseerStatusCmd)
	overseerCmd.AddCommand(overseerRestartCmd)

	overseerStartCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")
	overseerAttachCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")
	overseerRestartCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")

	rootCmd.AddCommand(overseerCmd)
}

func runOverseerStart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getOverseerSessionName()

	// Check if session already exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		return fmt.Errorf("Overseer session already running. Attach with: gt overseer attach")
	}

	if err := startOverseerSession(t, sessionName, overseerAgentOverride); err != nil {
		return err
	}

	fmt.Printf("%s Overseer session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt overseer attach"))

	return nil
}

// startOverseerSession creates and initializes the Overseer tmux session.
func startOverseerSession(t *tmux.Tmux, sessionName, agentOverride string) error {
	// Find workspace root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Overseer runs from its own directory
	overseerDir := filepath.Join(townRoot, "overseer")

	// Ensure overseer directory exists
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		return fmt.Errorf("creating overseer directory: %w", err)
	}

	// Ensure runtime settings exist
	runtimeConfig := config.ResolveRoleAgentConfig("overseer", townRoot, overseerDir)
	if err := runtime.EnsureSettingsForRole(overseerDir, overseerDir, "overseer", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: "overseer",
		Sender:    "daemon",
		Topic:     "patrol",
	}, "I am Overseer. Check gt hook. If no hook, create mol-overseer-patrol wisp and execute it.")
	startupCmd, err := config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
		Role:        "overseer",
		TownRoot:    townRoot,
		Prompt:      initialPrompt,
		Topic:       "patrol",
		SessionName: sessionName,
	}, "", initialPrompt, agentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	fmt.Println("Starting Overseer session...")
	if err := t.NewSessionWithCommand(sessionName, overseerDir, startupCmd); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "overseer",
		TownRoot:    townRoot,
		Agent:       agentOverride,
		SessionName: sessionName,
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionName, k, v)
	}

	// Record agent's pane_id for ZFC-compliant liveness checks.
	if paneID, err := t.GetPaneID(sessionName); err == nil {
		_ = t.SetEnvironment(sessionName, "GT_PANE_ID", paneID)
	}

	// Apply Overseer theme
	theme := tmux.OverseerTheme()
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Overseer", "patrol")

	// Wait for Claude to start
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		_ = t.KillSessionWithProcesses(sessionName)
		return fmt.Errorf("waiting for overseer to start: %w", err)
	}

	// Accept startup dialogs if they appear.
	_ = t.AcceptStartupDialogs(sessionName)

	time.Sleep(constants.ShutdownNotifyDelay)

	runtimeCfg := config.ResolveRoleAgentConfig("overseer", townRoot, "")
	_ = runtime.RunStartupFallback(t, sessionName, "overseer", runtimeCfg)

	return nil
}

func runOverseerStop(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getOverseerSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return errors.New("Overseer session is not running")
	}

	fmt.Println("Stopping Overseer session...")

	// Try graceful shutdown first
	_ = t.SendKeysRaw(sessionName, "C-c")
	time.Sleep(100 * time.Millisecond)

	// Kill the session
	if err := t.KillSessionWithProcesses(sessionName); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	fmt.Printf("%s Overseer session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runOverseerAttach(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getOverseerSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Overseer session not running, starting...")
		if err := startOverseerSession(t, sessionName, overseerAgentOverride); err != nil {
			return err
		}
	}

	return attachToTmuxSession(sessionName)
}

func runOverseerStatus(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getOverseerSessionName()

	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if running {
		fmt.Printf("%s Overseer is running (session: %s)\n",
			style.Bold.Render("✓"), sessionName)
	} else {
		fmt.Printf("%s Overseer is not running\n", style.Dim.Render("○"))
	}

	return nil
}

func runOverseerRestart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()
	sessionName := getOverseerSessionName()

	// Stop if running
	if running, _ := t.HasSession(sessionName); running {
		fmt.Println("Stopping Overseer session...")
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
		if err := t.KillSessionWithProcesses(sessionName); err != nil {
			return fmt.Errorf("killing session: %w", err)
		}
		// Brief pause for cleanup
		time.Sleep(500 * time.Millisecond)
	}

	// Start fresh using the same code path as runOverseerStart
	return runOverseerStart(cmd, args)
}

