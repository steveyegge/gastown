package cmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/channelevents"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	benchIterations int
	benchDryRun     bool
	benchJSON       bool
)

// BenchResult holds the latency percentile results for all bead operations.
type BenchResult struct {
	Iterations      int     `json:"iterations"`
	P50CreateMs     float64 `json:"p50_create_ms"`
	P95CreateMs     float64 `json:"p95_create_ms"`
	P99CreateMs     float64 `json:"p99_create_ms"`
	P50QueryMs      float64 `json:"p50_query_ms"`
	P95QueryMs      float64 `json:"p95_query_ms"`
	P99QueryMs      float64 `json:"p99_query_ms"`
	P50ListMs       float64 `json:"p50_list_ms"`
	P95ListMs       float64 `json:"p95_list_ms"`
	P99ListMs       float64 `json:"p99_list_ms"`
	TotalDurationMs float64 `json:"total_duration_ms"`
}

var benchCmd = &cobra.Command{
	Use:     "bench",
	GroupID: GroupDiag,
	Short:   "Benchmark bead operations against Dolt",
	Long: `Benchmark bead create, query, and list operations against the Dolt backend.

Runs N iterations of each operation and reports p50/p95/p99 latency.
Results are printed as a table and, with --json, as machine-readable JSON.
A wide event with the results is emitted to the "bench" channel.

Use --dry-run to run with synthetic latency data (no actual bd calls).
This is useful for CI/testing environments without a Dolt server.

Examples:
  gt bench                        # 100 iterations, table output
  gt bench --iterations 50        # 50 iterations
  gt bench --json                 # JSON report
  gt bench --dry-run              # synthetic data, no bd calls
  gt bench --dry-run --json       # JSON report with synthetic data`,
	RunE: runBench,
}

func init() {
	benchCmd.Flags().IntVarP(&benchIterations, "iterations", "n", 100,
		"Number of iterations per operation")
	benchCmd.Flags().BoolVar(&benchDryRun, "dry-run", false,
		"Use synthetic latency data (no actual bd calls)")
	benchCmd.Flags().BoolVar(&benchJSON, "json", false,
		"Output results as JSON")

	// bench is exempt from beads version checks — it manages its own bd calls
	beadsExemptCommands["bench"] = true

	rootCmd.AddCommand(benchCmd)
}

// runBench executes the benchmark suite.
func runBench(cmd *cobra.Command, args []string) error {
	start := time.Now()

	var (
		createMs []float64
		queryMs  []float64
		listMs   []float64
	)

	if benchDryRun {
		createMs = syntheticLatencies(benchIterations, 2.0, 15.0)
		queryMs = syntheticLatencies(benchIterations, 1.0, 8.0)
		listMs = syntheticLatencies(benchIterations, 3.0, 20.0)
	} else {
		// Resolve a working directory for bd commands.
		// Prefer the workspace root; fall back to the current directory.
		workDir := ""
		if root, err := workspace.FindFromCwd(); err == nil && root != "" {
			workDir = root
		}

		// --- CREATE benchmark ---
		var createdIDs []string
		for i := 0; i < benchIterations; i++ {
			title := fmt.Sprintf("bench-create-%d-%d", time.Now().UnixNano(), i)
			t0 := time.Now()
			out, err := BdCmd("create",
				"--title", title,
				"--type", "task",
				"--silent",
			).Dir(workDir).Output()
			elapsed := time.Since(t0).Seconds() * 1000
			if err != nil {
				return fmt.Errorf("bd create iteration %d: %w", i, err)
			}
			createMs = append(createMs, elapsed)
			id := strings.TrimSpace(string(out))
			if id != "" {
				createdIDs = append(createdIDs, id)
			}
		}

		// --- QUERY benchmark ---
		// Use the IDs we just created; fall back to synthetic IDs if none were returned.
		queryIDs := createdIDs
		if len(queryIDs) == 0 {
			// Try listing to get some real IDs to query.
			out, err := BdCmd("list", "--json", "--limit", "100").Dir(workDir).Output()
			if err == nil {
				var beads []struct{ ID string `json:"id"` }
				if json.Unmarshal(out, &beads) == nil {
					for _, b := range beads {
						queryIDs = append(queryIDs, b.ID)
					}
				}
			}
		}

		for i := 0; i < benchIterations; i++ {
			var id string
			if len(queryIDs) > 0 {
				id = queryIDs[i%len(queryIDs)]
			} else {
				// No IDs available — measure a failed show (still exercises the path)
				id = "nonexistent-bench-id"
			}
			t0 := time.Now()
			// Ignore errors — we want to measure the round-trip regardless.
			_ = BdCmd("show", id, "--json").Dir(workDir).Run()
			queryMs = append(queryMs, time.Since(t0).Seconds()*1000)
		}

		// --- LIST benchmark ---
		for i := 0; i < benchIterations; i++ {
			t0 := time.Now()
			_ = BdCmd("list").Dir(workDir).Run()
			listMs = append(listMs, time.Since(t0).Seconds()*1000)
		}

		// Clean up bench beads (best-effort, don't fail if cleanup errors).
		for _, id := range createdIDs {
			_ = BdCmd("close", id, "--reason", "bench cleanup").Dir(workDir).Run()
		}
	}

	totalMs := time.Since(start).Seconds() * 1000

	result := BenchResult{
		Iterations:      benchIterations,
		P50CreateMs:     benchPercentile(createMs, 0.50),
		P95CreateMs:     benchPercentile(createMs, 0.95),
		P99CreateMs:     benchPercentile(createMs, 0.99),
		P50QueryMs:      benchPercentile(queryMs, 0.50),
		P95QueryMs:      benchPercentile(queryMs, 0.95),
		P99QueryMs:      benchPercentile(queryMs, 0.99),
		P50ListMs:       benchPercentile(listMs, 0.50),
		P95ListMs:       benchPercentile(listMs, 0.95),
		P99ListMs:       benchPercentile(listMs, 0.99),
		TotalDurationMs: totalMs,
	}

	// Emit wide event (best-effort; skip if no workspace or channel error).
	_ = emitBenchEvent(result)

	out := cmd.OutOrStdout()

	if benchJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Print table.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Bench results (%d iterations)\n\n", benchIterations))
	b.WriteString(fmt.Sprintf("%-12s %10s %10s %10s\n", "Operation", "p50 (ms)", "p95 (ms)", "p99 (ms)"))
	b.WriteString(strings.Repeat("-", 46) + "\n")
	b.WriteString(fmt.Sprintf("%-12s %10.2f %10.2f %10.2f\n", "create",
		result.P50CreateMs, result.P95CreateMs, result.P99CreateMs))
	b.WriteString(fmt.Sprintf("%-12s %10.2f %10.2f %10.2f\n", "query",
		result.P50QueryMs, result.P95QueryMs, result.P99QueryMs))
	b.WriteString(fmt.Sprintf("%-12s %10.2f %10.2f %10.2f\n", "list",
		result.P50ListMs, result.P95ListMs, result.P99ListMs))
	b.WriteString(fmt.Sprintf("\nTotal wall time: %.0f ms\n", result.TotalDurationMs))
	_, err := fmt.Fprint(out, b.String())
	return err
}

