package doltserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFindMigratableDatabases_FollowsRedirect(t *testing.T) {
	// Setup: simulate a town with a rig that uses a redirect
	townRoot := t.TempDir()

	// Create rig directory with .beads/redirect -> mayor/rig/.beads
	rigName := "nexus"
	rigDir := filepath.Join(townRoot, rigName)
	rigBeadsDir := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write redirect file
	redirectPath := filepath.Join(rigBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the actual Dolt database at the redirected location
	actualDoltDir := filepath.Join(rigDir, "mayor", "rig", ".beads", "dolt", "beads", ".dolt")
	if err := os.MkdirAll(actualDoltDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .dolt-data directory (required by DefaultConfig)
	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)

	// Should find the rig database via redirect
	found := false
	for _, m := range migrations {
		if m.RigName == rigName {
			found = true
			expectedSource := filepath.Join(rigDir, "mayor", "rig", ".beads", "dolt", "beads")
			if m.SourcePath != expectedSource {
				t.Errorf("SourcePath = %q, want %q", m.SourcePath, expectedSource)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected to find migration for rig %q via redirect, got migrations: %v", rigName, migrations)
	}
}

func TestFindMigratableDatabases_NoRedirect(t *testing.T) {
	// Setup: rig with direct .beads/dolt/beads (no redirect)
	townRoot := t.TempDir()

	rigName := "simple"
	doltDir := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)

	found := false
	for _, m := range migrations {
		if m.RigName == rigName {
			found = true
			expectedSource := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads")
			if m.SourcePath != expectedSource {
				t.Errorf("SourcePath = %q, want %q", m.SourcePath, expectedSource)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected to find migration for rig %q, got migrations: %v", rigName, migrations)
	}
}

func TestEnsureMetadata_HQ(t *testing.T) {
	townRoot := t.TempDir()

	// Create .beads directory
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write existing metadata without dolt config
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"database": "beads.db", "custom_field": "preserved"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	if metadata["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", metadata["backend"])
	}
	if metadata["dolt_mode"] != "server" {
		t.Errorf("dolt_mode = %v, want server", metadata["dolt_mode"])
	}
	if metadata["dolt_database"] != "hq" {
		t.Errorf("dolt_database = %v, want hq", metadata["dolt_database"])
	}
	if metadata["custom_field"] != "preserved" {
		t.Errorf("custom_field was not preserved: %v", metadata["custom_field"])
	}
}

func TestEnsureMetadata_Rig(t *testing.T) {
	townRoot := t.TempDir()

	// Create rig with mayor/rig/.beads
	beadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "myrig"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	if metadata["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", metadata["backend"])
	}
	if metadata["dolt_database"] != "myrig" {
		t.Errorf("dolt_database = %v, want myrig", metadata["dolt_database"])
	}
	if metadata["jsonl_export"] != "issues.jsonl" {
		t.Errorf("jsonl_export = %v, want issues.jsonl", metadata["jsonl_export"])
	}
}

func TestEnsureMetadata_Idempotent(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run twice
	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("first EnsureMetadata failed: %v", err)
	}
	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("second EnsureMetadata failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	if metadata["dolt_database"] != "hq" {
		t.Errorf("dolt_database = %v, want hq", metadata["dolt_database"])
	}
}

func TestEnsureAllMetadata(t *testing.T) {
	townRoot := t.TempDir()

	// Create two databases in .dolt-data
	for _, name := range []string{"hq", "myrig"} {
		doltDir := filepath.Join(townRoot, ".dolt-data", name, ".dolt")
		if err := os.MkdirAll(doltDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create beads dirs
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads"), 0755); err != nil {
		t.Fatal(err)
	}

	updated, errs := EnsureAllMetadata(townRoot)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(updated) != 2 {
		t.Errorf("expected 2 updated, got %d: %v", len(updated), updated)
	}
}

func TestFindRigBeadsDir(t *testing.T) {
	townRoot := t.TempDir()

	// Test HQ
	if dir := findRigBeadsDir(townRoot, "hq"); dir != filepath.Join(townRoot, ".beads") {
		t.Errorf("hq beads dir = %q, want %q", dir, filepath.Join(townRoot, ".beads"))
	}

	// Test rig with mayor/rig/.beads
	mayorBeads := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if dir := findRigBeadsDir(townRoot, "myrig"); dir != mayorBeads {
		t.Errorf("myrig beads dir = %q, want %q", dir, mayorBeads)
	}

	// Test rig with only rig-root .beads
	rigBeads := filepath.Join(townRoot, "otherrig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if dir := findRigBeadsDir(townRoot, "otherrig"); dir != rigBeads {
		t.Errorf("otherrig beads dir = %q, want %q", dir, rigBeads)
	}
}

func TestMoveDir_SameFilesystem(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src")
	dest := filepath.Join(tmpDir, "dest")

	// Create source with nested content
	if err := os.MkdirAll(filepath.Join(src, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "subdir", "nested.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := moveDir(src, dest); err != nil {
		t.Fatalf("moveDir failed: %v", err)
	}

	// Source should be gone
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source directory still exists after move")
	}

	// Dest should have the content
	data, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	if err != nil {
		t.Fatalf("reading moved file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", string(data), "hello")
	}

	data, err = os.ReadFile(filepath.Join(dest, "subdir", "nested.txt"))
	if err != nil {
		t.Fatalf("reading moved nested file: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("nested file content = %q, want %q", string(data), "world")
	}
}

func TestMigrateRigFromBeads(t *testing.T) {
	townRoot := t.TempDir()

	// Create source database
	rigName := "testrig"
	sourcePath := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads")
	if err := os.MkdirAll(filepath.Join(sourcePath, ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, ".dolt", "config.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create beads dir for metadata
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := MigrateRigFromBeads(townRoot, rigName, sourcePath); err != nil {
		t.Fatalf("MigrateRigFromBeads failed: %v", err)
	}

	// Source should be gone
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Errorf("source directory still exists after migration")
	}

	// Target should have .dolt
	targetDir := filepath.Join(townRoot, ".dolt-data", rigName)
	if _, err := os.Stat(filepath.Join(targetDir, ".dolt")); err != nil {
		t.Errorf("target .dolt directory missing: %v", err)
	}

	// config.json should have been migrated
	data, err := os.ReadFile(filepath.Join(targetDir, ".dolt", "config.json"))
	if err != nil {
		t.Fatalf("reading migrated config: %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("config content = %q, want %q", string(data), `{}`)
	}
}

func TestMigrateRigFromBeads_AlreadyExists(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "existing"
	sourcePath := filepath.Join(townRoot, "src", ".beads", "dolt", "beads")
	if err := os.MkdirAll(filepath.Join(sourcePath, ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}

	// Target already exists
	targetDir := filepath.Join(townRoot, ".dolt-data", rigName, ".dolt")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := MigrateRigFromBeads(townRoot, rigName, sourcePath)
	if err == nil {
		t.Fatal("expected error for already-existing target, got nil")
	}
}

func TestFindMigratableDatabases_SkipsAlreadyMigrated(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "already"
	// Source exists
	sourceDir := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads", ".dolt")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Target also exists (already migrated)
	targetDir := filepath.Join(townRoot, ".dolt-data", rigName, ".dolt")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)

	for _, m := range migrations {
		if m.RigName == rigName {
			t.Errorf("should not include already-migrated rig %q", rigName)
		}
	}
}
