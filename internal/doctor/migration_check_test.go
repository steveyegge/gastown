package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationReadinessCheck_AllDolt(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with Dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write Dolt metadata
	metadata := `{"backend": "dolt", "database": "dolt"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewMigrationReadinessCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}

	readiness := check.Readiness()
	if !readiness.Ready {
		t.Errorf("Expected Ready=true, got false. Blockers: %v", readiness.Blockers)
	}
}

func TestMigrationReadinessCheck_SQLiteNeedsMigration(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with SQLite metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write SQLite metadata (or no backend field = defaults to SQLite)
	metadata := `{"backend": "sqlite", "database": "sqlite3"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewMigrationReadinessCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning for SQLite backend, got %v: %s", result.Status, result.Message)
	}

	readiness := check.Readiness()
	if readiness.Ready {
		t.Errorf("Expected Ready=false for SQLite backend, got true")
	}

	// Check that town-root rig is in the list
	found := false
	for _, rig := range readiness.Rigs {
		if rig.Name == "town-root" && rig.NeedsMigration {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected town-root to need migration, rigs: %v", readiness.Rigs)
	}
}

func TestUnmigratedRigCheck_AllDolt(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with Dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	metadata := `{"backend": "dolt"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewUnmigratedRigCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestUnmigratedRigCheck_SQLiteDetected(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with SQLite metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	metadata := `{"backend": "sqlite"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with empty rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewUnmigratedRigCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	// Should have town-root in details
	foundTownRoot := false
	for _, detail := range result.Details {
		if detail == "town-root" {
			foundTownRoot = true
			break
		}
	}
	if !foundTownRoot {
		t.Errorf("Expected 'town-root' in details, got: %v", result.Details)
	}
}

func TestDoltMetadataCheck_NoDoltData(t *testing.T) {
	tmpDir := t.TempDir()

	// No .dolt-data directory = dolt not in use
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewDoltMetadataCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK when no dolt data dir, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltMetadataCheck_MissingMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dolt-data/hq (dolt is in use)
	doltDataDir := filepath.Join(tmpDir, ".dolt-data", "hq", ".dolt")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .beads directory WITHOUT dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"database": "beads.db"}`), 0644); err != nil {
		t.Fatal(err)
	}

	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewDoltMetadataCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning for missing dolt metadata, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltMetadataCheck_CorrectMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dolt-data/hq
	doltDataDir := filepath.Join(tmpDir, ".dolt-data", "hq", ".dolt")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .beads directory WITH correct dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"database":"dolt","backend":"dolt","dolt_mode":"server","dolt_database":"hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewDoltMetadataCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK for correct dolt metadata, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltMetadataCheck_FixWritesMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dolt-data/hq
	doltDataDir := filepath.Join(tmpDir, ".dolt-data", "hq", ".dolt")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .beads directory without dolt metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"database": "beads.db"}`), 0644); err != nil {
		t.Fatal(err)
	}

	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewDoltMetadataCheck()

	// Run to detect missing metadata
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("Expected StatusWarning, got %v", result.Status)
	}

	// Fix should write dolt metadata
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Run again to verify fix
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltMetadataCheck_RigWithMayorBeads(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dolt-data/myrig
	doltDataDir := filepath.Join(tmpDir, ".dolt-data", "myrig", ".dolt")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig with mayor/rig/.beads (no metadata)
	mayorBeads := filepath.Join(tmpDir, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Rigs.json lists "myrig"
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"rigs":{"myrig":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewDoltMetadataCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning for rig without metadata, got %v: %s", result.Status, result.Message)
	}

	// Fix it
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify fix wrote to mayor/rig/.beads/metadata.json
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

func TestBdSupportsDolt(t *testing.T) {
	check := &MigrationReadinessCheck{}

	tests := []struct {
		version string
		want    bool
	}{
		{"bd version 0.49.3 (commit)", true},
		{"bd version 0.40.0 (commit)", true},
		{"bd version 0.39.9 (commit)", false},
		{"bd version 0.30.0 (commit)", false},
		{"bd version 1.0.0 (commit)", true},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := check.bdSupportsDolt(tt.version)
		if got != tt.want {
			t.Errorf("bdSupportsDolt(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
