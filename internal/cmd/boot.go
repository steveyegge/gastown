package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	bootStatusJSON    bool
	bootDegraded      bool
	bootAgentOverride string
)

var bootCmd = &cobra.Command{
	Use:     "boot",
	GroupID: GroupAgents,
	Short:   "Manage Boot (Deacon watchdog)",
	Long: `Manage Boot - the daemon's watchdog for Deacon triage.

Boot is a special dog that runs fresh on each daemon tick. It observes
the system state and decides whether to start/wake/nudge/interrupt the
Deacon, or do nothing. This centralizes the "when to wake" decision in
an agent that can reason about it.

Boot lifecycle:
  1. Daemon tick spawns Boot (fresh each time)
  2. Boot runs triage: observe, decide, act
  3. Boot cleans inbox (discards stale handoffs)
  4. Boot exits (or handoffs in non-degraded mode)

Location: ~/gt/deacon/dogs/boot/
Session: gt-boot`,
}

var bootStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Boot status",
	Long: `Show Boot's current status and last execution.

Displays:
  - Whether Boot is currently running
  - Last action taken (start/wake/nudge/nothing)
  - Timing information
  - Degraded mode status`,
	RunE: runBootStatus,
}

var bootSpawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Spawn Boot for triage",
	Long: `Spawn Boot to run the triage cycle.

This is normally called by the daemon. It spawns Boot in a fresh
tmux session (or subprocess in degraded mode) to observe and decide
what action to take on the Deacon.

Boot runs to completion and exits - it doesn't maintain state
between invocations.`,
	RunE: runBootSpawn,
}

var bootTriageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Run triage directly (degraded mode)",
	Long: `Run Boot's triage logic directly without Claude.

This is for degraded mode operation when tmux is unavailable.
It performs basic observation and takes conservative action:
  - If Deacon is not running: start it
  - If Deacon appears stuck: attempt restart
  - Otherwise: do nothing

Use --degraded flag when running in degraded mode.`,
	RunE: runBootTriage,
}

func init() {
	bootStatusCmd.Flags().BoolVar(&bootStatusJSON, "json", false, "Output as JSON")
	bootTriageCmd.Flags().BoolVar(&bootDegraded, "degraded", false, "Run in degraded mode (no tmux)")
	bootSpawnCmd.Flags().StringVar(&bootAgentOverride, "agent", "", "Agent alias to run Boot with (overrides town default)")

	bootCmd.AddCommand(bootStatusCmd)
	bootCmd.AddCommand(bootSpawnCmd)
	bootCmd.AddCommand(bootTriageCmd)

	rootCmd.AddCommand(bootCmd)
}

func getBootManager() (*boot.Boot, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("finding town root: %w", err)
	}

	return boot.New(townRoot), nil
}

// getBootSessionName returns the Boot session name.
func getBootSessionName() string {
	return boot.SessionName
}

