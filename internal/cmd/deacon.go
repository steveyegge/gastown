package cmd

import (
	"encoding/json"
	"errors"
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
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getDeaconSessionName returns the Deacon session name.
func getDeaconSessionName() string {
	return session.DeaconSessionName()
}

var deaconCmd = &cobra.Command{
	Use:     "deacon",
	Aliases: []string{"dea"},
	GroupID: GroupAgents,
	Short:   "Manage the Deacon session",
	RunE:    requireSubcommand,
	Long: `Manage the Deacon tmux session.

The Deacon is the hierarchical health-check orchestrator for Gas Town.
It monitors the Mayor and Witnesses, handles lifecycle requests, and
keeps the town running. Use the subcommands to start, stop, attach,
and check status.`,
}

var deaconStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"spawn"},
	Short:   "Start the Deacon session",
	Long: `Start the Deacon tmux session.

Creates a new detached tmux session for the Deacon and launches Claude.
The session runs in the workspace root directory.`,
	RunE: runDeaconStart,
}

var deaconStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Deacon session",
	Long: `Stop the Deacon tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runDeaconStop,
}

var deaconAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Deacon session",
	Long: `Attach to the running Deacon tmux session.

Attaches the current terminal to the Deacon's tmux session.
Detach with Ctrl-B D.`,
	RunE: runDeaconAttach,
}

var deaconStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Deacon session status",
	Long:  `Check if the Deacon tmux session is currently running.`,
	RunE:  runDeaconStatus,
}

var deaconRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Deacon session",
	Long: `Restart the Deacon tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runDeaconRestart,
}

var deaconHeartbeatCmd = &cobra.Command{
	Use:   "heartbeat [action]",
	Short: "Update the Deacon heartbeat",
	Long: `Update the Deacon heartbeat file.

The heartbeat signals to the daemon that the Deacon is alive and working.
Call this at the start of each wake cycle to prevent daemon pokes.

Examples:
  gt deacon heartbeat                    # Touch heartbeat with timestamp
  gt deacon heartbeat "checking mayor"   # Touch with action description`,
	RunE: runDeaconHeartbeat,
}

var deaconTriggerPendingCmd = &cobra.Command{
	Use:   "trigger-pending",
	Short: "Trigger pending polecat spawns (bootstrap mode)",
	Long: `Check inbox for POLECAT_STARTED messages and trigger ready polecats.

⚠️  BOOTSTRAP MODE ONLY - Uses regex detection (ZFC violation acceptable).

This command uses WaitForClaudeReady (regex) to detect when Claude is ready.
This is appropriate for daemon bootstrap when no AI is available.

In steady-state, the Deacon should use AI-based observation instead:
  gt deacon pending     # View pending spawns with captured output
  gt peek <session>     # Observe session output (AI analyzes)
  gt nudge <session>    # Trigger when AI determines ready

This command is typically called by the daemon during cold startup.`,
	RunE: runDeaconTriggerPending,
}

var deaconHealthCheckCmd = &cobra.Command{
	Use:   "health-check <agent>",
	Short: "Send a health check ping to an agent and track response",
	Long: `Send a HEALTH_CHECK nudge to an agent and wait for response.

This command is used by the Deacon during health rounds to detect stuck sessions.
It tracks consecutive failures and determines when force-kill is warranted.

The detection protocol:
1. Send HEALTH_CHECK nudge to the agent
2. Wait for agent to update their bead (configurable timeout, default 30s)
3. If no activity update, increment failure counter
4. After N consecutive failures (default 3), recommend force-kill

Exit codes:
  0 - Agent responded or is in cooldown (no action needed)
  1 - Error occurred
  2 - Agent should be force-killed (consecutive failures exceeded)

Examples:
  gt deacon health-check gastown/polecats/max
  gt deacon health-check gastown/witness --timeout=60s
  gt deacon health-check deacon --failures=5`,
	Args: cobra.ExactArgs(1),
	RunE: runDeaconHealthCheck,
}

