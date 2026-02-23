package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewUnregisteredBeadsDirsCheck(t *testing.T) {
	check := NewUnregisteredBeadsDirsCheck()

	if check.Name() != "unregistered-beads-dirs" {
		t.Errorf("expected name 'unregistered-beads-dirs', got %q", check.Name())
	}

	if check.CanFix() {
		t.Error("expected CanFix to return false")
	}

	if check.Category() != CategoryCleanup {
		t.Errorf("expected category %q, got %q", CategoryCleanup, check.Category())
	}
}

func TestUnregisteredBeadsDirs_Clean(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a registered rig with metadata
	setupRigsJSON(t, tmpDir, []string{"myrig"})
	writeBeadsMetadata(t, filepath.Join(tmpDir, "myrig"), "myrig_db")

	// Create system dirs (should be ignored)
	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755)

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		for _, d := range result.Details {
			t.Logf("  detail: %s", d)
		}
	}
}

func TestUnregisteredBeadsDirs_OrphanDir(t *testing.T) {
	tmpDir := t.TempDir()

	// No rigs registered
	setupRigsJSON(t, tmpDir, nil)

	// Create an unregistered directory with beads metadata
	writeBeadsMetadata(t, filepath.Join(tmpDir, "stale_rig"), "stale_db")

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	if len(result.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(result.Details))
	}
}

func TestUnregisteredBeadsDirs_IgnoresRegisteredRigs(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, []string{"rig_a", "rig_b"})
	writeBeadsMetadata(t, filepath.Join(tmpDir, "rig_a"), "db_a")
	writeBeadsMetadata(t, filepath.Join(tmpDir, "rig_b"), "db_b")

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestUnregisteredBeadsDirs_IgnoresSystemDirs(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, nil)

	// Create system dirs with beads metadata (should still be ignored)
	for _, sysDir := range []string{"mayor", "deacon", ".runtime"} {
		writeBeadsMetadata(t, filepath.Join(tmpDir, sysDir), "some_db")
	}

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestUnregisteredBeadsDirs_DeaconMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, nil)

	// Town beads uses "hq"
	writeBeadsMetadata(t, tmpDir, "hq")

	// Deacon uses a different database
	writeBeadsMetadata(t, filepath.Join(tmpDir, "deacon"), "beads_deacon")

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	if len(result.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(result.Details))
	}
}

func TestUnregisteredBeadsDirs_DeaconMatchesOK(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, nil)

	// Town and deacon both use "hq"
	writeBeadsMetadata(t, tmpDir, "hq")
	writeBeadsMetadata(t, filepath.Join(tmpDir, "deacon"), "hq")

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestUnregisteredBeadsDirs_MultipleOrphans(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, nil)

	writeBeadsMetadata(t, filepath.Join(tmpDir, "orphan_a"), "db_a")
	writeBeadsMetadata(t, filepath.Join(tmpDir, "orphan_b"), "db_b")
	writeBeadsMetadata(t, filepath.Join(tmpDir, "orphan_c"), "db_c")

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	if len(result.Details) != 3 {
		t.Errorf("expected 3 details, got %d", len(result.Details))
	}
}

func TestUnregisteredBeadsDirs_DirWithoutMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	setupRigsJSON(t, tmpDir, nil)

	// Directory with .beads/ but no metadata.json
	os.MkdirAll(filepath.Join(tmpDir, "random_dir", ".beads"), 0755)

	// Plain directory (no .beads at all)
	os.MkdirAll(filepath.Join(tmpDir, "plain_dir"), 0755)

	check := NewUnregisteredBeadsDirsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK (no metadata.json), got %v: %s", result.Status, result.Message)
	}
}

// writeBeadsMetadata creates a .beads/metadata.json in dir with the given dolt_database.
func writeBeadsMetadata(t *testing.T, dir string, doltDB string) {
	t.Helper()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"backend":"dolt","dolt_database":"` + doltDB + `","dolt_mode":"server"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
