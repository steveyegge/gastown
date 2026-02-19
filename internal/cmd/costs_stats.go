package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	// Stats subcommand flags
	statsJSON    bool
	statsDays    int
	statsGroupBy string

	// Preflight subcommand flags
	preflightRole    string
	preflightFormula string
	preflightJSON    bool
)

var costsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show aggregate statistics from the cost learning dataset",
	Long: `Display aggregate statistics from session cost data.

This reads ~/.gt/costs.jsonl and computes statistics that help identify
expensive patterns, successful strategies, and areas for improvement.

Statistics include:
  - Success rate by role and formula
  - Average cost by exit status (COMPLETED vs ESCALATED vs DEFERRED)
  - Token usage patterns by role
  - Top expensive sessions

Examples:
  gt costs stats              # Overall statistics
  gt costs stats --days 7     # Last 7 days only
  gt costs stats --group role # Group by role
  gt costs stats --json       # JSON output`,
	RunE: runCostsStats,
}

var costsPreflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Estimate expected cost and success probability before launching an agent",
	Long: `Estimate the expected cost and success probability for a given role or formula
based on historical data from the cost learning dataset.

This helps decide whether launching an agent is worth the cost, or if an
alternative strategy (diagnostic, escalation) would be more effective.

Examples:
  gt costs preflight --role polecat       # Polecat historical stats
  gt costs preflight --formula code-review  # Formula-specific stats
  gt costs preflight --role witness --json  # JSON output`,
	RunE: runCostsPreflight,
}

func init() {
	// Stats subcommand
	costsCmd.AddCommand(costsStatsCmd)
	costsStatsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	costsStatsCmd.Flags().IntVar(&statsDays, "days", 0, "Limit to last N days (0 = all)")
	costsStatsCmd.Flags().StringVar(&statsGroupBy, "group", "", "Group by: role, rig, formula, status")

	// Preflight subcommand
	costsCmd.AddCommand(costsPreflightCmd)
	costsPreflightCmd.Flags().StringVar(&preflightRole, "role", "", "Role to estimate (polecat, witness, etc.)")
	costsPreflightCmd.Flags().StringVar(&preflightFormula, "formula", "", "Formula to estimate")
	costsPreflightCmd.Flags().BoolVar(&preflightJSON, "json", false, "Output as JSON")
}

// CostStats represents aggregate statistics from the cost dataset.
type CostStats struct {
	TotalSessions int                `json:"total_sessions"`
	TotalCostUSD  float64            `json:"total_cost_usd"`
	ByStatus      map[string]*Bucket `json:"by_status,omitempty"`
	ByRole        map[string]*Bucket `json:"by_role,omitempty"`
	ByRig         map[string]*Bucket `json:"by_rig,omitempty"`
	ByFormula     map[string]*Bucket `json:"by_formula,omitempty"`
}

// Bucket holds aggregate stats for a group of sessions.
type Bucket struct {
	Count       int     `json:"count"`
	TotalCost   float64 `json:"total_cost_usd"`
	AvgCost     float64 `json:"avg_cost_usd"`
	MedianCost  float64 `json:"median_cost_usd"`
	P95Cost     float64 `json:"p95_cost_usd"`
	SuccessRate float64 `json:"success_rate"` // Fraction of COMPLETED sessions
	AvgTokensIn int     `json:"avg_tokens_in"`
	AvgTokenOut int     `json:"avg_tokens_out"`
	AvgWallTime float64 `json:"avg_wall_time_secs"`
	AvgTurns    float64 `json:"avg_turns"` // Avg API round-trips (high = potential thrash)
}