var deaconForceKillCmd = &cobra.Command{
	Use:   "force-kill <agent>",
	Short: "Force-kill an unresponsive agent session",
	Long: `Force-kill an agent session that has been detected as stuck.

This command is used by the Deacon when an agent fails consecutive health checks.
It performs the force-kill protocol:

1. Log the intervention (send mail to agent)
2. Kill the tmux session
3. Update agent bead state to "killed"
4. Notify mayor (optional, for visibility)

After force-kill, the agent is 'asleep'. Normal wake mechanisms apply:
- gt rig boot restarts it
- Or stays asleep until next activity trigger

This respects the cooldown period - won't kill if recently killed.

Examples:
  gt deacon force-kill gastown/polecats/max
  gt deacon force-kill gastown/witness --reason="unresponsive for 90s"`,
	Args: cobra.ExactArgs(1),
	RunE: runDeaconForceKill,
}

var deaconHealthStateCmd = &cobra.Command{
	Use:   "health-state",
	Short: "Show health check state for all monitored agents",
	Long: `Display the current health check state including:
- Consecutive failure counts
- Last ping and response times
- Force-kill history and cooldowns

This helps the Deacon understand which agents may need attention.`,
	RunE: runDeaconHealthState,
}

var deaconZombieScanCmd = &cobra.Command{
	Use:   "zombie-scan [rig]",
	Short: "Scan for idle polecats that should have been nuked",
	Long: `Backup check for polecats the Witness should have cleaned up.

Scans for "zombie" polecats that meet ALL of these criteria:
- State: idle or done (no active work)
- Session: not running (tmux session dead)
- No hooked work
- Last activity: older than threshold (default 10 minutes)

These are polecats that the Witness should have nuked but didn't.
This provides defense-in-depth against Witness failures.

Actions:
1. Log warning about witness failure
2. Nuke the zombie polecat directly
3. Notify mayor of witness issue (optional)

Examples:
  gt deacon zombie-scan                    # Scan all rigs
  gt deacon zombie-scan gastown            # Scan specific rig
  gt deacon zombie-scan --dry-run          # Preview only
  gt deacon zombie-scan --threshold=5m     # Custom staleness threshold`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeaconZombieScan,
}

var (
	triggerTimeout time.Duration

	// Health check flags
	healthCheckTimeout  time.Duration
	healthCheckFailures int
	healthCheckCooldown time.Duration

	// Force kill flags
	forceKillReason     string
	forceKillSkipNotify bool

	// Zombie scan flags
	zombieScanDryRun    bool
	zombieScanThreshold time.Duration
	zombieScanNuke      bool
)

func init() {
	deaconCmd.AddCommand(deaconStartCmd)
	deaconCmd.AddCommand(deaconStopCmd)
	deaconCmd.AddCommand(deaconAttachCmd)
	deaconCmd.AddCommand(deaconStatusCmd)
	deaconCmd.AddCommand(deaconRestartCmd)
	deaconCmd.AddCommand(deaconHeartbeatCmd)
	deaconCmd.AddCommand(deaconTriggerPendingCmd)
	deaconCmd.AddCommand(deaconHealthCheckCmd)
	deaconCmd.AddCommand(deaconForceKillCmd)
	deaconCmd.AddCommand(deaconHealthStateCmd)
	deaconCmd.AddCommand(deaconZombieScanCmd)

	// Flags for trigger-pending
	deaconTriggerPendingCmd.Flags().DurationVar(&triggerTimeout, "timeout", 2*time.Second,
		"Timeout for checking if Claude is ready")

	// Flags for health-check
	deaconHealthCheckCmd.Flags().DurationVar(&healthCheckTimeout, "timeout", 30*time.Second,
		"How long to wait for agent response")
	deaconHealthCheckCmd.Flags().IntVar(&healthCheckFailures, "failures", 3,
		"Number of consecutive failures before recommending force-kill")
	deaconHealthCheckCmd.Flags().DurationVar(&healthCheckCooldown, "cooldown", 5*time.Minute,
		"Minimum time between force-kills of same agent")

	// Flags for force-kill
	deaconForceKillCmd.Flags().StringVar(&forceKillReason, "reason", "",
		"Reason for force-kill (included in notifications)")
	deaconForceKillCmd.Flags().BoolVar(&forceKillSkipNotify, "skip-notify", false,
		"Skip sending notification mail to mayor")

	// Flags for zombie-scan
	deaconZombieScanCmd.Flags().BoolVarP(&zombieScanDryRun, "dry-run", "n", false,
		"Show what would be done without nuking")
	deaconZombieScanCmd.Flags().DurationVar(&zombieScanThreshold, "threshold", 10*time.Minute,
		"Staleness threshold for zombie detection")
	deaconZombieScanCmd.Flags().BoolVar(&zombieScanNuke, "nuke", true,
		"Nuke detected zombies (use --nuke=false to report only)")

	rootCmd.AddCommand(deaconCmd)
}

