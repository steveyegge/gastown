package krc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoPruneState_ShouldPrune(t *testing.T) {
	tests := []struct {
		name          string
		lastPruneTime time.Time
		interval      time.Duration
		want          bool
	}{
		{"never pruned", time.Time{}, 1 * time.Hour, true},
		{"interval elapsed", time.Now().Add(-2 * time.Hour), 1 * time.Hour, true},
		{"interval not elapsed", time.Now().Add(-30 * time.Minute), 1 * time.Hour, false},
		{"exactly at interval", time.Now().Add(-1 * time.Hour), 1 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &AutoPruneState{LastPruneTime: tt.lastPruneTime}
			if got := state.ShouldPrune(tt.interval); got != tt.want {
				t.Errorf("ShouldPrune(%v) = %v, want %v", tt.interval, got, tt.want)
			}
		})
	}
}

func TestAutoPruneState_TimeSinceLastPrune(t *testing.T) {
	state := &AutoPruneState{}
	if got := state.TimeSinceLastPrune(); got != -1 {
		t.Errorf("TimeSinceLastPrune for never-pruned = %v, want -1", got)
	}

	state.LastPruneTime = time.Now().Add(-5 * time.Minute)
	got := state.TimeSinceLastPrune()
	if got < 4*time.Minute || got > 6*time.Minute {
		t.Errorf("TimeSinceLastPrune = %v, want ~5m", got)
	}
}

func TestAutoPruneState_RecordPrune(t *testing.T) {
	state := &AutoPruneState{}

	result := &PruneResult{
		EventsProcessed: 100,
		EventsPruned:    20,
		EventsRetained:  80,
		BytesBefore:     10000,
		BytesAfter:      8000,
	}

	state.RecordPrune(result)

	if state.PruneCount != 1 {
		t.Errorf("PruneCount = %d, want 1", state.PruneCount)
	}
	if state.TotalPruned != 20 {
		t.Errorf("TotalPruned = %d, want 20", state.TotalPruned)
	}
	if state.TotalBytesFreed != 2000 {
		t.Errorf("TotalBytesFreed = %d, want 2000", state.TotalBytesFreed)
	}
	if state.LastResult != result {
		t.Error("LastResult not set")
	}

	// Record another
	result2 := &PruneResult{
		EventsPruned: 10,
		BytesBefore:  8000,
		BytesAfter:   7000,
	}
	state.RecordPrune(result2)

	if state.PruneCount != 2 {
		t.Errorf("PruneCount = %d, want 2", state.PruneCount)
	}
	if state.TotalPruned != 30 {
		t.Errorf("TotalPruned = %d, want 30", state.TotalPruned)
	}
	if state.TotalBytesFreed != 3000 {
		t.Errorf("TotalBytesFreed = %d, want 3000", state.TotalBytesFreed)
	}
}

func TestSaveAndLoadAutoPruneState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krc-autoprune-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	state := &AutoPruneState{
		LastPruneTime: time.Now().UTC().Truncate(time.Second),
		PruneCount:    5,
		TotalPruned:   100,
		TotalBytesFreed: 50000,
		LastResult: &PruneResult{
			EventsPruned: 20,
		},
	}

	if err := SaveAutoPruneState(tmpDir, state); err != nil {
		t.Fatalf("SaveAutoPruneState failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(tmpDir, AutoPruneStateFile)); err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	loaded, err := LoadAutoPruneState(tmpDir)
	if err != nil {
		t.Fatalf("LoadAutoPruneState failed: %v", err)
	}

	if !loaded.LastPruneTime.Equal(state.LastPruneTime) {
		t.Errorf("LastPruneTime = %v, want %v", loaded.LastPruneTime, state.LastPruneTime)
	}
	if loaded.PruneCount != state.PruneCount {
		t.Errorf("PruneCount = %d, want %d", loaded.PruneCount, state.PruneCount)
	}
	if loaded.TotalPruned != state.TotalPruned {
		t.Errorf("TotalPruned = %d, want %d", loaded.TotalPruned, state.TotalPruned)
	}
}

func TestLoadAutoPruneState_NoFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krc-autoprune-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	state, err := LoadAutoPruneState(tmpDir)
	if err != nil {
		t.Fatalf("LoadAutoPruneState failed: %v", err)
	}

	if !state.LastPruneTime.IsZero() {
		t.Errorf("expected zero LastPruneTime for new state")
	}
	if state.PruneCount != 0 {
		t.Errorf("expected zero PruneCount for new state")
	}
}
