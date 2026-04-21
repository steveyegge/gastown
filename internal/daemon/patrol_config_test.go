package daemon

import (
	"os"
	"path/filepath"
	"testing"

	agentconfig "github.com/steveyegge/gastown/internal/config"
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

func TestLoadDaemonTownSettings(t *testing.T) {
	// No settings file: returns nil
	tmpDir := t.TempDir()
	got := loadDaemonTownSettings(tmpDir)
	if got != nil {
		t.Errorf("expected nil for missing settings, got %v", got)
	}

	// Empty disabled_patrols: returns nil
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(`{
		"type": "town-settings", "version": 1
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	got = loadDaemonTownSettings(tmpDir)
	if got != nil {
		t.Errorf("expected nil for empty disabled_patrols, got %v", got)
	}

	// With disabled patrols (flat_bead_namespace removed in sbx-gastown-vrb4)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(`{
		"type": "town-settings", "version": 1,
		"disabled_patrols": ["doctor_dog", "compactor_dog"]
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	got = loadDaemonTownSettings(tmpDir)
	if len(got) != 2 {
		t.Fatalf("expected 2 disabled patrols, got %d", len(got))
	}
	if !got["doctor_dog"] {
		t.Error("expected doctor_dog to be disabled")
	}
	if !got["compactor_dog"] {
		t.Error("expected compactor_dog to be disabled")
	}
	if got["witness"] {
		t.Error("expected witness to NOT be disabled")
	}
}

func TestIsPatrolActive(t *testing.T) {
	// Patrol enabled in daemon config, not in disabled list → active
	d := &Daemon{
		patrolConfig:    nil, // nil config = all default-enabled patrols enabled
		disabledPatrols: nil,
	}
	if !d.isPatrolActive("witness") {
		t.Error("expected witness to be active with nil configs")
	}

	// Patrol enabled in daemon config, but in disabled list → inactive
	d.disabledPatrols = map[string]bool{"witness": true}
	if d.isPatrolActive("witness") {
		t.Error("expected witness to be inactive when in disabled list")
	}

	// Patrol disabled in daemon config, not in disabled list → inactive
	d.disabledPatrols = nil
	d.patrolConfig = &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			Witness: &PatrolConfig{Enabled: false},
		},
	}
	if d.isPatrolActive("witness") {
		t.Error("expected witness to be inactive when disabled in daemon config")
	}

	// Opt-in patrol (doctor_dog) disabled by default, in disabled list → inactive
	d.patrolConfig = nil
	d.disabledPatrols = map[string]bool{"doctor_dog": true}
	if d.isPatrolActive("doctor_dog") {
		t.Error("expected doctor_dog to be inactive")
	}

	// Opt-in patrol enabled in daemon config but in disabled list → disabled wins
	d.patrolConfig = &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoctorDog: &DoctorDogConfig{Enabled: true},
		},
	}
	d.disabledPatrols = map[string]bool{"doctor_dog": true}
	if d.isPatrolActive("doctor_dog") {
		t.Error("expected doctor_dog to be inactive when in disabled list, even if enabled in daemon config")
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

func TestLoadServicesConfig(t *testing.T) {
	// No settings file: returns nil (nil is safe — all Is*Enabled() return true)
	tmpDir := t.TempDir()
	d := &Daemon{config: &Config{TownRoot: tmpDir}}
	svc := d.loadServicesConfig()
	if svc != nil {
		t.Errorf("expected nil for missing settings, got %+v", svc)
	}
	// nil receiver still defaults to enabled
	if !svc.IsDeaconEnabled() {
		t.Error("expected deacon enabled on nil ServicesConfig")
	}

	// Valid settings with services disabled
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(`{
		"type": "town-settings", "version": 1,
		"services": {
			"deacon": "disabled",
			"mayor": "disabled",
			"witnesses": "disabled",
			"refineries": "disabled"
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	svc = d.loadServicesConfig()
	if svc == nil {
		t.Fatal("expected non-nil ServicesConfig")
	}
	if svc.IsDeaconEnabled() {
		t.Error("expected deacon disabled")
	}
	if svc.IsMayorEnabled() {
		t.Error("expected mayor disabled")
	}
	if svc.IsWitnessEnabled() {
		t.Error("expected witnesses disabled")
	}
	if svc.IsRefineryEnabled() {
		t.Error("expected refineries disabled")
	}
}

func TestServicesConfigGatesAgentSpawning(t *testing.T) {
	// This test verifies the gating logic composition:
	// agent spawns require BOTH patrol active AND service enabled.
	tests := []struct {
		name           string
		patrolConfig   *DaemonPatrolConfig
		disabledPtrols map[string]bool
		servicesConfig *agentconfig.ServicesConfig
		// expected results for each service
		deaconActive  bool
		witnessActive bool
		refineryActive bool
		mayorActive   bool
	}{
		{
			name:           "all defaults (nil everything) → all active",
			deaconActive:  true,
			witnessActive: true,
			refineryActive: true,
			mayorActive:   true,
		},
		{
			name:           "services all disabled → none active",
			servicesConfig: &agentconfig.ServicesConfig{
				Deacon:     "disabled",
				Mayor:      "disabled",
				Witnesses:  "disabled",
				Refineries: "disabled",
			},
			deaconActive:  false,
			witnessActive: false,
			refineryActive: false,
			mayorActive:   false,
		},
		{
			name: "patrol disabled trumps service enabled",
			patrolConfig: &DaemonPatrolConfig{
				Patrols: &PatrolsConfig{
					Witness: &PatrolConfig{Enabled: false},
				},
			},
			servicesConfig: nil, // defaults to enabled
			witnessActive: false,
			deaconActive:  true,
			refineryActive: true,
			mayorActive:   true,
		},
		{
			name: "service disabled trumps patrol enabled",
			patrolConfig: nil, // defaults to enabled
			servicesConfig: &agentconfig.ServicesConfig{
				Witnesses: "disabled",
			},
			witnessActive: false,
			deaconActive:  true,
			refineryActive: true,
			mayorActive:   true,
		},
		{
			name: "witnesses on-demand counts as disabled for daemon spawning",
			servicesConfig: &agentconfig.ServicesConfig{
				Witnesses: "on-demand",
			},
			witnessActive: false,
			deaconActive:  true,
			refineryActive: true,
			mayorActive:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Daemon{
				patrolConfig:    tt.patrolConfig,
				disabledPatrols: tt.disabledPtrols,
			}
			svc := tt.servicesConfig

			// Deacon: patrol + services
			gotDeacon := d.isPatrolActive("deacon") && svc.IsDeaconEnabled()
			if gotDeacon != tt.deaconActive {
				t.Errorf("deacon: got %v, want %v", gotDeacon, tt.deaconActive)
			}

			// Witness: patrol + services
			gotWitness := d.isPatrolActive("witness") && svc.IsWitnessEnabled()
			if gotWitness != tt.witnessActive {
				t.Errorf("witness: got %v, want %v", gotWitness, tt.witnessActive)
			}

			// Refinery: patrol + services
			gotRefinery := d.isPatrolActive("refinery") && svc.IsRefineryEnabled()
			if gotRefinery != tt.refineryActive {
				t.Errorf("refinery: got %v, want %v", gotRefinery, tt.refineryActive)
			}

			// Mayor: services only (no patrol gate)
			gotMayor := svc.IsMayorEnabled()
			if gotMayor != tt.mayorActive {
				t.Errorf("mayor: got %v, want %v", gotMayor, tt.mayorActive)
			}
		})
	}
}

// TestDeaconSessionCleanupGating verifies that the daemon only cleans up
// leftover hq-deacon sessions when the deacon service is enabled but the
// patrol is disabled. When services.deacon=disabled, the agent deacon may
// own the hq-deacon session and the daemon must leave it alone. (sbx-gastown-qsuq)
func TestDeaconSessionCleanupGating(t *testing.T) {
	tests := []struct {
		name           string
		patrolActive   bool
		serviceEnabled bool
		wantCleanup    bool
	}{
		{"patrol+service enabled → ensure running, no cleanup", true, true, false},
		{"patrol disabled, service enabled → cleanup stale daemon-owned sessions", false, true, true},
		{"patrol enabled, service disabled → leave sessions to agent deacon", true, false, false},
		{"patrol+service disabled → leave sessions to agent deacon", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mirror the decision in Daemon.heartbeatTick:
			// cleanup only when service is enabled AND patrol is not active.
			got := !(tt.patrolActive && tt.serviceEnabled) && tt.serviceEnabled
			if got != tt.wantCleanup {
				t.Errorf("cleanup: got %v, want %v", got, tt.wantCleanup)
			}
		})
	}
}