// PreflightEstimate is the output of the preflight command.
type PreflightEstimate struct {
	Label          string  `json:"label"`
	SampleSize     int     `json:"sample_size"`
	SuccessRate    float64 `json:"success_rate"`
	AvgCostUSD     float64 `json:"avg_cost_usd"`
	MedianCostUSD  float64 `json:"median_cost_usd"`
	P95CostUSD     float64 `json:"p95_cost_usd"`
	AvgWallTimeSec float64 `json:"avg_wall_time_secs"`
	AvgTokensIn    int     `json:"avg_tokens_in"`
	AvgTokensOut   int     `json:"avg_tokens_out"`
	AvgTurns       float64 `json:"avg_turns"`
	Recommendation string  `json:"recommendation"`
}

// readAllCostEntries reads all entries from ~/.gt/costs.jsonl.
func readAllCostEntries(maxDays int) ([]CostLogEntry, error) {
	logPath := getCostsLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading costs log: %w", err)
	}

	var entries []CostLogEntry
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry CostLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Filter by days if specified
	if maxDays > 0 && len(entries) > 0 {
		cutoff := entries[len(entries)-1].EndedAt.AddDate(0, 0, -maxDays)
		var filtered []CostLogEntry
		for _, e := range entries {
			if !e.EndedAt.Before(cutoff) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	return entries, nil
}

// computeStats computes aggregate statistics from cost entries.
func computeStats(entries []CostLogEntry) *CostStats {
	stats := &CostStats{
		ByStatus:  make(map[string]*Bucket),
		ByRole:    make(map[string]*Bucket),
		ByRig:     make(map[string]*Bucket),
		ByFormula: make(map[string]*Bucket),
	}

	// Collect costs per bucket for median/p95 computation
	statusCosts := make(map[string][]float64)
	roleCosts := make(map[string][]float64)
	rigCosts := make(map[string][]float64)
	formulaCosts := make(map[string][]float64)

	for _, e := range entries {
		stats.TotalSessions++
		stats.TotalCostUSD += e.CostUSD

		// By status
		status := e.ExitStatus
		if status == "" {
			status = "unknown"
		}
		addToBucketAccum(stats.ByStatus, statusCosts, status, e)

		// By role
		addToBucketAccum(stats.ByRole, roleCosts, e.Role, e)

		// By rig
		if e.Rig != "" {
			addToBucketAccum(stats.ByRig, rigCosts, e.Rig, e)
		}

		// By formula
		if e.FormulaName != "" {
			addToBucketAccum(stats.ByFormula, formulaCosts, e.FormulaName, e)
		}
	}

	// Finalize buckets with averages and percentiles
	finalizeBuckets(stats.ByStatus, statusCosts)
	finalizeBuckets(stats.ByRole, roleCosts)
	finalizeBuckets(stats.ByRig, rigCosts)
	finalizeBuckets(stats.ByFormula, formulaCosts)

	return stats
}

// addToBucketAccum adds an entry to both the bucket summary and the cost accumulator.
func addToBucketAccum(buckets map[string]*Bucket, costAccum map[string][]float64, key string, e CostLogEntry) {
	b, ok := buckets[key]
	if !ok {
		b = &Bucket{}
		buckets[key] = b
	}
	b.Count++
	b.TotalCost += e.CostUSD
	b.AvgTokensIn += e.InputTokens
	b.AvgTokenOut += e.OutputTokens
	b.AvgWallTime += e.WallTimeSecs
	b.AvgTurns += float64(e.TurnCount)
	if e.ExitStatus == ExitCompleted {
		b.SuccessRate++
	}
	costAccum[key] = append(costAccum[key], e.CostUSD)
}

// finalizeBuckets computes averages and percentiles for all buckets.
func finalizeBuckets(buckets map[string]*Bucket, costAccum map[string][]float64) {
	for key, b := range buckets {
		if b.Count > 0 {
			b.AvgCost = b.TotalCost / float64(b.Count)
			b.AvgTokensIn = b.AvgTokensIn / b.Count
			b.AvgTokenOut = b.AvgTokenOut / b.Count
			b.AvgWallTime = b.AvgWallTime / float64(b.Count)
			b.AvgTurns = b.AvgTurns / float64(b.Count)
			b.SuccessRate = b.SuccessRate / float64(b.Count)
		}
		costs := costAccum[key]
		sort.Float64s(costs)
		b.MedianCost = percentile(costs, 50)
		b.P95Cost = percentile(costs, 95)
	}
}

// percentile computes the p-th percentile of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p / 100 * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func runCostsStats(cmd *cobra.Command, args []string) error {
	entries, err := readAllCostEntries(statsDays)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println(style.Dim.Render("No cost data found. Costs are recorded when sessions end."))
		return nil
	}

	stats := computeStats(entries)

	if statsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}

	// Human-readable output
	fmt.Printf("\n%s Cost Statistics", style.Bold.Render("📊"))
	if statsDays > 0 {
		fmt.Printf(" (last %d days)", statsDays)
	}
	fmt.Printf("\n\n")
	fmt.Printf("  Sessions: %d\n", stats.TotalSessions)
	fmt.Printf("  Total:    $%.2f\n\n", stats.TotalCostUSD)

	// Determine which group to show based on --group flag
	switch statsGroupBy {
	case "role":
		printBucketTable("By Role", stats.ByRole)
	case "rig":
		printBucketTable("By Rig", stats.ByRig)
	case "formula":
		printBucketTable("By Formula", stats.ByFormula)
	case "status":
		printBucketTable("By Exit Status", stats.ByStatus)
	default:
		// Show all groups with data
		if len(stats.ByStatus) > 0 {
			printBucketTable("By Exit Status", stats.ByStatus)
		}
		if len(stats.ByRole) > 0 {
			printBucketTable("By Role", stats.ByRole)
		}
		if len(stats.ByFormula) > 0 {
			printBucketTable("By Formula", stats.ByFormula)
		}
	}

	return nil
}