func runDeaconStart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getDeaconSessionName()

	// Check if session already exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		return fmt.Errorf("Deacon session already running. Attach with: gt deacon attach")
	}

	if err := startDeaconSession(t, sessionName); err != nil {
		return err
	}

	fmt.Printf("%s Deacon session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt deacon attach"))

	return nil
}

// startDeaconSession creates and initializes the Deacon tmux session.
func startDeaconSession(t *tmux.Tmux, sessionName string) error {
	// Find workspace root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Deacon runs from its own directory (for correct role detection by gt prime)
	deaconDir := filepath.Join(townRoot, "deacon")

	// Ensure deacon directory exists
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}

	// Ensure deacon has patrol hooks (idempotent)
	if err := ensurePatrolHooks(deaconDir); err != nil {
		style.PrintWarning("Could not create deacon hooks: %v", err)
	}

	// Create session in deacon directory
	fmt.Println("Starting Deacon session...")
	if err := t.NewSession(sessionName, deaconDir); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "deacon")
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", "deacon")

	// Apply Deacon theme (non-fatal: theming failure doesn't affect operation)
	// Note: ConfigureGasTownSession includes cycle bindings
	theme := tmux.DeaconTheme()
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Deacon", "health-check")

	// Launch Claude directly (no shell respawn loop)
	// Restarts are handled by daemon via ensureDeaconRunning on each heartbeat
	// The startup hook handles context loading automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	if err := t.SendKeys(sessionName, config.BuildAgentStartupCommand("deacon", "deacon", "", "")); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}
	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: "deacon",
		Sender:    "daemon",
		Topic:     "patrol",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous patrol execution.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionName, session.PropulsionNudgeForRole("deacon", deaconDir)) // Non-fatal

	return nil
}

func runDeaconStop(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getDeaconSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return errors.New("Deacon session is not running")
	}

	fmt.Println("Stopping Deacon session...")

	// Try graceful shutdown first (best-effort interrupt)
	_ = t.SendKeysRaw(sessionName, "C-c")
	time.Sleep(100 * time.Millisecond)

	// Kill the session
	if err := t.KillSession(sessionName); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	fmt.Printf("%s Deacon session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runDeaconAttach(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getDeaconSessionName()

	// Check if session exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Deacon session not running, starting...")
		if err := startDeaconSession(t, sessionName); err != nil {
			return err
		}
	}
	// Session uses a respawn loop, so Claude restarts automatically if it exits

	// Use shared attach helper (smart: links if inside tmux, attaches if outside)
	return attachToTmuxSession(sessionName)
}

func runDeaconStatus(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getDeaconSessionName()

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
			fmt.Printf("%s Deacon session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
			fmt.Printf("  Status: %s\n", status)
			fmt.Printf("  Created: %s\n", info.Created)
			fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt deacon attach"))
		} else {
			fmt.Printf("%s Deacon session is %s\n",
				style.Bold.Render("●"),
				style.Bold.Render("running"))
		}
	} else {
		fmt.Printf("%s Deacon session is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt deacon start"))
	}

	return nil
}

func runDeaconRestart(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	sessionName := getDeaconSessionName()

	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	fmt.Println("Restarting Deacon...")

	if running {
		// Kill existing session
		if err := t.KillSession(sessionName); err != nil {
			style.PrintWarning("failed to kill session: %v", err)
		}
	}

	// Start fresh
	if err := runDeaconStart(cmd, args); err != nil {
		return err
	}

	fmt.Printf("%s Deacon restarted\n", style.Bold.Render("✓"))
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt deacon attach' to connect"))
	return nil
}

func runDeaconHeartbeat(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	action := ""
	if len(args) > 0 {
		action = strings.Join(args, " ")
	}

	if action != "" {
		if err := deacon.TouchWithAction(townRoot, action, 0, 0); err != nil {
			return fmt.Errorf("updating heartbeat: %w", err)
		}
		fmt.Printf("%s Heartbeat updated: %s\n", style.Bold.Render("✓"), action)
	} else {
		if err := deacon.Touch(townRoot); err != nil {
			return fmt.Errorf("updating heartbeat: %w", err)
		}
		fmt.Printf("%s Heartbeat updated\n", style.Bold.Render("✓"))
	}

	return nil
}

