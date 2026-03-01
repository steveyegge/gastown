package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/guardian"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Judgment command flags.
var (
	judgmentStatusJSON   bool
	judgmentStatusRig    string
	judgmentStatusWindow string

	judgmentHistoryPolecat string
	judgmentHistoryLimit   int
	judgmentHistoryJSON    bool

	judgmentRecordInput string
)

var judgmentCmd = &cobra.Command{
	Use:     "judgment",
	GroupID: GroupDiag,
	Short:   "Quality review judgment tracking",
	RunE:    requireSubcommand,
	Long: `View and manage quality review judgments recorded by the Refinery agent.

The Refinery reviews merge diffs and records scores via 'gt judgment record'.
Use 'status' to see per-worker summaries and 'history' for review details.`,
}

var judgmentStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Per-worker judgment summary (worst score first)",
	RunE:  runJudgmentStatus,
}

var judgmentHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Per-review history (newest first)",
	RunE:  runJudgmentHistory,
}

var judgmentRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a quality review result",
	Long: `Record a GuardianResult from the Refinery's quality-review formula step.

Input is a JSON object with fields:
  bead_id, score (0.0-1.0), recommendation (approve|request_changes|skip),
  worker, rig, issues (array), duration_ms, model

Reads from --input flag or stdin.`,
	RunE: runJudgmentRecord,
}

func init() {
	judgmentStatusCmd.Flags().BoolVar(&judgmentStatusJSON, "json", false, "Output as JSON")
	judgmentStatusCmd.Flags().StringVar(&judgmentStatusRig, "rig", "", "Filter by rig name")
	judgmentStatusCmd.Flags().StringVar(&judgmentStatusWindow, "window", "", "Time window (e.g., 7d, 24h, 2h30m)")

	judgmentHistoryCmd.Flags().StringVar(&judgmentHistoryPolecat, "polecat", "", "Filter by polecat name")
	judgmentHistoryCmd.Flags().IntVar(&judgmentHistoryLimit, "limit", 20, "Maximum results to show")
	judgmentHistoryCmd.Flags().BoolVar(&judgmentHistoryJSON, "json", false, "Output as JSON")

	judgmentRecordCmd.Flags().StringVar(&judgmentRecordInput, "input", "", "JSON input (reads stdin if omitted)")

	judgmentCmd.AddCommand(judgmentStatusCmd)
	judgmentCmd.AddCommand(judgmentHistoryCmd)
	judgmentCmd.AddCommand(judgmentRecordCmd)

	rootCmd.AddCommand(judgmentCmd)
}

// --- status subcommand ---

func runJudgmentStatus(cmd *cobra.Command, _ []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	state, err := guardian.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading judgment state: %w", err)
	}

	window, err := parseWindow(judgmentStatusWindow)
	if err != nil {
		return err
	}

	// Collect and filter workers.
	type workerSummary struct {
		Worker        string  `json:"worker"`
		Rig           string  `json:"rig,omitempty"`
		TotalReviews  int     `json:"total_reviews"`
		AvgScore      float64 `json:"avg_score"`
		RejectionRate float64 `json:"rejection_rate"`
		Status        string  `json:"status"`
	}

	var summaries []workerSummary
	cutoff := time.Now().Add(-window)

	for _, pj := range state.Workers {
		// Filter by window: only count results within the window.
		var inWindow []guardian.RecentResult
		for _, r := range pj.RecentResults {
			if r.ReviewedAt.After(cutoff) || r.ReviewedAt.IsZero() {
				inWindow = append(inWindow, r)
			}
		}
		if len(inWindow) == 0 {
			continue
		}

		// Compute windowed aggregates.
		var totalScore float64
		var rejections int
		for _, r := range inWindow {
			totalScore += r.Score
			if r.Recommendation == guardian.RecommendRequestChanges {
				rejections++
			}
		}
		avg := totalScore / float64(len(inWindow))
		rejRate := float64(rejections) / float64(len(inWindow))

		// Extract rig from first recent result that has one.
		rig := ""
		for _, r := range pj.RecentResults {
			if r.Rig != "" {
				rig = r.Rig
				break
			}
		}

		summaries = append(summaries, workerSummary{
			Worker:        pj.Worker,
			Rig:           rig,
			TotalReviews:  len(inWindow),
			AvgScore:      math.Round(avg*100) / 100,
			RejectionRate: math.Round(rejRate*100) / 100,
			Status:        guardian.StatusForScore(avg),
		})
	}

	// Sort by avg score ascending (worst first).
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].AvgScore < summaries[j].AvgScore
	})

	if judgmentStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summaries)
	}

	if len(summaries) == 0 {
		fmt.Println("No judgment data found.")
		return nil
	}

	// Table output.
	fmt.Printf("%-20s %-8s %-8s %-10s %-8s\n",
		"WORKER", "REVIEWS", "AVG", "REJECTION", "STATUS")
	fmt.Println(strings.Repeat("─", 58))

	for _, s := range summaries {
		fmt.Printf("%-20s %-8d %-8.2f %-10.0f%% %s\n",
			truncateJudgmentStr(s.Worker, 20),
			s.TotalReviews,
			s.AvgScore,
			s.RejectionRate*100,
			statusStyle(s.Status),
		)
	}

	return nil
}

// --- history subcommand ---

