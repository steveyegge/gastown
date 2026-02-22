package capacity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState with missing file: %v", err)
	}
	if state.Paused {
		t.Error("expected Paused=false for missing file")
	}
	if state.PausedBy != "" {
		t.Errorf("expected empty PausedBy, got %q", state.PausedBy)
	}
	if state.LastDispatchAt != "" {
		t.Errorf("expected empty LastDispatchAt, got %q", state.LastDispatchAt)
	}
	if state.LastDispatchCount != 0 {
		t.Errorf("expected LastDispatchCount=0, got %d", state.LastDispatchCount)
	}
}

func TestSaveAndLoadState_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	original := &SchedulerState{
		Paused:            true,
		PausedBy:          "test-user",
		PausedAt:          "2026-01-15T10:00:00Z",
		LastDispatchAt:    "2026-01-15T09:30:00Z",
		LastDispatchCount: 3,
	}

	if err := SaveState(tmpDir, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if loaded.Paused != original.Paused {
		t.Errorf("Paused: got %v, want %v", loaded.Paused, original.Paused)
	}
	if loaded.PausedBy != original.PausedBy {
		t.Errorf("PausedBy: got %q, want %q", loaded.PausedBy, original.PausedBy)
	}
	if loaded.PausedAt != original.PausedAt {
		t.Errorf("PausedAt: got %q, want %q", loaded.PausedAt, original.PausedAt)
	}
	if loaded.LastDispatchAt != original.LastDispatchAt {
		t.Errorf("LastDispatchAt: got %q, want %q", loaded.LastDispatchAt, original.LastDispatchAt)
	}
	if loaded.LastDispatchCount != original.LastDispatchCount {
		t.Errorf("LastDispatchCount: got %d, want %d", loaded.LastDispatchCount, original.LastDispatchCount)
	}
}

func TestSetPaused(t *testing.T) {
	state := &SchedulerState{}

	before := time.Now().UTC()
	state.SetPaused("admin")
	after := time.Now().UTC()

	if !state.Paused {
		t.Error("expected Paused=true after SetPaused")
	}
	if state.PausedBy != "admin" {
		t.Errorf("PausedBy: got %q, want %q", state.PausedBy, "admin")
	}

	ts, err := time.Parse(time.RFC3339, state.PausedAt)
	if err != nil {
		t.Fatalf("PausedAt is not valid RFC3339: %q, err: %v", state.PausedAt, err)
	}
	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("PausedAt %v not between %v and %v", ts, before, after)
	}
}

func TestSetResumed(t *testing.T) {
	state := &SchedulerState{
		Paused:   true,
		PausedBy: "admin",
		PausedAt: "2026-01-15T10:00:00Z",
	}

	state.SetResumed()

	if state.Paused {
		t.Error("expected Paused=false after SetResumed")
	}
	if state.PausedBy != "" {
		t.Errorf("PausedBy should be empty after SetResumed, got %q", state.PausedBy)
	}
	if state.PausedAt != "" {
		t.Errorf("PausedAt should be empty after SetResumed, got %q", state.PausedAt)
	}
}

func TestRecordDispatch(t *testing.T) {
	state := &SchedulerState{}

	before := time.Now().UTC()
	state.RecordDispatch(5)
	after := time.Now().UTC()

	if state.LastDispatchCount != 5 {
		t.Errorf("LastDispatchCount: got %d, want 5", state.LastDispatchCount)
	}

	ts, err := time.Parse(time.RFC3339, state.LastDispatchAt)
	if err != nil {
		t.Fatalf("LastDispatchAt is not valid RFC3339: %q, err: %v", state.LastDispatchAt, err)
	}
	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("LastDispatchAt %v not between %v and %v", ts, before, after)
	}
}

func TestSaveState_CreatesRuntimeDir(t *testing.T) {
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")

	// Confirm .runtime doesn't exist
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Fatal(".runtime should not exist before save")
	}

	state := &SchedulerState{Paused: true, PausedBy: "test"}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Confirm .runtime was created
	info, err := os.Stat(runtimeDir)
	if err != nil {
		t.Fatalf(".runtime should exist after save: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".runtime should be a directory")
	}

	// Confirm file exists in .runtime with new name
	sf := filepath.Join(runtimeDir, "scheduler-state.json")
	if _, err := os.Stat(sf); err != nil {
		t.Fatalf("scheduler-state.json should exist: %v", err)
	}
}

func TestLoadState_LegacyFallback(t *testing.T) {
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a legacy queue-state.json file
	legacyData := `{"paused": true, "paused_by": "legacy-user", "paused_at": "2026-01-15T10:00:00Z"}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "queue-state.json"), []byte(legacyData), 0644); err != nil {
		t.Fatal(err)
	}

	// LoadState should fall back to legacy path when scheduler-state.json doesn't exist
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState with legacy file: %v", err)
	}
	if !state.Paused {
		t.Error("expected Paused=true from legacy file")
	}
	if state.PausedBy != "legacy-user" {
		t.Errorf("PausedBy: got %q, want %q", state.PausedBy, "legacy-user")
	}
}
