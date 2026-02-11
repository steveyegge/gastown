package deacon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetPauseFile(t *testing.T) {
	townRoot := "/tmp/test-town"
	expected := filepath.Join(townRoot, ".runtime", "deacon", "paused.json")

	result := GetPauseFile(townRoot)
	if result != expected {
		t.Errorf("GetPauseFile() = %q, want %q", result, expected)
	}
}

func TestIsPaused_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	paused, state, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() error = %v", err)
	}
	if paused {
		t.Error("IsPaused() should return false when file doesn't exist")
	}
	if state != nil {
		t.Error("IsPaused() should return nil state when file doesn't exist")
	}
}

func TestIsPaused_ValidPaused(t *testing.T) {
	tmpDir := t.TempDir()

	// Create pause file with paused=true
	if err := Pause(tmpDir, "maintenance", "human"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	paused, state, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() error = %v", err)
	}
	if !paused {
		t.Error("IsPaused() should return true when paused")
	}
	if state == nil {
		t.Fatal("IsPaused() should return non-nil state when paused")
	}
	if !state.Paused {
		t.Error("state.Paused should be true")
	}
	if state.Reason != "maintenance" {
		t.Errorf("state.Reason = %q, want %q", state.Reason, "maintenance")
	}
	if state.PausedBy != "human" {
		t.Errorf("state.PausedBy = %q, want %q", state.PausedBy, "human")
	}
	if state.PausedAt.IsZero() {
		t.Error("state.PausedAt should not be zero")
	}
}

func TestIsPaused_ValidNotPaused(t *testing.T) {
	tmpDir := t.TempDir()
	pauseFile := GetPauseFile(tmpDir)

	// Create pause file with paused=false
	if err := os.MkdirAll(filepath.Dir(pauseFile), 0755); err != nil {
		t.Fatal(err)
	}
	state := PauseState{Paused: false, Reason: "test"}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(pauseFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	paused, ps, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() error = %v", err)
	}
	if paused {
		t.Error("IsPaused() should return false when paused=false in file")
	}
	if ps == nil {
		t.Fatal("IsPaused() should return non-nil state when file exists")
	}
	if ps.Paused {
		t.Error("state.Paused should be false")
	}
}

func TestIsPaused_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	pauseFile := GetPauseFile(tmpDir)

	// Create pause file with corrupt JSON
	if err := os.MkdirAll(filepath.Dir(pauseFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pauseFile, []byte("{not valid json!!!"), 0600); err != nil {
		t.Fatal(err)
	}

	paused, state, err := IsPaused(tmpDir)
	if err == nil {
		t.Fatal("IsPaused() should return error for corrupt JSON")
	}
	if paused {
		t.Error("IsPaused() should return false on error")
	}
	if state != nil {
		t.Error("IsPaused() should return nil state on error")
	}
}

func TestIsPaused_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	pauseFile := GetPauseFile(tmpDir)

	// Create a directory where the file should be to cause a read error
	if err := os.MkdirAll(pauseFile, 0755); err != nil {
		t.Fatal(err)
	}

	paused, state, err := IsPaused(tmpDir)
	if err == nil {
		t.Fatal("IsPaused() should return error when file can't be read")
	}
	if paused {
		t.Error("IsPaused() should return false on error")
	}
	if state != nil {
		t.Error("IsPaused() should return nil state on error")
	}
}

