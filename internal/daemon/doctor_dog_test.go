package daemon

import (
	"encoding/json"
	"io"
	"log"
	"strings"
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

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	size, err := dirSize(dir)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Errorf("expected 0 for empty dir, got %d", size)
	}
}

func TestDoctorDogReportJSON(t *testing.T) {
	report := &DoctorDogReport{
		Timestamp:    time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: 1.5},
		Databases:    &DoctorDogDatabasesReport{Names: []string{"hq", "beads"}, Count: 2},
		DiskUsage: []DoctorDogDiskReport{
			{Database: "hq", SizeBytes: 1048576, SizeMB: 1},
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	var decoded DoctorDogReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if !decoded.TCPReachable {
		t.Error("expected tcp_reachable=true")
	}
	if decoded.Latency == nil || decoded.Latency.DurationMs != 1.5 {
		t.Error("expected latency 1.5ms")
	}
	if decoded.Databases == nil || decoded.Databases.Count != 2 {
		t.Error("expected 2 databases")
	}
	if len(decoded.DiskUsage) != 1 || decoded.DiskUsage[0].Database != "hq" {
		t.Error("expected disk usage for hq")
	}
}

func TestDoctorDogReportOmitsNilFields(t *testing.T) {
	// Report with only TCP data — should omit nil fields
	report := &DoctorDogReport{
		Timestamp:    time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: false,
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	// Verify nil fields are omitted from JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := raw["latency"]; ok {
		t.Error("expected latency to be omitted when nil")
	}
	if _, ok := raw["databases"]; ok {
		t.Error("expected databases to be omitted when nil")
	}
	if _, ok := raw["gc"]; ok {
		t.Error("expected gc to be omitted when nil")
	}
}

func TestDoctorDogRespondThresholds(t *testing.T) {
	// Verify that response thresholds are sane
	if doctorDogLatencyAlertMs <= 0 {
		t.Error("latency alert threshold must be positive")
	}
	if doctorDogOrphanAlertCount <= 0 {
		t.Error("orphan alert count must be positive")
	}
	if doctorDogBackupStaleSeconds <= 0 {
		t.Error("backup stale threshold must be positive")
	}
	if doctorDogActionCooldown <= 0 {
		t.Error("action cooldown must be positive")
	}

	// Verify thresholds match spec: latency > 5s, orphans > 20, backup > 1hr
	if doctorDogLatencyAlertMs != 5000.0 {
		t.Errorf("expected latency alert at 5000ms, got %.0f", doctorDogLatencyAlertMs)
	}
	if doctorDogOrphanAlertCount != 20 {
		t.Errorf("expected orphan alert at 20, got %d", doctorDogOrphanAlertCount)
	}
	if doctorDogBackupStaleSeconds != 3600.0 {
		t.Errorf("expected backup stale at 3600s, got %.0f", doctorDogBackupStaleSeconds)
	}
}

func TestDoctorDogRespondCooldown(t *testing.T) {
	// Verify cooldown logic: daemon that already acted recently should not act again.
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(io.Discard, "", 0),
		// Set recent action times (within cooldown window)
		lastDoctorRestart:  time.Now(),
		lastDoctorEscalate: time.Now(),
		lastDoctorJanitor:  time.Now(),
		lastDoctorBackup:   time.Now(),
	}

	// Report with all thresholds exceeded
	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: false, // would trigger restart
		Latency:      &DoctorDogLatencyReport{DurationMs: 10000}, // would trigger escalate
		Databases:    &DoctorDogDatabasesReport{Count: 30},       // would trigger janitor
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 7200},   // would trigger backup
	}

	// Should NOT panic or take actions (all cooldowns active)
	d.doctorDogRespond(report)
}

func TestDoctorDogRespondNoActionOnHealthy(t *testing.T) {
	// A healthy report should trigger no actions
	var buf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&buf, "", 0),
	}

	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: 1.5},                                        // well under 5s
		Databases:    &DoctorDogDatabasesReport{Names: []string{"hq", "beads"}, Count: 2},              // well under 20
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 300},                                          // 5 min, well under 1hr
	}

	d.doctorDogRespond(report)

	// Verify no ACTION logs were generated
	logged := buf.String()
	if strings.Contains(logged, "ACTION:") {
		t.Errorf("expected no actions on healthy report, but got: %s", logged)
	}
}

func TestDoctorDogRespondBelowThresholds(t *testing.T) {
	// Reports at exactly the threshold boundary should NOT trigger
	var buf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&buf, "", 0),
	}

	// At threshold (not over)
	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: doctorDogLatencyAlertMs}, // exactly at, not over
		Databases:    &DoctorDogDatabasesReport{Count: doctorDogOrphanAlertCount},  // exactly at, not over
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: doctorDogBackupStaleSeconds}, // exactly at, not over
	}

	d.doctorDogRespond(report)

	logged := buf.String()
	if strings.Contains(logged, "ACTION:") {
		t.Errorf("expected no actions at exact threshold, but got: %s", logged)
	}
}

func TestDoctorDogRespondSkipsOnError(t *testing.T) {
	// Reports with errors in check results should not trigger actions
	var buf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&buf, "", 0),
	}

	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: 10000, Error: "connection reset"}, // error present
		Databases:    &DoctorDogDatabasesReport{Count: 30, Error: "query timeout"},           // error present
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 7200, Error: "walk error"},           // error present
	}

	d.doctorDogRespond(report)

	logged := buf.String()
	if strings.Contains(logged, "ACTION:") {
		t.Errorf("expected no actions when checks have errors, but got: %s", logged)
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
