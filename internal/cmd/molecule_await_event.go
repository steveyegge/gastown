package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	awaitEventChannel     string
	awaitEventTimeout     string
	awaitEventBackoffBase string
	awaitEventBackoffMult int
	awaitEventBackoffMax  string
	awaitEventQuiet       bool
	awaitEventAgentBead   string
	awaitEventCleanup     bool
)

var moleculeAwaitEventCmd = &cobra.Command{
	Use:   "await-event",
	Short: "Wait for a file-based event on a named channel",
	Long: `Wait for event files to appear in ~/gt/events/<channel>/, with optional backoff.

Unlike await-signal (which subscribes to the generic beads activity feed),
await-event watches a dedicated event channel directory using inotifywait.
Events are only received when explicitly emitted via emit-event.sh.

Use this for role-specific subscriptions where the generic beads feed would
cause too many false wakes. For example, the refinery only cares about
MERGE_READY events, not all beads activity.

EVENT FORMAT:
Events are JSON files in ~/gt/events/<channel>/*.event, created by
~/gt/scripts/emit-event.sh. Each file contains:
  {"type": "...", "channel": "...", "timestamp": "...", "payload": {...}}

BEHAVIOR:
1. Check for already-pending events (return immediately if found)
2. If none, block via inotifywait until a new .event file appears
3. On wake, return all pending event file paths and contents
4. With --cleanup, delete processed event files automatically

BACKOFF MODE:
Same as await-signal: base * multiplier^idle_cycles, capped at max.
Idle cycles tracked on agent bead labels.

EXIT CODES:
  0 - Event(s) found or timeout
  1 - Error

EXAMPLES:
  # Wait for refinery events with 10min timeout
  gt mol step await-event --channel refinery --timeout 10m

  # Backoff mode with agent bead tracking
  gt mol step await-event --channel refinery --agent-bead VAS-refinery \
    --backoff-base 60s --backoff-mult 2 --backoff-max 10m

  # Auto-cleanup processed events
  gt mol step await-event --channel refinery --cleanup`,
	RunE: runMoleculeAwaitEvent,
}

// AwaitEventResult is the result of an await-event operation.
type AwaitEventResult struct {
	Reason     string        `json:"reason"`                // "event" or "timeout"
	Elapsed    time.Duration `json:"elapsed"`               // how long we waited
	Events     []EventFile   `json:"events,omitempty"`      // event files found
	IdleCycles int           `json:"idle_cycles,omitempty"` // current idle cycle count
}

// EventFile represents a single event file.
type EventFile struct {
	Path    string          `json:"path"`
	Content json.RawMessage `json:"content"`
}

func init() {
	moleculeAwaitEventCmd.Flags().StringVar(&awaitEventChannel, "channel", "",
		"Event channel name (required, e.g., 'refinery')")
	moleculeAwaitEventCmd.Flags().StringVar(&awaitEventTimeout, "timeout", "60s",
		"Maximum time to wait for event (e.g., 30s, 5m, 10m)")
	moleculeAwaitEventCmd.Flags().StringVar(&awaitEventBackoffBase, "backoff-base", "",
		"Base interval for exponential backoff (e.g., 60s)")
	moleculeAwaitEventCmd.Flags().IntVar(&awaitEventBackoffMult, "backoff-mult", 2,
		"Multiplier for exponential backoff (default: 2)")
	moleculeAwaitEventCmd.Flags().StringVar(&awaitEventBackoffMax, "backoff-max", "",
		"Maximum interval cap for backoff (e.g., 10m)")
	moleculeAwaitEventCmd.Flags().StringVar(&awaitEventAgentBead, "agent-bead", "",
		"Agent bead ID for tracking idle cycles")
	moleculeAwaitEventCmd.Flags().BoolVar(&awaitEventQuiet, "quiet", false,
		"Suppress output (for scripting)")
	moleculeAwaitEventCmd.Flags().BoolVar(&awaitEventCleanup, "cleanup", false,
		"Delete event files after reading them")
	moleculeAwaitEventCmd.Flags().BoolVar(&moleculeJSON, "json", false,
		"Output as JSON")
	_ = moleculeAwaitEventCmd.MarkFlagRequired("channel")

	moleculeStepCmd.AddCommand(moleculeAwaitEventCmd)
}

