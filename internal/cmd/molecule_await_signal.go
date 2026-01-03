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

	// Read current idle cycles from agent bead (if specified)
	var idleCycles int
	if awaitSignalAgentBead != "" {
		labels, err := getAgentLabels(awaitSignalAgentBead, beadsDir)
		if err != nil {
			// Agent bead might not exist yet - that's OK, start at 0
			if !awaitSignalQuiet {
				fmt.Printf("%s Could not read agent bead (starting at idle=0): %v\n",
					style.Dim.Render("⚠"), err)
			}
		} else if idleStr, ok := labels["idle"]; ok {
			if n, err := parseIntSimple(idleStr); err == nil {
				idleCycles = n
			}
		}
	}

	// Calculate effective timeout (uses idle cycles if backoff mode)
	timeout, err := calculateEffectiveTimeout(idleCycles)
	if err != nil {
		return fmt.Errorf("invalid timeout configuration: %w", err)
	}

	if !awaitSignalQuiet && !moleculeJSON {
		if awaitSignalAgentBead != "" {
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

	// On timeout, increment idle cycles on agent bead
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
	} else if result.Reason == "signal" && awaitSignalAgentBead != "" {
		// On signal, report current idle cycles (caller should reset)
		result.IdleCycles = idleCycles
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

// GetCurrentStepBackoff retrieves backoff config from the current step.
// This is used by patrol agents to get the timeout for await-signal.
func GetCurrentStepBackoff(workDir string) (*beads.BackoffConfig, error) {
	b := beads.New(workDir)

	// Get current agent's hook
	// This would need to query the pinned/hooked bead and parse its description
	// for backoff configuration. For now, return nil (use defaults).
	_ = b

	return nil, nil
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
