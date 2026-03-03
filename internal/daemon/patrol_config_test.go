package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPatrolConfig(t *testing.T) {
	// Create a temp dir with test config
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test config
	configJSON := `{
		"type": "daemon-patrol-config",
		"version": 1,
		"patrols": {
			"refinery": {"enabled": false},
			"witness": {"enabled": true}
		}
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "daemon.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	config := LoadPatrolConfig(tmpDir)
	if config == nil {
		t.Fatal("expected config to be loaded")
	}

	// Test enabled flags
	if IsPatrolEnabled(config, "refinery") {
		t.Error("expected refinery to be disabled")
	}
	if !IsPatrolEnabled(config, "witness") {
		t.Error("expected witness to be enabled")
	}
	if !IsPatrolEnabled(config, "deacon") {
		t.Error("expected deacon to be enabled (default)")
	}
}

func TestIsPatrolEnabled_NilConfig(t *testing.T) {
	// Should default to enabled when config is nil
	if !IsPatrolEnabled(nil, "refinery") {
		t.Error("expected default to be enabled")
	}
}

func TestIsPatrolEnabled_DoltRemotes(t *testing.T) {
	// dolt_remotes defaults to disabled even with nil config (opt-in patrol)
	if IsPatrolEnabled(nil, "dolt_remotes") {
		t.Error("expected dolt_remotes to be disabled with nil config")
	}

	// dolt_remotes defaults to disabled when patrols section exists but DoltRemotes is nil
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	if IsPatrolEnabled(config, "dolt_remotes") {
		t.Error("expected dolt_remotes to be disabled by default")
	}

	// Explicitly enabled
	config.Patrols.DoltRemotes = &DoltRemotesConfig{Enabled: true}
	if !IsPatrolEnabled(config, "dolt_remotes") {
		t.Error("expected dolt_remotes to be enabled when configured")
	}

	// Explicitly disabled
	config.Patrols.DoltRemotes = &DoltRemotesConfig{Enabled: false}
	if IsPatrolEnabled(config, "dolt_remotes") {
		t.Error("expected dolt_remotes to be disabled when explicitly disabled")
	}
}

func TestSaveAndLoadPatrolConfig(t *testing.T) {
	tmpDir := t.TempDir()

	threshold := 500
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			ScheduledMaintenance: &ScheduledMaintenanceConfig{
				Enabled:   true,
				Window:    "03:00",
				Interval:  "daily",
				Threshold: &threshold,
			},
		},
	}

	// Save
	if err := SavePatrolConfig(tmpDir, config); err != nil {
		t.Fatalf("SavePatrolConfig failed: %v", err)
	}

	// Load back
	loaded := LoadPatrolConfig(tmpDir)
	if loaded == nil {
		t.Fatal("expected config to be loaded")
	}

	if !IsPatrolEnabled(loaded, "scheduled_maintenance") {
		t.Error("expected scheduled_maintenance to be enabled")
	}
	sm := loaded.Patrols.ScheduledMaintenance
	if sm.Window != "03:00" {
		t.Errorf("expected window 03:00, got %q", sm.Window)
	}
	if sm.Interval != "daily" {
		t.Errorf("expected interval daily, got %q", sm.Interval)
	}
	if sm.Threshold == nil || *sm.Threshold != 500 {
		t.Errorf("expected threshold 500, got %v", sm.Threshold)
	}
}

func TestDoltRemotesInterval(t *testing.T) {
	// Default interval
	if got := doltRemotesInterval(nil); got != defaultDoltRemotesInterval {
		t.Errorf("expected default interval %v, got %v", defaultDoltRemotesInterval, got)
	}

	// Custom interval
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoltRemotes: &DoltRemotesConfig{
				Enabled:  true,
				Interval: 5 * 60 * 1000000000, // 5 minutes in nanoseconds
			},
		},
	}
	if got := doltRemotesInterval(config); got != 5*60*1000000000 {
		t.Errorf("expected 5m interval, got %v", got)
	}
}