// benchPercentile returns the p-th percentile (0.0–1.0) of the samples using
// the nearest-rank (ceiling) method. For p=0.5 on [1,2,...,10] this returns 5
// (the 5th element, 1-indexed). Returns 0 for an empty slice.
func benchPercentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	n := len(sorted)
	// Nearest-rank ceiling: rank = ceil(p * n), index = rank - 1.
	// This matches the intuitive definition: p50 of N items returns the item
	// at position ceil(N/2).
	rank := int(p*float64(n) + 0.9999) // ceil via add-then-truncate
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// syntheticLatencies generates N latency values drawn from a uniform distribution
// between minMs and maxMs. Used for --dry-run so tests can run without Dolt.
func syntheticLatencies(n int, minMs, maxMs float64) []float64 {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // non-crypto use
	out := make([]float64, n)
	for i := range out {
		out[i] = minMs + rng.Float64()*(maxMs-minMs)
	}
	return out
}

// emitBenchEvent emits a wide event with bench results to the "bench" channel.
// Errors are silently ignored (best-effort).
func emitBenchEvent(r BenchResult) error {
	// Verify bd is installed — skip silently if not available.
	if _, err := exec.LookPath("bd"); err != nil {
		return nil
	}

	payload := []string{
		fmt.Sprintf("event_type=bench.complete"),
		fmt.Sprintf("iterations=%d", r.Iterations),
		fmt.Sprintf("p50_create_ms=%.2f", r.P50CreateMs),
		fmt.Sprintf("p95_create_ms=%.2f", r.P95CreateMs),
		fmt.Sprintf("p99_create_ms=%.2f", r.P99CreateMs),
		fmt.Sprintf("p50_query_ms=%.2f", r.P50QueryMs),
		fmt.Sprintf("p95_query_ms=%.2f", r.P95QueryMs),
		fmt.Sprintf("p99_query_ms=%.2f", r.P99QueryMs),
		fmt.Sprintf("p50_list_ms=%.2f", r.P50ListMs),
		fmt.Sprintf("p95_list_ms=%.2f", r.P95ListMs),
		fmt.Sprintf("p99_list_ms=%.2f", r.P99ListMs),
		fmt.Sprintf("total_duration_ms=%.0f", r.TotalDurationMs),
	}

	_, err := channelevents.Emit("bench", "bench.complete", payload)

	return err
}
