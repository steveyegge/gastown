package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/guardian"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	judgmentJSON   bool
	judgmentRig    string
	judgmentWindow string

	judgmentHistPolecat string
	judgmentHistLimit   int
	judgmentHistJSON    bool
)

func init() {
	rootCmd.AddCommand(judgmentCmd)
	judgmentCmd.AddCommand(judgmentStatusCmd)
	judgmentCmd.AddCommand(judgmentHistoryCmd)

	judgmentStatusCmd.Flags().BoolVar(&judgmentJSON, "json", false, "Output as JSON")
	judgmentStatusCmd.Flags().StringVar(&judgmentRig, "rig", "", "Filter by rig name")
	judgmentStatusCmd.Flags().StringVar(&judgmentWindow, "window", "24h", "Time window for stats")

	judgmentHistoryCmd.Flags().StringVar(&judgmentHistPolecat, "polecat", "", "Filter by polecat name")
	judgmentHistoryCmd.Flags().IntVar(&judgmentHistLimit, "limit", 20, "Maximum results to show")
	judgmentHistoryCmd.Flags().BoolVar(&judgmentHistJSON, "json", false, "Output as JSON")
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
  gt judgment history    Show individual review history`,
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
		data, _ := json.MarshalIndent(rows, "", "  ")
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
		data, _ := json.MarshalIndent(rows, "", "  ")
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
		return fmt.Sprintf("● %s", status)
	case "WARN":
		return fmt.Sprintf("● %s", status)
	case "BREACH":
		return fmt.Sprintf("● %s", status)
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
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid days: %s", window)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	return time.ParseDuration(window)
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
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

