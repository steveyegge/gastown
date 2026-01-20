package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Witness command flags
var (
	witnessStatusJSON    bool
	witnessAgentOverride string
	witnessEnvOverrides  []string
)

var witnessCmd = &cobra.Command{
	Use:     "witness",
	GroupID: GroupAgents,
	Short:   "Manage the Witness (per-rig polecat health monitor)",
	RunE:    requireSubcommand,
	Long: `Manage the Witness - the per-rig polecat health monitor.

The Witness patrols a single rig, watching over its polecats:
  - Detects stalled polecats (crashed or stuck mid-work)
  - Nudges unresponsive sessions back to life
  - Cleans up zombie polecats (finished but failed to exit)
  - Nukes sandboxes when polecats complete via 'gt done'

The Witness does NOT force session cycles or interrupt working polecats.
Polecats manage their own sessions (via gt handoff). The Witness handles
failures and edge cases only.

One Witness per rig. The Deacon monitors all Witnesses.

Role shortcuts: "witness" in mail/nudge addresses resolves to this rig's Witness.`,
}

var witnessStartCmd = &cobra.Command{
	Use:     "start <rig>",
	Aliases: []string{"spawn"},
	Short:   "Start the witness",
	Long: `Start the Witness for a rig.

Launches the monitoring agent which watches for stuck polecats and orphaned
sandboxes, taking action to keep work flowing.

Self-Cleaning Model: Polecats nuke themselves after work. The Witness handles
crash recovery (restart with hooked work) and orphan cleanup (nuke abandoned
sandboxes). There is no "idle" state - polecats either have work or don't exist.

Examples:
  gt witness start greenplace
  gt witness start greenplace --agent codex
  gt witness start greenplace --env ANTHROPIC_MODEL=claude-3-haiku
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
  gt witness restart greenplace
  gt witness restart greenplace --agent codex
  gt witness restart greenplace --env ANTHROPIC_MODEL=claude-3-haiku`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessRestart,
}

func init() {
	// Start flags
	witnessStartCmd.Flags().StringVar(&witnessAgentOverride, "agent", "", "Agent alias to run the Witness with (overrides town default)")
	witnessStartCmd.Flags().StringArrayVar(&witnessEnvOverrides, "env", nil, "Environment variable override (KEY=VALUE, can be repeated)")

	// Status flags
	witnessStatusCmd.Flags().BoolVar(&witnessStatusJSON, "json", false, "Output as JSON")

	// Restart flags
	witnessRestartCmd.Flags().StringVar(&witnessAgentOverride, "agent", "", "Agent alias to run the Witness with (overrides town default)")
	witnessRestartCmd.Flags().StringArrayVar(&witnessEnvOverrides, "env", nil, "Environment variable override (KEY=VALUE, can be repeated)")

	// Add subcommands
	witnessCmd.AddCommand(witnessStartCmd)
	witnessCmd.AddCommand(witnessStopCmd)
	witnessCmd.AddCommand(witnessRestartCmd)
	witnessCmd.AddCommand(witnessStatusCmd)
	witnessCmd.AddCommand(witnessAttachCmd)

	rootCmd.AddCommand(witnessCmd)
}

// getWitnessManager creates a witness manager for a rig.
// agentOverride optionally specifies a different agent alias to use.
// envOverrides is an optional list of KEY=VALUE strings to merge with base env vars.
func getWitnessManager(rigName string, agentOverride string, envOverrides ...string) (*witness.Manager, error) {
	townRoot, r, err := getRig(rigName)
	if err != nil {
		return nil, err
	}

	agentName, _ := config.ResolveRoleAgentName("witness", townRoot, r.Path)
	if agentOverride != "" {
		agentName = agentOverride
	}
	return factory.New(townRoot).WitnessManager(r, agentName, envOverrides...), nil
}

