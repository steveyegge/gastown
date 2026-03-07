package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	witnessLogGrep    string
	witnessLogPolecat string
	witnessLogSince   string
	witnessLogType    string
	witnessLogTail    int
	witnessLogLive    bool
	witnessLogJSON    bool
)

var witnessLogCmd = &cobra.Command{
	Use:   "log <rig>",
	Short: "View aggregated logs for a rig's agents",
	Long: `View aggregated logs from all agents in a rig (witness + polecats).

Sources:
  - Town log events scoped to the rig (lifecycle events)
  - Live tmux pane captures from active sessions (with --live)

Examples:
  gt witness log gastown                    # Last 50 rig events
  gt witness log gastown --since 1h         # Events from last hour
  gt witness log gastown --polecat ace      # Events for polecat ace only
  gt witness log gastown --grep "crash"     # Search for "crash" in logs
  gt witness log gastown --live             # Include live pane output
  gt witness log gastown --type spawn       # Show only spawn events
  gt witness log gastown -n 100 --json      # Last 100 events as JSON`,
	Args: cobra.ExactArgs(1),
	RunE: runWitnessLog,
}

func init() {
	witnessLogCmd.Flags().StringVar(&witnessLogGrep, "grep", "", "Search pattern (case-insensitive)")
	witnessLogCmd.Flags().StringVar(&witnessLogPolecat, "polecat", "", "Filter to specific polecat")
	witnessLogCmd.Flags().StringVar(&witnessLogSince, "since", "", "Show events since duration (e.g., 1h, 30m)")
	witnessLogCmd.Flags().StringVar(&witnessLogType, "type", "", "Filter by event type")
	witnessLogCmd.Flags().IntVarP(&witnessLogTail, "tail", "n", 50, "Number of entries to show")
	witnessLogCmd.Flags().BoolVar(&witnessLogLive, "live", false, "Include live pane captures from active sessions")
	witnessLogCmd.Flags().BoolVar(&witnessLogJSON, "json", false, "Output as JSON")

	witnessCmd.AddCommand(witnessLogCmd)
}

func runWitnessLog(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	q := witness.LogQuery{
		Rig:     r,
		Polecat: witnessLogPolecat,
		Type:    witnessLogType,
		Grep:    witnessLogGrep,
		Tail:    witnessLogTail,
		Live:    witnessLogLive,
	}

	if witnessLogSince != "" {
		duration, err := time.ParseDuration(witnessLogSince)
		if err != nil {
			return fmt.Errorf("invalid --since duration: %w", err)
		}
		q.Since = duration
	}

	result := witness.AggregateLogs(townRoot, q)

	// JSON output
	if witnessLogJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Print non-fatal errors as warnings
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "%s %s\n", style.Warning.Render("warning:"), e)
	}

	if len(result.Entries) == 0 {
		fmt.Printf("%s No log entries match filter\n", style.Dim.Render("○"))
		return nil
	}

	// Print header
	fmt.Printf("%s Logs for %s (%d entries)\n\n",
		style.Bold.Render("◆"), rigName, len(result.Entries))

	// Print entries
	for _, entry := range result.Entries {
		printLogEntry(entry)
	}

	return nil
}

func printLogEntry(e witness.LogEntry) {
	ts := e.Timestamp.Format("15:04:05")

	var sourceTag string
	switch {
	case e.Source == "townlog":
		sourceTag = style.Dim.Render("log")
	default:
		sourceTag = style.Bold.Render("pane")
	}

	var typeStr string
	switch e.Type {
	case "spawn":
		typeStr = style.Success.Render(e.Type)
	case "done":
		typeStr = style.Success.Render(e.Type)
	case "crash":
		typeStr = style.Error.Render(e.Type)
	case "kill":
		typeStr = style.Warning.Render(e.Type)
	case "nudge":
		typeStr = style.Dim.Render(e.Type)
	case "output":
		typeStr = ""
	default:
		typeStr = e.Type
	}

	if typeStr != "" {
		fmt.Printf("%s [%s] %s %s %s\n",
			style.Dim.Render(ts), sourceTag, typeStr, style.Dim.Render(e.Agent), e.Content)
	} else {
		// Pane output: show agent and content without type
		fmt.Printf("%s [%s] %s %s\n",
			style.Dim.Render(ts), sourceTag, style.Dim.Render(e.Agent), e.Content)
	}
}
