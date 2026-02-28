package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/guardian"
	"github.com/steveyegge/gastown/internal/telemetry"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	judgmentJSON   bool
	judgmentRig    string
	judgmentWindow string

	judgmentHistPolecat string
	judgmentHistLimit   int
	judgmentHistJSON    bool

	judgmentRecordInput string
)

func init() {
	rootCmd.AddCommand(judgmentCmd)
	judgmentCmd.AddCommand(judgmentStatusCmd)
	judgmentCmd.AddCommand(judgmentHistoryCmd)
	judgmentCmd.AddCommand(judgmentRecordCmd)

	judgmentStatusCmd.Flags().BoolVar(&judgmentJSON, "json", false, "Output as JSON")
	judgmentStatusCmd.Flags().StringVar(&judgmentRig, "rig", "", "Filter by rig name")
	judgmentStatusCmd.Flags().StringVar(&judgmentWindow, "window", "24h", "Time window for stats")

	judgmentHistoryCmd.Flags().StringVar(&judgmentHistPolecat, "polecat", "", "Filter by polecat name")
	judgmentHistoryCmd.Flags().IntVar(&judgmentHistLimit, "limit", 20, "Maximum results to show")
	judgmentHistoryCmd.Flags().BoolVar(&judgmentHistJSON, "json", false, "Output as JSON")

	judgmentRecordCmd.Flags().StringVar(&judgmentRecordInput, "input", "", "JSON input (default: read from stdin)")
}

var judgmentCmd = &cobra.Command{
	Use:     "judgment",
	GroupID: GroupDiag,
	Short:   "View Guardian quality review data",
	Long: `View Guardian quality review scores and history.

The Guardian reviews merge diffs for code quality and records scores.
Phase 1 is measurement-only: scores are recorded but do not gate merges.

Subcommands:
  gt judgment status     Show per-polecat quality summary
  gt judgment history    Show individual review history
  gt judgment record     Record a quality review result`,
}

var judgmentStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show per-polecat quality summary",
	Long: `Display a table of polecat quality metrics from Guardian reviews.

Shows average score, rejection rate, and status for each polecat.

Status thresholds:
  OK      avg score >= 0.60
  WARN    avg score 0.45-0.60
  BREACH  avg score < 0.45

Examples:
  gt judgment status              # Summary for all polecats
  gt judgment status --json       # JSON output
  gt judgment status --rig myrig  # Filter by rig
  gt judgment status --window 7d  # Last 7 days`,
	RunE: runJudgmentStatus,
}

var judgmentHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show individual review history",
	Long: `Display per-review history from Guardian state.

Shows bead ID, score, issue count, recommendation, and timestamp.

Examples:
  gt judgment history                      # Recent reviews
  gt judgment history --polecat Toast      # Filter by polecat
  gt judgment history --limit 50           # Show more results
  gt judgment history --json               # JSON output`,
	RunE: runJudgmentHistory,
}

func runJudgmentStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	stateDir := townRoot
	if judgmentRig != "" {
		stateDir = filepath.Join(townRoot, judgmentRig)
	}

	state, err := guardian.LoadState(stateDir)
	if err != nil {
		return fmt.Errorf("loading Guardian state: %w", err)
	}

	if len(state.Workers) == 0 {
		if judgmentJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No Guardian review data. Enable judgment in settings to start collecting quality scores.")
		}
		return nil
	}

	// Filter by time window
	windowDur, err := parseWindow(judgmentWindow)
	if err != nil {
		return fmt.Errorf("invalid --window: %w", err)
	}
	cutoff := time.Now().Add(-windowDur)

	// Collect and sort workers
	type workerRow struct {
		Name          string  `json:"name"`
		Reviews       int     `json:"reviews"`
		AvgScore      float64 `json:"avg_score"`
		RejectionRate float64 `json:"rejection_rate"`
		Status        string  `json:"status"`
	}

	var rows []workerRow
	for _, pj := range state.Workers {
		// Filter recent results within window
		var recentInWindow []guardian.RecentResult
		for _, r := range pj.RecentResults {
			if r.ReviewedAt.After(cutoff) {
				recentInWindow = append(recentInWindow, r)
			}
		}
		if len(recentInWindow) == 0 {
			continue
		}

		// Recompute stats for the window
		var totalScore float64
		var rejections int
		for _, r := range recentInWindow {
			totalScore += r.Score
			if r.Recommendation == "request_changes" {
				rejections++
			}
		}
		avgScore := totalScore / float64(len(recentInWindow))
		rejRate := float64(rejections) / float64(len(recentInWindow))

		status := "OK"
		if avgScore < 0.45 {
			status = "BREACH"
		} else if avgScore < 0.60 {
			status = "WARN"
		}

		rows = append(rows, workerRow{
			Name:          pj.Worker,
			Reviews:       len(recentInWindow),
			AvgScore:      avgScore,
			RejectionRate: rejRate,
			Status:        status,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].AvgScore < rows[j].AvgScore // worst first
	})

	if judgmentJSON {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Table output
	fmt.Printf("%-20s  %6s  %8s  %10s  %s\n",
		"POLECAT", "MRs", "AVG", "REJ RATE", "STATUS")
	fmt.Println(strings.Repeat("─", 60))

	for _, row := range rows {
		statusIcon := statusStyle(row.Status)
		fmt.Printf("%-20s  %6d  %8.2f  %9.0f%%  %s\n",
			row.Name,
			row.Reviews,
			row.AvgScore,
			row.RejectionRate*100,
			statusIcon,
		)
	}

	return nil
}

func runJudgmentHistory(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	state, err := guardian.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading Guardian state: %w", err)
	}

	if len(state.Workers) == 0 {
		if judgmentHistJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No Guardian review history.")
		}
		return nil
	}

	// Collect all results, optionally filtered by polecat
	type historyRow struct {
		Worker         string    `json:"worker"`
		BeadID         string    `json:"bead_id"`
		Score          float64   `json:"score"`
		IssueCount     int       `json:"issue_count"`
		Recommendation string    `json:"recommendation"`
		ReviewedAt     time.Time `json:"reviewed_at"`
	}

	var rows []historyRow
	for _, pj := range state.Workers {
		if judgmentHistPolecat != "" && !strings.Contains(pj.Worker, judgmentHistPolecat) {
			continue
		}
		for _, r := range pj.RecentResults {
			rows = append(rows, historyRow{
				Worker:         pj.Worker,
				BeadID:         r.BeadID,
				Score:          r.Score,
				IssueCount:     r.IssueCount,
				Recommendation: r.Recommendation,
				ReviewedAt:     r.ReviewedAt,
			})
		}
	}

	// Sort by time, most recent first
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ReviewedAt.After(rows[j].ReviewedAt)
	})

	// Apply limit
	if judgmentHistLimit > 0 && len(rows) > judgmentHistLimit {
		rows = rows[:judgmentHistLimit]
	}

	if judgmentHistJSON {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if len(rows) == 0 {
		fmt.Println("No review history found.")
		return nil
	}

	fmt.Printf("%-16s  %5s  %6s  %s  %-16s  %s\n",
		"BEAD", "SCORE", "ISSUES", "REC", "WORKER", "TIME")
	fmt.Println(strings.Repeat("─", 80))

	for _, row := range rows {
		recShort := row.Recommendation
		if len(recShort) > 8 {
			recShort = recShort[:8]
		}
		age := formatJudgmentAge(time.Since(row.ReviewedAt))
		fmt.Printf("%-16s  %5.2f  %6d  %-8s  %-16s  %s\n",
			truncateJudgmentStr(row.BeadID, 16),
			row.Score,
			row.IssueCount,
			recShort,
			truncateJudgmentStr(row.Worker, 16),
			age,
		)
	}

	return nil
}

func statusStyle(status string) string {
	switch status {
	case "OK":
		return "\033[32m● OK\033[0m"     // green
	case "WARN":
		return "\033[33m● WARN\033[0m"   // yellow
	case "BREACH":
		return "\033[31m● BREACH\033[0m" // red
	default:
		return status
	}
}

