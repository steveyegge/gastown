package daemon

import (
	"testing"
	"time"
)

func TestPolecatReaperConfig_Defaults(t *testing.T) {
	// PolecatReaperConfig should exist and have sensible fields
	cfg := &PolecatReaperConfig{
		Enabled:     true,
		IntervalStr: "60s",
	}

	if !cfg.Enabled {
		t.Error("expected enabled to be true")
	}
	if cfg.IntervalStr != "60s" {
		t.Errorf("expected interval 60s, got %s", cfg.IntervalStr)
	}
	if cfg.DryRun {
		t.Error("expected dry_run to default to false")
	}
}

func TestPolecatReaperInterval_Default(t *testing.T) {
	// With nil config, should return default interval (60s)
	interval := polecatReaperInterval(nil)
	if interval != defaultPolecatReaperInterval {
		t.Errorf("polecatReaperInterval(nil) = %v, want %v", interval, defaultPolecatReaperInterval)
	}
}

func TestPolecatReaperInterval_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			PolecatReaper: &PolecatReaperConfig{
				Enabled:     true,
				IntervalStr: "90s",
			},
		},
	}

	interval := polecatReaperInterval(config)
	if interval != 90*time.Second {
		t.Errorf("polecatReaperInterval() = %v, want 90s", interval)
	}
}

func TestPolecatReaperInterval_InvalidFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			PolecatReaper: &PolecatReaperConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}

	interval := polecatReaperInterval(config)
	if interval != defaultPolecatReaperInterval {
		t.Errorf("polecatReaperInterval() = %v, want default %v", interval, defaultPolecatReaperInterval)
	}
}

func TestPolecatReaperInterval_EmptyString(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			PolecatReaper: &PolecatReaperConfig{
				Enabled: true,
			},
		},
	}

	interval := polecatReaperInterval(config)
	if interval != defaultPolecatReaperInterval {
		t.Errorf("polecatReaperInterval() = %v, want default %v", interval, defaultPolecatReaperInterval)
	}
}

func TestIsPatrolEnabled_PolecatReaper_OptIn(t *testing.T) {
	// polecat_reaper is opt-in: disabled by default with nil config
	if IsPatrolEnabled(nil, "polecat_reaper") {
		t.Error("expected polecat_reaper to be disabled with nil config (opt-in)")
	}

	// Disabled when patrols section exists but PolecatReaper is nil
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	if IsPatrolEnabled(config, "polecat_reaper") {
		t.Error("expected polecat_reaper to be disabled by default")
	}

	// Explicitly enabled
	config.Patrols.PolecatReaper = &PolecatReaperConfig{Enabled: true}
	if !IsPatrolEnabled(config, "polecat_reaper") {
		t.Error("expected polecat_reaper to be enabled when configured")
	}

	// Explicitly disabled
	config.Patrols.PolecatReaper = &PolecatReaperConfig{Enabled: false}
	if IsPatrolEnabled(config, "polecat_reaper") {
		t.Error("expected polecat_reaper to be disabled when explicitly disabled")
	}
}

func TestPatrolsConfig_PolecatReaperField(t *testing.T) {
	// Verify the PolecatReaper field exists on PatrolsConfig
	p := &PatrolsConfig{
		PolecatReaper: &PolecatReaperConfig{
			Enabled:          true,
			IntervalStr:      "60s",
			DryRun:           false,
			IdleThresholdStr: "120s",
		},
	}

	if p.PolecatReaper == nil {
		t.Fatal("expected PolecatReaper to be set")
	}
	if !p.PolecatReaper.Enabled {
		t.Error("expected enabled")
	}
	if p.PolecatReaper.IdleThresholdStr != "120s" {
		t.Errorf("expected idle_threshold 120s, got %s", p.PolecatReaper.IdleThresholdStr)
	}
}

func TestDefaultLifecycleConfig_IncludesPolecatReaper(t *testing.T) {
	config := DefaultLifecycleConfig()
	if config.Patrols.PolecatReaper == nil {
		t.Fatal("expected PolecatReaper in default lifecycle config")
	}
	if !config.Patrols.PolecatReaper.Enabled {
		t.Error("expected PolecatReaper to be enabled by default")
	}
	if config.Patrols.PolecatReaper.IntervalStr != "60s" {
		t.Errorf("expected interval 60s, got %s", config.Patrols.PolecatReaper.IntervalStr)
	}
}

func TestEnsureLifecycleDefaults_FillsPolecatReaper(t *testing.T) {
	// Config with everything except polecat_reaper
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{Enabled: true},
		},
	}

	changed := EnsureLifecycleDefaults(config)
	if !changed {
		t.Error("expected changes")
	}
	if config.Patrols.PolecatReaper == nil {
		t.Fatal("expected PolecatReaper to be filled in")
	}
	if !config.Patrols.PolecatReaper.Enabled {
		t.Error("expected PolecatReaper to be enabled")
	}
}

func TestEnsureLifecycleDefaults_PreservesExistingPolecatReaper(t *testing.T) {
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			PolecatReaper: &PolecatReaperConfig{
				Enabled:     true,
				IntervalStr: "120s", // User customized
				DryRun:      true,
			},
		},
	}

	EnsureLifecycleDefaults(config)

	// User's config should be preserved
	if config.Patrols.PolecatReaper.IntervalStr != "120s" {
		t.Errorf("expected preserved interval 120s, got %s", config.Patrols.PolecatReaper.IntervalStr)
	}
	if !config.Patrols.PolecatReaper.DryRun {
		t.Error("expected preserved dry_run true")
	}
}