func runBootStatus(cmd *cobra.Command, args []string) error {
	b, err := getBootManager()
	if err != nil {
		return err
	}

	status, err := b.LoadStatus()
	if err != nil {
		return fmt.Errorf("loading status: %w", err)
	}

	isRunning := b.IsRunning()
	sessionAlive := b.IsSessionAlive()

	if bootStatusJSON {
		output := map[string]interface{}{
			"running":       isRunning,
			"session_alive": sessionAlive,
			"degraded":      b.IsDegraded(),
			"boot_dir":      b.Dir(),
			"last_status":   status,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Pretty print
	fmt.Println(style.Bold.Render("Boot Status"))
	fmt.Println()

	if isRunning {
		fmt.Printf("  State: %s\n", style.Bold.Render("running"))
	} else {
		fmt.Printf("  State: %s\n", style.Dim.Render("idle"))
	}

	if sessionAlive {
		fmt.Printf("  Session: %s (alive)\n", boot.SessionName)
	} else {
		fmt.Printf("  Session: %s\n", style.Dim.Render("not running"))
	}

	if b.IsDegraded() {
		fmt.Printf("  Mode: %s\n", style.Bold.Render("DEGRADED"))
	} else {
		fmt.Printf("  Mode: normal\n")
	}

	fmt.Println()
	fmt.Println(style.Dim.Render("Last Execution:"))

	if status.StartedAt.IsZero() {
		fmt.Printf("  %s\n", style.Dim.Render("(no executions recorded)"))
	} else {
		if !status.CompletedAt.IsZero() {
			duration := status.CompletedAt.Sub(status.StartedAt)
			fmt.Printf("  Completed: %s (%s ago)\n",
				status.CompletedAt.Format("15:04:05"),
				formatDurationAgo(time.Since(status.CompletedAt)))
			fmt.Printf("  Duration:  %s\n", duration.Round(time.Millisecond))
		} else {
			fmt.Printf("  Started: %s\n", status.StartedAt.Format("15:04:05"))
		}

		if status.LastAction != "" {
			fmt.Printf("  Action:  %s", status.LastAction)
			if status.Target != "" {
				fmt.Printf(" → %s", status.Target)
			}
			fmt.Println()
		}

		if status.Error != "" {
			fmt.Printf("  Error:   %s\n", style.Bold.Render(status.Error))
		}
	}

	fmt.Println()
	fmt.Printf("  Dir: %s\n", b.Dir())

	return nil
}

func runBootSpawn(cmd *cobra.Command, args []string) error {
	b, err := getBootManager()
	if err != nil {
		return err
	}

	if b.IsRunning() {
		fmt.Println("Boot is already running - skipping spawn")
		return nil
	}

	// Save starting status
	status := &boot.Status{
		Running:   true,
		StartedAt: time.Now(),
	}
	if err := b.SaveStatus(status); err != nil {
		return fmt.Errorf("saving status: %w", err)
	}

	// Spawn Boot
	if err := b.Spawn(bootAgentOverride); err != nil {
		status.Error = err.Error()
		status.CompletedAt = time.Now()
		status.Running = false
		_ = b.SaveStatus(status)
		return fmt.Errorf("spawning boot: %w", err)
	}

	if b.IsDegraded() {
		fmt.Println("Boot spawned in degraded mode (subprocess)")
	} else {
		fmt.Printf("Boot spawned in session: %s\n", boot.SessionName)
	}

	return nil
}

func runBootTriage(cmd *cobra.Command, args []string) error {
	b, err := getBootManager()
	if err != nil {
		return err
	}

	// Acquire lock
	if err := b.AcquireLock(); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer func() { _ = b.ReleaseLock() }()

	startTime := time.Now()
	status := &boot.Status{
		Running:   true,
		StartedAt: startTime,
	}

	// In degraded mode, we do basic mechanical triage
	// without full Claude reasoning capability
	action, target, triageErr := runDegradedTriage(b)

	status.LastAction = action
	status.Target = target
	status.Running = false
	status.CompletedAt = time.Now()

	if triageErr != nil {
		status.Error = triageErr.Error()
	}

	if err := b.SaveStatus(status); err != nil {
		return fmt.Errorf("saving status: %w", err)
	}

	if triageErr != nil {
		return triageErr
	}

	fmt.Printf("Triage complete: %s", action)
	if target != "" {
		fmt.Printf(" → %s", target)
	}
	fmt.Println()

	return nil
}

// runDegradedTriage performs basic Deacon health check without AI reasoning.
// This is a mechanical fallback when full Claude sessions aren't available.
func runDegradedTriage(b *boot.Boot) (action, target string, err error) {
	tm := b.Tmux()

	// Check if Deacon session exists
	deaconSession := getDeaconSessionName()
	hasDeacon, err := tm.HasSession(deaconSession)
	if err != nil {
		return "error", "deacon", fmt.Errorf("checking deacon session: %w", err)
	}

	if !hasDeacon {
		// Deacon not running - this is unusual, daemon should have restarted it
		// In degraded mode, we just report - let daemon handle restart
		return "report", "deacon-missing", nil
	}

	// Deacon exists - check heartbeat to detect stuck sessions
	// A session can exist but be stuck (not making progress)
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		hb := deacon.ReadHeartbeat(townRoot)
		if hb.ShouldPoke() {
			// Heartbeat is stale (>15 min) - Deacon is stuck
			// Nudge the session to try to wake it up
			age := hb.Age()
			if age > 30*time.Minute {
				// Very stuck - restart the session
				fmt.Printf("Deacon heartbeat is %s old - restarting session\n", age.Round(time.Minute))
				if err := tm.KillSessionWithProcesses(deaconSession); err == nil {
					return "restart", "deacon-stuck", nil
				}
			} else {
				// Stuck but not critically - try nudging first
				fmt.Printf("Deacon heartbeat is %s old - nudging session\n", age.Round(time.Minute))
				_ = tm.NudgeSession(deaconSession, "HEALTH_CHECK: heartbeat is stale, respond to confirm responsiveness")
				return "nudge", "deacon-stale", nil
			}
		}
	}

	return "nothing", "", nil
}

// formatDurationAgo formats a duration for human display.
func formatDurationAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d min", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}
