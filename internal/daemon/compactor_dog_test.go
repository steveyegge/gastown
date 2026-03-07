package daemon

import (
	"fmt"
	"testing"
	"time"
)

func TestCompactorDogInterval(t *testing.T) {
	// Default interval
	if got := compactorDogInterval(nil); got != defaultCompactorDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCompactorDogInterval, got)
	}

	// Custom interval
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CompactorDog: &CompactorDogConfig{
				Enabled:     true,
				IntervalStr: "12h",
			},
		},
	}
	if got := compactorDogInterval(config); got != 12*time.Hour {
		t.Errorf("expected 12h interval, got %v", got)
	}

	// Invalid interval falls back to default
	config.Patrols.CompactorDog.IntervalStr = "invalid"
	if got := compactorDogInterval(config); got != defaultCompactorDogInterval {
		t.Errorf("expected default interval for invalid config, got %v", got)
	}
}

func TestCompactorDogThreshold(t *testing.T) {
	// Default threshold
	if got := compactorDogThreshold(nil); got != defaultCompactorCommitThreshold {
		t.Errorf("expected default threshold %d, got %d", defaultCompactorCommitThreshold, got)
	}

	// Custom threshold
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CompactorDog: &CompactorDogConfig{
				Enabled:   true,
				Threshold: 1000,
			},
		},
	}
	if got := compactorDogThreshold(config); got != 1000 {
		t.Errorf("expected threshold 1000, got %d", got)
	}

	// Zero threshold falls back to default
	config.Patrols.CompactorDog.Threshold = 0
	if got := compactorDogThreshold(config); got != defaultCompactorCommitThreshold {
		t.Errorf("expected default threshold for zero value, got %d", got)
	}
}

func TestCompactorDogMode(t *testing.T) {
	// Default mode is flatten
	if got := compactorDogMode(nil); got != "flatten" {
		t.Errorf("expected default mode 'flatten', got %q", got)
	}

	// Explicit surgical mode
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CompactorDog: &CompactorDogConfig{
				Enabled: true,
				Mode:    "surgical",
			},
		},
	}
	if got := compactorDogMode(config); got != "surgical" {
		t.Errorf("expected mode 'surgical', got %q", got)
	}

	// Unknown mode falls back to flatten
	config.Patrols.CompactorDog.Mode = "unknown"
	if got := compactorDogMode(config); got != "flatten" {
		t.Errorf("expected mode 'flatten' for unknown value, got %q", got)
	}

	// Empty mode falls back to flatten
	config.Patrols.CompactorDog.Mode = ""
	if got := compactorDogMode(config); got != "flatten" {
		t.Errorf("expected mode 'flatten' for empty value, got %q", got)
	}
}

func TestCompactorDogKeepRecent(t *testing.T) {
	// Default keep_recent
	if got := compactorDogKeepRecent(nil); got != 50 {
		t.Errorf("expected default keep_recent 50, got %d", got)
	}

	// Custom keep_recent
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CompactorDog: &CompactorDogConfig{
				Enabled:    true,
				KeepRecent: 100,
			},
		},
	}
	if got := compactorDogKeepRecent(config); got != 100 {
		t.Errorf("expected keep_recent 100, got %d", got)
	}

	// Zero keep_recent falls back to default
	config.Patrols.CompactorDog.KeepRecent = 0
	if got := compactorDogKeepRecent(config); got != 50 {
		t.Errorf("expected default keep_recent for zero value, got %d", got)
	}
}

func TestIsPatrolEnabled_CompactorDog(t *testing.T) {
	// Nil config: disabled (opt-in patrol)
	if IsPatrolEnabled(nil, "compactor_dog") {
		t.Error("expected compactor_dog to be disabled with nil config")
	}

	// Empty patrols: disabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	if IsPatrolEnabled(config, "compactor_dog") {
		t.Error("expected compactor_dog to be disabled by default")
	}

	// Explicitly enabled
	config.Patrols.CompactorDog = &CompactorDogConfig{Enabled: true}
	if !IsPatrolEnabled(config, "compactor_dog") {
		t.Error("expected compactor_dog to be enabled when configured")
	}

	// Explicitly disabled
	config.Patrols.CompactorDog = &CompactorDogConfig{Enabled: false}
	if IsPatrolEnabled(config, "compactor_dog") {
		t.Error("expected compactor_dog to be disabled when explicitly disabled")
	}
}

func TestIsConcurrentWriteError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"rebase execution failed", fmt.Errorf("rebase execution failed: some details"), true},
		{"concurrency abort", fmt.Errorf("concurrency abort: main HEAD moved"), true},
		{"graph error", fmt.Errorf("commit graph changed during operation"), true},
		{"cannot rebase", fmt.Errorf("cannot rebase: branch diverged"), true},
		{"unrelated error", fmt.Errorf("connection refused"), false},
		{"table not found", fmt.Errorf("table 'foo' not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConcurrentWriteError(tt.err); got != tt.want {
				t.Errorf("isConcurrentWriteError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
