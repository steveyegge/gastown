package guardian

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestStateFilePath(t *testing.T) {
	path := StateFilePath("/home/user/gt")
	expected := filepath.Join("/home/user/gt", "guardian", "judgment-state.json")
	if path != expected {
		t.Errorf("StateFilePath = %q, want %q", path, expected)
	}
}

func TestLoadState_NotExist(t *testing.T) {
	state, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Workers) != 0 {
		t.Errorf("expected empty workers, got %d", len(state.Workers))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	state := NewGuardianState()
	state.AddResult("polecat-Toast", &GuardianResult{
		BeadID:         "gt-abc123",
		Score:          0.85,
		Recommendation: "approve",
		Issues:         []GuardianIssue{{Severity: "minor", Category: "style", Description: "nit"}},
		ReviewedAt:     time.Now(),
		Worker:         "polecat-Toast",
	})
	state.AddResult("polecat-Toast", &GuardianResult{
		BeadID:         "gt-def456",
		Score:          0.40,
		Recommendation: "request_changes",
		ReviewedAt:     time.Now(),
		Worker:         "polecat-Toast",
	})

	if err := SaveState(dir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	pj, ok := loaded.Workers["polecat-Toast"]
	if !ok {
		t.Fatal("expected polecat-Toast in workers")
	}
	if pj.TotalReviews != 2 {
		t.Errorf("TotalReviews = %d, want 2", pj.TotalReviews)
	}
	if len(pj.RecentResults) != 2 {
		t.Errorf("RecentResults = %d, want 2", len(pj.RecentResults))
	}
}

func TestAddResult_Aggregates(t *testing.T) {
	state := NewGuardianState()

	// Add 3 results: 0.9 approve, 0.3 request_changes, 0.6 approve
	state.AddResult("toast", &GuardianResult{Score: 0.9, Recommendation: "approve", Worker: "toast"})
	state.AddResult("toast", &GuardianResult{Score: 0.3, Recommendation: "request_changes", Worker: "toast"})
	state.AddResult("toast", &GuardianResult{Score: 0.6, Recommendation: "approve", Worker: "toast"})

	pj := state.Workers["toast"]

	// Avg score: (0.9 + 0.3 + 0.6) / 3 = 0.6
	if pj.AvgScore < 0.59 || pj.AvgScore > 0.61 {
		t.Errorf("AvgScore = %f, want ~0.6", pj.AvgScore)
	}

	// Rejection rate: 1/3 ≈ 0.333
	if pj.RejectionRate < 0.32 || pj.RejectionRate > 0.34 {
		t.Errorf("RejectionRate = %f, want ~0.333", pj.RejectionRate)
	}
}

func TestAddResult_RingBufferCap(t *testing.T) {
	state := NewGuardianState()

	// Add more than MaxRecentResults
	for i := 0; i < MaxRecentResults+10; i++ {
		state.AddResult("toast", &GuardianResult{
			BeadID: "gt-test",
			Score:  0.5,
			Worker: "toast",
		})
	}

	pj := state.Workers["toast"]
	if len(pj.RecentResults) != MaxRecentResults {
		t.Errorf("RecentResults = %d, want %d (capped)", len(pj.RecentResults), MaxRecentResults)
	}
	if pj.TotalReviews != MaxRecentResults+10 {
		t.Errorf("TotalReviews = %d, want %d", pj.TotalReviews, MaxRecentResults+10)
	}
}

func TestRecomputeAggregates_Empty(t *testing.T) {
	pj := &PolecatJudgment{}
	pj.recomputeAggregates()

	if pj.AvgScore != 0 {
		t.Errorf("AvgScore = %f, want 0", pj.AvgScore)
	}
	if pj.RejectionRate != 0 {
		t.Errorf("RejectionRate = %f, want 0", pj.RejectionRate)
	}
}

func TestSaveState_Atomic(t *testing.T) {
	dir := t.TempDir()

	state := NewGuardianState()
	state.AddResult("toast", &GuardianResult{Score: 0.8, Recommendation: "approve", Worker: "toast"})

	if err := SaveState(dir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify file exists and is valid
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded.Workers) != 1 {
		t.Errorf("expected 1 worker, got %d", len(loaded.Workers))
	}
}

func TestConcurrentSaveState(t *testing.T) {
	dir := t.TempDir()

	// Run multiple concurrent saves to detect race conditions
	const goroutines = 10
	errc := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			state := NewGuardianState()
			state.AddResult("toast", &GuardianResult{
				BeadID: fmt.Sprintf("gt-test-%d", n),
				Score:  0.5,
				Worker: "toast",
			})
			errc <- SaveState(dir, state)
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errc; err != nil {
			t.Errorf("concurrent SaveState error: %v", err)
		}
	}

	// Verify file is valid JSON after concurrent writes
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState after concurrent writes: %v", err)
	}
	if loaded.Workers == nil {
		t.Fatal("expected non-nil workers")
	}
}

func TestMultipleWorkers(t *testing.T) {
	state := NewGuardianState()

	state.AddResult("toast", &GuardianResult{Score: 0.9, Recommendation: "approve", Worker: "toast"})
	state.AddResult("max", &GuardianResult{Score: 0.3, Recommendation: "request_changes", Worker: "max"})

	if len(state.Workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(state.Workers))
	}

	toast := state.Workers["toast"]
	if toast.AvgScore != 0.9 {
		t.Errorf("toast AvgScore = %f, want 0.9", toast.AvgScore)
	}

	max := state.Workers["max"]
	if max.RejectionRate != 1.0 {
		t.Errorf("max RejectionRate = %f, want 1.0", max.RejectionRate)
	}
}