func printBucketTable(title string, buckets map[string]*Bucket) {
	if len(buckets) == 0 {
		return
	}

	fmt.Printf("%s\n", style.Bold.Render(title))
	fmt.Printf("  %-20s %6s %10s %10s %10s %8s\n",
		"Name", "Count", "Avg Cost", "P50", "P95", "Success")
	fmt.Printf("  %s\n", strings.Repeat("─", 70))

	// Sort keys for deterministic output
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		b := buckets[key]
		fmt.Printf("  %-20s %6d %10s %10s %10s %7.0f%%\n",
			key,
			b.Count,
			fmt.Sprintf("$%.2f", b.AvgCost),
			fmt.Sprintf("$%.2f", b.MedianCost),
			fmt.Sprintf("$%.2f", b.P95Cost),
			b.SuccessRate*100)
	}
	fmt.Println()
}

func runCostsPreflight(cmd *cobra.Command, args []string) error {
	if preflightRole == "" && preflightFormula == "" {
		return fmt.Errorf("specify --role or --formula for preflight estimation")
	}

	entries, err := readAllCostEntries(0) // Use all data for preflight
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println(style.Dim.Render("No cost data found. Run some sessions first to build the learning dataset."))
		return nil
	}

	// Filter entries by role or formula
	var filtered []CostLogEntry
	label := ""
	for _, e := range entries {
		if preflightRole != "" && e.Role == preflightRole {
			filtered = append(filtered, e)
			label = "role:" + preflightRole
		} else if preflightFormula != "" && e.FormulaName == preflightFormula {
			filtered = append(filtered, e)
			label = "formula:" + preflightFormula
		}
	}

	if len(filtered) == 0 {
		if preflightRole != "" {
			fmt.Printf("%s No historical data for role %q\n", style.Dim.Render("○"), preflightRole)
		} else {
			fmt.Printf("%s No historical data for formula %q\n", style.Dim.Render("○"), preflightFormula)
		}
		return nil
	}

	// Compute estimate
	estimate := computePreflight(label, filtered)

	if preflightJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(estimate)
	}

	// Human-readable output
	fmt.Printf("\n%s Preflight Estimate: %s\n\n", style.Bold.Render("🔮"), label)
	fmt.Printf("  Sample size:    %d sessions\n", estimate.SampleSize)
	fmt.Printf("  Success rate:   %.0f%%\n", estimate.SuccessRate*100)
	fmt.Printf("  Avg cost:       $%.2f\n", estimate.AvgCostUSD)
	fmt.Printf("  Median cost:    $%.2f\n", estimate.MedianCostUSD)
	fmt.Printf("  P95 cost:       $%.2f\n", estimate.P95CostUSD)
	if estimate.AvgWallTimeSec > 0 {
		fmt.Printf("  Avg wall time:  %.0fs\n", estimate.AvgWallTimeSec)
	}
	if estimate.AvgTokensIn > 0 {
		fmt.Printf("  Avg tokens in:  %d\n", estimate.AvgTokensIn)
		fmt.Printf("  Avg tokens out: %d\n", estimate.AvgTokensOut)
	}
	if estimate.AvgTurns > 0 {
		fmt.Printf("  Avg turns:      %.0f\n", estimate.AvgTurns)
	}
	fmt.Printf("\n  %s %s\n\n", style.Bold.Render("Recommendation:"), estimate.Recommendation)

	return nil
}

