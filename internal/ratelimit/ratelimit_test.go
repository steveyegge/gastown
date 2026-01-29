package ratelimit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetStateFile(t *testing.T) {
	townRoot := "/tmp/test-town"
	expected := "/tmp/test-town/.runtime/ratelimit/state.json"
	result := GetStateFile(townRoot)

	if result != expected {
		t.Errorf("GetStateFile(%q) = %q, expected %q", townRoot, result, expected)
	}
}

func TestGetState_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	state, err := GetState(tmpDir)
	if err != nil {
		t.Fatalf("GetState should not error for missing file, got %v", err)
	}
	if state != nil {
		t.Error("expected nil state for non-existent file")
	}
}

func TestRecordRateLimit_CreatesStateFile(t *testing.T) {
	tmpDir := t.TempDir()

	err := RecordRateLimit(tmpDir, 5*time.Minute, "test-agent", "API rate limit")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	// Verify file exists
	stateFile := GetStateFile(tmpDir)
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("state file should exist: %v", err)
	}

	// Verify contents
	state, err := GetState(tmpDir)
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if !state.Active {
		t.Error("expected Active=true")
	}
	if state.RecordedBy != "test-agent" {
		t.Errorf("expected RecordedBy='test-agent', got %q", state.RecordedBy)
	}
	if state.Reason != "API rate limit" {
		t.Errorf("expected Reason='API rate limit', got %q", state.Reason)
	}
	if state.RetryAfterSeconds != 300 {
		t.Errorf("expected RetryAfterSeconds=300, got %d", state.RetryAfterSeconds)
	}
}

