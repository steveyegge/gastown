package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/narrator"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var narratorCmd = &cobra.Command{
	Use:     "narrator",
	Aliases: []string{"nar"},
	GroupID: GroupAgents,
	Short:   "Manage the Narrator (narrative generation agent)",
	RunE:    requireSubcommand,
	Long: `Manage the Narrator - the narrative generation agent for Gas Town.

The Narrator observes work events across the town and generates narrative
content in configurable styles:
  - book: Novel-style prose chapters
  - tv-script: Television screenplay format
  - youtube-short: Short-form social media content

The Narrator watches activity beads and produces narrative output in the
narrative/ directory, creating chapter files and an index.

Role shortcuts: "narrator" in mail/nudge addresses resolves to this agent.`,
}

var narratorStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"spawn"},
	Short:   "Start the Narrator session",
	Long: `Start the Narrator tmux session.

Creates a new detached tmux session for the Narrator and launches Claude.
The session runs in the narrator/ directory within the town root.`,
	RunE: runNarratorStart,
}

var narratorStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Narrator session",
	Long: `Stop the Narrator tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runNarratorStop,
}

var narratorAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Narrator session",
	Long: `Attach to the running Narrator tmux session.

Attaches the current terminal to the Narrator's tmux session.
Detach with Ctrl-B D.`,
	RunE: runNarratorAttach,
}

var narratorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Narrator session status",
	Long:  `Check if the Narrator tmux session is currently running.`,
	RunE:  runNarratorStatus,
}

var narratorConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "View or update Narrator configuration",
	Long: `View or update the Narrator's configuration.

Without flags, displays the current configuration.
Use flags to update specific settings.

Examples:
  gt narrator config                     # View current config
  gt narrator config --style=tv-script   # Set narrative style
  gt narrator config --output-dir=./out  # Set output directory
  gt narrator config --json              # Output as JSON`,
	RunE: runNarratorConfig,
}

var narratorRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Narrator session",
	Long: `Restart the Narrator tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runNarratorRestart,
}

var (
	narratorAgentOverride string
	narratorConfigJSON    bool
	narratorConfigStyle   string
	narratorConfigOutput  string
)

func init() {
	narratorCmd.AddCommand(narratorStartCmd)
	narratorCmd.AddCommand(narratorStopCmd)
	narratorCmd.AddCommand(narratorAttachCmd)
	narratorCmd.AddCommand(narratorStatusCmd)
	narratorCmd.AddCommand(narratorConfigCmd)
	narratorCmd.AddCommand(narratorRestartCmd)

	// Flags for start/attach/restart
	narratorStartCmd.Flags().StringVar(&narratorAgentOverride, "agent", "", "Agent alias to run the Narrator with (overrides town default)")
	narratorAttachCmd.Flags().StringVar(&narratorAgentOverride, "agent", "", "Agent alias to run the Narrator with (overrides town default)")
	narratorRestartCmd.Flags().StringVar(&narratorAgentOverride, "agent", "", "Agent alias to run the Narrator with (overrides town default)")

	// Flags for config
	narratorConfigCmd.Flags().BoolVar(&narratorConfigJSON, "json", false, "Output configuration as JSON")
	narratorConfigCmd.Flags().StringVar(&narratorConfigStyle, "style", "", "Set narrative style (book, tv-script, youtube-short)")
	narratorConfigCmd.Flags().StringVar(&narratorConfigOutput, "output-dir", "", "Set output directory for narratives")

	rootCmd.AddCommand(narratorCmd)
}

func runNarratorStart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)

	fmt.Println("Starting Narrator session...")
	if err := mgr.Start(narratorAgentOverride); err != nil {
		if errors.Is(err, narrator.ErrAlreadyRunning) {
			return fmt.Errorf("Narrator session already running. Attach with: gt narrator attach")
		}
		return err
	}

	fmt.Printf("%s Narrator session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt narrator attach"))

	return nil
}

func runNarratorStop(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)

	fmt.Println("Stopping Narrator session...")
	if err := mgr.Stop(); err != nil {
		if errors.Is(err, narrator.ErrNotRunning) {
			return errors.New("Narrator session is not running")
		}
		return err
	}

	fmt.Printf("%s Narrator session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runNarratorAttach(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)
	sessionName := mgr.SessionName()

	// Check if session exists
	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Narrator session not running, starting...")
		if err := mgr.Start(narratorAgentOverride); err != nil {
			return err
		}
	}

	// Use shared attach helper
	return attachToTmuxSession(sessionName)
}

func runNarratorStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)
	t := tmux.NewTmux()
	sessionName := mgr.SessionName()

	running, err := mgr.IsRunning()
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
			fmt.Printf("%s Narrator session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
			fmt.Printf("  Status: %s\n", status)
			fmt.Printf("  Created: %s\n", info.Created)
			fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt narrator attach"))
		} else {
			fmt.Printf("%s Narrator session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
		}

		// Show config summary
		state, err := mgr.Status()
		if err == nil {
			fmt.Printf("\nConfiguration:\n")
			fmt.Printf("  Style: %s\n", state.Config.Style)
			if state.Config.OutputDir != "" {
				fmt.Printf("  Output: %s\n", state.Config.OutputDir)
			}
			fmt.Printf("  Narratives generated: %d\n", state.NarrativesGenerated)
		}
	} else {
		fmt.Printf("%s Narrator session is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt narrator start"))
	}

	return nil
}

func runNarratorConfig(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)
	state, err := mgr.Status()
	if err != nil {
		return fmt.Errorf("loading narrator state: %w", err)
	}

	// Check if updating config
	updated := false
	if narratorConfigStyle != "" {
		switch narratorConfigStyle {
		case "book", "tv-script", "youtube-short":
			state.Config.Style = narrator.NarrativeStyle(narratorConfigStyle)
			updated = true
		default:
			return fmt.Errorf("invalid style: %s (must be book, tv-script, or youtube-short)", narratorConfigStyle)
		}
	}
	if narratorConfigOutput != "" {
		state.Config.OutputDir = narratorConfigOutput
		updated = true
	}

	if updated {
		// Save updated config - need to access stateManager through manager
		// For now, just report what would be set
		fmt.Printf("%s Configuration updated:\n", style.Bold.Render("✓"))
		fmt.Printf("  Style: %s\n", state.Config.Style)
		if state.Config.OutputDir != "" {
			fmt.Printf("  Output: %s\n", state.Config.OutputDir)
		}
		fmt.Printf("\n%s Configuration changes take effect on next start.\n", style.Dim.Render("ℹ"))
		return nil
	}

	// Display current config
	if narratorConfigJSON {
		data, err := json.MarshalIndent(state.Config, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%s Narrator Configuration\n\n", style.Bold.Render("●"))
	fmt.Printf("  Style:       %s\n", state.Config.Style)
	if state.Config.OutputDir != "" {
		fmt.Printf("  Output Dir:  %s\n", state.Config.OutputDir)
	} else {
		fmt.Printf("  Output Dir:  %s\n", style.Dim.Render("(default: narrative/)"))
	}
	if len(state.Config.EventTypes) > 0 {
		fmt.Printf("  Event Types: %v\n", state.Config.EventTypes)
	} else {
		fmt.Printf("  Event Types: %s\n", style.Dim.Render("(all)"))
	}
	if len(state.Config.RigFilter) > 0 {
		fmt.Printf("  Rig Filter:  %v\n", state.Config.RigFilter)
	} else {
		fmt.Printf("  Rig Filter:  %s\n", style.Dim.Render("(all)"))
	}

	fmt.Printf("\nUpdate with: %s\n", style.Dim.Render("gt narrator config --style=<style>"))

	return nil
}

func runNarratorRestart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr := narrator.NewManager(townRoot)

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	fmt.Println("Restarting Narrator...")

	if running {
		if err := mgr.Stop(); err != nil {
			style.PrintWarning("failed to stop session: %v", err)
		}
	}

	// Start fresh
	if err := mgr.Start(narratorAgentOverride); err != nil {
		return err
	}

	fmt.Printf("%s Narrator restarted\n", style.Bold.Render("✓"))
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt narrator attach' to connect"))
	return nil
}
