package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/upgrade"
)

// createTestWorkspace creates a test workspace with 0.1.x layout
func createTestWorkspace(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Create 0.1.x style layout
	// town.json at root
	townConfig := map[string]interface{}{
		"name":       "test-town",
		"gt_version": "0.1.5",
	}
	townData, _ := json.MarshalIndent(townConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "town.json"), townData, 0644); err != nil {
		t.Fatalf("creating town.json: %v", err)
	}

	// rigs.json at root
	rigsConfig := map[string]interface{}{
		"rigs": []interface{}{},
	}
	rigsData, _ := json.MarshalIndent(rigsConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "rigs.json"), rigsData, 0644); err != nil {
		t.Fatalf("creating rigs.json: %v", err)
	}

	// Create a test rig
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0755); err != nil {
		t.Fatalf("creating rig crew dir: %v", err)
	}

	return tmpDir
}

// createV02Workspace creates a test workspace with 0.2.x layout
func createV02Workspace(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Create 0.2.x style layout
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("creating mayor dir: %v", err)
	}

	// town.json in mayor/
	townConfig := map[string]interface{}{
		"name":       "test-town",
		"gt_version": "0.2.0",
	}
	townData, _ := json.MarshalIndent(townConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), townData, 0644); err != nil {
		t.Fatalf("creating town.json: %v", err)
	}

	// Create a test rig with settings/
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0755); err != nil {
		t.Fatalf("creating rig crew dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, "settings"), 0755); err != nil {
		t.Fatalf("creating rig settings dir: %v", err)
	}

	return tmpDir
}

func TestDetectLayout_V01x(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	layout, err := DetectLayout(tmpDir)
	if err != nil {
		t.Fatalf("DetectLayout failed: %v", err)
	}

	if layout.Type != LayoutV01x {
		t.Errorf("expected layout type %s, got %s", LayoutV01x, layout.Type)
	}

	if !layout.HasLegacyTownJSON {
		t.Error("expected HasLegacyTownJSON to be true")
	}

	if layout.Version != "0.1.5" {
		t.Errorf("expected version 0.1.5, got %s", layout.Version)
	}
}

func TestDetectLayout_V02x(t *testing.T) {
	tmpDir := createV02Workspace(t)

	layout, err := DetectLayout(tmpDir)
	if err != nil {
		t.Fatalf("DetectLayout failed: %v", err)
	}

	if layout.Type != LayoutV02x {
		t.Errorf("expected layout type %s, got %s", LayoutV02x, layout.Type)
	}

	if layout.HasLegacyTownJSON {
		t.Error("expected HasLegacyTownJSON to be false")
	}

	if layout.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", layout.Version)
	}
}

func TestNeedsMigration_V01x(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	result, err := NeedsMigration(tmpDir)
	if err != nil {
		t.Fatalf("NeedsMigration failed: %v", err)
	}

	if !result.NeedsMigration {
		t.Error("expected migration to be needed for 0.1.x workspace")
	}

	if len(result.MigrationPath) == 0 {
		t.Error("expected non-empty migration path")
	}
}

func TestNeedsMigration_V02x(t *testing.T) {
	tmpDir := createV02Workspace(t)

	result, err := NeedsMigration(tmpDir)
	if err != nil {
		t.Fatalf("NeedsMigration failed: %v", err)
	}

	if result.NeedsMigration {
		t.Error("expected no migration needed for 0.2.x workspace")
	}
}

func TestBackupManager_CreateAndRestore(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	// Create backup
	bm := NewBackupManager(tmpDir)
	backupDir, manifest, err := bm.CreateBackup("test-migration", "0.1.5", "0.2.0")
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if len(manifest.Files) == 0 {
		t.Error("expected at least one file in backup manifest")
	}

	// Verify backup files exist
	if _, err := os.Stat(filepath.Join(backupDir, "manifest.json")); err != nil {
		t.Errorf("backup manifest not found: %v", err)
	}

	// Modify the original file
	newTownConfig := map[string]interface{}{
		"name":       "modified-town",
		"gt_version": "0.2.0",
	}
	townData, _ := json.MarshalIndent(newTownConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "town.json"), townData, 0644); err != nil {
		t.Fatalf("modifying town.json: %v", err)
	}

	// Restore backup
	if err := bm.RestoreBackup(backupDir); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restoration
	data, _ := os.ReadFile(filepath.Join(tmpDir, "town.json"))
	var restored map[string]interface{}
	json.Unmarshal(data, &restored)

	if restored["name"] != "test-town" {
		t.Errorf("expected restored name to be 'test-town', got %v", restored["name"])
	}
}

func TestMigration_FindMigration(t *testing.T) {
	ver, _ := upgrade.ParseVersion("0.1.5")

	migration, err := FindMigration(ver)
	if err != nil {
		t.Fatalf("FindMigration failed: %v", err)
	}

	if migration.ID != "v0_1_to_v0_2" {
		t.Errorf("expected migration ID v0_1_to_v0_2, got %s", migration.ID)
	}
}

