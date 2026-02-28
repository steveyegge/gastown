package guardian

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStateFilePath(t *testing.T) {
	got := StateFilePath("/tmp/mytown")
	want := filepath.Join("/tmp/mytown", "guardian", "judgment-state.json")
	if got != want {
		t.Errorf("StateFilePath = %q, want %q", got, want)
	}
}

func TestLoadState_NotExist(t *testing.T) {
	state, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Workers) != 0 {
		t.Errorf("expected empty workers, got %d", len(state.Workers))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	townRoot := t.TempDir()

	state := NewGuardianState()
	state.AddResult("polecat-Toast", RecentResult{
		BeadID:         "gt-abc123",
		Score:          0.85,
		Recommendation: RecommendApprove,
		IssueCount:     1,
		ReviewedAt:     time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	})

	if err := SaveState(townRoot, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(townRoot)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	pj, ok := loaded.Workers["polecat-Toast"]
	if !ok {
		t.Fatal("expected polecat-Toast in workers")
	}
	if pj.TotalReviews != 1 {
		t.Errorf("TotalReviews = %d, want 1", pj.TotalReviews)
	}
	if pj.AvgScore != 0.85 {
		t.Errorf("AvgScore = %f, want 0.85", pj.AvgScore)
	}
	if len(pj.RecentResults) != 1 {
		t.Errorf("RecentResults len = %d, want 1", len(pj.RecentResults))
	}
	if pj.RecentResults[0].BeadID != "gt-abc123" {
		t.Errorf("BeadID = %q, want %q", pj.RecentResults[0].BeadID, "gt-abc123")
	}
}

func TestAddResult_Aggregates(t *testing.T) {
	state := NewGuardianState()

	// Add an approve with score 0.90
	state.AddResult("w1", RecentResult{
		Score:          0.90,
		Recommendation: RecommendApprove,
	})
	// Add a rejection with score 0.30
	state.AddResult("w1", RecentResult{
		Score:          0.30,
		Recommendation: RecommendRequestChanges,
	})

	pj := state.Workers["w1"]
	if pj.TotalReviews != 2 {
		t.Errorf("TotalReviews = %d, want 2", pj.TotalReviews)
	}

	wantAvg := 0.60
	if diff := pj.AvgScore - wantAvg; diff > 0.001 || diff < -0.001 {
		t.Errorf("AvgScore = %f, want %f", pj.AvgScore, wantAvg)
	}

	wantRej := 0.50
	if diff := pj.RejectionRate - wantRej; diff > 0.001 || diff < -0.001 {
		t.Errorf("RejectionRate = %f, want %f", pj.RejectionRate, wantRej)
	}
}

func TestAddResult_RingBufferCap(t *testing.T) {
	state := NewGuardianState()

	for i := 0; i < MaxRecentResults+20; i++ {
		state.AddResult("w1", RecentResult{
			BeadID: "gt-" + string(rune('A'+i%26)),
			Score:  0.80,
		})
	}

	pj := state.Workers["w1"]
	if len(pj.RecentResults) != MaxRecentResults {
		t.Errorf("RecentResults len = %d, want %d", len(pj.RecentResults), MaxRecentResults)
	}
	if pj.TotalReviews != MaxRecentResults+20 {
		t.Errorf("TotalReviews = %d, want %d", pj.TotalReviews, MaxRecentResults+20)
	}
}

func TestRecomputeAggregates_Empty(t *testing.T) {
	pj := &PolecatJudgment{}
	recomputeAggregates(pj)
	if pj.AvgScore != 0 {
		t.Errorf("AvgScore = %f, want 0", pj.AvgScore)
	}
	if pj.RejectionRate != 0 {
		t.Errorf("RejectionRate = %f, want 0", pj.RejectionRate)
	}
}

func TestSaveState_Atomic(t *testing.T) {
	townRoot := t.TempDir()

	state := NewGuardianState()
	state.AddResult("w1", RecentResult{Score: 0.75})

	if err := SaveState(townRoot, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify the state file exists and the temp file does not.
	path := StateFilePath(townRoot)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file should exist: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}
}

func TestConcurrentSaveState(t *testing.T) {
	townRoot := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			state := NewGuardianState()
			state.AddResult("w1", RecentResult{Score: float64(n) / 10.0})
			_ = SaveState(townRoot, state)
		}(i)
	}
	wg.Wait()

	// Verify we can still load a valid state after concurrent writes.
	loaded, err := LoadState(townRoot)
	if err != nil {
		t.Fatalf("LoadState after concurrent saves: %v", err)
	}
	if loaded.Workers == nil {
		t.Fatal("expected non-nil workers")
	}
}

func TestMultipleWorkers(t *testing.T) {
	state := NewGuardianState()

	state.AddResult("polecat-A", RecentResult{Score: 0.90, Recommendation: RecommendApprove})
	state.AddResult("polecat-B", RecentResult{Score: 0.40, Recommendation: RecommendRequestChanges})
	state.AddResult("polecat-B", RecentResult{Score: 0.50, Recommendation: RecommendApprove})

	if len(state.Workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(state.Workers))
	}

	a := state.Workers["polecat-A"]
	if a.AvgScore != 0.90 {
		t.Errorf("polecat-A AvgScore = %f, want 0.90", a.AvgScore)
	}

	b := state.Workers["polecat-B"]
	wantAvg := 0.45
	if diff := b.AvgScore - wantAvg; diff > 0.001 || diff < -0.001 {
		t.Errorf("polecat-B AvgScore = %f, want %f", b.AvgScore, wantAvg)
	}
	wantRej := 0.50
	if diff := b.RejectionRate - wantRej; diff > 0.001 || diff < -0.001 {
		t.Errorf("polecat-B RejectionRate = %f, want %f", b.RejectionRate, wantRej)
	}
}
