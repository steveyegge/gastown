package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestDefaultConfig(t *testing.T) {
	townRoot := "/tmp/test-town"
	config := DefaultConfig(townRoot)

	if config.HeartbeatInterval != 5*time.Minute {
		t.Errorf("expected HeartbeatInterval 5m, got %v", config.HeartbeatInterval)
	}
	if config.TownRoot != townRoot {
		t.Errorf("expected TownRoot %q, got %q", townRoot, config.TownRoot)
	}
	if config.LogFile != filepath.Join(townRoot, "daemon", "daemon.log") {
		t.Errorf("expected LogFile in daemon dir, got %q", config.LogFile)
	}
	if config.PidFile != filepath.Join(townRoot, "daemon", "daemon.pid") {
		t.Errorf("expected PidFile in daemon dir, got %q", config.PidFile)
	}
}

func TestStateFile(t *testing.T) {
	townRoot := "/tmp/test-town"
	expected := filepath.Join(townRoot, "daemon", "state.json")
	result := StateFile(townRoot)

	if result != expected {
		t.Errorf("StateFile(%q) = %q, expected %q", townRoot, result, expected)
	}
}

func TestLoadState_NonExistent(t *testing.T) {
	// Create temp dir that doesn't have a state file
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Errorf("LoadState should not error for missing file, got %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Running {
		t.Error("expected Running=false for empty state")
	}
	if state.PID != 0 {
		t.Errorf("expected PID=0 for empty state, got %d", state.PID)
	}
}

func TestLoadState_ExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create daemon directory
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a state file
	startTime := time.Now().Truncate(time.Second)
	testState := &State{
		Running:        true,
		PID:            12345,
		StartedAt:      startTime,
		LastHeartbeat:  startTime,
		HeartbeatCount: 42,
	}

	data, err := json.MarshalIndent(testState, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load and verify
	loaded, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}
	if !loaded.Running {
		t.Error("expected Running=true")
	}
	if loaded.PID != 12345 {
		t.Errorf("expected PID=12345, got %d", loaded.PID)
	}
	if loaded.HeartbeatCount != 42 {
		t.Errorf("expected HeartbeatCount=42, got %d", loaded.HeartbeatCount)
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create daemon directory with invalid JSON
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "state.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = LoadState(tmpDir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := &State{
		Running:        true,
		PID:            9999,
		StartedAt:      time.Now(),
		LastHeartbeat:  time.Now(),
		HeartbeatCount: 100,
	}

	// SaveState should create daemon directory if needed
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	// Verify file exists
	stateFile := StateFile(tmpDir)
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("state file should exist: %v", err)
	}

	// Verify contents
	loaded, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}
	if loaded.PID != 9999 {
		t.Errorf("expected PID=9999, got %d", loaded.PID)
	}
	if loaded.HeartbeatCount != 100 {
		t.Errorf("expected HeartbeatCount=100, got %d", loaded.HeartbeatCount)
	}
}

func TestSaveLoadState_Roundtrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	original := &State{
		Running:        true,
		PID:            54321,
		StartedAt:      time.Now().Truncate(time.Second),
		LastHeartbeat:  time.Now().Truncate(time.Second),
		HeartbeatCount: 1000,
	}

	if err := SaveState(tmpDir, original); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	loaded, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}

	if loaded.Running != original.Running {
		t.Errorf("Running mismatch: got %v, want %v", loaded.Running, original.Running)
	}
	if loaded.PID != original.PID {
		t.Errorf("PID mismatch: got %d, want %d", loaded.PID, original.PID)
	}
	if loaded.HeartbeatCount != original.HeartbeatCount {
		t.Errorf("HeartbeatCount mismatch: got %d, want %d", loaded.HeartbeatCount, original.HeartbeatCount)
	}
	// Time comparison with truncation to handle JSON serialization
	if !loaded.StartedAt.Truncate(time.Second).Equal(original.StartedAt) {
		t.Errorf("StartedAt mismatch: got %v, want %v", loaded.StartedAt, original.StartedAt)
	}
}

func TestListPolecatWorktrees_SkipsHiddenDirs(t *testing.T) {
	tmpDir := t.TempDir()
	polecatsDir := filepath.Join(tmpDir, "some-rig", "polecats")

	if err := os.MkdirAll(filepath.Join(polecatsDir, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(polecatsDir, "furiosa"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatsDir, "not-a-dir.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	polecats, err := listPolecatWorktrees(polecatsDir)
	if err != nil {
		t.Fatalf("listPolecatWorktrees returned error: %v", err)
	}

	if slices.Contains(polecats, ".claude") {
		t.Fatalf("expected hidden dir .claude to be ignored, got %v", polecats)
	}
	if !slices.Contains(polecats, "furiosa") {
		t.Fatalf("expected furiosa to be included, got %v", polecats)
	}
}

// NOTE: TestIsWitnessSession removed - isWitnessSession function was deleted
// as part of ZFC cleanup. Witness poking is now Deacon's responsibility.

func TestLifecycleAction_Constants(t *testing.T) {
	// Verify constants have expected string values
	if ActionCycle != "cycle" {
		t.Errorf("expected ActionCycle='cycle', got %q", ActionCycle)
	}
	if ActionRestart != "restart" {
		t.Errorf("expected ActionRestart='restart', got %q", ActionRestart)
	}
	if ActionShutdown != "shutdown" {
		t.Errorf("expected ActionShutdown='shutdown', got %q", ActionShutdown)
	}
}

func TestLifecycleRequest_Serialization(t *testing.T) {
	request := &LifecycleRequest{
		From:      "mayor",
		Action:    ActionCycle,
		Timestamp: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var loaded LifecycleRequest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if loaded.From != request.From {
		t.Errorf("From mismatch: got %q, want %q", loaded.From, request.From)
	}
	if loaded.Action != request.Action {
		t.Errorf("Action mismatch: got %q, want %q", loaded.Action, request.Action)
	}
}

func TestIsShutdownInProgress_NoLockFile(t *testing.T) {
	tmpDir := t.TempDir()

	d := &Daemon{
		config: &Config{TownRoot: tmpDir},
	}

	// No lock file exists - should return false
	if d.isShutdownInProgress() {
		t.Error("expected false when lock file doesn't exist")
	}
}

func TestIsShutdownInProgress_StaleLockFile(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lockDir, "shutdown.lock")

	// Create a stale lock file (not actually locked)
	if err := os.WriteFile(lockPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	d := &Daemon{
		config: &Config{TownRoot: tmpDir},
	}

	// File exists but not locked - should return false
	if d.isShutdownInProgress() {
		t.Error("expected false when lock file exists but is not locked")
	}

	// File should still exist - flock files are never removed to prevent
	// a race where concurrent callers lock different inodes
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("expected lock file to be preserved (flock files should not be removed)")
	}
}

func TestIsShutdownInProgress_ActiveLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lockDir, "shutdown.lock")

	// Create and hold the lock (simulating active shutdown)
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	if !locked {
		t.Fatal("expected to acquire lock")
	}
	defer func() { _ = lock.Unlock() }()

	d := &Daemon{
		config: &Config{TownRoot: tmpDir},
	}

	// File exists and is locked - should return true
	if !d.isShutdownInProgress() {
		t.Error("expected true when lock file is actively held")
	}

	// File should still exist (we're still holding the lock)
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file should still exist: %v", err)
	}
}