func runJudgmentHistory(cmd *cobra.Command, _ []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	state, err := guardian.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading judgment state: %w", err)
	}

	// Collect all results, optionally filtered by polecat.
	type historyEntry struct {
		Worker         string    `json:"worker"`
		BeadID         string    `json:"bead_id"`
		Score          float64   `json:"score"`
		Recommendation string    `json:"recommendation"`
		IssueCount     int       `json:"issue_count"`
		ReviewedAt     time.Time `json:"reviewed_at"`
		Age            string    `json:"age"`
	}

	var entries []historyEntry
	for _, pj := range state.Workers {
		if judgmentHistoryPolecat != "" && pj.Worker != judgmentHistoryPolecat {
			continue
		}
		for _, r := range pj.RecentResults {
			entries = append(entries, historyEntry{
				Worker:         pj.Worker,
				BeadID:         r.BeadID,
				Score:          r.Score,
				Recommendation: r.Recommendation,
				IssueCount:     r.IssueCount,
				ReviewedAt:     r.ReviewedAt,
				Age:            formatJudgmentAge(time.Since(r.ReviewedAt)),
			})
		}
	}

	// Sort newest first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ReviewedAt.After(entries[j].ReviewedAt)
	})

	// Apply limit.
	if judgmentHistoryLimit > 0 && len(entries) > judgmentHistoryLimit {
		entries = entries[:judgmentHistoryLimit]
	}

	if judgmentHistoryJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	if len(entries) == 0 {
		fmt.Println("No judgment history found.")
		return nil
	}

	fmt.Printf("%-16s %-12s %-6s %-16s %-6s %-8s\n",
		"WORKER", "BEAD", "SCORE", "RECOMMENDATION", "ISSUES", "AGE")
	fmt.Println(strings.Repeat("─", 68))

	for _, e := range entries {
		fmt.Printf("%-16s %-12s %-6.2f %-16s %-6d %-8s\n",
			truncateJudgmentStr(e.Worker, 16),
			truncateJudgmentStr(e.BeadID, 12),
			e.Score,
			e.Recommendation,
			e.IssueCount,
			e.Age,
		)
	}

	return nil
}

// --- record subcommand ---

func runJudgmentRecord(cmd *cobra.Command, _ []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Read input from flag or stdin.
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
		return fmt.Errorf("empty input: provide JSON via --input flag or stdin")
	}

	var result guardian.GuardianResult
	if err := json.Unmarshal(inputData, &result); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	if err := validateRecordInput(&result); err != nil {
		return err
	}

	// Set reviewed_at if not provided.
	if result.ReviewedAt.IsZero() {
		result.ReviewedAt = time.Now().UTC()
	}

	// Load, update, save state.
	state, err := guardian.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading judgment state: %w", err)
	}

	state.AddResult(result.Worker, guardian.RecentResult{
		BeadID:         result.BeadID,
		Rig:            result.Rig,
		Score:          result.Score,
		Recommendation: result.Recommendation,
		IssueCount:     len(result.Issues),
		ReviewedAt:     result.ReviewedAt,
	})

	if err := guardian.SaveState(townRoot, state); err != nil {
		return fmt.Errorf("saving judgment state: %w", err)
	}

	// Emit feed event.
	eventType := events.TypeGuardianReview
	if result.Recommendation == guardian.RecommendSkip {
		eventType = events.TypeGuardianSkipped
	}
	_ = events.LogFeed(eventType, result.Worker,
		events.GuardianPayload(result.BeadID, result.Worker, result.Rig, result.Recommendation, result.Score))

	// TODO: add OTel telemetry when internal/telemetry package is created

	fmt.Printf("Recorded judgment for %s: score=%.2f recommendation=%s\n",
		result.Worker, result.Score, result.Recommendation)

	return nil
}

// --- validation ---

func validateRecordInput(r *guardian.GuardianResult) error {
	if r.Worker == "" {
		return fmt.Errorf("worker is required")
	}
	if r.Rig == "" {
		return fmt.Errorf("rig is required")
	}
	if r.Score < 0.0 || r.Score > 1.0 {
		return fmt.Errorf("score must be between 0.0 and 1.0, got %f", r.Score)
	}
	switch r.Recommendation {
	case guardian.RecommendApprove, guardian.RecommendRequestChanges, guardian.RecommendSkip:
		// valid
	default:
		return fmt.Errorf("recommendation must be approve, request_changes, or skip, got %q", r.Recommendation)
	}
	return nil
}

// --- helpers ---

// dayPattern matches integer-day durations like "7d", "30d".
var dayPattern = regexp.MustCompile(`^(\d+)d$`)

// parseWindow parses a time window string into a duration.
// Accepts: "" (24h default), "7d", "24h", "2h30m", "90m".
// Rejects: "0d", negative durations, float days like "1.5d".
func parseWindow(window string) (time.Duration, error) {
	if window == "" {
		return 24 * time.Hour, nil
	}

	// Check for day suffix (e.g., "7d").
	if m := dayPattern.FindStringSubmatch(window); m != nil {
		days, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("invalid window %q: %w", window, err)
		}
		if days <= 0 {
			return 0, fmt.Errorf("invalid window %q: must be positive", window)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Reject float-day patterns like "1.5d".
	if strings.HasSuffix(window, "d") {
		return 0, fmt.Errorf("invalid window %q: day values must be integers", window)
	}

	// Try standard Go duration parsing.
	d, err := time.ParseDuration(window)
	if err != nil {
		return 0, fmt.Errorf("invalid window %q: %w", window, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid window %q: must be positive", window)
	}

	return d, nil
}

// truncateJudgmentStr truncates a string to max runes, appending "…" if truncated.
// Operates on []rune for UTF-8 safety.
func truncateJudgmentStr(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// formatJudgmentAge formats a duration as a human-friendly age string.
func formatJudgmentAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// statusStyle returns a styled status string.
func statusStyle(status string) string {
	switch status {
	case guardian.StatusOK:
		return style.Success.Render(status)
	case guardian.StatusWarn:
		return style.Warning.Render(status)
	case guardian.StatusBreach:
		return style.Error.Render(status)
	default:
		return status
	}
}

