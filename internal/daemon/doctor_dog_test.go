package daemon

import (
	"encoding/json"
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
	if _, ok := raw["zombies"]; ok {
		t.Error("expected zombies to be omitted when nil")
	}
	if _, ok := raw["recommendations"]; ok {
		t.Error("expected recommendations to be omitted when nil")
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

func TestDoctorDogRespondGeneratesRecommendations(t *testing.T) {
	// Report with all thresholds exceeded should generate recommendations
	var buf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&buf, "", 0),
	}

	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: false,                                           // triggers restart_server
		Latency:      &DoctorDogLatencyReport{DurationMs: 10000},      // triggers escalate_latency
		Databases:    &DoctorDogDatabasesReport{Count: 30},            // triggers run_cleanup
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 7200},        // triggers sync_backup
	}

	d.doctorDogRespond(report)

	// Verify recommendations were generated
	if len(report.Recommendations) != 4 {
		t.Fatalf("expected 4 recommendations, got %d", len(report.Recommendations))
	}

	// Verify specific recommendations
	actions := make(map[string]DoctorDogRecommendation)
	for _, r := range report.Recommendations {
		actions[r.Action] = r
	}

	if r, ok := actions["restart_server"]; !ok {
		t.Error("expected restart_server recommendation")
	} else if r.Severity != "critical" {
		t.Errorf("restart_server severity: expected critical, got %s", r.Severity)
	}

	if r, ok := actions["escalate_latency"]; !ok {
		t.Error("expected escalate_latency recommendation")
	} else if r.Severity != "high" {
		t.Errorf("escalate_latency severity: expected high, got %s", r.Severity)
	}

	if r, ok := actions["run_cleanup"]; !ok {
		t.Error("expected run_cleanup recommendation")
	} else if r.Severity != "warning" {
		t.Errorf("run_cleanup severity: expected warning, got %s", r.Severity)
	}

	if r, ok := actions["sync_backup"]; !ok {
		t.Error("expected sync_backup recommendation")
	} else if r.Severity != "warning" {
		t.Errorf("sync_backup severity: expected warning, got %s", r.Severity)
	}

	// Verify RECOMMEND logs (not ACTION)
	logged := buf.String()
	if strings.Contains(logged, "ACTION:") {
		t.Errorf("expected no ACTION logs, but got: %s", logged)
	}
	if !strings.Contains(logged, "RECOMMEND:") {
		t.Errorf("expected RECOMMEND logs, but got: %s", logged)
	}
}

func TestDoctorDogRespondNoRecommendationsOnHealthy(t *testing.T) {
	// A healthy report should generate no recommendations
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
		Latency:      &DoctorDogLatencyReport{DurationMs: 1.5},
		Databases:    &DoctorDogDatabasesReport{Names: []string{"hq", "beads"}, Count: 2},
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 300},
	}

	d.doctorDogRespond(report)

	if len(report.Recommendations) != 0 {
		t.Errorf("expected no recommendations on healthy report, got %d: %+v",
			len(report.Recommendations), report.Recommendations)
	}

	logged := buf.String()
	if strings.Contains(logged, "RECOMMEND:") {
		t.Errorf("expected no RECOMMEND logs on healthy report, but got: %s", logged)
	}
}

func TestDoctorDogRespondBelowThresholds(t *testing.T) {
	// Reports at exactly the threshold boundary should NOT trigger recommendations
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
		Latency:      &DoctorDogLatencyReport{DurationMs: defaultDoctorDogLatencyAlertMs},
		Databases:    &DoctorDogDatabasesReport{Count: defaultDoctorDogOrphanAlertCount},
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: defaultDoctorDogBackupStaleSeconds},
	}

	d.doctorDogRespond(report)

	if len(report.Recommendations) != 0 {
		t.Errorf("expected no recommendations at exact threshold, got %d: %+v",
			len(report.Recommendations), report.Recommendations)
	}
}

func TestDoctorDogRespondSkipsOnError(t *testing.T) {
	// Reports with errors in check results should not generate recommendations
	var buf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&buf, "", 0),
	}

	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: 10000, Error: "connection reset"},
		Databases:    &DoctorDogDatabasesReport{Count: 30, Error: "query timeout"},
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 7200, Error: "walk error"},
	}

	d.doctorDogRespond(report)

	if len(report.Recommendations) != 0 {
		t.Errorf("expected no recommendations when checks have errors, got %d: %+v",
			len(report.Recommendations), report.Recommendations)
	}
}

func TestDoctorDogRespondWithCustomThresholds(t *testing.T) {
	// Custom thresholds should be respected
	var buf strings.Builder
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DoctorDog: &DoctorDogConfig{
				Enabled:            true,
				LatencyAlertMs:     100.0,  // Very low threshold
				OrphanAlertCount:   5,      // Very low threshold
				BackupStaleSeconds: 60.0,   // Very low threshold
			},
		},
	}
	d := &Daemon{
		config:       &Config{TownRoot: t.TempDir()},
		logger:       log.New(&buf, "", 0),
		patrolConfig: config,
	}

	// Values that exceed custom thresholds but are under defaults
	report := &DoctorDogReport{
		Timestamp:    time.Now(),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: true,
		Latency:      &DoctorDogLatencyReport{DurationMs: 200},   // > 100 custom, < 5000 default
		Databases:    &DoctorDogDatabasesReport{Count: 10},        // > 5 custom, < 20 default
		BackupAge:    &DoctorDogBackupReport{AgeSeconds: 120},     // > 60 custom, < 3600 default
	}

	d.doctorDogRespond(report)

	// Should generate 3 recommendations (latency, janitor, backup) using custom thresholds
	if len(report.Recommendations) != 3 {
		t.Fatalf("expected 3 recommendations with custom thresholds, got %d: %+v",
			len(report.Recommendations), report.Recommendations)
	}

	actions := make(map[string]bool)
	for _, r := range report.Recommendations {
		actions[r.Action] = true
	}
	if !actions["escalate_latency"] {
		t.Error("expected escalate_latency recommendation")
	}
	if !actions["run_cleanup"] {
		t.Error("expected run_cleanup recommendation")
	}
	if !actions["sync_backup"] {
		t.Error("expected sync_backup recommendation")
	}
}

func TestDoctorDogRecommendationJSON(t *testing.T) {
	// Verify recommendations serialize/deserialize correctly
	report := &DoctorDogReport{
		Timestamp:    time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC),
		Host:         "127.0.0.1",
		Port:         3307,
		TCPReachable: false,
		Recommendations: []DoctorDogRecommendation{
			{Action: "restart_server", Reason: "TCP unreachable", Severity: "critical"},
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded DoctorDogReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(decoded.Recommendations))
	}
	r := decoded.Recommendations[0]
	if r.Action != "restart_server" || r.Severity != "critical" {
		t.Errorf("unexpected recommendation: %+v", r)
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