func runWitnessStart(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	townRoot, _, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Convert env overrides from []string to map
	envOverrides := make(map[string]string)
	for _, override := range witnessEnvOverrides {
		if parts := splitEnvOverride(override); len(parts) == 2 {
			envOverrides[parts[0]] = parts[1]
		}
	}

	fmt.Printf("Starting witness for %s...\n", rigName)

	// Build start options (agent resolved automatically, with optional override)
	var opts []factory.StartOption
	opts = append(opts, factory.WithAgent(witnessAgentOverride))
	if len(envOverrides) > 0 {
		opts = append(opts, factory.WithEnvOverrides(envOverrides))
	}

	// Use factory.Start() with WitnessAddress
	id := agent.WitnessAddress(rigName)
	if _, err := factory.Start(townRoot, id, opts...); err != nil {
		if err == agent.ErrAlreadyRunning {
			fmt.Printf("%s Witness is already running\n", style.Dim.Render("⚠"))
			fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
			return nil
		}
		return fmt.Errorf("starting witness: %w", err)
	}

	fmt.Printf("%s Witness started for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness status' to check progress"))
	return nil
}

// splitEnvOverride splits "KEY=VALUE" into [KEY, VALUE], or returns nil if invalid.
func splitEnvOverride(s string) []string {
	for i, c := range s {
		if c == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}

// parseEnvOverrides converts a slice of KEY=VALUE strings into a map.
// Invalid entries (without =) are silently skipped.
func parseEnvOverrides(overrides []string) map[string]string {
	result := make(map[string]string)
	for _, override := range overrides {
		if parts := splitEnvOverride(override); len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func runWitnessStop(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Use factory.Agents().Stop() with WitnessAddress
	agents := factory.Agents()
	id := agent.WitnessAddress(rigName)

	if !agents.Exists(id) {
		fmt.Printf("%s Witness is not running\n", style.Dim.Render("⚠"))
		return nil
	}

	if err := agents.Stop(id, true); err != nil {
		return fmt.Errorf("stopping witness: %w", err)
	}

	fmt.Printf("%s Witness stopped for %s\n", style.Bold.Render("✓"), rigName)
	return nil
}

func runWitnessStatus(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, err := getWitnessManager(rigName, "")
	if err != nil {
		return err
	}

	w, err := mgr.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	// Check actual session state (more reliable than state file)
	agents := factory.Agents()
	witnessID := agent.WitnessAddress(rigName)
	sessionRunning := agents.Exists(witnessID)
	sessionName := witnessSessionName(rigName)

	// Reconcile state: session is the source of truth for background mode
	if sessionRunning && w.State != agent.StateRunning {
		w.State = agent.StateRunning
	} else if !sessionRunning && w.State == agent.StateRunning {
		w.State = agent.StateStopped
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
	case agent.StateRunning:
		stateStr = style.Bold.Render("● running")
	case agent.StateStopped:
		stateStr = style.Dim.Render("○ stopped")
	case agent.StatePaused:
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

// witnessSessionName returns the session name for a rig's witness.
// Used by status.go for display purposes.
func witnessSessionName(rigName string) string {
	return fmt.Sprintf("gt-%s-witness", rigName)
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

	// Verify rig exists and get townRoot
	townRoot, _, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Ensure session exists (creates if needed, agent auto-resolved)
	witnessID := agent.WitnessAddress(rigName)
	if _, err := factory.Start(townRoot, witnessID); err != nil && err != agent.ErrAlreadyRunning {
		return err
	} else if err == nil {
		fmt.Printf("Started witness session for %s\n", rigName)
	}

	// Compute session name
	sessionName := fmt.Sprintf("gt-%s-witness", rigName)

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

	townRoot, _, err := getRig(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Restarting witness for %s...\n", rigName)

	// Convert env overrides from []string to map
	envOverrides := make(map[string]string)
	for _, override := range witnessEnvOverrides {
		if parts := splitEnvOverride(override); len(parts) == 2 {
			envOverrides[parts[0]] = parts[1]
		}
	}

	// Build start options with KillExisting (agent auto-resolved, with optional override)
	var opts []factory.StartOption
	opts = append(opts, factory.WithKillExisting())
	opts = append(opts, factory.WithAgent(witnessAgentOverride))
	if len(envOverrides) > 0 {
		opts = append(opts, factory.WithEnvOverrides(envOverrides))
	}

	// Use factory.Start() with WitnessAddress
	id := agent.WitnessAddress(rigName)
	if _, err := factory.Start(townRoot, id, opts...); err != nil {
		return fmt.Errorf("starting witness: %w", err)
	}

	fmt.Printf("%s Witness restarted for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt witness attach' to connect"))
	return nil
}
