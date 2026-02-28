package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLifecycleConfig(t *testing.T) {
	config := DefaultLifecycleConfig()

	if config.Type != "daemon-patrol-config" {
		t.Errorf("expected type daemon-patrol-config, got %s", config.Type)
	}
	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
	if config.Patrols == nil {
		t.Fatal("expected patrols to be non-nil")
	}

	p := config.Patrols

	// Verify all patrols are enabled with expected defaults
	if p.WispReaper == nil || !p.WispReaper.Enabled {
		t.Error("expected wisp_reaper to be enabled")
	}
	if p.WispReaper.IntervalStr != "30m" {
		t.Errorf("expected wisp_reaper interval 30m, got %s", p.WispReaper.IntervalStr)
	}
	if p.WispReaper.DeleteAgeStr != "168h" {
		t.Errorf("expected wisp_reaper delete_age 168h, got %s", p.WispReaper.DeleteAgeStr)
	}

	if p.CompactorDog == nil || !p.CompactorDog.Enabled {
		t.Error("expected compactor_dog to be enabled")
	}
	if p.CompactorDog.Threshold != 500 {
		t.Errorf("expected compactor_dog threshold 500, got %d", p.CompactorDog.Threshold)
	}

	if p.DoctorDog == nil || !p.DoctorDog.Enabled {
		t.Error("expected doctor_dog to be enabled")
	}

	if p.JanitorDog == nil || !p.JanitorDog.Enabled {
		t.Error("expected janitor_dog to be enabled")
	}

	if p.JsonlGitBackup == nil || !p.JsonlGitBackup.Enabled {
		t.Error("expected jsonl_git_backup to be enabled")
	}
	if p.JsonlGitBackup.Scrub == nil || !*p.JsonlGitBackup.Scrub {
		t.Error("expected jsonl_git_backup scrub to be true")
	}

	if p.DoltBackup == nil || !p.DoltBackup.Enabled {
		t.Error("expected dolt_backup to be enabled")
	}

	if p.ScheduledMaintenance == nil || !p.ScheduledMaintenance.Enabled {
		t.Error("expected scheduled_maintenance to be enabled")
	}
	if p.ScheduledMaintenance.Window != "03:00" {
		t.Errorf("expected maintenance window 03:00, got %s", p.ScheduledMaintenance.Window)
	}
	if p.ScheduledMaintenance.Threshold == nil || *p.ScheduledMaintenance.Threshold != 1000 {
		t.Error("expected maintenance threshold 1000")
	}
}

func TestEnsureLifecycleDefaults_NilConfig(t *testing.T) {
	if EnsureLifecycleDefaults(nil) {
		t.Error("expected false for nil config")
	}
}

func TestEnsureLifecycleDefaults_EmptyConfig(t *testing.T) {
	config := &DaemonPatrolConfig{Type: "daemon-patrol-config", Version: 1}
	changed := EnsureLifecycleDefaults(config)

	if !changed {
		t.Error("expected changes for empty config")
	}
	if config.Patrols == nil {
		t.Fatal("expected patrols to be set")
	}
	if config.Patrols.WispReaper == nil || !config.Patrols.WispReaper.Enabled {
		t.Error("expected wisp_reaper to be set")
	}
	if config.Patrols.CompactorDog == nil || !config.Patrols.CompactorDog.Enabled {
		t.Error("expected compactor_dog to be set")
	}
}

func TestEnsureLifecycleDefaults_PreservesExisting(t *testing.T) {
	// Config with user-customized wisp_reaper
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:     true,
				IntervalStr: "1h", // User customized to 1h
				DeleteAgeStr: "336h", // User customized to 14 days
			},
		},
	}

	changed := EnsureLifecycleDefaults(config)

	if !changed {
		t.Error("expected changes (other patrols were nil)")
	}

	// User's wisp_reaper should be preserved
	if config.Patrols.WispReaper.IntervalStr != "1h" {
		t.Errorf("expected preserved interval 1h, got %s", config.Patrols.WispReaper.IntervalStr)
	}
	if config.Patrols.WispReaper.DeleteAgeStr != "336h" {
		t.Errorf("expected preserved delete_age 336h, got %s", config.Patrols.WispReaper.DeleteAgeStr)
	}

	// Other patrols should be filled in
	if config.Patrols.CompactorDog == nil || !config.Patrols.CompactorDog.Enabled {
		t.Error("expected compactor_dog to be filled in")
	}
	if config.Patrols.DoctorDog == nil {
		t.Error("expected doctor_dog to be filled in")
	}
}