func runMoleculeAwaitEvent(cmd *cobra.Command, args []string) error {
	// Resolve event directory
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		// Fallback to ~/gt
		home, _ := os.UserHomeDir()
		townRoot = filepath.Join(home, "gt")
	}
	eventDir := filepath.Join(townRoot, "events", awaitEventChannel)
	if err := os.MkdirAll(eventDir, 0755); err != nil {
		return fmt.Errorf("creating event directory: %w", err)
	}

	// Read current idle cycles from agent bead (same pattern as await-signal)
	var idleCycles int
	var beadsDir string
	if awaitEventAgentBead != "" {
		workDir, wdErr := findLocalBeadsDir()
		if wdErr == nil {
			beadsDir = beads.ResolveBeadsDir(workDir)
			labels, labErr := getAgentLabels(awaitEventAgentBead, beadsDir)
			if labErr != nil {
				if !awaitEventQuiet {
					fmt.Printf("%s Could not read agent bead (starting at idle=0): %v\n",
						style.Dim.Render("⚠"), labErr)
				}
			} else {
				if idleStr, ok := labels["idle"]; ok {
					if n, parseErr := parseIntSimple(idleStr); parseErr == nil {
						idleCycles = n
					}
				}
			}
		}
	}

	// Calculate timeout (with backoff if configured)
	timeout, err := calculateEventTimeout(idleCycles)
	if err != nil {
		return fmt.Errorf("invalid timeout configuration: %w", err)
	}

	if !awaitEventQuiet && !moleculeJSON {
		fmt.Printf("%s Awaiting event on channel %q (timeout: %v, idle: %d)...\n",
			style.Dim.Render("⏳"), awaitEventChannel, timeout, idleCycles)
	}

	startTime := time.Now()

	// Wait for events
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := waitForEventFiles(ctx, eventDir)
	if err != nil {
		return fmt.Errorf("event watch failed: %w", err)
	}
	result.Elapsed = time.Since(startTime)

	// Update agent bead idle cycles (same pattern as await-signal)
	if awaitEventAgentBead != "" && beadsDir != "" {
		if result.Reason == "timeout" {
			newIdle := idleCycles + 1
			if setErr := setAgentIdleCycles(awaitEventAgentBead, beadsDir, newIdle); setErr != nil {
				if !awaitEventQuiet {
					fmt.Printf("%s Failed to update idle count: %v\n",
						style.Dim.Render("⚠"), setErr)
				}
			} else {
				result.IdleCycles = newIdle
			}
		} else if result.Reason == "event" {
			_ = updateAgentHeartbeat(awaitEventAgentBead, beadsDir)
			result.IdleCycles = idleCycles
		}
	}

	// Cleanup event files if requested
	if awaitEventCleanup && result.Reason == "event" {
		for _, ef := range result.Events {
			_ = os.Remove(ef.Path)
		}
	}

	// Output
	if moleculeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if !awaitEventQuiet {
		switch result.Reason {
		case "event":
			fmt.Printf("%s %d event(s) received after %v\n",
				style.Bold.Render("✓"), len(result.Events), result.Elapsed.Round(time.Millisecond))
			for _, ef := range result.Events {
				// Show event type from content
				var parsed map[string]interface{}
				if json.Unmarshal(ef.Content, &parsed) == nil {
					if t, ok := parsed["type"].(string); ok {
						fmt.Printf("  %s %s\n", style.Dim.Render("→"), t)
					}
				}
			}
		case "timeout":
			fmt.Printf("%s Timeout after %v (idle cycle: %d)\n",
				style.Dim.Render("⏱"), result.Elapsed.Round(time.Millisecond), result.IdleCycles)
		}
	}

	return nil
}

// calculateEventTimeout mirrors calculateEffectiveTimeout for await-event.
func calculateEventTimeout(idleCycles int) (time.Duration, error) {
	if awaitEventBackoffBase != "" {
		base, err := time.ParseDuration(awaitEventBackoffBase)
		if err != nil {
			return 0, fmt.Errorf("invalid backoff-base: %w", err)
		}
		timeout := base
		for i := 0; i < idleCycles; i++ {
			timeout *= time.Duration(awaitEventBackoffMult)
		}
		if awaitEventBackoffMax != "" {
			maxDur, err := time.ParseDuration(awaitEventBackoffMax)
			if err != nil {
				return 0, fmt.Errorf("invalid backoff-max: %w", err)
			}
			if timeout > maxDur {
				timeout = maxDur
			}
		}
		return timeout, nil
	}
	return time.ParseDuration(awaitEventTimeout)
}

// waitForEventFiles checks for pending events, then blocks via inotifywait.
func waitForEventFiles(ctx context.Context, eventDir string) (*AwaitEventResult, error) {
	// Check for already-pending events
	events, err := readPendingEvents(eventDir)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		return &AwaitEventResult{
			Reason: "event",
			Events: events,
		}, nil
	}

	// Calculate remaining timeout from context
	deadline, ok := ctx.Deadline()
	if !ok {
		return &AwaitEventResult{Reason: "timeout"}, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return &AwaitEventResult{Reason: "timeout"}, nil
	}

	// Block via inotifywait
	timeoutSecs := int(remaining.Seconds())
	if timeoutSecs < 1 {
		timeoutSecs = 1
	}

	cmd := exec.CommandContext(ctx, "inotifywait",
		"-t", fmt.Sprintf("%d", timeoutSecs),
		"-e", "create",
		"--format", "%f",
		eventDir,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	err = cmd.Run()

	// inotifywait exit codes: 0=event, 1=error, 2=timeout
	// When context is canceled, we get a killed process error
	if ctx.Err() != nil {
		return &AwaitEventResult{Reason: "timeout"}, nil
	}

	// Check for events regardless of exit code (race condition safety)
	events, _ = readPendingEvents(eventDir)
	if len(events) > 0 {
		return &AwaitEventResult{
			Reason: "event",
			Events: events,
		}, nil
	}

	// No events found — timeout
	return &AwaitEventResult{Reason: "timeout"}, nil
}

// readPendingEvents reads all .event files from the directory.
func readPendingEvents(dir string) ([]EventFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []EventFile
	var paths []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".event") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(paths) // oldest first

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}
		events = append(events, EventFile{
			Path:    path,
			Content: json.RawMessage(data),
		})
	}

	return events, nil
}
