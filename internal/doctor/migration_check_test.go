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

	// Create dolt directory to make it fully migrated
	if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755); err != nil {
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

	// Create a beads.db to make it recognizable
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte(""), 0644); err != nil {
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

	if result.Status != StatusError {
		t.Errorf("Expected StatusError for SQLite backend, got %v: %s", result.Status, result.Message)
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
			if rig.State != StateNeverMigrated {
				t.Errorf("Expected state never-migrated, got %s", rig.State)
			}
			break
		}
	}
	if !found {
		t.Errorf("Expected town-root to need migration, rigs: %v", readiness.Rigs)
	}
}

func TestRigBackendStatusCheck_AllDolt(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with Dolt metadata + dolt dir
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755); err != nil {
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
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestRigBackendStatusCheck_SQLiteDetected(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads directory with SQLite metadata + beads.db
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	metadata := `{"backend": "sqlite"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte(""), 0644); err != nil {
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
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("Expected StatusError, got %v: %s", result.Status, result.Message)
	}

	// Should mention never-migrated in details
	foundNeverMigrated := false
	for _, detail := range result.Details {
		if contains(detail, "never migrated") {
			foundNeverMigrated = true
			break
		}
	}
	if !foundNeverMigrated {
		t.Errorf("Expected 'never migrated' in details, got: %v", result.Details)
	}
}

func TestRigBackendStatusCheck_PartiallyMigrated(t *testing.T) {
	// Create temp directory structure: sqlite metadata but dolt-data exists
	tmpDir := t.TempDir()

	// Create .beads directory with sqlite metadata
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend": "sqlite"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// BUT also create .dolt-data/hq/.dolt (centralized dolt data exists)
	doltDataDir := filepath.Join(tmpDir, ".dolt-data", "hq", ".dolt")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
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
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("Expected StatusError, got %v: %s", result.Status, result.Message)
	}

	// Should mention partially-migrated
	foundPartial := false
	for _, detail := range result.Details {
		if contains(detail, "partially migrated") {
			foundPartial = true
			break
		}
	}
	if !foundPartial {
		t.Errorf("Expected 'partially migrated' in details, got: %v", result.Details)
	}
}

func TestRigBackendStatusCheck_JSONLOnly(t *testing.T) {
	// Rig with JSONL but no database backend at all
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Only JSONL, no metadata, no beads.db, no dolt
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(`{"id":"test"}`), 0644); err != nil {
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
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("Expected StatusError, got %v: %s", result.Status, result.Message)
	}

	// Should classify as never-migrated with JSONL evidence
	foundJSONL := false
	for _, detail := range result.Details {
		if contains(detail, "JSONL only") {
			foundJSONL = true
			break
		}
	}
	if !foundJSONL {
		t.Errorf("Expected 'JSONL only' in details, got: %v", result.Details)
	}
}

func TestRigBackendStatusCheck_NoBeadsSkipped(t *testing.T) {
	// Rig registered but no beads dir at all — should not be reported as issue
	tmpDir := t.TempDir()

	// No .beads at town root

	// Create mayor directory with one rig
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {"myrig": {"git_url": "git@example.com:test.git"}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// myrig directory exists but no beads
	if err := os.MkdirAll(filepath.Join(tmpDir, "myrig", "mayor", "rig"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	// No beads = nothing to migrate, should be OK (0 fully migrated though)
	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK for no-beads rigs, got %v: %s (details: %v)", result.Status, result.Message, result.Details)
	}
}

func TestRigBackendStatusCheck_MixedStates(t *testing.T) {
	tmpDir := t.TempDir()

	// Town root: fully migrated (dolt metadata + dolt dir)
	townBeads := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(filepath.Join(townBeads, "dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor directory with two rigs
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version": 1, "rigs": {"rigA": {}, "rigB": {}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// rigA: fully migrated
	rigABeads := filepath.Join(tmpDir, "rigA", "mayor", "rig", ".beads")
	if err := os.MkdirAll(filepath.Join(rigABeads, "dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigABeads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// rigB: never migrated (sqlite)
	rigBBeads := filepath.Join(tmpDir, "rigB", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBBeads, "metadata.json"), []byte(`{"backend":"sqlite"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBBeads, "beads.db"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewRigBackendStatusCheck()
	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("Expected StatusError for mixed states, got %v: %s", result.Status, result.Message)
	}

	// Message should mention both counts
	if !contains(result.Message, "1 never-migrated") {
		t.Errorf("Expected '1 never-migrated' in message, got: %s", result.Message)
	}
	if !contains(result.Message, "2 fully-migrated") {
		t.Errorf("Expected '2 fully-migrated' in message, got: %s", result.Message)
	}
}

func TestClassifyRigMigration_States(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(tmpDir string)
		expectedState MigrationState
		needsMigrate  bool
	}{
		{
			name: "fully migrated with dolt dir",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(filepath.Join(beads, "dolt"), 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644)
			},
			expectedState: StateFullyMigrated,
			needsMigrate:  false,
		},
		{
			name: "fully migrated with dolt-data",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(beads, 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644)
				os.MkdirAll(filepath.Join(dir, ".dolt-data", "testrig", ".dolt"), 0755)
			},
			expectedState: StateFullyMigrated,
			needsMigrate:  false,
		},
		{
			name: "never migrated sqlite",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(beads, 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"sqlite"}`), 0644)
				os.WriteFile(filepath.Join(beads, "beads.db"), []byte(""), 0644)
			},
			expectedState: StateNeverMigrated,
			needsMigrate:  true,
		},
		{
			name: "never migrated JSONL only",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(beads, 0755)
				os.WriteFile(filepath.Join(beads, "issues.jsonl"), []byte("{}"), 0644)
			},
			expectedState: StateNeverMigrated,
			needsMigrate:  true,
		},
		{
			name: "partially migrated - dolt data exists but metadata says sqlite",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(beads, 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"sqlite"}`), 0644)
				os.WriteFile(filepath.Join(beads, "beads.db"), []byte(""), 0644)
				os.MkdirAll(filepath.Join(dir, ".dolt-data", "testrig", ".dolt"), 0755)
			},
			expectedState: StatePartiallyMigrated,
			needsMigrate:  true,
		},
		{
			name: "partially migrated - metadata says dolt but no dolt db",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(beads, 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644)
			},
			expectedState: StatePartiallyMigrated,
			needsMigrate:  true,
		},
		{
			name: "partially migrated - dolt dir in beads but metadata says sqlite",
			setup: func(dir string) {
				beads := filepath.Join(dir, ".beads")
				os.MkdirAll(filepath.Join(beads, "dolt"), 0755)
				os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"backend":"sqlite"}`), 0644)
			},
			expectedState: StatePartiallyMigrated,
			needsMigrate:  true,
		},
		{
			name: "no beads dir",
			setup: func(dir string) {
				// Don't create .beads
			},
			expectedState: StateNoBeads,
			needsMigrate:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			result := classifyRigMigration("testrig", "testrig", filepath.Join(tmpDir, ".beads"), tmpDir)

			if result.State != tt.expectedState {
				t.Errorf("Expected state %s, got %s", tt.expectedState, result.State)
			}
			if result.NeedsMigration != tt.needsMigrate {
				t.Errorf("Expected NeedsMigration=%v, got %v", tt.needsMigrate, result.NeedsMigration)
			}
		})
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

// NewUnmigratedRigCheck backward compatibility test
func TestNewUnmigratedRigCheck_BackwardCompat(t *testing.T) {
	check := NewUnmigratedRigCheck()
	if check.Name() != "rig-backend-status" {
		t.Errorf("Expected name 'rig-backend-status', got %q", check.Name())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
