package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	awaitSignalTimeout     string
	awaitSignalBackoffBase string
	awaitSignalBackoffMult int
	awaitSignalBackoffMax  string
	awaitSignalQuiet       bool
	awaitSignalAgentBead   string
)

var moleculeAwaitSignalCmd = &cobra.Command{
	Use:   "await-signal",
	Short: "Wait for activity feed signal with timeout",
	Long: `Wait for any activity on the beads feed, with optional backoff.

This command is the primary wake mechanism for patrol agents. It subscribes
to 'bd activity --follow' and returns immediately when any line of output
is received (indicating beads activity).

If no activity occurs within the timeout, the command returns with exit code 0
but sets the AWAIT_SIGNAL_REASON environment variable to "timeout".

The timeout can be specified directly or via backoff configuration for
exponential wait patterns.

BACKOFF MODE:
When backoff parameters are provided, the effective timeout is calculated as:
  min(base * multiplier^idle_cycles, max)

The idle_cycles value is read from the agent bead's "idle" label, enabling
exponential backoff that persists across invocations. When a signal is
received, the caller should reset idle:0 on the agent bead.

EXIT CODES:
  0 - Signal received or timeout (check output for which)
  1 - Error starting feed subscription

EXAMPLES:
  # Simple wait with 60s timeout
  gt mol await-signal --timeout 60s

  # Backoff mode with agent bead tracking:
  gt mol await-signal --agent-bead gt-gastown-witness \
    --backoff-base 30s --backoff-mult 2 --backoff-max 5m

  # On timeout, the agent bead's idle:N label is auto-incremented
  # On signal, caller should reset: gt agent state gt-gastown-witness --set idle=0

  # Quiet mode (no output, for scripting)
  gt mol await-signal --timeout 30s --quiet`,
	RunE: runMoleculeAwaitSignal,
}

// AwaitSignalResult is the result of an await-signal operation.
type AwaitSignalResult struct {
	Reason     string        `json:"reason"`               // "signal" or "timeout"
	Elapsed    time.Duration `json:"elapsed"`              // how long we waited
	Signal     string        `json:"signal,omitempty"`     // the line that woke us (if signal)
	IdleCycles int           `json:"idle_cycles,omitempty"` // current idle cycle count (after update)
}

func init() {
	moleculeAwaitSignalCmd.Flags().StringVar(&awaitSignalTimeout, "timeout", "60s",
		"Maximum time to wait for signal (e.g., 30s, 5m)")
	moleculeAwaitSignalCmd.Flags().StringVar(&awaitSignalBackoffBase, "backoff-base", "",
		"Base interval for exponential backoff (e.g., 30s)")
	moleculeAwaitSignalCmd.Flags().IntVar(&awaitSignalBackoffMult, "backoff-mult", 2,
		"Multiplier for exponential backoff (default: 2)")
	moleculeAwaitSignalCmd.Flags().StringVar(&awaitSignalBackoffMax, "backoff-max", "",
		"Maximum interval cap for backoff (e.g., 10m)")
	moleculeAwaitSignalCmd.Flags().StringVar(&awaitSignalAgentBead, "agent-bead", "",
		"Agent bead ID for tracking idle cycles (reads/writes idle:N label)")
	moleculeAwaitSignalCmd.Flags().BoolVar(&awaitSignalQuiet, "quiet", false,
		"Suppress output (for scripting)")
	moleculeAwaitSignalCmd.Flags().BoolVar(&moleculeJSON, "json", false,
		"Output as JSON")

	moleculeStepCmd.AddCommand(moleculeAwaitSignalCmd)
}

