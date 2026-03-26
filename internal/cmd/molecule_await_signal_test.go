package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCalculateEffectiveTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeout     string
		backoffBase string
		backoffMult int
		backoffMax  string
		idleCycles  int
		want        time.Duration
		wantErr     bool
	}{
		{
			name:    "simple timeout 60s",
			timeout: "60s",
			want:    60 * time.Second,
		},
		{
			name:    "simple timeout 5m",
			timeout: "5m",
			want:    5 * time.Minute,
		},
		{
			name:        "backoff base only, idle=0",
			timeout:     "60s",
			backoffBase: "30s",
			idleCycles:  0,
			want:        30 * time.Second,
		},
		{
			name:        "backoff with idle=1, mult=2",
			timeout:     "60s",
			backoffBase: "30s",
			backoffMult: 2,
			idleCycles:  1,
			want:        60 * time.Second,
		},
		{
			name:        "backoff with idle=2, mult=2",
			timeout:     "60s",
			backoffBase: "30s",
			backoffMult: 2,
			idleCycles:  2,
			want:        2 * time.Minute,
		},
		{
			name:        "backoff with max cap",
			timeout:     "60s",
			backoffBase: "30s",
			backoffMult: 2,
			backoffMax:  "5m",
			idleCycles:  10, // Would be 30s * 2^10 = ~8.5h but capped at 5m
			want:        5 * time.Minute,
		},
		{
			name:        "backoff base exceeds max",
			timeout:     "60s",
			backoffBase: "15m",
			backoffMax:  "10m",
			want:        10 * time.Minute,
		},
		{
			name:    "invalid timeout",
			timeout: "invalid",
			wantErr: true,
		},
		{
			name:        "invalid backoff base",
			timeout:     "60s",
			backoffBase: "invalid",
			wantErr:     true,
		},
		{
			name:        "invalid backoff max",
			timeout:     "60s",
			backoffBase: "30s",
			backoffMax:  "invalid",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set package-level variables
			awaitSignalTimeout = tt.timeout
			awaitSignalBackoffBase = tt.backoffBase
			awaitSignalBackoffMult = tt.backoffMult
			if tt.backoffMult == 0 {
				awaitSignalBackoffMult = 2 // default
			}
			awaitSignalBackoffMax = tt.backoffMax

			got, err := calculateEffectiveTimeout(tt.idleCycles)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateEffectiveTimeout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("calculateEffectiveTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAwaitSignalResult(t *testing.T) {
	// Test that result struct marshals correctly
	result := AwaitSignalResult{
		Reason:  "signal",
		Elapsed: 5 * time.Second,
		Signal:  "[12:34:56] + gt-abc created · New issue",
	}

	if result.Reason != "signal" {
		t.Errorf("expected reason 'signal', got %q", result.Reason)
	}
	if result.Signal == "" {
		t.Error("expected signal to be set")
	}
}

func TestWaitForEventsFile_MissingFile(t *testing.T) {
	// When the events file doesn't exist, waitForEventsFile creates it and
	// waits for new events. With no events, it should return timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := waitForEventsFile(ctx, filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != "timeout" {
		t.Errorf("expected reason 'timeout', got %q", result.Reason)
	}
}

func TestWaitForEventsFile_Timeout(t *testing.T) {
	// When no new events are appended, waitForEventsFile should return timeout.
	eventsPath := filepath.Join(t.TempDir(), ".events.jsonl")
	if err := os.WriteFile(eventsPath, []byte(`{"ts":"2024-01-01","type":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := waitForEventsFile(ctx, eventsPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != "timeout" {
		t.Errorf("expected reason 'timeout', got %q", result.Reason)
	}
}

func TestWaitForEventsFile_Signal(t *testing.T) {
	// When a new event is appended, waitForEventsFile should return signal.
	eventsPath := filepath.Join(t.TempDir(), ".events.jsonl")
	// Write initial content (will be skipped — we seek to end)
	if err := os.WriteFile(eventsPath, []byte(`{"ts":"old","type":"ignore"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Append a new line after a short delay
	go func() {
		time.Sleep(300 * time.Millisecond)
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(`{"ts":"new","type":"sling","actor":"test"}` + "\n")
	}()

	result, err := waitForEventsFile(ctx, eventsPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != "signal" {
		t.Errorf("expected reason 'signal', got %q", result.Reason)
	}
	if result.Signal == "" {
		t.Error("expected signal line to be set")
	}
}

func TestWaitForActivitySignal_PathWiring(t *testing.T) {
	// Verify waitForActivitySignal constructs the correct events path from
	// townRoot. The events file should be at <townRoot>/.events.jsonl.
	townRoot := t.TempDir()
	eventsPath := filepath.Join(townRoot, ".events.jsonl")
	if err := os.WriteFile(eventsPath, []byte(`{"ts":"old","type":"ignore"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Append a new event after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(`{"ts":"new","type":"sling"}` + "\n")
	}()

	result, err := waitForActivitySignal(ctx, townRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != "signal" {
		t.Errorf("expected reason 'signal', got %q", result.Reason)
	}
}

func TestBackoffWindowResumption(t *testing.T) {
	// Test the backoff window resumption logic that makes await-signal
	// resilient to interrupts. When a backoff-until timestamp is in the
	// future and remaining time <= full timeout, use remaining time.
	now := time.Now()

	tests := []struct {
		name           string
		fullTimeout    time.Duration
		backoffUntil   time.Time
		wantResumed    bool
		wantApproxTime time.Duration // approximate expected timeout
	}{
		{
			name:           "no stored window - use full timeout",
			fullTimeout:    5 * time.Minute,
			backoffUntil:   time.Time{}, // zero value
			wantResumed:    false,
			wantApproxTime: 5 * time.Minute,
		},
		{
			name:           "window in future - resume with remaining",
			fullTimeout:    5 * time.Minute,
			backoffUntil:   now.Add(2 * time.Minute),
			wantResumed:    true,
			wantApproxTime: 2 * time.Minute,
		},
		{
			name:           "window expired - use full timeout",
			fullTimeout:    5 * time.Minute,
			backoffUntil:   now.Add(-1 * time.Minute), // in the past
			wantResumed:    false,
			wantApproxTime: 5 * time.Minute,
		},
		{
			name:           "window exceeds full timeout (stale) - use full timeout",
			fullTimeout:    2 * time.Minute,
			backoffUntil:   now.Add(10 * time.Minute), // remaining > full
			wantResumed:    false,
			wantApproxTime: 2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := tt.fullTimeout
			resumed := false

			if !tt.backoffUntil.IsZero() && tt.backoffUntil.After(now) {
				remaining := tt.backoffUntil.Sub(now)
				if remaining <= tt.fullTimeout {
					timeout = remaining
					resumed = true
				}
			}

			if resumed != tt.wantResumed {
				t.Errorf("resumed = %v, want %v", resumed, tt.wantResumed)
			}

			// Allow 2s tolerance for timing
			diff := timeout - tt.wantApproxTime
			if diff < 0 {
				diff = -diff
			}
			if diff > 2*time.Second {
				t.Errorf("timeout = %v, want ~%v (diff: %v)", timeout, tt.wantApproxTime, diff)
			}
		})
	}
}
