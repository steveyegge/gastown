package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Witness command flags
var (
	witnessForeground bool
	witnessStatusJSON bool
)

var witnessCmd = &cobra.Command{
	Use:     "witness",
	GroupID: GroupAgents,
	Short:   "Manage the polecat monitoring agent",
	RunE:    requireSubcommand,
	Long: `Manage the Witness monitoring agent for a rig.

The Witness monitors polecats for stuck/idle state, nudges polecats
that seem blocked, and reports status to the mayor.`,
}

var witnessStartCmd = &cobra.Command{
	Use:     "start <rig>",
	Aliases: []string{"spawn"},
	Short:   "Start the witness",
	Long: `Start the Witness for a rig.

Launches the monitoring agent which watches polecats for stuck or idle
states and takes action to keep work flowing.

Examples:
  gt witness start greenplace
  gt witness start greenplace --foreground`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessStart,
}

var witnessStopCmd = &cobra.Command{
	Use:   "stop <rig>",
	Short: "Stop the witness",
	Long: `Stop a running Witness.

Gracefully stops the witness monitoring agent.`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessStop,
}

var witnessStatusCmd = &cobra.Command{
	Use:   "status <rig>",
	Short: "Show witness status",
	Long: `Show the status of a rig's Witness.

Displays running state, monitored polecats, and statistics.`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessStatus,
}

var witnessAttachCmd = &cobra.Command{
	Use:     "attach [rig]",
	Aliases: []string{"at"},
	Short:   "Attach to witness session",
	Long: `Attach to the Witness tmux session for a rig.

Attaches the current terminal to the witness's tmux session.
Detach with Ctrl-B D.

If the witness is not running, this will start it first.
If rig is not specified, infers it from the current directory.

Examples:
  gt witness attach greenplace
  gt witness attach          # infer rig from cwd`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWitnessAttach,
}

var witnessRestartCmd = &cobra.Command{
	Use:   "restart <rig>",
	Short: "Restart the witness",
	Long: `Restart the Witness for a rig.

Stops the current session (if running) and starts a fresh one.

Examples:
  gt witness restart greenplace`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessRestart,
}

func init() {
	// Start flags
	witnessStartCmd.Flags().BoolVar(&witnessForeground, "foreground", false, "Run in foreground (default: background)")

	// Status flags
	witnessStatusCmd.Flags().BoolVar(&witnessStatusJSON, "json", false, "Output as JSON")

	// Add subcommands
	witnessCmd.AddCommand(witnessStartCmd)
	witnessCmd.AddCommand(witnessStopCmd)
	witnessCmd.AddCommand(witnessRestartCmd)
	witnessCmd.AddCommand(witnessStatusCmd)
	witnessCmd.AddCommand(witnessAttachCmd)

	rootCmd.AddCommand(witnessCmd)
}

// getWitnessManager creates a witness manager for a rig.
func getWitnessManager(rigName string) (*witness.Manager, *rig.Rig, error) {
	_, r, err := getRig(rigName)
	if err != nil {
		return nil, nil, err
	}

	mgr := witness.NewManager(r)
	return mgr, r, nil
}

func runWitnessStart(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, r, err := getWitnessManager(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Starting witness for %s...\n", rigName)

	if witnessForeground {
		// Foreground mode is no longer supported - patrol logic moved to mol-witness-patrol
		if err := mgr.Start(); err != nil {
			if err == witness.ErrAlreadyRunning {
				fmt.Printf("%s Witness is already running\n", style.Dim.Render("⚠"))
				return nil
			}
			return fmt.Errorf("starting witness: %w", err)
		}
		fmt.Printf("%s Note: Foreground mode no longer runs patrol loop\n", style.Dim.Render("⚠"))
		fmt.Printf("  %s\n", style.Dim.Render("Patrol logic is now handled by mol-witness-patrol molecule"))
		return nil
	}

	// Background mode: create tmux session with Claude
	created, err := ensureWitnessSession(rigName, r)
	if err != nil {
		return err
	}

	if !created {
		fmt.Printf("%s Witness session already running\n", style.Dim.Render("⚠"))
		fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
		return nil
	}

	// Update manager state to reflect running session (non-fatal: state file update)
	_ = mgr.Start()

	fmt.Printf("%s Witness started for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness status' to check progress"))
	return nil
}

func runWitnessStop(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, _, err := getWitnessManager(rigName)
	if err != nil {
		return err
	}

	// Kill tmux session if it exists
	t := tmux.NewTmux()
	sessionName := witnessSessionName(rigName)
	running, _ := t.HasSession(sessionName)
	if running {
		if err := t.KillSession(sessionName); err != nil {
			style.PrintWarning("failed to kill session: %v", err)
		}
	}

	// Update state file
	if err := mgr.Stop(); err != nil {
		if err == witness.ErrNotRunning && !running {
			fmt.Printf("%s Witness is not running\n", style.Dim.Render("⚠"))
			return nil
		}
		// Even if manager.Stop fails, if we killed the session it's stopped
		if !running {
			return fmt.Errorf("stopping witness: %w", err)
		}
	}

	fmt.Printf("%s Witness stopped for %s\n", style.Bold.Render("✓"), rigName)
	return nil
}