// computePreflight generates a preflight estimate from filtered cost entries.
func computePreflight(label string, entries []CostLogEntry) *PreflightEstimate {
	est := &PreflightEstimate{
		Label:      label,
		SampleSize: len(entries),
	}

	var totalCost, totalWall float64
	var totalTokensIn, totalTokensOut, totalTurns int
	var completedCount int
	costs := make([]float64, 0, len(entries))

	for _, e := range entries {
		totalCost += e.CostUSD
		totalWall += e.WallTimeSecs
		totalTokensIn += e.InputTokens
		totalTokensOut += e.OutputTokens
		totalTurns += e.TurnCount
		costs = append(costs, e.CostUSD)
		if e.ExitStatus == ExitCompleted {
			completedCount++
		}
	}

	n := float64(len(entries))
	est.AvgCostUSD = totalCost / n
	est.AvgWallTimeSec = totalWall / n
	est.AvgTokensIn = int(float64(totalTokensIn) / n)
	est.AvgTokensOut = int(float64(totalTokensOut) / n)
	est.AvgTurns = float64(totalTurns) / n

	// Success rate only from entries that have exit_status
	entriesWithStatus := 0
	for _, e := range entries {
		if e.ExitStatus != "" {
			entriesWithStatus++
		}
	}
	if entriesWithStatus > 0 {
		est.SuccessRate = float64(completedCount) / float64(entriesWithStatus)
	}

	sort.Float64s(costs)
	est.MedianCostUSD = percentile(costs, 50)
	est.P95CostUSD = percentile(costs, 95)

	// Generate recommendation
	est.Recommendation = generateRecommendation(est)

	return est
}

// generateRecommendation produces a human-readable recommendation based on stats.
func generateRecommendation(est *PreflightEstimate) string {
	if est.SampleSize < 5 {
		return "Insufficient data — proceed with caution"
	}

	// High success, reasonable cost
	if est.SuccessRate >= 0.7 && est.P95CostUSD < 5.0 {
		return "Good expected value — launch recommended"
	}

	// High success but expensive
	if est.SuccessRate >= 0.7 && est.P95CostUSD >= 5.0 {
		return fmt.Sprintf("High success rate (%.0f%%) but expensive (p95: $%.2f) — launch if budget allows",
			est.SuccessRate*100, est.P95CostUSD)
	}

	// Low success
	if est.SuccessRate < 0.3 && est.SampleSize >= 10 {
		return fmt.Sprintf("Low success rate (%.0f%%) — consider alternative strategy or escalation",
			est.SuccessRate*100)
	}

	// Moderate success
	if est.SuccessRate < 0.5 {
		return fmt.Sprintf("Moderate success rate (%.0f%%) — review context before launching",
			est.SuccessRate*100)
	}

	return "Reasonable expected value — launch recommended"
}
