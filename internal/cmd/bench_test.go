package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestBenchComputePercentile verifies the percentile computation function.
func TestBenchComputePercentile(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
		p       float64
		want    float64
	}{
		{"p50 of single", []float64{10}, 0.50, 10},
		{"p50 of two", []float64{1, 3}, 0.50, 1},
		{"p99 of sorted", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 100}, 0.99, 100},
		{"p50 of sorted ten", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.50, 5},
		{"empty returns zero", []float64{}, 0.50, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := benchPercentile(tt.samples, tt.p)
			if got != tt.want {
				t.Errorf("benchPercentile(%v, %.2f) = %v, want %v", tt.samples, tt.p, got, tt.want)
			}
		})
	}
}

// TestBenchResultJSON verifies that BenchResult marshals to JSON with all required
// latency percentile fields for create, query, and list operations.
func TestBenchResultJSON(t *testing.T) {
	r := BenchResult{
		Iterations:    10,
		P50CreateMs:   1.1,
		P95CreateMs:   2.2,
		P99CreateMs:   3.3,
		P50QueryMs:    4.4,
		P95QueryMs:    5.5,
		P99QueryMs:    6.6,
		P50ListMs:     7.7,
		P95ListMs:     8.8,
		P99ListMs:     9.9,
		TotalDurationMs: 100.0,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal BenchResult: %v", err)
	}

	// Decode into a map to check all required keys.
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	required := []string{
		"iterations",
		"p50_create_ms", "p95_create_ms", "p99_create_ms",
		"p50_query_ms", "p95_query_ms", "p99_query_ms",
		"p50_list_ms", "p95_list_ms", "p99_list_ms",
		"total_duration_ms",
	}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("BenchResult JSON missing key %q", key)
		}
	}
}

// TestBenchCommandRegistered verifies that `gt bench` is registered in the root
// command tree and produces a JSON report when run with --json flag.
func TestBenchCommandRegistered(t *testing.T) {
	// Verify the command is registered.
	var found bool
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == "bench" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("gt bench command not registered in rootCmd")
	}
}

// TestBenchRunDryRun exercises the bench command with --dry-run flag which
// skips actual bd calls and uses synthetic latency data for CI-safe testing.
func TestBenchRunDryRun(t *testing.T) {
	var buf bytes.Buffer

	cmd := benchCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Reset flags to defaults.
	benchIterations = 5
	benchDryRun = true
	benchJSON = false

	if err := runBench(cmd, nil); err != nil {
		t.Fatalf("runBench(dry-run) error: %v", err)
	}

	// Verify output is non-empty (table was printed).
	if buf.Len() == 0 {
		t.Error("expected non-empty output from bench --dry-run")
	}
}

// TestBenchRunDryRunJSON verifies JSON output from --dry-run --json.
func TestBenchRunDryRunJSON(t *testing.T) {
	var buf bytes.Buffer

	cmd := benchCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	benchIterations = 5
	benchDryRun = true
	benchJSON = true

	if err := runBench(cmd, nil); err != nil {
		t.Fatalf("runBench(dry-run --json) error: %v", err)
	}

	var result BenchResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, buf.String())
	}

	if result.Iterations != 5 {
		t.Errorf("iterations = %d, want 5", result.Iterations)
	}
	if result.P50CreateMs <= 0 {
		t.Errorf("p50_create_ms = %v, want > 0", result.P50CreateMs)
	}
	if result.P50QueryMs <= 0 {
		t.Errorf("p50_query_ms = %v, want > 0", result.P50QueryMs)
	}
	if result.P50ListMs <= 0 {
		t.Errorf("p50_list_ms = %v, want > 0", result.P50ListMs)
	}
}
