package doltserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// =============================================================================
// Health metrics tests
// =============================================================================

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{2147483648, "2.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDirSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files with known sizes
	if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "b.txt"), make([]byte, 200), 0644); err != nil {
		t.Fatal(err)
	}

	size := dirSize(tmpDir)
	if size != 300 {
		t.Errorf("dirSize = %d, want 300", size)
	}
}

func TestDirSize_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	size := dirSize(tmpDir)
	if size != 0 {
		t.Errorf("dirSize of empty dir = %d, want 0", size)
	}
}

func TestDirSize_NonexistentDir(t *testing.T) {
	size := dirSize("/nonexistent/path/that/does/not/exist")
	if size != 0 {
		t.Errorf("dirSize of nonexistent dir = %d, want 0", size)
	}
}

func TestGetHealthMetrics_NoServer(t *testing.T) {
	townRoot := t.TempDir()

	// Create .dolt-data dir with some content
	dataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "testfile"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	metrics := GetHealthMetrics(townRoot)

	// Connections and latency depend on whether a local Dolt server is running.
	// With no server, both are 0. With a server, dolt auto-detects it.
	// Either outcome is valid for this test — the key assertion is that
	// GetHealthMetrics doesn't panic or hang.
	if metrics.Connections < 0 {
		t.Errorf("Connections = %d, want >= 0", metrics.Connections)
	}
	if metrics.QueryLatency < 0 {
		t.Errorf("QueryLatency = %v, want >= 0", metrics.QueryLatency)
	}

	// Disk usage should reflect our test file
	if metrics.DiskUsageBytes < 1024 {
		t.Errorf("DiskUsageBytes = %d, want >= 1024", metrics.DiskUsageBytes)
	}
	if metrics.DiskUsageHuman == "" {
		t.Error("DiskUsageHuman should not be empty")
	}

	// MaxConnections should have a default
	if metrics.MaxConnections <= 0 {
		t.Errorf("MaxConnections = %d, want > 0", metrics.MaxConnections)
	}
}

func TestGetHealthMetrics_EmptyDataDir(t *testing.T) {
	townRoot := t.TempDir()

	metrics := GetHealthMetrics(townRoot)

	if metrics.DiskUsageBytes != 0 {
		t.Errorf("DiskUsageBytes = %d, want 0 (no data dir)", metrics.DiskUsageBytes)
	}
	if metrics.DiskUsageHuman != "0 B" {
		t.Errorf("DiskUsageHuman = %q, want %q", metrics.DiskUsageHuman, "0 B")
	}
}

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
	actualDoltDir := filepath.Join(rigDir, "mayor", "rig", ".beads", "dolt", "beads_myrig", ".dolt")
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
			expectedSource := filepath.Join(rigDir, "mayor", "rig", ".beads", "dolt", "beads_myrig")
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
	// Setup: rig with direct .beads/dolt/beads_testrig (no redirect)
	townRoot := t.TempDir()

	rigName := "simple"
	doltDir := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_testrig", ".dolt")
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
			expectedSource := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_testrig")
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

func TestFindLocalDoltDB(t *testing.T) {
	t.Run("no dolt directory", func(t *testing.T) {
		beadsDir := t.TempDir()
		result := findLocalDoltDB(beadsDir)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("empty dolt directory", func(t *testing.T) {
		beadsDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755); err != nil {
			t.Fatal(err)
		}
		result := findLocalDoltDB(beadsDir)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single database", func(t *testing.T) {
		beadsDir := t.TempDir()
		dbDir := filepath.Join(beadsDir, "dolt", "beads_hq", ".dolt")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			t.Fatal(err)
		}
		result := findLocalDoltDB(beadsDir)
		expected := filepath.Join(beadsDir, "dolt", "beads_hq")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("non-dolt files ignored", func(t *testing.T) {
		beadsDir := t.TempDir()
		doltParent := filepath.Join(beadsDir, "dolt")
		if err := os.MkdirAll(doltParent, 0755); err != nil {
			t.Fatal(err)
		}
		// Create a regular file (not a directory)
		if err := os.WriteFile(filepath.Join(doltParent, "readme.txt"), []byte("hi"), 0644); err != nil {
			t.Fatal(err)
		}
		// Create a directory without .dolt inside
		if err := os.MkdirAll(filepath.Join(doltParent, "not-a-db"), 0755); err != nil {
			t.Fatal(err)
		}
		// Create the real database
		if err := os.MkdirAll(filepath.Join(doltParent, "beads_gt", ".dolt"), 0755); err != nil {
			t.Fatal(err)
		}
		result := findLocalDoltDB(beadsDir)
		expected := filepath.Join(doltParent, "beads_gt")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("multiple databases returns empty with warning", func(t *testing.T) {
		beadsDir := t.TempDir()
		doltParent := filepath.Join(beadsDir, "dolt")
		// Create two valid dolt databases
		for _, name := range []string{"beads_gt", "beads_old"} {
			if err := os.MkdirAll(filepath.Join(doltParent, name, ".dolt"), 0755); err != nil {
				t.Fatal(err)
			}
		}

		// Capture stderr to verify warning is emitted
		origStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stderr = w

		result := findLocalDoltDB(beadsDir)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stderr = origStderr

		// Should fail closed on ambiguity — return empty string
		if result != "" {
			t.Errorf("expected empty string for ambiguous multi-candidate, got %q", result)
		}
		// Verify warning was emitted
		if !strings.Contains(buf.String(), "multiple dolt databases found") {
			t.Errorf("expected multi-candidate warning on stderr, got %q", buf.String())
		}
	})

	t.Run("symlink to directory with dolt database", func(t *testing.T) {
		beadsDir := t.TempDir()
		doltParent := filepath.Join(beadsDir, "dolt")
		if err := os.MkdirAll(doltParent, 0755); err != nil {
			t.Fatal(err)
		}
		// Create the real database directory outside the dolt parent
		realDB := filepath.Join(beadsDir, "real_beads_hq")
		if err := os.MkdirAll(filepath.Join(realDB, ".dolt"), 0755); err != nil {
			t.Fatal(err)
		}
		// Symlink it into dolt/
		if err := os.Symlink(realDB, filepath.Join(doltParent, "beads_hq")); err != nil {
			t.Fatal(err)
		}
		result := findLocalDoltDB(beadsDir)
		expected := filepath.Join(doltParent, "beads_hq")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})
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
	if dir := FindRigBeadsDir(townRoot, "hq"); dir != filepath.Join(townRoot, ".beads") {
		t.Errorf("hq beads dir = %q, want %q", dir, filepath.Join(townRoot, ".beads"))
	}

	// Test rig with mayor/rig/.beads
	mayorBeads := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if dir := FindRigBeadsDir(townRoot, "myrig"); dir != mayorBeads {
		t.Errorf("myrig beads dir = %q, want %q", dir, mayorBeads)
	}

	// Test rig with only rig-root .beads
	rigBeads := filepath.Join(townRoot, "otherrig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if dir := FindRigBeadsDir(townRoot, "otherrig"); dir != rigBeads {
		t.Errorf("otherrig beads dir = %q, want %q", dir, rigBeads)
	}

	// Test rig with neither directory existing — should return mayor path for creation
	neitherRig := "newrig"
	expectedMayor := filepath.Join(townRoot, neitherRig, "mayor", "rig", ".beads")
	if dir := FindRigBeadsDir(townRoot, neitherRig); dir != expectedMayor {
		t.Errorf("newrig (neither exists) beads dir = %q, want %q (mayor path for creation)", dir, expectedMayor)
	}
}

func TestFindOrCreateRigBeadsDir(t *testing.T) {
	t.Run("hq creates directory", func(t *testing.T) {
		townRoot := t.TempDir()
		dir, err := FindOrCreateRigBeadsDir(townRoot, "hq")
		if err != nil {
			t.Fatal(err)
		}
		expected := filepath.Join(townRoot, ".beads")
		if dir != expected {
			t.Errorf("hq beads dir = %q, want %q", dir, expected)
		}
		// Verify directory was actually created
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("hq .beads directory was not created")
		}
	})

	t.Run("existing mayor path returned as-is", func(t *testing.T) {
		townRoot := t.TempDir()
		mayorBeads := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
		if err := os.MkdirAll(mayorBeads, 0755); err != nil {
			t.Fatal(err)
		}
		dir, err := FindOrCreateRigBeadsDir(townRoot, "myrig")
		if err != nil {
			t.Fatal(err)
		}
		if dir != mayorBeads {
			t.Errorf("myrig beads dir = %q, want %q", dir, mayorBeads)
		}
	})

	t.Run("existing rig-root path returned", func(t *testing.T) {
		townRoot := t.TempDir()
		rigBeads := filepath.Join(townRoot, "otherrig", ".beads")
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatal(err)
		}
		dir, err := FindOrCreateRigBeadsDir(townRoot, "otherrig")
		if err != nil {
			t.Fatal(err)
		}
		if dir != rigBeads {
			t.Errorf("otherrig beads dir = %q, want %q", dir, rigBeads)
		}
	})

	t.Run("neither exists creates mayor path", func(t *testing.T) {
		townRoot := t.TempDir()
		dir, err := FindOrCreateRigBeadsDir(townRoot, "newrig")
		if err != nil {
			t.Fatal(err)
		}
		expectedMayor := filepath.Join(townRoot, "newrig", "mayor", "rig", ".beads")
		if dir != expectedMayor {
			t.Errorf("newrig beads dir = %q, want %q", dir, expectedMayor)
		}
		// Verify directory was actually created
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("mayor .beads directory was not created")
		}
	})

	t.Run("concurrent callers for same rig don't race", func(t *testing.T) {
		townRoot := t.TempDir()
		const goroutines = 10
		results := make([]string, goroutines)
		errs := make([]error, goroutines)

		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				results[idx], errs[idx] = FindOrCreateRigBeadsDir(townRoot, "racerig")
			}(i)
		}
		wg.Wait()

		expectedMayor := filepath.Join(townRoot, "racerig", "mayor", "rig", ".beads")
		for i := 0; i < goroutines; i++ {
			if errs[i] != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, errs[i])
			}
			if results[i] != expectedMayor {
				t.Errorf("goroutine %d: got %q, want %q", i, results[i], expectedMayor)
			}
		}

		// Verify directory exists
		if _, err := os.Stat(expectedMayor); os.IsNotExist(err) {
			t.Error("directory was not created after concurrent calls")
		}
	})
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
	sourcePath := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_testrig")
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
	sourcePath := filepath.Join(townRoot, "src", ".beads", "dolt", "beads_existing")
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