func runWitnessStatus(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, _, err := getWitnessManager(rigName)
	if err != nil {
		return err
	}

	w, err := mgr.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	// Check actual tmux session state (more reliable than state file)
	t := tmux.NewTmux()
	sessionName := witnessSessionName(rigName)
	sessionRunning, _ := t.HasSession(sessionName)

	// Reconcile state: tmux session is the source of truth for background mode
	if sessionRunning && w.State != witness.StateRunning {
		w.State = witness.StateRunning
	} else if !sessionRunning && w.State == witness.StateRunning {
		w.State = witness.StateStopped
	}

	// JSON output
	if witnessStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(w)
	}

	// Human-readable output
	fmt.Printf("%s Witness: %s\n\n", style.Bold.Render(AgentTypeIcons[AgentWitness]), rigName)

	stateStr := string(w.State)
	switch w.State {
	case witness.StateRunning:
		stateStr = style.Bold.Render("● running")
	case witness.StateStopped:
		stateStr = style.Dim.Render("○ stopped")
	case witness.StatePaused:
		stateStr = style.Dim.Render("⏸ paused")
	}
	fmt.Printf("  State: %s\n", stateStr)
	if sessionRunning {
		fmt.Printf("  Session: %s\n", sessionName)
	}

	if w.StartedAt != nil {
		fmt.Printf("  Started: %s\n", w.StartedAt.Format("2006-01-02 15:04:05"))
	}

	// Show monitored polecats
	fmt.Printf("\n  %s\n", style.Bold.Render("Monitored Polecats:"))
	if len(w.MonitoredPolecats) == 0 {
		fmt.Printf("    %s\n", style.Dim.Render("(none)"))
	} else {
		for _, p := range w.MonitoredPolecats {
			fmt.Printf("    • %s\n", p)
		}
	}

	return nil
}

// witnessSessionName returns the tmux session name for a rig's witness.
func witnessSessionName(rigName string) string {
	return fmt.Sprintf("gt-%s-witness", rigName)
}

// ensureWitnessSession creates a witness tmux session if it doesn't exist.
// Returns true if a new session was created, false if it already existed (and is healthy).
// Implements 'ensure' semantics: if session exists but Claude is dead (zombie), kills and recreates.
func ensureWitnessSession(rigName string, r *rig.Rig) (bool, error) {
	t := tmux.NewTmux()
	sessionName := witnessSessionName(rigName)

	// Check if session already exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return false, fmt.Errorf("checking session: %w", err)
	}

	if running {
		// Session exists - check if Claude is actually running (healthy vs zombie)
		if t.IsClaudeRunning(sessionName) {
			// Healthy - Claude is running
			return false, nil
		}
		// Zombie - tmux alive but Claude dead. Kill and recreate.
		fmt.Printf("%s Detected zombie session (tmux alive, Claude dead). Recreating...\n", style.Dim.Render("⚠"))
		if err := t.KillSession(sessionName); err != nil {
			return false, fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Working directory is the witness's rig clone (if it exists) or witness dir
	// This ensures gt prime detects the Witness role correctly
	witnessDir := filepath.Join(r.Path, "witness", "rig")
	if _, err := os.Stat(witnessDir); os.IsNotExist(err) {
		// Try witness/ without rig subdirectory
		witnessDir = filepath.Join(r.Path, "witness")
		if _, err := os.Stat(witnessDir); os.IsNotExist(err) {
			// Fall back to rig path (shouldn't happen in normal setup)
			witnessDir = r.Path
		}
	}

	// Ensure Claude settings exist (autonomous role needs mail in SessionStart)
	if err := claude.EnsureSettingsForRole(witnessDir, "witness"); err != nil {
		return false, fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Create new tmux session
	if err := t.NewSession(sessionName, witnessDir); err != nil {
		return false, fmt.Errorf("creating session: %w", err)
	}

	// Set environment
	bdActor := fmt.Sprintf("%s/witness", rigName)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "witness")
	_ = t.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Apply Gas Town theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(rigName)
	_ = t.ConfigureGasTownSession(sessionName, theme, rigName, "witness", "witness")

	// Launch Claude directly (no shell respawn loop)
	// Restarts are handled by daemon via LIFECYCLE mail or deacon health-scan
	// NOTE: No gt prime injection needed - SessionStart hook handles it automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	if err := t.SendKeys(sessionName, config.BuildAgentStartupCommand("witness", bdActor, "", "")); err != nil {
		return false, fmt.Errorf("sending command: %w", err)
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}
	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/witness", rigName)
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "patrol",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous patrol execution.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionName, session.PropulsionNudgeForRole("witness", witnessDir)) // Non-fatal

	return true, nil
}

func runWitnessAttach(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	// Infer rig from cwd if not provided
	if rigName == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig: %w\nUsage: gt witness attach <rig>", err)
		}
	}

	// Verify rig exists
	_, r, err := getWitnessManager(rigName)
	if err != nil {
		return err
	}

	sessionName := witnessSessionName(rigName)

	// Ensure session exists (creates if needed)
	created, err := ensureWitnessSession(rigName, r)
	if err != nil {
		return err
	}

	if created {
		fmt.Printf("Started witness session for %s\n", rigName)
	}

	// Attach to the session
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	attachCmd := exec.Command(tmuxPath, "attach-session", "-t", sessionName)
	attachCmd.Stdin = os.Stdin
	attachCmd.Stdout = os.Stdout
	attachCmd.Stderr = os.Stderr
	return attachCmd.Run()
}

func runWitnessRestart(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, r, err := getWitnessManager(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Restarting witness for %s...\n", rigName)

	// Kill tmux session if it exists
	t := tmux.NewTmux()
	sessionName := witnessSessionName(rigName)
	running, _ := t.HasSession(sessionName)
	if running {
		if err := t.KillSession(sessionName); err != nil {
			style.PrintWarning("failed to kill session: %v", err)
		}
	}

	// Update state file to stopped (non-fatal: state file update)
	_ = mgr.Stop()

	// Start fresh
	created, err := ensureWitnessSession(rigName, r)
	if err != nil {
		return fmt.Errorf("starting witness: %w", err)
	}

	if created {
		_ = mgr.Start() // non-fatal: state file update
	}

	fmt.Printf("%s Witness restarted for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
	return nil
}