func TestIsPaused_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	pauseFile := GetPauseFile(tmpDir)

	// Create empty pause file
	if err := os.MkdirAll(filepath.Dir(pauseFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pauseFile, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	paused, state, err := IsPaused(tmpDir)
	if err == nil {
		t.Fatal("IsPaused() should return error for empty file")
	}
	if paused {
		t.Error("IsPaused() should return false on error")
	}
	if state != nil {
		t.Error("IsPaused() should return nil state on error")
	}
}

func TestPause_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := Pause(tmpDir, "testing", "test-runner"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	pauseFile := GetPauseFile(tmpDir)
	if _, err := os.Stat(pauseFile); os.IsNotExist(err) {
		t.Fatal("Pause() should create the pause file")
	}
}

func TestPause_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	pauseFile := GetPauseFile(tmpDir)
	pauseDir := filepath.Dir(pauseFile)

	// Verify directory doesn't exist
	if _, err := os.Stat(pauseDir); !os.IsNotExist(err) {
		t.Fatal("pause directory should not exist initially")
	}

	if err := Pause(tmpDir, "test", "tester"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(pauseDir)
	if err != nil {
		t.Fatalf("pause directory should exist after Pause(): %v", err)
	}
	if !info.IsDir() {
		t.Error("pause directory should be a directory")
	}
}

func TestPause_FileContent(t *testing.T) {
	tmpDir := t.TempDir()

	if err := Pause(tmpDir, "maintenance window", "mayor"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	pauseFile := GetPauseFile(tmpDir)
	data, err := os.ReadFile(pauseFile)
	if err != nil {
		t.Fatalf("reading pause file: %v", err)
	}

	var state PauseState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("pause file should be valid JSON: %v", err)
	}
	if !state.Paused {
		t.Error("state.Paused should be true")
	}
	if state.Reason != "maintenance window" {
		t.Errorf("state.Reason = %q, want %q", state.Reason, "maintenance window")
	}
	if state.PausedBy != "mayor" {
		t.Errorf("state.PausedBy = %q, want %q", state.PausedBy, "mayor")
	}
	if state.PausedAt.IsZero() {
		t.Error("state.PausedAt should not be zero")
	}
}

func TestPause_EmptyReasonAndPausedBy(t *testing.T) {
	tmpDir := t.TempDir()

	if err := Pause(tmpDir, "", ""); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	paused, state, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() error = %v", err)
	}
	if !paused {
		t.Error("should be paused even with empty reason/pausedBy")
	}
	if state.Reason != "" {
		t.Errorf("state.Reason = %q, want empty", state.Reason)
	}
	if state.PausedBy != "" {
		t.Errorf("state.PausedBy = %q, want empty", state.PausedBy)
	}
}

func TestResume_RemovesFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Pause first
	if err := Pause(tmpDir, "test", "tester"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	// Verify file exists
	pauseFile := GetPauseFile(tmpDir)
	if _, err := os.Stat(pauseFile); os.IsNotExist(err) {
		t.Fatal("pause file should exist after Pause()")
	}

	// Resume
	if err := Resume(tmpDir); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(pauseFile); !os.IsNotExist(err) {
		t.Error("pause file should be removed after Resume()")
	}
}

func TestResume_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Resume when no pause file exists should not error
	err := Resume(tmpDir)
	if err != nil {
		t.Errorf("Resume() error = %v, should succeed when no file exists", err)
	}
}

func TestPauseResumeRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially not paused
	paused, _, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("initial IsPaused() error = %v", err)
	}
	if paused {
		t.Error("should not be paused initially")
	}

	// Pause
	if err := Pause(tmpDir, "round-trip test", "tester"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	// Verify paused
	paused, state, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() after Pause error = %v", err)
	}
	if !paused {
		t.Error("should be paused after Pause()")
	}
	if state.Reason != "round-trip test" {
		t.Errorf("state.Reason = %q, want %q", state.Reason, "round-trip test")
	}

	// Resume
	if err := Resume(tmpDir); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	// Verify not paused
	paused, _, err = IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() after Resume error = %v", err)
	}
	if paused {
		t.Error("should not be paused after Resume()")
	}
}

func TestPause_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Pause with first reason
	if err := Pause(tmpDir, "reason1", "user1"); err != nil {
		t.Fatalf("first Pause() error = %v", err)
	}

	// Pause again with different reason (should overwrite)
	if err := Pause(tmpDir, "reason2", "user2"); err != nil {
		t.Fatalf("second Pause() error = %v", err)
	}

	_, state, err := IsPaused(tmpDir)
	if err != nil {
		t.Fatalf("IsPaused() error = %v", err)
	}
	if state.Reason != "reason2" {
		t.Errorf("state.Reason = %q, want %q (should overwrite)", state.Reason, "reason2")
	}
	if state.PausedBy != "user2" {
		t.Errorf("state.PausedBy = %q, want %q (should overwrite)", state.PausedBy, "user2")
	}
}