func TestMigration_StepCheck(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	ver, _ := upgrade.ParseVersion("0.1.5")
	toVer, _ := upgrade.ParseVersion("0.2.0")

	ctx := &Context{
		TownRoot:    tmpDir,
		FromVersion: ver,
		ToVersion:   toVer,
	}

	// Test CreateMayorDirectoryStep
	step := &CreateMayorDirectoryStep{}

	needed, err := step.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !needed {
		t.Error("expected CreateMayorDirectoryStep to be needed")
	}

	// Create mayor dir and check again
	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)

	needed, err = step.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if needed {
		t.Error("expected CreateMayorDirectoryStep to not be needed after creation")
	}
}

func TestMigration_MoveConfigFiles(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	ver, _ := upgrade.ParseVersion("0.1.5")
	toVer, _ := upgrade.ParseVersion("0.2.0")

	ctx := &Context{
		TownRoot:    tmpDir,
		FromVersion: ver,
		ToVersion:   toVer,
	}

	// Create mayor dir first
	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)

	// Test MoveConfigFilesStep
	step := &MoveConfigFilesStep{}

	// Execute
	if err := step.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify files were moved
	if _, err := os.Stat(filepath.Join(tmpDir, "mayor", "town.json")); err != nil {
		t.Errorf("town.json not moved to mayor/: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "mayor", "rigs.json")); err != nil {
		t.Errorf("rigs.json not moved to mayor/: %v", err)
	}

	// Original files should be gone
	if _, err := os.Stat(filepath.Join(tmpDir, "town.json")); !os.IsNotExist(err) {
		t.Error("original town.json should be removed")
	}

	// Test rollback
	if err := step.Rollback(ctx); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify files were restored
	if _, err := os.Stat(filepath.Join(tmpDir, "town.json")); err != nil {
		t.Errorf("town.json not restored: %v", err)
	}
}

func TestMigration_DryRun(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	engine := NewEngine(tmpDir)

	migration, _ := GetMigration("v0_1_to_v0_2")

	result, err := engine.Run(migration, true) // dry run
	if err != nil {
		t.Fatalf("Run dry-run failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected dry-run to succeed, got error: %s", result.Error)
	}

	// Verify no changes were made
	if _, err := os.Stat(filepath.Join(tmpDir, "mayor")); !os.IsNotExist(err) {
		t.Error("dry-run should not create mayor directory")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "town.json")); err != nil {
		t.Error("dry-run should not move town.json")
	}
}

func TestMigration_FullMigration(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	engine := NewEngine(tmpDir)
	engine.SetLogger(func(format string, args ...interface{}) {
		// Silent logging for tests
	})

	migration, _ := GetMigration("v0_1_to_v0_2")

	result, err := engine.Run(migration, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected migration to succeed, got error: %s", result.Error)
	}

	// Verify migration results
	// town.json should be in mayor/
	if _, err := os.Stat(filepath.Join(tmpDir, "mayor", "town.json")); err != nil {
		t.Error("town.json should be in mayor/ after migration")
	}

	// Original town.json should be gone
	if _, err := os.Stat(filepath.Join(tmpDir, "town.json")); !os.IsNotExist(err) {
		t.Error("original town.json should be removed after migration")
	}

	// Backup should exist
	if result.BackupPath == "" {
		t.Error("expected backup path to be set")
	}
}

func TestVerifier(t *testing.T) {
	tmpDir := createV02Workspace(t)

	v := NewVerifier(tmpDir)
	result := v.Verify()

	if !result.Success {
		t.Errorf("expected verification to succeed, got errors: %v", result.Errors)
	}
}

func TestMigrationRegistry(t *testing.T) {
	// Verify V0_1_to_V0_2 is registered
	migrations := GetMigrations()

	found := false
	for _, m := range migrations {
		if m.ID == "v0_1_to_v0_2" {
			found = true
			break
		}
	}

	if !found {
		t.Error("v0_1_to_v0_2 migration not found in registry")
	}
}

func TestGetMigrationPath(t *testing.T) {
	from, _ := upgrade.ParseVersion("0.1.5")
	to, _ := upgrade.ParseVersion("0.2.0")

	path, err := GetMigrationPath(from, to)
	if err != nil {
		t.Fatalf("GetMigrationPath failed: %v", err)
	}

	if len(path) == 0 {
		t.Error("expected non-empty migration path")
	}

	if path[0].ID != "v0_1_to_v0_2" {
		t.Errorf("expected first migration to be v0_1_to_v0_2, got %s", path[0].ID)
	}
}

func TestEngine_Preview(t *testing.T) {
	tmpDir := createTestWorkspace(t)

	engine := NewEngine(tmpDir)
	migration, _ := GetMigration("v0_1_to_v0_2")

	preview, err := engine.Preview(migration)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if preview.MigrationID != "v0_1_to_v0_2" {
		t.Errorf("expected migration ID v0_1_to_v0_2, got %s", preview.MigrationID)
	}

	if len(preview.Steps) == 0 {
		t.Error("expected non-empty steps in preview")
	}

	// At least some steps should be pending
	if preview.PendingStepCount() == 0 {
		t.Error("expected at least one pending step in preview")
	}
}