func parseWindow(window string) (time.Duration, error) {
	if window == "" {
		return 24 * time.Hour, nil
	}

	// Support "7d" style
	if strings.HasSuffix(window, "d") {
		days := window[:len(window)-1]
		var d int
		n, err := fmt.Sscanf(days, "%d", &d)
		if err != nil || n != 1 {
			return 0, fmt.Errorf("invalid days: %s", window)
		}
		// Reject trailing characters (e.g. "1.5d" parses "1" but leaves ".5")
		if fmt.Sprintf("%d", d) != days {
			return 0, fmt.Errorf("invalid days: %s", window)
		}
		if d <= 0 {
			return 0, fmt.Errorf("days must be positive: %s", window)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	dur, err := time.ParseDuration(window)
	if err != nil {
		return 0, err
	}
	if dur <= 0 {
		return 0, fmt.Errorf("window must be positive: %s", window)
	}
	return dur, nil
}

func formatJudgmentAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncateJudgmentStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

var judgmentRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a quality review result",
	Long: `Record a Guardian quality review result from JSON input.

Accepts a GuardianResult JSON object via stdin or --input flag,
validates it, persists to judgment-state.json, and records OTel telemetry.

This is the write API used by formula steps to record review scores.

JSON format:
  {
    "bead_id": "gt-abc123",
    "score": 0.85,
    "recommendation": "approve",
    "issues": [{"severity": "minor", "category": "style", "description": "nit"}],
    "worker": "polecat-Toast",
    "rig": "myrig"
  }

Examples:
  echo '{"bead_id":"gt-abc","score":0.9,"recommendation":"approve","worker":"toast","rig":"myrig"}' | gt judgment record
  gt judgment record --input '{"bead_id":"gt-abc","score":0.9,"recommendation":"approve","worker":"toast","rig":"myrig"}'`,
	RunE: runJudgmentRecord,
}

func runJudgmentRecord(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Read JSON input from flag or stdin
	var inputData []byte
	if judgmentRecordInput != "" {
		inputData = []byte(judgmentRecordInput)
	} else {
		inputData, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	if len(inputData) == 0 {
		return fmt.Errorf("no input provided; pass JSON via stdin or --input flag")
	}

	// Parse and validate
	var result guardian.GuardianResult
	if err := json.Unmarshal(inputData, &result); err != nil {
		return fmt.Errorf("invalid JSON input: %w", err)
	}

	if err := validateRecordInput(&result); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Set reviewed_at if not provided
	if result.ReviewedAt.IsZero() {
		result.ReviewedAt = time.Now()
	}

	// Persist to state
	stateDir := townRoot
	state, err := guardian.LoadState(stateDir)
	if err != nil {
		return fmt.Errorf("loading Guardian state: %w", err)
	}

	state.AddResult(result.Worker, &result)

	if err := guardian.SaveState(stateDir, state); err != nil {
		return fmt.Errorf("saving Guardian state: %w", err)
	}

	// Record OTel telemetry
	telemetry.RecordGuardianResult(
		context.Background(),
		result.Worker,
		result.Rig,
		result.Recommendation,
		result.Score,
		result.DurationMs,
	)

	// Log event for feed visibility
	eventType := events.TypeGuardianReview
	if result.Recommendation == "skip" {
		eventType = events.TypeGuardianSkipped
	}
	_ = events.LogFeed(eventType, result.Worker, map[string]interface{}{
		"bead":           result.BeadID,
		"score":          result.Score,
		"recommendation": result.Recommendation,
		"issues":         len(result.Issues),
		"worker":         result.Worker,
	})

	fmt.Printf("Recorded: worker=%s score=%.2f recommendation=%s issues=%d\n",
		result.Worker, result.Score, result.Recommendation, len(result.Issues))

	return nil
}

func validateRecordInput(r *guardian.GuardianResult) error {
	if r.Worker == "" {
		return fmt.Errorf("worker is required")
	}
	if r.Rig == "" {
		return fmt.Errorf("rig is required")
	}
	if r.Score < 0 || r.Score > 1 {
		return fmt.Errorf("score must be between 0.0 and 1.0, got %f", r.Score)
	}
	switch r.Recommendation {
	case "approve", "request_changes", "skip":
		// valid
	case "":
		return fmt.Errorf("recommendation is required (approve, request_changes, or skip)")
	default:
		return fmt.Errorf("invalid recommendation %q (must be approve, request_changes, or skip)", r.Recommendation)
	}
	return nil
}