func runDeaconTriggerPending(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Step 1: Check inbox for new POLECAT_STARTED messages
	pending, err := polecat.CheckInboxForSpawns(townRoot)
	if err != nil {
		return fmt.Errorf("checking inbox: %w", err)
	}

	if len(pending) == 0 {
		fmt.Printf("%s No pending spawns\n", style.Dim.Render("○"))
		return nil
	}

	fmt.Printf("%s Found %d pending spawn(s)\n", style.Bold.Render("●"), len(pending))

	// Step 2: Try to trigger each pending spawn
	results, err := polecat.TriggerPendingSpawns(townRoot, triggerTimeout)
	if err != nil {
		return fmt.Errorf("triggering: %w", err)
	}

	// Report results
	triggered := 0
	for _, r := range results {
		if r.Triggered {
			triggered++
			fmt.Printf("  %s Triggered %s/%s\n",
				style.Bold.Render("✓"),
				r.Spawn.Rig, r.Spawn.Polecat)
		} else if r.Error != nil {
			fmt.Printf("  %s %s/%s: %v\n",
				style.Dim.Render("⚠"),
				r.Spawn.Rig, r.Spawn.Polecat, r.Error)
		}
	}

	// Step 3: Prune stale pending spawns (older than 5 minutes)
	pruned, _ := polecat.PruneStalePending(townRoot, 5*time.Minute)
	if pruned > 0 {
		fmt.Printf("  %s Pruned %d stale spawn(s)\n", style.Dim.Render("○"), pruned)
	}

	// Summary
	remaining := len(pending) - triggered
	if remaining > 0 {
		fmt.Printf("%s %d spawn(s) still waiting for Claude\n",
			style.Dim.Render("○"), remaining)
	}

	return nil
}

