package ratelimit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectRateLimit(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty", "", false},
		{"normal output", "Hello, world!", false},
		{"rate limit", "You are being rate limited", true},
		{"rate-limit hyphen", "Error: rate-limit exceeded", true},
		{"429 error", "HTTP error 429 Too Many Requests", true},
		{"please wait", "Please wait 30 seconds before trying again", true},
		{"quota exceeded", "You have exceeded your quota", true},
		{"overloaded", "The API is currently overloaded", true},
		{"throttled", "Request was throttled", true},
		{"case insensitive", "RATE LIMIT detected", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectRateLimit(tt.output)
			if got != tt.want {
				t.Errorf("DetectRateLimit(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestExtractRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   time.Duration
	}{
		{"no retry", "Error occurred", 0},
		{"retry after seconds", "retry after 30 seconds", 30 * time.Second},
		{"wait seconds", "wait 60 seconds", 60 * time.Second},
		{"retry after minutes", "retry after 2 minutes", 2 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRetryAfter(tt.output)
			if got != tt.want {
				t.Errorf("ExtractRetryAfter(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestTracker_LoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755); err != nil {
		t.Fatalf("creating test dir: %v", err)
	}

	tracker := NewTracker(rigPath)

	// Initially no state
	if err := tracker.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if tracker.State().Limited {
		t.Error("Expected no rate limit initially")
	}

	// Record a rate limit
	tracker.RecordRateLimit("test-source", "test-account")

	// Save
	if err := tracker.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load in new tracker
	tracker2 := NewTracker(rigPath)
	if err := tracker2.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	state := tracker2.State()
	if !state.Limited {
		t.Error("Expected rate limit to be persisted")
	}
	if state.Source != "test-source" {
		t.Errorf("Source = %q, want %q", state.Source, "test-source")
	}
	if state.Account != "test-account" {
		t.Errorf("Account = %q, want %q", state.Account, "test-account")
	}
	if state.ConsecutiveHits != 1 {
		t.Errorf("ConsecutiveHits = %d, want 1", state.ConsecutiveHits)
	}
}

func TestTracker_BackoffCalculation(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	_ = os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755)

	tracker := NewTracker(rigPath)
	_ = tracker.Load()

	// First hit: 30 seconds backoff
	tracker.RecordRateLimit("test", "")
	state := tracker.State()
	backoff1 := time.Until(state.BackoffUntil)
	if backoff1 < 29*time.Second || backoff1 > 31*time.Second {
		t.Errorf("First backoff = %v, want ~30s", backoff1)
	}

	// Second hit: 60 seconds backoff
	tracker.RecordRateLimit("test", "")
	state = tracker.State()
	backoff2 := time.Until(state.BackoffUntil)
	if backoff2 < 59*time.Second || backoff2 > 61*time.Second {
		t.Errorf("Second backoff = %v, want ~60s", backoff2)
	}

	// Third hit: 120 seconds backoff
	tracker.RecordRateLimit("test", "")
	state = tracker.State()
	backoff3 := time.Until(state.BackoffUntil)
	if backoff3 < 119*time.Second || backoff3 > 121*time.Second {
		t.Errorf("Third backoff = %v, want ~120s", backoff3)
	}
}

func TestTracker_ShouldDefer(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	_ = os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755)

	tracker := NewTracker(rigPath)
	_ = tracker.Load()

	// No rate limit - should not defer
	if tracker.ShouldDefer() {
		t.Error("ShouldDefer() = true, want false (no rate limit)")
	}

	// Record rate limit - should defer
	tracker.RecordRateLimit("test", "")
	if !tracker.ShouldDefer() {
		t.Error("ShouldDefer() = false, want true (backoff active)")
	}

	// Clear - should not defer
	tracker.Clear()
	if tracker.ShouldDefer() {
		t.Error("ShouldDefer() = true, want false (cleared)")
	}
}

func TestTracker_RecordSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	_ = os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755)

	tracker := NewTracker(rigPath)
	_ = tracker.Load()

	// Set up rate limit
	tracker.RecordRateLimit("test", "")
	tracker.RecordRateLimit("test", "")
	if state := tracker.State(); state.ConsecutiveHits != 2 {
		t.Errorf("ConsecutiveHits = %d, want 2", state.ConsecutiveHits)
	}

	// Record success - should reset
	tracker.RecordSuccess()
	state := tracker.State()
	if state.Limited {
		t.Error("Expected rate limit to be cleared after success")
	}
	if state.ConsecutiveHits != 0 {
		t.Errorf("ConsecutiveHits = %d, want 0 after success", state.ConsecutiveHits)
	}
}