func TestIsRateLimited_Active(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit that will reset in 5 minutes
	err := RecordRateLimit(tmpDir, 5*time.Minute, "test", "test limit")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	isLimited, state, err := IsRateLimited(tmpDir)
	if err != nil {
		t.Fatalf("IsRateLimited error: %v", err)
	}
	if !isLimited {
		t.Error("expected isLimited=true for active rate limit")
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if !state.Active {
		t.Error("expected Active=true in state")
	}
}

func TestIsRateLimited_Expired(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit that already expired (negative duration creates past reset time)
	state := &State{
		Active:     true,
		ResetAt:    time.Now().Add(-1 * time.Minute), // Already past
		RecordedAt: time.Now().Add(-2 * time.Minute),
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	isLimited, stateOut, err := IsRateLimited(tmpDir)
	if err != nil {
		t.Fatalf("IsRateLimited error: %v", err)
	}
	if isLimited {
		t.Error("expected isLimited=false for expired rate limit")
	}
	if stateOut == nil {
		t.Fatal("expected non-nil state (for reference)")
	}
}

func TestShouldWake_NotYetReset(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit that resets in 5 minutes
	err := RecordRateLimit(tmpDir, 5*time.Minute, "test", "test limit")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	shouldWake, state, err := ShouldWake(tmpDir)
	if err != nil {
		t.Fatalf("ShouldWake error: %v", err)
	}
	if shouldWake {
		t.Error("expected shouldWake=false before reset time")
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
}

func TestShouldWake_AfterReset(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit that already reset
	state := &State{
		Active:       true,
		ResetAt:      time.Now().Add(-1 * time.Minute), // Already past
		RecordedAt:   time.Now().Add(-5 * time.Minute),
		WakeAttempts: 0,
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	shouldWake, stateOut, err := ShouldWake(tmpDir)
	if err != nil {
		t.Fatalf("ShouldWake error: %v", err)
	}
	if !shouldWake {
		t.Error("expected shouldWake=true after reset time")
	}
	if stateOut == nil {
		t.Fatal("expected non-nil state")
	}
}

func TestShouldWake_MaxAttemptsReached(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit with max attempts reached
	state := &State{
		Active:          true,
		ResetAt:         time.Now().Add(-1 * time.Minute), // Already past
		RecordedAt:      time.Now().Add(-5 * time.Minute),
		WakeAttempts:    3, // Max attempts
		LastWakeAttempt: time.Now().Add(-5 * time.Minute),
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	shouldWake, _, err := ShouldWake(tmpDir)
	if err != nil {
		t.Fatalf("ShouldWake error: %v", err)
	}
	if shouldWake {
		t.Error("expected shouldWake=false when max attempts reached")
	}
}

func TestShouldWake_TooSoonAfterLastAttempt(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit with a recent wake attempt
	state := &State{
		Active:          true,
		ResetAt:         time.Now().Add(-1 * time.Minute), // Already past
		RecordedAt:      time.Now().Add(-5 * time.Minute),
		WakeAttempts:    1,
		LastWakeAttempt: time.Now().Add(-30 * time.Second), // Less than 2 minutes ago
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	shouldWake, _, err := ShouldWake(tmpDir)
	if err != nil {
		t.Fatalf("ShouldWake error: %v", err)
	}
	if shouldWake {
		t.Error("expected shouldWake=false when last attempt too recent")
	}
}

func TestRecordWakeAttempt(t *testing.T) {
	tmpDir := t.TempDir()

	// Record initial rate limit
	state := &State{
		Active:       true,
		ResetAt:      time.Now().Add(-1 * time.Minute),
		RecordedAt:   time.Now().Add(-5 * time.Minute),
		WakeAttempts: 0,
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	// Record wake attempt
	if err := RecordWakeAttempt(tmpDir); err != nil {
		t.Fatalf("RecordWakeAttempt error: %v", err)
	}

	// Verify attempt was recorded
	updated, err := GetState(tmpDir)
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}
	if updated.WakeAttempts != 1 {
		t.Errorf("expected WakeAttempts=1, got %d", updated.WakeAttempts)
	}
	if updated.LastWakeAttempt.IsZero() {
		t.Error("expected LastWakeAttempt to be set")
	}
}

func TestClear(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a rate limit state
	err := RecordRateLimit(tmpDir, 5*time.Minute, "test", "test limit")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	// Verify it exists
	stateFile := GetStateFile(tmpDir)
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file should exist before clear: %v", err)
	}

	// Clear it
	if err := Clear(tmpDir); err != nil {
		t.Fatalf("Clear error: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("state file should not exist after clear")
	}
}

func TestClear_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Clear should not error on non-existent file
	if err := Clear(tmpDir); err != nil {
		t.Errorf("Clear should not error on non-existent file: %v", err)
	}
}

func TestTimeUntilReset_Active(t *testing.T) {
	tmpDir := t.TempDir()

	// Record a rate limit that resets in 5 minutes
	err := RecordRateLimit(tmpDir, 5*time.Minute, "test", "test limit")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	remaining, err := TimeUntilReset(tmpDir)
	if err != nil {
		t.Fatalf("TimeUntilReset error: %v", err)
	}

	// Should be close to 5 minutes (allow some tolerance for test execution time)
	if remaining < 4*time.Minute || remaining > 5*time.Minute+time.Second {
		t.Errorf("expected remaining ~5 minutes, got %v", remaining)
	}
}

func TestTimeUntilReset_Expired(t *testing.T) {
	tmpDir := t.TempDir()

	// Record an expired rate limit
	state := &State{
		Active:     true,
		ResetAt:    time.Now().Add(-1 * time.Minute),
		RecordedAt: time.Now().Add(-5 * time.Minute),
	}
	if err := SaveState(tmpDir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	remaining, err := TimeUntilReset(tmpDir)
	if err != nil {
		t.Fatalf("TimeUntilReset error: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0 for expired limit, got %v", remaining)
	}
}

func TestTimeUntilReset_NoState(t *testing.T) {
	tmpDir := t.TempDir()

	remaining, err := TimeUntilReset(tmpDir)
	if err != nil {
		t.Fatalf("TimeUntilReset error: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0 for no state, got %v", remaining)
	}
}

func TestStateFile_CreatesParentDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// The .runtime/ratelimit directory shouldn't exist yet
	ratelimitDir := filepath.Join(tmpDir, ".runtime", "ratelimit")
	if _, err := os.Stat(ratelimitDir); !os.IsNotExist(err) {
		t.Fatal("directory should not exist before test")
	}

	// RecordRateLimit should create parent directories
	err := RecordRateLimit(tmpDir, 5*time.Minute, "test", "test")
	if err != nil {
		t.Fatalf("RecordRateLimit error: %v", err)
	}

	// Directory should now exist
	if _, err := os.Stat(ratelimitDir); err != nil {
		t.Errorf("directory should exist after RecordRateLimit: %v", err)
	}
}