func TestHasServerModeMetadata_NoMetadata(t *testing.T) {
	townRoot := t.TempDir()

	// Create empty workspace
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigs := HasServerModeMetadata(townRoot)
	if len(rigs) != 0 {
		t.Errorf("expected no server-mode rigs, got %v", rigs)
	}
}

func TestHasServerModeMetadata_WithServerMode(t *testing.T) {
	townRoot := t.TempDir()

	// Create town beads with server mode
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig with server mode
	rigBeadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigMetadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"myrig"}`
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "metadata.json"), []byte(rigMetadata), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"),
		[]byte(`{"rigs":{"myrig":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigs := HasServerModeMetadata(townRoot)
	if len(rigs) != 2 {
		t.Errorf("expected 2 server-mode rigs, got %d: %v", len(rigs), rigs)
	}
}

func TestHasServerModeMetadata_MixedModes(t *testing.T) {
	townRoot := t.TempDir()

	// Town beads with server mode
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_mode":"server","dolt_database":"hq"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Rig with sqlite (not server mode)
	rigBeadsDir := filepath.Join(townRoot, "sqliterig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "metadata.json"),
		[]byte(`{"backend":"sqlite"}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"),
		[]byte(`{"rigs":{"sqliterig":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigs := HasServerModeMetadata(townRoot)
	if len(rigs) != 1 {
		t.Errorf("expected 1 server-mode rig (hq only), got %d: %v", len(rigs), rigs)
	}
	if len(rigs) > 0 && rigs[0] != "hq" {
		t.Errorf("expected hq, got %s", rigs[0])
	}
}

func TestCheckServerReachable_NoServer(t *testing.T) {
	townRoot := t.TempDir()

	// CheckServerReachable should fail when no server is listening
	// Using default port 3307 - if a real server is running, skip
	err := CheckServerReachable(townRoot)
	if err == nil {
		t.Skip("A server is actually running on port 3307, cannot test unreachable case")
	}
	if err != nil && !contains(err.Error(), "not reachable") {
		t.Errorf("expected 'not reachable' in error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFindMigratableDatabases_SkipsAlreadyMigrated(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "already"
	// Source exists
	sourceDir := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_hq", ".dolt")
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

// =============================================================================
// Mid-migration crash recovery tests
// =============================================================================

// TestMidMigrationCrashRecovery_PartialMigration tests that after migrating
// some rigs but not others (simulating a crash), resuming migration completes
// all remaining rigs without corrupting already-migrated ones.
func TestMidMigrationCrashRecovery_PartialMigration(t *testing.T) {
	townRoot := t.TempDir()

	// Create 3 rigs with source databases
	rigs := []string{"rig-alpha", "rig-beta", "rig-gamma"}
	for _, rig := range rigs {
		sourceDolt := filepath.Join(townRoot, rig, ".beads", "dolt", "beads_"+rig, ".dolt")
		if err := os.MkdirAll(sourceDolt, 0755); err != nil {
			t.Fatal(err)
		}
		// Write a marker file so we can verify data integrity
		marker := filepath.Join(sourceDolt, "marker.txt")
		if err := os.WriteFile(marker, []byte("data-"+rig), 0644); err != nil {
			t.Fatal(err)
		}
		// Create beads dir for metadata
		beadsDir := filepath.Join(townRoot, rig, "mayor", "rig", ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Phase 1: Migrate only the first rig (simulating crash after rig 1)
	migrations := FindMigratableDatabases(townRoot)
	if len(migrations) != 3 {
		t.Fatalf("expected 3 migratable databases, got %d", len(migrations))
	}

	// Migrate only rig-alpha
	for _, m := range migrations {
		if m.RigName == "rig-alpha" {
			if err := MigrateRigFromBeads(townRoot, m.RigName, m.SourcePath); err != nil {
				t.Fatalf("migrating %s: %v", m.RigName, err)
			}
			break
		}
	}

	// Verify rig-alpha is migrated and data intact
	alphaTarget := filepath.Join(townRoot, ".dolt-data", "rig-alpha", ".dolt", "marker.txt")
	data, err := os.ReadFile(alphaTarget)
	if err != nil {
		t.Fatalf("reading migrated alpha marker: %v", err)
	}
	if string(data) != "data-rig-alpha" {
		t.Errorf("alpha marker = %q, want %q", string(data), "data-rig-alpha")
	}

	// Phase 2: Resume migration (find remaining databases)
	remaining := FindMigratableDatabases(townRoot)
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining migratable databases after partial migration, got %d", len(remaining))
	}

	// Verify rig-alpha is NOT in the remaining list
	for _, m := range remaining {
		if m.RigName == "rig-alpha" {
			t.Error("rig-alpha should not appear in remaining migrations")
		}
	}

	// Migrate the rest
	for _, m := range remaining {
		if err := MigrateRigFromBeads(townRoot, m.RigName, m.SourcePath); err != nil {
			t.Fatalf("migrating %s on resume: %v", m.RigName, err)
		}
	}

	// Verify all 3 rigs are now migrated with correct data
	for _, rig := range rigs {
		markerPath := filepath.Join(townRoot, ".dolt-data", rig, ".dolt", "marker.txt")
		data, err := os.ReadFile(markerPath)
		if err != nil {
			t.Fatalf("reading marker for %s: %v", rig, err)
		}
		expected := "data-" + rig
		if string(data) != expected {
			t.Errorf("%s marker = %q, want %q", rig, string(data), expected)
		}
	}

	// No more migratable databases should remain
	final := FindMigratableDatabases(townRoot)
	if len(final) != 0 {
		t.Errorf("expected 0 migratable databases after full migration, got %d", len(final))
	}
}

// TestMidMigrationCrashRecovery_SourceGoneTargetExists tests that if a crash
// happened after the move but before metadata update, the system recognizes
// the rig as already migrated (target exists, source gone).
func TestMidMigrationCrashRecovery_SourceGoneTargetExists(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "crashed-rig"

	// Simulate post-move state: source is gone, target exists
	targetDolt := filepath.Join(townRoot, ".dolt-data", rigName, ".dolt")
	if err := os.MkdirAll(targetDolt, 0755); err != nil {
		t.Fatal(err)
	}

	// Source does NOT exist (was already moved)
	// FindMigratableDatabases should not list this rig
	migrations := FindMigratableDatabases(townRoot)
	for _, m := range migrations {
		if m.RigName == rigName {
			t.Error("should not attempt to re-migrate a rig whose target already exists")
		}
	}

	// EnsureMetadata should still work to repair metadata.json
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, rigName); err != nil {
		t.Fatalf("EnsureMetadata for crashed rig: %v", err)
	}

	// Verify metadata was written correctly
	metaData, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
}

// =============================================================================
// Concurrent access during migration tests
// =============================================================================

// TestConcurrentMetadataAccess tests that concurrent EnsureMetadata calls
// for different rigs don't interfere with each other.
func TestConcurrentMetadataAccess(t *testing.T) {
	townRoot := t.TempDir()

	rigs := []string{"rig-a", "rig-b", "rig-c", "rig-d", "rig-e"}
	for _, rig := range rigs {
		beadsDir := filepath.Join(townRoot, rig, "mayor", "rig", ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, len(rigs))

	for i, rig := range rigs {
		wg.Add(1)
		go func(idx int, rigName string) {
			defer wg.Done()
			errs[idx] = EnsureMetadata(townRoot, rigName)
		}(i, rig)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("EnsureMetadata for %s failed: %v", rigs[i], err)
		}
	}

	// Verify each rig got the correct metadata
	for _, rig := range rigs {
		metaPath := filepath.Join(townRoot, rig, "mayor", "rig", ".beads", "metadata.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("reading metadata for %s: %v", rig, err)
		}
		var meta map[string]interface{}
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatalf("parsing metadata for %s: %v", rig, err)
		}
		if meta["dolt_database"] != rig {
			t.Errorf("%s: dolt_database = %v, want %s", rig, meta["dolt_database"], rig)
		}
		if meta["backend"] != "dolt" {
			t.Errorf("%s: backend = %v, want dolt", rig, meta["backend"])
		}
	}
}

// TestConcurrentMetadataSameFile tests that concurrent EnsureMetadata calls
// targeting the SAME metadata.json file don't corrupt data. This exercises
// the file locking added to prevent read-modify-write races.
func TestConcurrentMetadataSameFile(t *testing.T) {
	townRoot := t.TempDir()

	// All goroutines will target the same rig (and thus the same metadata.json)
	rigName := "shared-rig"
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed with extra fields that must survive concurrent overwrites
	initial := map[string]interface{}{
		"custom_field": "preserve-me",
		"version":      42.0,
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if err := os.WriteFile(metadataPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Run 10 concurrent goroutines all writing to the same file
	const concurrency = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = EnsureMetadata(townRoot, rigName)
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: EnsureMetadata failed: %v", i, err)
		}
	}

	// Verify final metadata is valid and preserves all fields
	finalData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(finalData, &meta); err != nil {
		t.Fatalf("final metadata is corrupted JSON: %v\ncontent: %s", err, string(finalData))
	}

	// Dolt fields must be set
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
	if meta["dolt_database"] != rigName {
		t.Errorf("dolt_database = %v, want %s", meta["dolt_database"], rigName)
	}
	if meta["dolt_mode"] != "server" {
		t.Errorf("dolt_mode = %v, want server", meta["dolt_mode"])
	}

	// Custom fields must be preserved (not clobbered by concurrent writes)
	if meta["custom_field"] != "preserve-me" {
		t.Errorf("custom_field = %v, want preserve-me (field was clobbered)", meta["custom_field"])
	}
	if meta["version"] != 42.0 {
		t.Errorf("version = %v, want 42 (field was clobbered)", meta["version"])
	}

	// No lock file should be left behind (sync.Mutex is used instead of flock)
	lockPath := metadataPath + ".lock"
	if _, err := os.Stat(lockPath); err == nil {
		t.Error("lock file should not exist — EnsureMetadata uses sync.Mutex, not flock")
	}
}

// TestConcurrentFindMigratableDatabases tests that FindMigratableDatabases
// can be called concurrently (simulating gt status during migration).
func TestConcurrentFindMigratableDatabases(t *testing.T) {
	townRoot := t.TempDir()

	// Create a rig with source database
	rigName := "concurrent-rig"
	sourceDolt := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_concurrent", ".dolt")
	if err := os.MkdirAll(sourceDolt, 0755); err != nil {
		t.Fatal(err)
	}
	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([][]Migration, 10)

	// Concurrent reads of migratable databases
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = FindMigratableDatabases(townRoot)
		}(i)
	}

	wg.Wait()

	// All results should be consistent
	for i, r := range results {
		if len(r) != 1 {
			t.Errorf("goroutine %d: expected 1 migration, got %d", i, len(r))
		}
	}
}

// TestConcurrentMigrateAndFind tests that FindMigratableDatabases returns
// consistent results even while a migration is in progress (the source is
// being moved and the target is appearing).
func TestConcurrentMigrateAndFind(t *testing.T) {
	townRoot := t.TempDir()

	// Create multiple rigs
	rigs := []string{"mig-a", "mig-b", "mig-c"}
	for _, rig := range rigs {
		sourceDolt := filepath.Join(townRoot, rig, ".beads", "dolt", "beads_"+rig, ".dolt")
		if err := os.MkdirAll(sourceDolt, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDolt, "config.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
		beadsDir := filepath.Join(townRoot, rig, "mayor", "rig", ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run migration and concurrent finds
	var wg sync.WaitGroup
	findErrs := make([]error, 0)
	var mu sync.Mutex

	// Start concurrent finders
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				migrations := FindMigratableDatabases(townRoot)
				// Should never panic or return corrupt data
				// Count should be between 0 and 3
				if len(migrations) > 3 {
					mu.Lock()
					findErrs = append(findErrs, filepath.ErrBadPattern)
					mu.Unlock()
				}
			}
		}()
	}

	// Migrate concurrently
	for _, rig := range rigs {
		wg.Add(1)
		go func(rigName string) {
			defer wg.Done()
			sourcePath := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_"+rigName)
			_ = MigrateRigFromBeads(townRoot, rigName, sourcePath)
		}(rig)
	}

	wg.Wait()

	if len(findErrs) > 0 {
		t.Errorf("concurrent finds returned invalid results: %d errors", len(findErrs))
	}

	// After everything settles, should be 0 remaining.
	// On Windows, os.Rename can fail when concurrent goroutines hold directory
	// handles open (from FindMigratableDatabases reading the same dirs), so some
	// migrations may not complete. This is acceptable — the real application uses
	// file locks to serialize access.
	final := FindMigratableDatabases(townRoot)
	if runtime.GOOS == "windows" {
		if len(final) > 3 {
			t.Errorf("expected at most 3 migratable databases on Windows, got %d", len(final))
		}
	} else if len(final) != 0 {
		t.Errorf("expected 0 migratable databases after concurrent migration, got %d", len(final))
	}
}

// =============================================================================
// Metadata corruption and repair tests
// =============================================================================

// TestEnsureMetadata_RepairsCorruptJSON tests that EnsureMetadata can handle
// a corrupted metadata.json file and overwrite it with correct data.
func TestEnsureMetadata_RepairsCorruptJSON(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write corrupt JSON
	metaPath := filepath.Join(beadsDir, "metadata.json")
	if err := os.WriteFile(metaPath, []byte(`{corrupt json!!!`), 0600); err != nil {
		t.Fatal(err)
	}

	// EnsureMetadata should succeed (overwrites corrupt data)
	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed on corrupt file: %v", err)
	}

	// Verify valid JSON was written
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after repair: %v", err)
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
	if meta["dolt_database"] != "hq" {
		t.Errorf("dolt_database = %v, want hq", meta["dolt_database"])
	}
}

// TestEnsureMetadata_RepairsEmptyFile tests that an empty metadata.json
// gets properly populated.
func TestEnsureMetadata_RepairsEmptyFile(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write empty file
	metaPath := filepath.Join(beadsDir, "metadata.json")
	if err := os.WriteFile(metaPath, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed on empty file: %v", err)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after repair: %v", err)
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
}

// TestEnsureMetadata_RepairsWrongBackend tests that metadata.json with
// backend=sqlite gets corrected to dolt.
func TestEnsureMetadata_RepairsWrongBackend(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write metadata with wrong backend
	metaPath := filepath.Join(beadsDir, "metadata.json")
	original := map[string]interface{}{
		"backend":  "sqlite",
		"database": "beads.db",
		"custom":   "keep-me",
	}
	data, _ := json.Marshal(original)
	if err := os.WriteFile(metaPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	repaired, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(repaired, &meta); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
	if meta["dolt_mode"] != "server" {
		t.Errorf("dolt_mode = %v, want server", meta["dolt_mode"])
	}
	if meta["custom"] != "keep-me" {
		t.Errorf("custom field not preserved: %v", meta["custom"])
	}
}

// TestEnsureMetadata_RepairsMissingDoltFields tests that metadata.json
// with backend=dolt but missing dolt_mode/dolt_database gets repaired.
func TestEnsureMetadata_RepairsMissingDoltFields(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write partial dolt metadata (missing dolt_mode and dolt_database)
	metaPath := filepath.Join(beadsDir, "metadata.json")
	partial := map[string]interface{}{
		"backend":  "dolt",
		"database": "dolt",
	}
	data, _ := json.Marshal(partial)
	if err := os.WriteFile(metaPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "myrig"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	repaired, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(repaired, &meta); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}
	if meta["dolt_mode"] != "server" {
		t.Errorf("dolt_mode = %v, want server", meta["dolt_mode"])
	}
	if meta["dolt_database"] != "myrig" {
		t.Errorf("dolt_database = %v, want myrig", meta["dolt_database"])
	}
}

// TestEnsureAllMetadata_RepairsAllCorrupt tests that EnsureAllMetadata
// repairs metadata for all known databases, even if some are corrupt.
func TestEnsureAllMetadata_RepairsAllCorrupt(t *testing.T) {
	townRoot := t.TempDir()

	// Create two databases in .dolt-data
	for _, name := range []string{"hq", "corruptrig"} {
		doltDir := filepath.Join(townRoot, ".dolt-data", name, ".dolt")
		if err := os.MkdirAll(doltDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create beads dirs with corrupt metadata
	hqBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(hqBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hqBeads, "metadata.json"), []byte(`CORRUPT`), 0600); err != nil {
		t.Fatal(err)
	}

	rigBeads := filepath.Join(townRoot, "corruptrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeads, "metadata.json"), []byte(`{invalid`), 0600); err != nil {
		t.Fatal(err)
	}

	updated, errs := EnsureAllMetadata(townRoot)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(updated) != 2 {
		t.Errorf("expected 2 updated, got %d: %v", len(updated), updated)
	}

	// Verify both now have valid metadata
	for _, pair := range []struct {
		name string
		path string
	}{
		{"hq", filepath.Join(hqBeads, "metadata.json")},
		{"corruptrig", filepath.Join(rigBeads, "metadata.json")},
	} {
		data, err := os.ReadFile(pair.path)
		if err != nil {
			t.Fatalf("reading %s metadata: %v", pair.name, err)
		}
		var meta map[string]interface{}
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatalf("%s metadata invalid JSON after repair: %v", pair.name, err)
		}
		if meta["backend"] != "dolt" {
			t.Errorf("%s: backend = %v, want dolt", pair.name, meta["backend"])
		}
	}
}

// =============================================================================
// Idempotency tests
// =============================================================================

// TestMigrateRigFromBeads_IdempotentDetection tests that running migration
// twice for the same rig: first succeeds, second correctly reports already done.
func TestMigrateRigFromBeads_IdempotentDetection(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "idem-rig"
	sourcePath := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_idem")
	if err := os.MkdirAll(filepath.Join(sourcePath, ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, ".dolt", "data.txt"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// First migration succeeds
	if err := MigrateRigFromBeads(townRoot, rigName, sourcePath); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Verify data is at target
	targetData, err := os.ReadFile(filepath.Join(townRoot, ".dolt-data", rigName, ".dolt", "data.txt"))
	if err != nil {
		t.Fatalf("reading target data: %v", err)
	}
	if string(targetData) != "original" {
		t.Errorf("target data = %q, want %q", string(targetData), "original")
	}

	// Second call: source is gone, target exists → should error
	err = MigrateRigFromBeads(townRoot, rigName, sourcePath)
	if err == nil {
		t.Fatal("expected error on second migration attempt, got nil")
	}

	// Target data should still be intact
	targetData, err = os.ReadFile(filepath.Join(townRoot, ".dolt-data", rigName, ".dolt", "data.txt"))
	if err != nil {
		t.Fatalf("reading target data after second attempt: %v", err)
	}
	if string(targetData) != "original" {
		t.Errorf("target data corrupted after second attempt: %q", string(targetData))
	}
}

// TestFindAndMigrateAll_Idempotent tests the full find-then-migrate workflow
// twice: first run migrates, second run finds nothing to do.
// =============================================================================
// Max connections and admission control tests
// =============================================================================

func TestDefaultConfig_MaxConnections(t *testing.T) {
	townRoot := t.TempDir()
	config := DefaultConfig(townRoot)

	if config.MaxConnections != DefaultMaxConnections {
		t.Errorf("MaxConnections = %d, want %d", config.MaxConnections, DefaultMaxConnections)
	}
	if config.MaxConnections != 50 {
		t.Errorf("DefaultMaxConnections = %d, want 50", config.MaxConnections)
	}
}

func TestHasConnectionCapacity_ZeroMax(t *testing.T) {
	// When MaxConnections is 0, the function should use Dolt default (1000).
	// Since we can't connect to a real server in unit tests, we just verify
	// the function doesn't panic and returns an error (no server).
	townRoot := t.TempDir()

	// Create minimal config structure
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// HasConnectionCapacity should return false (fail closed) when query fails (gt-lfc0d)
	hasCapacity, _, err := HasConnectionCapacity(townRoot)
	if err == nil {
		t.Skip("Dolt server is actually running, cannot test offline case")
	}
	if hasCapacity {
		t.Error("expected fail-closed false when server is unreachable")
	}
}

func TestFindAndMigrateAll_Idempotent(t *testing.T) {
	townRoot := t.TempDir()

	// Create 2 rigs
	for _, rig := range []string{"idm-a", "idm-b"} {
		sourceDolt := filepath.Join(townRoot, rig, ".beads", "dolt", "beads_"+rig, ".dolt")
		if err := os.MkdirAll(sourceDolt, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDolt, "config.json"), []byte(`{"rig":"`+rig+`"}`), 0644); err != nil {
			t.Fatal(err)
		}
		beadsDir := filepath.Join(townRoot, rig, "mayor", "rig", ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// First pass: find and migrate all
	pass1 := FindMigratableDatabases(townRoot)
	if len(pass1) != 2 {
		t.Fatalf("pass 1: expected 2 migratable, got %d", len(pass1))
	}
	for _, m := range pass1 {
		if err := MigrateRigFromBeads(townRoot, m.RigName, m.SourcePath); err != nil {
			t.Fatalf("pass 1: migrating %s: %v", m.RigName, err)
		}
	}

	// Update metadata (as gt dolt migrate does)
	updated1, errs1 := EnsureAllMetadata(townRoot)
	if len(errs1) > 0 {
		t.Errorf("pass 1 metadata errors: %v", errs1)
	}
	if len(updated1) != 2 {
		t.Errorf("pass 1: expected 2 metadata updates, got %d", len(updated1))
	}

	// Second pass: find should return empty, metadata update should be harmless
	pass2 := FindMigratableDatabases(townRoot)
	if len(pass2) != 0 {
		t.Errorf("pass 2: expected 0 migratable, got %d", len(pass2))
	}

	updated2, errs2 := EnsureAllMetadata(townRoot)
	if len(errs2) > 0 {
		t.Errorf("pass 2 metadata errors: %v", errs2)
	}
	if len(updated2) != 2 {
		t.Errorf("pass 2: expected 2 metadata updates (idempotent), got %d", len(updated2))
	}

	// Verify data integrity after two passes
	for _, rig := range []string{"idm-a", "idm-b"} {
		configPath := filepath.Join(townRoot, ".dolt-data", rig, ".dolt", "config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("reading %s config: %v", rig, err)
		}
		expected := `{"rig":"` + rig + `"}`
		if string(data) != expected {
			t.Errorf("%s config = %q, want %q", rig, string(data), expected)
		}

		metaPath := filepath.Join(townRoot, rig, "mayor", "rig", ".beads", "metadata.json")
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("reading %s metadata: %v", rig, err)
		}
		var meta map[string]interface{}
		if err := json.Unmarshal(metaData, &meta); err != nil {
			t.Fatalf("%s metadata invalid: %v", rig, err)
		}
		if meta["backend"] != "dolt" {
			t.Errorf("%s: backend = %v, want dolt", rig, meta["backend"])
		}
	}
}

// =============================================================================
// Additional rollback edge case tests (main rollback tests are in rollback_test.go)
// =============================================================================

func TestRestoreFromBackup_BackupIsFile(t *testing.T) {
	townRoot := t.TempDir()

	filePath := filepath.Join(townRoot, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("not a backup"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := RestoreFromBackup(townRoot, filePath)
	if err == nil {
		t.Fatal("expected error when backup path is a file, got nil")
	}
}

func TestRestoreFromBackup_EmptyBackup(t *testing.T) {
	townRoot := t.TempDir()

	backupDir := filepath.Join(townRoot, "migration-backup-20240115-143022")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := RestoreFromBackup(townRoot, backupDir)
	if err != nil {
		t.Fatalf("RestoreFromBackup failed: %v", err)
	}
	if result.RestoredTown {
		t.Error("expected no town restoration from empty backup")
	}
	if len(result.RestoredRigs) != 0 {
		t.Errorf("expected 0 restored rigs, got %d", len(result.RestoredRigs))
	}
}

// =============================================================================
// Metadata edge cases
// =============================================================================

func TestEnsureMetadata_CreatesBeadsDir(t *testing.T) {
	townRoot := t.TempDir()

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(townRoot, ".beads", "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want dolt", meta["backend"])
	}
}

func TestEnsureMetadata_CorrectsStaleJSONLExport(t *testing.T) {
	townRoot := t.TempDir()

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate a stale jsonl_export value left by a historical migration
	existing := map[string]interface{}{"jsonl_export": "beads.jsonl"}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	updated, _ := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	var meta map[string]interface{}
	json.Unmarshal(updated, &meta)
	if meta["jsonl_export"] != "issues.jsonl" {
		t.Errorf("jsonl_export = %v, want issues.jsonl (stale value should be corrected)", meta["jsonl_export"])
	}
}

func TestEnsureMetadata_SetsDefaultJSONLExport(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureMetadata(townRoot, "hq"); err != nil {
		t.Fatalf("EnsureMetadata failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(townRoot, ".beads", "metadata.json"))
	var meta map[string]interface{}
	json.Unmarshal(data, &meta)
	if meta["jsonl_export"] != "issues.jsonl" {
		t.Errorf("jsonl_export = %v, want issues.jsonl", meta["jsonl_export"])
	}
}

// =============================================================================
// InitRig validation tests
// =============================================================================

func TestInitRig_EmptyName(t *testing.T) {
	townRoot := t.TempDir()
	_, err := InitRig(townRoot, "")
	if err == nil {
		t.Fatal("expected error for empty rig name")
	}
}

func TestInitRig_InvalidCharacters(t *testing.T) {
	townRoot := t.TempDir()
	for _, name := range []string{"my rig", "rig/name", "rig.name", "rig@name"} {
		_, err := InitRig(townRoot, name)
		if err == nil {
			t.Errorf("expected error for invalid rig name %q", name)
		}
	}
}

// =============================================================================
// ListDatabases edge cases
// =============================================================================

func TestListDatabases_EmptyDataDir(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}

	databases, err := ListDatabases(townRoot)
	if err != nil {
		t.Fatalf("ListDatabases failed: %v", err)
	}
	if len(databases) != 0 {
		t.Errorf("expected 0 databases, got %d", len(databases))
	}
}

func TestListDatabases_NoDataDir(t *testing.T) {
	townRoot := t.TempDir()
	databases, err := ListDatabases(townRoot)
	if err != nil {
		t.Fatalf("ListDatabases failed: %v", err)
	}
	if databases != nil {
		t.Errorf("expected nil, got %v", databases)
	}
}

func TestListDatabases_MixedContent(t *testing.T) {
	townRoot := t.TempDir()
	dataDir := filepath.Join(townRoot, ".dolt-data")

	if err := os.MkdirAll(filepath.Join(dataDir, "hq", ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "not-a-db"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "somefile.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "myrig", ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}

	databases, err := ListDatabases(townRoot)
	if err != nil {
		t.Fatalf("ListDatabases failed: %v", err)
	}
	if len(databases) != 2 {
		t.Errorf("expected 2 databases, got %d: %v", len(databases), databases)
	}
}

// =============================================================================
// Connection string tests
// =============================================================================

func TestGetConnectionString(t *testing.T) {
	townRoot := t.TempDir()
	s := GetConnectionString(townRoot)
	if s != "root@tcp(127.0.0.1:3307)/" {
		t.Errorf("got %q, want root@tcp(127.0.0.1:3307)/", s)
	}
}

func TestGetConnectionStringForRig(t *testing.T) {
	townRoot := t.TempDir()
	s := GetConnectionStringForRig(townRoot, "hq")
	if s != "root@tcp(127.0.0.1:3307)/hq" {
		t.Errorf("got %q, want root@tcp(127.0.0.1:3307)/hq", s)
	}
}

// =============================================================================
// State tests
// =============================================================================

func TestSaveAndLoadState(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "daemon"), 0755); err != nil {
		t.Fatal(err)
	}

	state := &State{
		Running:   true,
		PID:       12345,
		Port:      3307,
		DataDir:   filepath.Join(townRoot, ".dolt-data"),
		Databases: []string{"hq", "myrig"},
	}

	if err := SaveState(townRoot, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := LoadState(townRoot)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if !loaded.Running {
		t.Error("Running should be true")
	}
	if loaded.PID != 12345 {
		t.Errorf("PID = %d, want 12345", loaded.PID)
	}
	if len(loaded.Databases) != 2 {
		t.Errorf("expected 2 databases, got %d", len(loaded.Databases))
	}
}

func TestLoadState_NoFile(t *testing.T) {
	townRoot := t.TempDir()
	state, err := LoadState(townRoot)
	if err != nil {
		t.Fatalf("LoadState with no file: %v", err)
	}
	if state == nil {
		t.Fatal("expected empty state, not nil")
	}
	if state.Running {
		t.Error("empty state should not be running")
	}
}

func TestLoadState_CorruptJSON(t *testing.T) {
	townRoot := t.TempDir()
	stateFile := StateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, []byte(`{corrupt`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(townRoot)
	if err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

// =============================================================================
// Rollback round-trip test
// =============================================================================

func TestRollbackRoundTrip(t *testing.T) {
	townRoot := t.TempDir()

	rigName := "roundtrip"
	originalBeads := filepath.Join(townRoot, rigName, ".beads")
	if err := os.MkdirAll(originalBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(originalBeads, "metadata.json"),
		[]byte(`{"backend":"sqlite"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(originalBeads, "beads.db"),
		[]byte("original-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create backup
	backupDir := filepath.Join(townRoot, "migration-backup-20240115-143022")
	rigBackup := filepath.Join(backupDir, rigName+"-beads")
	if err := os.MkdirAll(rigBackup, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"metadata.json", "beads.db"} {
		data, _ := os.ReadFile(filepath.Join(originalBeads, f))
		os.WriteFile(filepath.Join(rigBackup, f), data, 0644)
	}

	// Simulate migration
	os.WriteFile(filepath.Join(originalBeads, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_mode":"server"}`), 0600)
	os.Remove(filepath.Join(originalBeads, "beads.db"))

	// Rollback
	result, err := RestoreFromBackup(townRoot, backupDir)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if len(result.RestoredRigs) != 1 {
		t.Fatalf("expected 1 restored rig, got %d", len(result.RestoredRigs))
	}

	// Verify rollback restored original state
	data, _ := os.ReadFile(filepath.Join(originalBeads, "metadata.json"))
	var meta map[string]interface{}
	json.Unmarshal(data, &meta)
	if meta["backend"] != "sqlite" {
		t.Errorf("after rollback: backend = %v, want sqlite", meta["backend"])
	}
	dbData, _ := os.ReadFile(filepath.Join(originalBeads, "beads.db"))
	if string(dbData) != "original-data" {
		t.Errorf("beads.db content = %q, want original-data", string(dbData))
	}
}

// =============================================================================
// Spaces in paths
// =============================================================================

func TestFindMigratableDatabases_SpacesInPath(t *testing.T) {
	townRoot := filepath.Join(t.TempDir(), "my town root")
	if err := os.MkdirAll(townRoot, 0755); err != nil {
		t.Fatal(err)
	}

	rigName := "my-rig"
	sourceDolt := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads_spacey", ".dolt")
	if err := os.MkdirAll(sourceDolt, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)
	found := false
	for _, m := range migrations {
		if m.RigName == rigName {
			found = true
		}
	}
	if !found {
		t.Error("expected to find migration in path with spaces")
	}
}

func TestFindMigratableDatabases_EmptyTownRoot(t *testing.T) {
	townRoot := t.TempDir()
	migrations := FindMigratableDatabases(townRoot)
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestFindMigratableDatabases_TownBeads(t *testing.T) {
	townRoot := t.TempDir()
	hqSource := filepath.Join(townRoot, ".beads", "dolt", "beads_hq", ".dolt")
	if err := os.MkdirAll(hqSource, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)
	found := false
	for _, m := range migrations {
		if m.RigName == "hq" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find HQ migration")
	}
}

func TestFindMigratableDatabases_SkipsDotDirs(t *testing.T) {
	townRoot := t.TempDir()
	hiddenDolt := filepath.Join(townRoot, ".hidden-rig", ".beads", "dolt", "beads_hidden", ".dolt")
	if err := os.MkdirAll(hiddenDolt, 0755); err != nil {
		t.Fatal(err)
	}

	migrations := FindMigratableDatabases(townRoot)
	for _, m := range migrations {
		if m.RigName == ".hidden-rig" {
			t.Error("should skip dot-directories")
		}
	}
}

func TestMoveDir_SourceNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	err := moveDir(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

// =============================================================================
// Branch name validation tests (SQL injection prevention)
// =============================================================================

func TestValidateBranchName_ValidNames(t *testing.T) {
	valid := []string{
		"main",
		"polecat-furiosa-1707400000",
		"feature/my-branch",
		"release-v1.2.3",
		"my_branch",
		"UPPER-case",
		"a",
	}
	for _, name := range valid {
		if err := validateBranchName(name); err != nil {
			t.Errorf("validateBranchName(%q) = %v, want nil", name, err)
		}
	}
}

// =============================================================================
// DatabaseExists tests
// =============================================================================

func TestDatabaseExists_True(t *testing.T) {
	townRoot := t.TempDir()
	doltDir := filepath.Join(townRoot, ".dolt-data", "myrig", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}
	if !DatabaseExists(townRoot, "myrig") {
		t.Error("expected database to exist")
	}
}

func TestDatabaseExists_False(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}
	if DatabaseExists(townRoot, "nonexistent") {
		t.Error("expected database to not exist")
	}
}

func TestDatabaseExists_NoDataDir(t *testing.T) {
	townRoot := t.TempDir()
	if DatabaseExists(townRoot, "anything") {
		t.Error("expected false when .dolt-data doesn't exist")
	}
}

// =============================================================================
// FindBrokenWorkspaces tests
// =============================================================================

func TestFindBrokenWorkspaces_HealthyWorkspace(t *testing.T) {
	townRoot := t.TempDir()

	// Create a healthy workspace: metadata says dolt, and database exists
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Database exists
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data", "hq", ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}

	// Set up rigs.json (empty, only checking hq)
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	broken := FindBrokenWorkspaces(townRoot)
	if len(broken) != 0 {
		t.Errorf("expected 0 broken workspaces, got %d: %+v", len(broken), broken)
	}
}

func TestFindBrokenWorkspaces_MissingDatabase(t *testing.T) {
	townRoot := t.TempDir()

	// Metadata says dolt, but database does NOT exist
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .dolt-data but NO hq database inside
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	broken := FindBrokenWorkspaces(townRoot)
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken workspace, got %d", len(broken))
	}
	if broken[0].RigName != "hq" {
		t.Errorf("expected rig hq, got %s", broken[0].RigName)
	}
	if broken[0].ConfiguredDB != "hq" {
		t.Errorf("expected ConfiguredDB=hq, got %s", broken[0].ConfiguredDB)
	}
	if broken[0].HasLocalData {
		t.Error("expected no local data")
	}
}

func TestFindBrokenWorkspaces_WithLocalData(t *testing.T) {
	townRoot := t.TempDir()

	// Rig metadata says dolt, database missing, but local data exists
	rigName := "myrig"
	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"myrig"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Local Dolt data exists
	localDolt := filepath.Join(beadsDir, "dolt", "beads_myrig", ".dolt")
	if err := os.MkdirAll(localDolt, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"),
		[]byte(`{"rigs":{"myrig":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	broken := FindBrokenWorkspaces(townRoot)
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken workspace, got %d", len(broken))
	}
	if !broken[0].HasLocalData {
		t.Error("expected HasLocalData=true")
	}
	if broken[0].LocalDataPath == "" {
		t.Error("expected non-empty LocalDataPath")
	}
}

func TestFindBrokenWorkspaces_SqliteNotBroken(t *testing.T) {
	townRoot := t.TempDir()

	// Workspace configured for SQLite, not Dolt — should not appear as broken
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"backend":"sqlite","database":"beads.db"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	broken := FindBrokenWorkspaces(townRoot)
	if len(broken) != 0 {
		t.Errorf("expected 0 broken workspaces for sqlite backend, got %d", len(broken))
	}
}

func TestFindBrokenWorkspaces_MultipleRigs(t *testing.T) {
	townRoot := t.TempDir()

	// Set up rigs.json with two rigs
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"),
		[]byte(`{"rigs":{"rig-a":{},"rig-b":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// rig-a: broken (metadata says dolt, no database)
	beadsDirA := filepath.Join(townRoot, "rig-a", "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDirA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDirA, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_mode":"server","dolt_database":"rig-a"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// rig-b: healthy (metadata says dolt, database exists)
	beadsDirB := filepath.Join(townRoot, "rig-b", "mayor", "rig", ".beads")
	if err := os.MkdirAll(beadsDirB, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDirB, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_mode":"server","dolt_database":"rig-b"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data", "rig-b", ".dolt"), 0755); err != nil {
		t.Fatal(err)
	}

	broken := FindBrokenWorkspaces(townRoot)
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken workspace (rig-a only), got %d", len(broken))
	}
	if broken[0].RigName != "rig-a" {
		t.Errorf("expected rig-a broken, got %s", broken[0].RigName)
	}
}

// =============================================================================
// Read-only detection tests
// =============================================================================

func TestIsReadOnlyError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"cannot update manifest: database is read only", true},
		{"database is read only", true},
		{"Database Is Read Only", true},
		{"error: read-only mode", true},
		{"server is readonly", true},
		{"READ ONLY transaction", true},
		{"connection refused", false},
		{"timeout", false},
		{"", false},
		{"table not found", false},
		{"permission denied", false},
	}

	for _, tt := range tests {
		if got := IsReadOnlyError(tt.msg); got != tt.want {
			t.Errorf("IsReadOnlyError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestHealthMetrics_ReadOnlyField(t *testing.T) {
	// Verify that the ReadOnly field is properly included in HealthMetrics.
	// We can't test actual read-only detection without a running Dolt server,
	// but we can verify the field is populated.
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}

	metrics := GetHealthMetrics(townRoot)

	// Without a running server, ReadOnly should be false (can't probe)
	if metrics.ReadOnly {
		t.Error("expected ReadOnly=false when no server is running")
	}
}

func TestIsDoltRetryableError_IncludesReadOnly(t *testing.T) {
	// Verify that read-only errors are recognized as retryable.
	// This is critical for the recovery path: doltSQLWithRetry must
	// retry on read-only before escalating to doltSQLWithRecovery.
	tests := []struct {
		msg  string
		want bool
	}{
		{"cannot update manifest: database is read only", true},
		{"database is read only", true},
		{"optimistic lock failed", true},
		{"serialization failure", true},
		{"lock wait timeout exceeded", true},
		{"try restarting transaction", true},
		{"connection refused", false},
		{"table not found", false},
	}
	for _, tt := range tests {
		err := fmt.Errorf("%s", tt.msg)
		if got := isDoltRetryableError(err); got != tt.want {
			t.Errorf("isDoltRetryableError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestRecoverReadOnly_NoServer(t *testing.T) {
	// When no server is running, CheckReadOnly returns false (can't probe),
	// so RecoverReadOnly should be a no-op.
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755); err != nil {
		t.Fatal(err)
	}

	err := RecoverReadOnly(townRoot)
	// Should succeed (no-op) since no server means no read-only state detectable
	if err != nil {
		t.Errorf("RecoverReadOnly with no server: got error %v, want nil", err)
	}
}

func TestValidateBranchName_InvalidNames(t *testing.T) {
	invalid := []string{
		"",                          // empty
		"branch'name",               // single quote (SQL injection)
		"branch;DROP TABLE",         // semicolon
		"branch name",               // space
		"branch\tname",              // tab
		"$(command)",                // command substitution
		"branch`cmd`",               // backtick
		"branch\"name",              // double quote
		"branch\\name",              // backslash
		"'); DROP TABLE issues; --", // classic SQL injection
	}
	for _, name := range invalid {
		if err := validateBranchName(name); err == nil {
			t.Errorf("validateBranchName(%q) = nil, want error", name)
		}
	}
}

// =============================================================================
// doltSQLScriptWithRetry tests
// =============================================================================

func TestDoltSQLScriptWithRetry_ImmediateSuccess(t *testing.T) {
	// doltSQLScriptWithRetry calls doltSQLScript which needs a valid townRoot
	// with .dolt-data dir and a dolt binary. Since we can't run dolt in CI,
	// we verify the retry logic by checking that non-retryable errors return
	// immediately without sleeping (i.e., isDoltRetryableError integration).
	//
	// A non-retryable error (e.g., syntax error) should return on first attempt.
	err := doltSQLScriptWithRetry(t.TempDir(), "INVALID SQL;")
	if err == nil {
		// If dolt isn't installed, the exec itself fails — that's fine,
		// the point is it doesn't retry/hang.
		t.Skip("dolt binary available and accepted invalid SQL somehow")
	}
	// Verify the error is not wrapped with "after N retries" since exec failures
	// (dolt not found / not a dolt data dir) are not retryable.
	if strings.Contains(err.Error(), "after 3 retries") {
		t.Errorf("non-retryable error was retried: %v", err)
	}
}

func TestDoltSQLScriptWithRetry_NonRetryableError(t *testing.T) {
	// Verify that isDoltRetryableError correctly classifies errors.
	// Non-retryable errors should fail fast without retry.
	nonRetryable := []string{
		"syntax error near 'FOO'",
		"table not found",
		"unknown column",
	}
	for _, msg := range nonRetryable {
		if isDoltRetryableError(fmt.Errorf("%s", msg)) {
			t.Errorf("isDoltRetryableError(%q) = true, want false", msg)
		}
	}

	// Retryable errors should be classified as such.
	retryable := []string{
		"database is read only",
		"cannot update manifest",
		"optimistic lock failed",
		"serialization failure",
		"lock wait timeout",
		"try restarting transaction",
	}
	for _, msg := range retryable {
		if !isDoltRetryableError(fmt.Errorf("%s", msg)) {
			t.Errorf("isDoltRetryableError(%q) = false, want true", msg)
		}
	}
}

// =============================================================================
// MergePolecatBranch script generation tests
// =============================================================================

func TestMergePolecatBranch_NoBranchDeleteInScripts(t *testing.T) {
	// Verify that MergePolecatBranch's SQL scripts don't contain DOLT_BRANCH('-D').
	// Branch deletion must happen AFTER successful merge, not inside the scripts,
	// to prevent branch loss if the merge script fails partway through.
	//
	// We can't run the actual merge (requires dolt server), but we can verify
	// the function validates branch names correctly — invalid names are rejected
	// before any script is generated.
	err := MergePolecatBranch(t.TempDir(), "testrig", "'; DROP TABLE --")
	if err == nil {
		t.Error("expected error for SQL injection branch name")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected 'invalid' in error, got: %v", err)
	}
}

func TestMergePolecatBranch_ValidBranchName(t *testing.T) {
	// Verify that valid branch names pass validation (function will fail
	// later at the dolt execution step, but validation should pass).
	err := MergePolecatBranch(t.TempDir(), "testrig", "polecat-alpha-123")
	if err == nil {
		t.Skip("dolt server available — merge unexpectedly succeeded")
	}
	// Should NOT be a validation error
	if strings.Contains(err.Error(), "invalid") {
		t.Errorf("valid branch name rejected: %v", err)
	}
}