func runMoleculeAwaitSignal(cmd *cobra.Command, args []string) error {
	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	beadsDir := beads.ResolveBeadsDir(workDir)

	// Read current idle cycles and backoff window from agent bead (if specified)
	var idleCycles int
	var backoffUntil time.Time // zero value means no active window
	if awaitSignalAgentBead != "" {
		labels, err := getAgentLabels(awaitSignalAgentBead, beadsDir)
		if err != nil {
			// Agent bead might not exist yet - that's OK, start at 0
			if !awaitSignalQuiet {
				fmt.Printf("%s Could not read agent bead (starting at idle=0): %v\n",
					style.Dim.Render("⚠"), err)
			}
		} else {
			if idleStr, ok := labels["idle"]; ok {
				if n, err := parseIntSimple(idleStr); err == nil {
					idleCycles = n
				}
			}
			if untilStr, ok := labels["backoff-until"]; ok {
				if ts, err := parseIntSimple(untilStr); err == nil && ts > 0 {
					backoffUntil = time.Unix(int64(ts), 0)
				}
			}
		}
	}

	// Calculate full timeout from backoff formula (uses idle cycles)
	fullTimeout, err := calculateEffectiveTimeout(idleCycles)
	if err != nil {
		return fmt.Errorf("invalid timeout configuration: %w", err)
	}

	// Determine effective timeout: resume from persisted window or start fresh.
	// This makes backoff resilient to interrupts (e.g., nudges that kill the
	// running await-signal). If the process is interrupted and relaunched within
	// the same backoff window, it sleeps only for the remaining time.
	timeout := fullTimeout
	resumed := false
	now := time.Now()
	if awaitSignalAgentBead != "" && !backoffUntil.IsZero() && backoffUntil.After(now) {
		remaining := backoffUntil.Sub(now)
		// Sanity: remaining should not exceed the calculated full timeout.
		// If idle:N was reset externally, the stored window may be stale.
		if remaining <= fullTimeout {
			timeout = remaining
			resumed = true
		}
	}

	// Persist the backoff window end time so interrupted invocations can resume.
	if awaitSignalAgentBead != "" && !resumed {
		windowEnd := now.Add(timeout)
		if err := setAgentBackoffUntil(awaitSignalAgentBead, beadsDir, windowEnd); err != nil {
			if !awaitSignalQuiet {
				fmt.Printf("%s Failed to persist backoff window: %v\n",
					style.Dim.Render("⚠"), err)
			}
		}
	}

	if !awaitSignalQuiet && !moleculeJSON {
		if resumed {
			fmt.Printf("%s Resuming backoff (remaining: %v, idle: %d)...\n",
				style.Dim.Render("⏳"), timeout.Round(time.Second), idleCycles)
		} else if awaitSignalAgentBead != "" {
			fmt.Printf("%s Awaiting signal (timeout: %v, idle: %d)...\n",
				style.Dim.Render("⏳"), timeout, idleCycles)
		} else {
			fmt.Printf("%s Awaiting signal (timeout: %v)...\n",
				style.Dim.Render("⏳"), timeout)
		}
	}

	startTime := time.Now()

	// Start bd activity --follow
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := waitForActivitySignal(ctx, workDir)
	if err != nil {
		return fmt.Errorf("feed subscription failed: %w", err)
	}

	result.Elapsed = time.Since(startTime)

	// On timeout, increment idle cycles and clear backoff window
	if result.Reason == "timeout" && awaitSignalAgentBead != "" {
		newIdleCycles := idleCycles + 1
		if err := setAgentIdleCycles(awaitSignalAgentBead, beadsDir, newIdleCycles); err != nil {
			if !awaitSignalQuiet {
				fmt.Printf("%s Failed to update agent bead idle count: %v\n",
					style.Dim.Render("⚠"), err)
			}
		} else {
			result.IdleCycles = newIdleCycles
		}
		// Clear the backoff window — timeout completed normally
		_ = clearAgentBackoffUntil(awaitSignalAgentBead, beadsDir)
	} else if result.Reason == "signal" && awaitSignalAgentBead != "" {
		// On signal, update last_activity to prove agent is alive
		if err := updateAgentHeartbeat(awaitSignalAgentBead, beadsDir); err != nil {
			if !awaitSignalQuiet {
				fmt.Printf("%s Failed to update agent heartbeat: %v\n",
					style.Dim.Render("⚠"), err)
			}
		}
		// Report current idle cycles (caller should reset)
		result.IdleCycles = idleCycles
		// Clear the backoff window — woken by real activity
		_ = clearAgentBackoffUntil(awaitSignalAgentBead, beadsDir)
	}

	// Output result
	if moleculeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if !awaitSignalQuiet {
		switch result.Reason {
		case "signal":
			fmt.Printf("%s Signal received after %v\n",
				style.Bold.Render("✓"), result.Elapsed.Round(time.Millisecond))
			if result.Signal != "" {
				// Truncate long signals
				sig := result.Signal
				if len(sig) > 80 {
					sig = sig[:77] + "..."
				}
				fmt.Printf("  %s\n", style.Dim.Render(sig))
			}
		case "timeout":
			if awaitSignalAgentBead != "" {
				fmt.Printf("%s Timeout after %v (idle cycle: %d)\n",
					style.Dim.Render("⏱"), result.Elapsed.Round(time.Millisecond), result.IdleCycles)
			} else {
				fmt.Printf("%s Timeout after %v (no activity)\n",
					style.Dim.Render("⏱"), result.Elapsed.Round(time.Millisecond))
			}
		}
	}

	return nil
}