func TestEnsureLifecycleDefaults_FullyConfigured(t *testing.T) {
	// Config with all patrols already set (even if disabled)
	threshold := 2000
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			WispReaper:   &WispReaperConfig{Enabled: false},
			CompactorDog: &CompactorDogConfig{Enabled: false},
			DoctorDog:    &DoctorDogConfig{Enabled: false},
			JanitorDog:   &JanitorDogConfig{Enabled: false},
			JsonlGitBackup:       &JsonlGitBackupConfig{Enabled: false},
			DoltBackup:           &DoltBackupConfig{Enabled: false},
			ScheduledMaintenance: &ScheduledMaintenanceConfig{Enabled: false, Threshold: &threshold},
		},
	}

	changed := EnsureLifecycleDefaults(config)

	if changed {
		t.Error("expected no changes for fully configured config")
	}

	// User's disabled settings should be preserved
	if config.Patrols.WispReaper.Enabled {
		t.Error("expected wisp_reaper to remain disabled")
	}
	if config.Patrols.ScheduledMaintenance.Threshold == nil || *config.Patrols.ScheduledMaintenance.Threshold != 2000 {
		t.Error("expected threshold to remain 2000")
	}
}

func TestEnsureLifecycleConfigFile_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := EnsureLifecycleConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created
	configFile := filepath.Join(mayorDir, "daemon.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	var config DaemonPatrolConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if config.Patrols == nil {
		t.Fatal("expected patrols in created config")
	}
	if config.Patrols.WispReaper == nil || !config.Patrols.WispReaper.Enabled {
		t.Error("expected wisp_reaper to be enabled in new config")
	}
}

func TestEnsureLifecycleConfigFile_ExistingPartial(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write partial config with just env and wisp_reaper
	existing := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Env:     map[string]string{"GT_DOLT_PORT": "3307"},
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:     true,
				IntervalStr: "1h",
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	configFile := filepath.Join(mayorDir, "daemon.json")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	err := EnsureLifecycleConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify
	data, _ = os.ReadFile(configFile)
	var config DaemonPatrolConfig
	json.Unmarshal(data, &config)

	// Existing env preserved
	if config.Env["GT_DOLT_PORT"] != "3307" {
		t.Error("expected env to be preserved")
	}

	// Existing wisp_reaper preserved
	if config.Patrols.WispReaper.IntervalStr != "1h" {
		t.Errorf("expected preserved interval 1h, got %s", config.Patrols.WispReaper.IntervalStr)
	}

	// New patrols filled in
	if config.Patrols.CompactorDog == nil || !config.Patrols.CompactorDog.Enabled {
		t.Error("expected compactor_dog to be added")
	}
	if config.Patrols.DoctorDog == nil || !config.Patrols.DoctorDog.Enabled {
		t.Error("expected doctor_dog to be added")
	}
	if config.Patrols.ScheduledMaintenance == nil || !config.Patrols.ScheduledMaintenance.Enabled {
		t.Error("expected scheduled_maintenance to be added")
	}
}

func TestEnsureLifecycleConfigFile_AlreadyComplete(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write fully configured file
	config := DefaultLifecycleConfig()
	data, _ := json.MarshalIndent(config, "", "  ")
	configFile := filepath.Join(mayorDir, "daemon.json")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Get mod time before
	info1, _ := os.Stat(configFile)

	err := EnsureLifecycleConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should not have been rewritten (same mod time)
	info2, _ := os.Stat(configFile)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("expected file to not be rewritten when already complete")
	}
}