// ensurePatrolHooks creates .claude/settings.json with hooks for patrol roles.
// This is idempotent - if hooks already exist, it does nothing.
func ensurePatrolHooks(workspacePath string) error {
	settingsPath := filepath.Join(workspacePath, ".claude", "settings.json")

	// Check if already exists
	if _, err := os.Stat(settingsPath); err == nil {
		return nil // Already exists
	}

	claudeDir := filepath.Join(workspacePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	// Standard patrol hooks
	// Note: SessionStart nudges Deacon for GUPP backstop (agent wake notification)
	hooksJSON := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt prime && gt mail check --inject && gt nudge deacon session-started"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt prime"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt mail check --inject"
          }
        ]
      }
    ]
  }
}
`
	return os.WriteFile(settingsPath, []byte(hooksJSON), 0600)
}

// runDeaconHealthCheck implements the health-check command.
// It sends a HEALTH_CHECK nudge to an agent, waits for response, and tracks state.
func runDeaconHealthCheck(cmd *cobra.Command, args []string) error {
	agent := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load health check state
	state, err := deacon.LoadHealthCheckState(townRoot)
	if err != nil {
		return fmt.Errorf("loading health check state: %w", err)
	}
	agentState := state.GetAgentState(agent)

	// Check if agent is in cooldown
	if agentState.IsInCooldown(healthCheckCooldown) {
		remaining := agentState.CooldownRemaining(healthCheckCooldown)
		fmt.Printf("%s Agent %s is in cooldown (remaining: %s)\n",
			style.Dim.Render("○"), agent, remaining.Round(time.Second))
		return nil
	}

	// Get agent bead info before ping (for baseline)
	beadID, sessionName, err := agentAddressToIDs(agent)
	if err != nil {
		return fmt.Errorf("invalid agent address: %w", err)
	}

	t := tmux.NewTmux()

	// Check if session exists
	exists, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		fmt.Printf("%s Agent %s session not running\n", style.Dim.Render("○"), agent)
		return nil
	}

	// Get current bead update time
	baselineTime, err := getAgentBeadUpdateTime(townRoot, beadID)
	if err != nil {
		// Bead might not exist yet - that's okay
		baselineTime = time.Time{}
	}

	// Record ping
	agentState.RecordPing()

	// Send health check nudge
	if err := t.NudgeSession(sessionName, "HEALTH_CHECK: respond with any action to confirm responsiveness"); err != nil {
		return fmt.Errorf("sending nudge: %w", err)
	}

	fmt.Printf("%s Sent HEALTH_CHECK to %s, waiting %s...\n",
		style.Bold.Render("→"), agent, healthCheckTimeout)

	// Wait for response
	deadline := time.Now().Add(healthCheckTimeout)
	responded := false

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second) // Check every 2 seconds

		newTime, err := getAgentBeadUpdateTime(townRoot, beadID)
		if err != nil {
			continue
		}

		// If bead was updated after our baseline, agent responded
		if newTime.After(baselineTime) {
			responded = true
			break
		}
	}

	// Record result
	if responded {
		agentState.RecordResponse()
		if err := deacon.SaveHealthCheckState(townRoot, state); err != nil {
			style.PrintWarning("failed to save health check state: %v", err)
		}
		fmt.Printf("%s Agent %s responded (failures reset to 0)\n",
			style.Bold.Render("✓"), agent)
		return nil
	}

	// No response - record failure
	agentState.RecordFailure()
	if err := deacon.SaveHealthCheckState(townRoot, state); err != nil {
		style.PrintWarning("failed to save health check state: %v", err)
	}

	fmt.Printf("%s Agent %s did not respond (consecutive failures: %d/%d)\n",
		style.Dim.Render("⚠"), agent, agentState.ConsecutiveFailures, healthCheckFailures)

	// Check if force-kill threshold reached
	if agentState.ShouldForceKill(healthCheckFailures) {
		fmt.Printf("%s Agent %s should be force-killed\n", style.Bold.Render("✗"), agent)
		os.Exit(2) // Exit code 2 = should force-kill
	}

	return nil
}

// runDeaconForceKill implements the force-kill command.
// It kills a stuck agent session and updates its bead state.
func runDeaconForceKill(cmd *cobra.Command, args []string) error {
	agent := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load health check state
	state, err := deacon.LoadHealthCheckState(townRoot)
	if err != nil {
		return fmt.Errorf("loading health check state: %w", err)
	}
	agentState := state.GetAgentState(agent)

	// Check cooldown (unless bypassed)
	if agentState.IsInCooldown(healthCheckCooldown) {
		remaining := agentState.CooldownRemaining(healthCheckCooldown)
		return fmt.Errorf("agent %s is in cooldown (remaining: %s) - cannot force-kill yet",
			agent, remaining.Round(time.Second))
	}

	// Get session name
	_, sessionName, err := agentAddressToIDs(agent)
	if err != nil {
		return fmt.Errorf("invalid agent address: %w", err)
	}

	t := tmux.NewTmux()

	// Check if session exists
	exists, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		fmt.Printf("%s Agent %s session not running\n", style.Dim.Render("○"), agent)
		return nil
	}

	// Build reason
	reason := forceKillReason
	if reason == "" {
		reason = fmt.Sprintf("unresponsive after %d consecutive health check failures",
			agentState.ConsecutiveFailures)
	}

	// Step 1: Log the intervention (send mail to agent)
	fmt.Printf("%s Sending force-kill notification to %s...\n", style.Dim.Render("1."), agent)
	mailBody := fmt.Sprintf("Deacon detected %s as unresponsive.\nReason: %s\nAction: force-killing session", agent, reason)
	sendMail(townRoot, agent, "FORCE_KILL: unresponsive", mailBody)

	// Step 2: Kill the tmux session
	fmt.Printf("%s Killing tmux session %s...\n", style.Dim.Render("2."), sessionName)
	if err := t.KillSession(sessionName); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	// Step 3: Update agent bead state (optional - best effort)
	fmt.Printf("%s Updating agent bead state to 'killed'...\n", style.Dim.Render("3."))
	updateAgentBeadState(townRoot, agent, "killed", reason)

	// Step 4: Notify mayor (optional)
	if !forceKillSkipNotify {
		fmt.Printf("%s Notifying mayor...\n", style.Dim.Render("4."))
		notifyBody := fmt.Sprintf("Agent %s was force-killed by Deacon.\nReason: %s", agent, reason)
		sendMail(townRoot, "mayor/", "Agent killed: "+agent, notifyBody)
	}

	// Record force-kill in state
	agentState.RecordForceKill()
	if err := deacon.SaveHealthCheckState(townRoot, state); err != nil {
		style.PrintWarning("failed to save health check state: %v", err)
	}

	fmt.Printf("%s Force-killed agent %s (total kills: %d)\n",
		style.Bold.Render("✓"), agent, agentState.ForceKillCount)
	fmt.Printf("  %s\n", style.Dim.Render("Agent is now 'asleep'. Use 'gt rig boot' to restart."))

	return nil
}

// runDeaconHealthState shows the current health check state.
func runDeaconHealthState(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	state, err := deacon.LoadHealthCheckState(townRoot)
	if err != nil {
		return fmt.Errorf("loading health check state: %w", err)
	}

	if len(state.Agents) == 0 {
		fmt.Printf("%s No health check state recorded yet\n", style.Dim.Render("○"))
		return nil
	}

	fmt.Printf("%s Health Check State (updated %s)\n\n",
		style.Bold.Render("●"),
		state.LastUpdated.Format(time.RFC3339))

	for agentID, agentState := range state.Agents {
		fmt.Printf("Agent: %s\n", style.Bold.Render(agentID))

		if !agentState.LastPingTime.IsZero() {
			fmt.Printf("  Last ping: %s ago\n", time.Since(agentState.LastPingTime).Round(time.Second))
		}
		if !agentState.LastResponseTime.IsZero() {
			fmt.Printf("  Last response: %s ago\n", time.Since(agentState.LastResponseTime).Round(time.Second))
		}

		fmt.Printf("  Consecutive failures: %d\n", agentState.ConsecutiveFailures)
		fmt.Printf("  Total force-kills: %d\n", agentState.ForceKillCount)

		if !agentState.LastForceKillTime.IsZero() {
			fmt.Printf("  Last force-kill: %s ago\n", time.Since(agentState.LastForceKillTime).Round(time.Second))
			if agentState.IsInCooldown(healthCheckCooldown) {
				remaining := agentState.CooldownRemaining(healthCheckCooldown)
				fmt.Printf("  Cooldown: %s remaining\n", remaining.Round(time.Second))
			}
		}
		fmt.Println()
	}

	return nil
}

// runDeaconZombieScan scans for idle polecats that should have been nuked by the Witness.
// This is a defense-in-depth backup check.
func runDeaconZombieScan(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()

	// Get list of rigs to scan
	var rigsToScan []string
	if len(args) > 0 {
		rigsToScan = []string{args[0]}
	} else {
		// Scan all rigs by finding directories with polecats/ subdirectories
		entries, err := os.ReadDir(townRoot)
		if err != nil {
			return fmt.Errorf("reading town root: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// Skip non-rig directories
			if entry.Name() == "deacon" || entry.Name() == "mayor" ||
				entry.Name() == "plugins" || entry.Name() == "docs" ||
				strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			// Check if it has a polecats directory
			polecatsDir := filepath.Join(townRoot, entry.Name(), "polecats")
			if info, err := os.Stat(polecatsDir); err == nil && info.IsDir() {
				rigsToScan = append(rigsToScan, entry.Name())
			}
		}
	}

	if len(rigsToScan) == 0 {
		fmt.Printf("%s No rigs found to scan\n", style.Dim.Render("○"))
		return nil
	}

	fmt.Printf("%s Scanning for zombie polecats (threshold: %s)...\n",
		style.Bold.Render("🧟"), zombieScanThreshold)

	var zombies []zombieInfo
	for _, rigName := range rigsToScan {
		rigZombies, err := scanRigForZombies(townRoot, rigName, t)
		if err != nil {
			style.PrintWarning("failed to scan rig %s: %v", rigName, err)
			continue
		}
		zombies = append(zombies, rigZombies...)
	}

	if len(zombies) == 0 {
		fmt.Printf("%s No zombies found (all polecats healthy)\n", style.Bold.Render("✓"))
		return nil
	}

	// Report zombies
	fmt.Printf("\n%s Found %d zombie(s):\n\n", style.Bold.Render("⚠"), len(zombies))
	for _, z := range zombies {
		fmt.Printf("  %s %s/%s\n", style.Dim.Render("🧟"), z.rig, z.name)
		fmt.Printf("    State: %s, Session: %s\n", z.state, z.sessionStatus)
		fmt.Printf("    Hooked work: %s\n", z.hookedWork)
		fmt.Printf("    Last activity: %s ago\n", z.staleness.Round(time.Second))
		fmt.Printf("    Reason: %s\n", z.reason)
		fmt.Println()
	}

	// Nuke zombies if enabled
	if zombieScanNuke && !zombieScanDryRun {
		fmt.Printf("%s Nuking zombies...\n", style.Bold.Render("💀"))
		for _, z := range zombies {
			if err := nukeZombie(townRoot, z, t); err != nil {
				style.PrintWarning("failed to nuke %s/%s: %v", z.rig, z.name, err)
			} else {
				fmt.Printf("  %s Nuked %s/%s\n", style.Bold.Render("✓"), z.rig, z.name)
			}
		}

		// Notify mayor about witness failure
		notifyMayorOfWitnessFailure(townRoot, zombies)
	} else if zombieScanDryRun {
		fmt.Printf("%s Dry run - would nuke %d zombie(s)\n", style.Dim.Render("ℹ"), len(zombies))
	}

	return nil
}

// zombieInfo holds information about a detected zombie polecat.
type zombieInfo struct {
	rig           string
	name          string
	state         string
	sessionStatus string
	hookedWork    string
	staleness     time.Duration
	reason        string
	sessionName   string
}

// scanRigForZombies scans a rig for zombie polecats.
func scanRigForZombies(townRoot, rigName string, t *tmux.Tmux) ([]zombieInfo, error) {
	rigPath := filepath.Join(townRoot, rigName)
	polecatsDir := filepath.Join(rigPath, "polecats")

	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No polecats dir
		}
		return nil, err
	}

	var zombies []zombieInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Build session name for this polecat
		sessionName := fmt.Sprintf("gt-%s-%s", rigName, name)

		// Check if session is running
		sessionRunning, _ := t.HasSession(sessionName)

		// Check for hooked work
		hookedWork := checkPolecatHookedWork(townRoot, rigName, name)

		// Get last activity time from polecat directory
		polecatPath := filepath.Join(polecatsDir, name)
		staleness := getPolecatStaleness(polecatPath)

		// Determine if this is a zombie
		state := "unknown"
		if sessionRunning {
			state = "session_running"
			continue // Not a zombie if session is running
		}
		state = "session_dead"

		// Check all zombie criteria
		if hookedWork != "" {
			// Has hooked work - not a zombie (just needs to be started)
			continue
		}

		if staleness < zombieScanThreshold {
			// Recently active - not stale enough
			continue
		}

		// This is a zombie
		zombies = append(zombies, zombieInfo{
			rig:           rigName,
			name:          name,
			state:         state,
			sessionStatus: "not running",
			hookedWork:    "none",
			staleness:     staleness,
			reason:        fmt.Sprintf("idle for %s with no session or hooked work", staleness.Round(time.Minute)),
			sessionName:   sessionName,
		})
	}

	return zombies, nil
}

// checkPolecatHookedWork checks if a polecat has hooked work.
func checkPolecatHookedWork(townRoot, rigName, polecatName string) string {
	// Query beads for hooked issues assigned to this polecat
	assignee := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	cmd := exec.Command("bd", "list", "--status=hooked", "--assignee="+assignee, "--json")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var issues []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(output, &issues); err != nil || len(issues) == 0 {
		return ""
	}

	return issues[0].ID
}

// getPolecatStaleness returns how long since the polecat was last active.
func getPolecatStaleness(polecatPath string) time.Duration {
	// Check .beads/last-touched if it exists
	lastTouchedPath := filepath.Join(polecatPath, ".beads", "last-touched")
	if info, err := os.Stat(lastTouchedPath); err == nil {
		return time.Since(info.ModTime())
	}

	// Fall back to directory modification time
	if info, err := os.Stat(polecatPath); err == nil {
		return time.Since(info.ModTime())
	}

	// Very stale if we can't determine
	return 24 * time.Hour
}

// nukeZombie cleans up a zombie polecat.
func nukeZombie(townRoot string, z zombieInfo, t *tmux.Tmux) error { //nolint:unparam // error return kept for future use
	// Step 1: Kill tmux session if somehow still exists
	if exists, _ := t.HasSession(z.sessionName); exists {
		_ = t.KillSession(z.sessionName)
	}

	// Step 2: Run gt polecat nuke to clean up
	cmd := exec.Command("gt", "polecat", "nuke", z.name, "--rig="+z.rig, "--force")
	cmd.Dir = townRoot
	if err := cmd.Run(); err != nil {
		// Non-fatal - polecat might already be cleaned up
		style.PrintWarning("polecat nuke returned error (may be already cleaned): %v", err)
	}

	return nil
}

// notifyMayorOfWitnessFailure notifies the mayor about witness cleanup failures.
func notifyMayorOfWitnessFailure(townRoot string, zombies []zombieInfo) {
	if len(zombies) == 0 {
		return
	}

	// Group by rig
	rigCounts := make(map[string]int)
	for _, z := range zombies {
		rigCounts[z.rig]++
	}

	var details strings.Builder
	details.WriteString("Deacon detected zombie polecats that Witness should have cleaned:\n\n")
	for rig, count := range rigCounts {
		details.WriteString(fmt.Sprintf("- %s: %d zombie(s)\n", rig, count))
	}
	details.WriteString("\nDeacon has nuked them directly. Check Witness health.")

	sendMail(townRoot, "mayor/", "⚠️ Witness cleanup failure detected", details.String())
}

// agentAddressToIDs converts an agent address to bead ID and session name.
// Supports formats: "gastown/polecats/max", "gastown/witness", "deacon", "mayor"
// Note: Town-level agents (Mayor, Deacon) use hq- prefix bead IDs stored in town beads.
func agentAddressToIDs(address string) (beadID, sessionName string, err error) {
	switch address {
	case "deacon":
		return beads.DeaconBeadIDTown(), session.DeaconSessionName(), nil
	case "mayor":
		return beads.MayorBeadIDTown(), session.MayorSessionName(), nil
	}

	parts := strings.Split(address, "/")
	switch len(parts) {
	case 2:
		// rig/role: "gastown/witness", "gastown/refinery"
		rig, role := parts[0], parts[1]
		switch role {
		case "witness":
			return fmt.Sprintf("gt-%s-witness", rig), fmt.Sprintf("gt-%s-witness", rig), nil
		case "refinery":
			return fmt.Sprintf("gt-%s-refinery", rig), fmt.Sprintf("gt-%s-refinery", rig), nil
		default:
			return "", "", fmt.Errorf("unknown role: %s", role)
		}
	case 3:
		// rig/type/name: "gastown/polecats/max", "gastown/crew/alpha"
		rig, agentType, name := parts[0], parts[1], parts[2]
		switch agentType {
		case "polecats":
			return fmt.Sprintf("gt-%s-polecat-%s", rig, name), fmt.Sprintf("gt-%s-%s", rig, name), nil
		case "crew":
			return fmt.Sprintf("gt-%s-crew-%s", rig, name), fmt.Sprintf("gt-%s-crew-%s", rig, name), nil
		default:
			return "", "", fmt.Errorf("unknown agent type: %s", agentType)
		}
	default:
		return "", "", fmt.Errorf("invalid agent address format: %s (expected rig/type/name or rig/role)", address)
	}
}

// getAgentBeadUpdateTime gets the update time from an agent bead.
func getAgentBeadUpdateTime(townRoot, beadID string) (time.Time, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}

	var issues []struct {
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return time.Time{}, err
	}

	if len(issues) == 0 {
		return time.Time{}, fmt.Errorf("bead not found: %s", beadID)
	}

	return time.Parse(time.RFC3339, issues[0].UpdatedAt)
}

// sendMail sends a mail message using gt mail send.
func sendMail(townRoot, to, subject, body string) {
	cmd := exec.Command("gt", "mail", "send", to, "-s", subject, "-m", body)
	cmd.Dir = townRoot
	_ = cmd.Run() // Best effort
}

// updateAgentBeadState updates an agent bead's state.
func updateAgentBeadState(townRoot, agent, state, _ string) { // reason unused but kept for API consistency
	beadID, _, err := agentAddressToIDs(agent)
	if err != nil {
		return
	}

	// Use bd agent state command
	cmd := exec.Command("bd", "agent", "state", beadID, state)
	cmd.Dir = townRoot
	_ = cmd.Run() // Best effort
}