// calculateEffectiveTimeout determines the timeout based on flags.
// If backoff parameters are provided, uses exponential backoff formula:
//   min(base * multiplier^idleCycles, max)
// Otherwise uses the simple --timeout value.
func calculateEffectiveTimeout(idleCycles int) (time.Duration, error) {
	// If backoff base is set, use backoff mode
	if awaitSignalBackoffBase != "" {
		base, err := time.ParseDuration(awaitSignalBackoffBase)
		if err != nil {
			return 0, fmt.Errorf("invalid backoff-base: %w", err)
		}

		// Apply exponential backoff: base * multiplier^idleCycles
		timeout := base
		for i := 0; i < idleCycles; i++ {
			timeout *= time.Duration(awaitSignalBackoffMult)
		}

		// Apply max cap if specified
		if awaitSignalBackoffMax != "" {
			maxDur, err := time.ParseDuration(awaitSignalBackoffMax)
			if err != nil {
				return 0, fmt.Errorf("invalid backoff-max: %w", err)
			}
			if timeout > maxDur {
				timeout = maxDur
			}
		}

		return timeout, nil
	}

	// Simple timeout mode
	return time.ParseDuration(awaitSignalTimeout)
}

// waitForActivitySignal starts bd activity --follow and waits for any output.
// Returns immediately when a line is received, or when context is canceled.
func waitForActivitySignal(ctx context.Context, workDir string) (*AwaitSignalResult, error) {
	// Start bd activity --follow
	cmd := exec.CommandContext(ctx, "bd", "activity", "--follow")
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting bd activity: %w", err)
	}

	// Channel for results
	signalCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Read lines in goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			// Got a line - this is our signal
			signalCh <- scanner.Text()
		} else if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	// Wait for signal, error, or timeout
	select {
	case signal := <-signalCh:
		// Got activity signal - kill the process and return
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return &AwaitSignalResult{
			Reason: "signal",
			Signal: signal,
		}, nil

	case err := <-errCh:
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("reading from feed: %w", err)

	case <-ctx.Done():
		// Timeout - kill process and return timeout result
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return &AwaitSignalResult{
			Reason: "timeout",
		}, nil
	}
}

// parseIntSimple parses a string to int without using strconv.
func parseIntSimple(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, nil
}

// updateAgentHeartbeat updates the last_activity timestamp on an agent bead.
// This proves the agent is alive and processing signals.
func updateAgentHeartbeat(agentBead, beadsDir string) error {
	cmd := exec.Command("bd", "agent", "heartbeat", agentBead)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	return cmd.Run()
}

// setAgentIdleCycles sets the idle:N label on an agent bead.
// Uses read-modify-write pattern to update only the idle label.
func setAgentIdleCycles(agentBead, beadsDir string, cycles int) error {
	// Read all current labels
	allLabels, err := getAllAgentLabels(agentBead, beadsDir)
	if err != nil {
		return err
	}

	// Build new label list: keep non-idle labels, add new idle value
	var newLabels []string
	for _, label := range allLabels {
		// Skip any existing idle:* label
		if len(label) > 5 && label[:5] == "idle:" {
			continue
		}
		newLabels = append(newLabels, label)
	}

	// Add new idle value
	newLabels = append(newLabels, fmt.Sprintf("idle:%d", cycles))

	// Use bd update with --set-labels to replace all labels
	args := []string{"update", agentBead}
	for _, label := range newLabels {
		args = append(args, "--set-labels="+label)
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setting idle label: %w", err)
	}

	return nil
}

// setAgentBackoffUntil persists a backoff-until:TIMESTAMP label on the agent bead.
// This allows interrupted await-signal invocations to resume with remaining time
// instead of restarting the full backoff period.
func setAgentBackoffUntil(agentBead, beadsDir string, until time.Time) error {
	allLabels, err := getAllAgentLabels(agentBead, beadsDir)
	if err != nil {
		return err
	}

	var newLabels []string
	for _, label := range allLabels {
		if len(label) > 14 && label[:14] == "backoff-until:" {
			continue // Strip existing backoff-until
		}
		newLabels = append(newLabels, label)
	}
	newLabels = append(newLabels, fmt.Sprintf("backoff-until:%d", until.Unix()))

	args := []string{"update", agentBead}
	for _, label := range newLabels {
		args = append(args, "--set-labels="+label)
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setting backoff-until label: %w", err)
	}
	return nil
}

// clearAgentBackoffUntil removes the backoff-until label from the agent bead.
// Called when await-signal completes normally (timeout or signal received).
func clearAgentBackoffUntil(agentBead, beadsDir string) error {
	allLabels, err := getAllAgentLabels(agentBead, beadsDir)
	if err != nil {
		return err
	}

	var newLabels []string
	found := false
	for _, label := range allLabels {
		if len(label) > 14 && label[:14] == "backoff-until:" {
			found = true
			continue // Strip backoff-until
		}
		newLabels = append(newLabels, label)
	}

	if !found {
		return nil // Nothing to clear
	}

	args := []string{"update", agentBead}
	if len(newLabels) == 0 {
		args = append(args, "--set-labels=")
	} else {
		for _, label := range newLabels {
			args = append(args, "--set-labels="+label)
		}
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clearing backoff-until label: %w", err)
	}
	return nil
}
