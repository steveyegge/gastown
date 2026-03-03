package daemon

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDoctorDogInterval(t *testing.T) {
	// Default interval
	if got := doctorDogInterval(nil); got != defaultDoctorDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultDoctorDogInterval, got)
	}

	// Custom interval
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoctorDog: &DoctorDogConfig{
				Enabled:     true,
				IntervalStr: "10m",
			},
		},
	}
	if got := doctorDogInterval(config); got != 10*time.Minute {
		t.Errorf("expected 10m interval, got %v", got)
	}

	// Invalid interval falls back to default
	config.Patrols.DoctorDog.IntervalStr = "invalid"
	if got := doctorDogInterval(config); got != defaultDoctorDogInterval {
		t.Errorf("expected default interval for invalid config, got %v", got)
	}
}

func TestDoctorDogDatabases(t *testing.T) {
	// Default databases
	dbs := doctorDogDatabases(nil)
	if len(dbs) != 6 {
		t.Errorf("expected 6 default databases, got %d", len(dbs))
	}

	// Custom databases
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoctorDog: &DoctorDogConfig{
				Enabled:   true,
				Databases: []string{"hq", "beads"},
			},
		},
	}
	dbs = doctorDogDatabases(config)
	if len(dbs) != 2 {
		t.Errorf("expected 2 custom databases, got %d", len(dbs))
	}
}

func TestIsPatrolEnabled_DoctorDog(t *testing.T) {
	// Nil config: disabled (opt-in patrol)
	if IsPatrolEnabled(nil, "doctor_dog") {
		t.Error("expected doctor_dog to be disabled with nil config")
	}

	// Empty patrols: disabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	if IsPatrolEnabled(config, "doctor_dog") {
		t.Error("expected doctor_dog to be disabled by default")
	}

	// Explicitly enabled
	config.Patrols.DoctorDog = &DoctorDogConfig{Enabled: true}
	if !IsPatrolEnabled(config, "doctor_dog") {
		t.Error("expected doctor_dog to be enabled when configured")
	}

	// Explicitly disabled
	config.Patrols.DoctorDog = &DoctorDogConfig{Enabled: false}
	if IsPatrolEnabled(config, "doctor_dog") {
		t.Error("expected doctor_dog to be disabled when explicitly disabled")
	}
}

func TestDoctorDogDefaultThresholds(t *testing.T) {
	// Verify default thresholds are sane
	if defaultDoctorDogLatencyAlertMs <= 0 {
		t.Error("latency alert threshold must be positive")
	}
	if defaultDoctorDogOrphanAlertCount <= 0 {
		t.Error("orphan alert count must be positive")
	}
	if defaultDoctorDogBackupStaleSeconds <= 0 {
		t.Error("backup stale threshold must be positive")
	}

	// Verify defaults match spec: latency > 5s, orphans > 20, backup > 1hr
	if defaultDoctorDogLatencyAlertMs != 5000.0 {
		t.Errorf("expected latency alert at 5000ms, got %.0f", defaultDoctorDogLatencyAlertMs)
	}
	if defaultDoctorDogOrphanAlertCount != 20 {
		t.Errorf("expected orphan alert at 20, got %d", defaultDoctorDogOrphanAlertCount)
	}
	if defaultDoctorDogBackupStaleSeconds != 3600.0 {
		t.Errorf("expected backup stale at 3600s, got %.0f", defaultDoctorDogBackupStaleSeconds)
	}
}

func TestDoctorDogThresholds(t *testing.T) {
	// Nil config returns defaults
	lat, orphan, backup := doctorDogThresholds(nil)
	if lat != defaultDoctorDogLatencyAlertMs {
		t.Errorf("expected default latency %.0f, got %.0f", defaultDoctorDogLatencyAlertMs, lat)
	}
	if orphan != defaultDoctorDogOrphanAlertCount {
		t.Errorf("expected default orphan %d, got %d", defaultDoctorDogOrphanAlertCount, orphan)
	}
	if backup != defaultDoctorDogBackupStaleSeconds {
		t.Errorf("expected default backup %.0f, got %.0f", defaultDoctorDogBackupStaleSeconds, backup)
	}

	// Custom config overrides
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoctorDog: &DoctorDogConfig{
				Enabled:            true,
				LatencyAlertMs:     3000.0,
				OrphanAlertCount:   10,
				BackupStaleSeconds: 1800.0,
			},
		},
	}
	lat, orphan, backup = doctorDogThresholds(config)
	if lat != 3000.0 {
		t.Errorf("expected custom latency 3000, got %.0f", lat)
	}
	if orphan != 10 {
		t.Errorf("expected custom orphan 10, got %d", orphan)
	}
	if backup != 1800.0 {
		t.Errorf("expected custom backup 1800, got %.0f", backup)
	}

	// Partial override: only latency, rest use defaults
	config.Patrols.DoctorDog = &DoctorDogConfig{
		Enabled:        true,
		LatencyAlertMs: 2000.0,
	}
	lat, orphan, backup = doctorDogThresholds(config)
	if lat != 2000.0 {
		t.Errorf("expected custom latency 2000, got %.0f", lat)
	}
	if orphan != defaultDoctorDogOrphanAlertCount {
		t.Errorf("expected default orphan, got %d", orphan)
	}
	if backup != defaultDoctorDogBackupStaleSeconds {
		t.Errorf("expected default backup, got %.0f", backup)
	}
}

func TestDoctorDogConfigBackwardsCompat(t *testing.T) {
	// Verify that configs with the old max_db_count field can still be parsed
	// (JSON decoder ignores unknown fields by default).
	jsonData := `{"enabled": true, "interval": "3m", "max_db_count": 10}`

	var config DoctorDogConfig
	if err := json.Unmarshal([]byte(jsonData), &config); err != nil {
		t.Fatalf("failed to unmarshal config with old max_db_count field: %v", err)
	}

	if !config.Enabled {
		t.Error("expected enabled=true")
	}
	if config.IntervalStr != "3m" {
		t.Errorf("expected interval=3m, got %s", config.IntervalStr)
	}
}

func TestDoctorDogConfigThresholdFields(t *testing.T) {
	// Verify new threshold fields parse from JSON correctly
	jsonData := `{"enabled": true, "latency_alert_ms": 3000, "orphan_alert_count": 15, "backup_stale_seconds": 1800}`

	var config DoctorDogConfig
	if err := json.Unmarshal([]byte(jsonData), &config); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if config.LatencyAlertMs != 3000.0 {
		t.Errorf("expected latency_alert_ms=3000, got %.0f", config.LatencyAlertMs)
	}
	if config.OrphanAlertCount != 15 {
		t.Errorf("expected orphan_alert_count=15, got %d", config.OrphanAlertCount)
	}
	if config.BackupStaleSeconds != 1800.0 {
		t.Errorf("expected backup_stale_seconds=1800, got %.0f", config.BackupStaleSeconds)
	}
}
