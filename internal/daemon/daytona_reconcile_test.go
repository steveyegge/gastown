package daemon

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// TestReconcileDaytonaWorkspaces_SkipsWhenNoTownConfig verifies that
// reconcileDaytonaWorkspaces returns silently when town config is missing.
func TestReconcileDaytonaWorkspaces_SkipsWhenNoTownConfig(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	// No mayor/town.json exists — should skip silently with a warning.
	d.reconcileDaytonaWorkspaces()

	if !strings.Contains(logBuf.String(), "cannot load town config") {
		t.Errorf("expected warning about missing town config, got: %q", logBuf.String())
	}
}

// TestReconcileDaytonaWorkspaces_SkipsWhenNoInstallationID verifies that
// reconcileDaytonaWorkspaces returns silently when InstallationID is empty.
func TestReconcileDaytonaWorkspaces_SkipsWhenNoInstallationID(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Town config with no InstallationID.
	townCfg := `{"type":"town","version":1,"name":"test","created_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townCfg), 0644); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	d.reconcileDaytonaWorkspaces()

	if !strings.Contains(logBuf.String(), "no installation ID") {
		t.Errorf("expected warning about missing installation ID, got: %q", logBuf.String())
	}
}

// TestReconcileDaytonaWorkspaces_SkipsLocalRigs verifies that rigs without
// RemoteBackend are skipped during reconciliation.
func TestReconcileDaytonaWorkspaces_SkipsLocalRigs(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	setupTownConfig(t, townRoot, "abc12345-test-uuid")

	// Create a local rig (no remote_backend).
	rigDir := filepath.Join(townRoot, "localrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json listing the rig.
	setupRigsJSON(t, townRoot, "localrig")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	d.reconcileDaytonaWorkspaces()

	// Should NOT log "reconciliation starting" since no daytona rigs exist.
	if strings.Contains(logBuf.String(), "reconciliation starting") {
		t.Errorf("should not start reconciliation for local-only rigs, got: %q", logBuf.String())
	}
}

// TestReconcileDaytonaWorkspaces_RunsForRemoteRig verifies that rigs with
// RemoteBackend trigger reconciliation (even if it fails due to no daytona CLI).
func TestReconcileDaytonaWorkspaces_RunsForRemoteRig(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	setupTownConfig(t, townRoot, "abc12345-test-uuid")

	// Create a remote rig with RemoteBackend.
	rigDir := filepath.Join(townRoot, "remotrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath,
		[]byte(`{"remote_backend":{"provider":"daytona"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	setupRigsJSON(t, townRoot, "remotrig")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		bdPath:  "bd", // will fail since no real bd but that's OK
	}

	d.reconcileDaytonaWorkspaces()

	// Should log "reconciliation starting" since a daytona rig exists.
	if !strings.Contains(logBuf.String(), "reconciliation starting") {
		t.Errorf("expected reconciliation to start for remote rig, got: %q", logBuf.String())
	}
}

// TestReconcileDaytonaWorkspaces_NoKnownRigs verifies early return when
// no rigs are registered.
func TestReconcileDaytonaWorkspaces_NoKnownRigs(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	setupTownConfig(t, townRoot, "abc12345-test-uuid")
	// No rigs.json file at all.

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	d.reconcileDaytonaWorkspaces()

	// Should not log anything about reconciliation.
	if strings.Contains(logBuf.String(), "reconcil") {
		t.Errorf("should not reconcile with no known rigs, got: %q", logBuf.String())
	}
}

// TestReconcileDaytonaRig_LogsHealthyReport verifies that reconcileDaytonaRig
// logs the discovery report with healthy/orphaned counts.
func TestReconcileDaytonaRig_LogsHealthyReport(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		bdPath:  "bd",
	}

	// reconcileDaytonaRig will call ListOwned which shells out to `daytona list`.
	// Since daytona isn't installed, it will log a warning about list failure.
	// This is the expected behavior — we verify the function runs and logs.
	d.reconcileDaytonaRig("testrig", "abc12345", &config.RemoteBackend{Provider: "daytona"})

	// Should log about list failure (daytona not installed in test env).
	if !strings.Contains(logBuf.String(), "list workspaces failed") {
		t.Errorf("expected log about workspace list failure, got: %q", logBuf.String())
	}
}

// TestDaytonaReconcileInterval_DefaultWhenNilConfig verifies the default interval.
func TestDaytonaReconcileInterval_DefaultWhenNilConfig(t *testing.T) {
	t.Parallel()
	if got := daytonaReconcileInterval(nil); got != 30*time.Minute {
		t.Errorf("expected 30m default, got %v", got)
	}
}

// TestDaytonaReconcileInterval_CustomInterval verifies a custom interval is used.
func TestDaytonaReconcileInterval_CustomInterval(t *testing.T) {
	t.Parallel()
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DaytonaReconcile: &DaytonaReconcileConfig{
				Enabled:     true,
				IntervalStr: "15m",
			},
		},
	}
	if got := daytonaReconcileInterval(config); got != 15*time.Minute {
		t.Errorf("expected 15m, got %v", got)
	}
}

// TestDaytonaReconcileInterval_InvalidFallsBackToDefault verifies invalid interval uses default.
func TestDaytonaReconcileInterval_InvalidFallsBackToDefault(t *testing.T) {
	t.Parallel()
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			DaytonaReconcile: &DaytonaReconcileConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}
	if got := daytonaReconcileInterval(config); got != 30*time.Minute {
		t.Errorf("expected 30m default for invalid interval, got %v", got)
	}
}

// TestIsPatrolEnabled_DaytonaReconcile verifies the patrol enable check.
func TestIsPatrolEnabled_DaytonaReconcile(t *testing.T) {
	t.Parallel()

	// Nil config → disabled (opt-in patrol).
	if IsPatrolEnabled(nil, "daytona_reconcile") {
		t.Error("expected disabled for nil config")
	}

	// Nil DaytonaReconcile → disabled.
	config := &DaemonPatrolConfig{Patrols: &PatrolsConfig{}}
	if IsPatrolEnabled(config, "daytona_reconcile") {
		t.Error("expected disabled for nil DaytonaReconcile")
	}

	// Explicitly enabled.
	config.Patrols.DaytonaReconcile = &DaytonaReconcileConfig{Enabled: true}
	if !IsPatrolEnabled(config, "daytona_reconcile") {
		t.Error("expected enabled when Enabled=true")
	}

	// Explicitly disabled.
	config.Patrols.DaytonaReconcile = &DaytonaReconcileConfig{Enabled: false}
	if IsPatrolEnabled(config, "daytona_reconcile") {
		t.Error("expected disabled when Enabled=false")
	}
}

// --- Test helpers ---

// setupTownConfig creates a valid mayor/town.json with the given installation ID.
func setupTownConfig(t *testing.T, townRoot, installationID string) {
	t.Helper()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := map[string]interface{}{
		"type":            "town",
		"version":         1,
		"name":            "test",
		"installation_id": installationID,
		"created_at":      "2025-01-01T00:00:00Z",
	}
	data, err := json.Marshal(townCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// setupRigsJSON creates a mayor/rigs.json with the given rig names.
func setupRigsJSON(t *testing.T, townRoot string, rigNames ...string) {
	t.Helper()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigs := make(map[string]interface{})
	for _, name := range rigNames {
		rigs[name] = map[string]interface{}{}
	}
	data, err := json.Marshal(map[string]interface{}{"rigs": rigs})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
