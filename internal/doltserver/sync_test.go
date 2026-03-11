package doltserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFindRemote_NoRemote verifies FindRemote returns empty when no remote is configured.
func TestFindRemote_NoRemote(t *testing.T) {
	// Create a minimal dolt database directory
	dbDir := t.TempDir()
	doltDir := filepath.Join(dbDir, ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatalf("mkdir .dolt: %v", err)
	}

	// Initialize a bare dolt repo so "dolt remote -v" works
	if err := initDoltDB(dbDir); err != nil {
		t.Skipf("dolt not available: %v", err)
	}

	name, url, err := FindRemote(dbDir)
	if err != nil {
		t.Fatalf("FindRemote: %v", err)
	}
	if name != "" || url != "" {
		t.Errorf("expected empty remote, got name=%q url=%q", name, url)
	}
}

// TestSyncDatabases_EmptyDir verifies SyncDatabases handles missing data dir gracefully.
func TestSyncDatabases_EmptyDir(t *testing.T) {
	townRoot := t.TempDir()
	// No .dolt-data directory exists
	opts := SyncOptions{}
	results := SyncDatabases(townRoot, opts)
	// Should return empty or a single error result, not panic
	for _, r := range results {
		if r.Error != nil {
			// Acceptable — no data dir
			return
		}
	}
	// Also acceptable: empty results
}

// TestSyncDatabases_FilterSkipsOthers verifies the filter option.
func TestSyncDatabases_FilterSkipsOthers(t *testing.T) {
	townRoot := t.TempDir()
	dataDir := filepath.Join(townRoot, ".dolt-data")

	// Create two fake database dirs with noms/manifest
	for _, db := range []string{"alpha", "beta"} {
		nomsDir := filepath.Join(dataDir, db, ".dolt", "noms")
		if err := os.MkdirAll(nomsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nomsDir, "manifest"), []byte("x"), 0644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}

	opts := SyncOptions{Filter: "alpha", DryRun: true}
	results := SyncDatabases(townRoot, opts)

	for _, r := range results {
		if r.Database == "beta" {
			t.Errorf("filter=alpha but beta was included in results")
		}
	}
}

// TestSyncDatabasesSQL_EmptyDir verifies SyncDatabasesSQL handles missing data dir.
func TestSyncDatabasesSQL_EmptyDir(t *testing.T) {
	townRoot := t.TempDir()
	opts := SyncOptions{}
	results := SyncDatabasesSQL(townRoot, opts)
	for _, r := range results {
		if r.Error != nil {
			return // acceptable
		}
	}
}

// TestSyncDatabasesSQL_FilterSkipsOthers verifies the SQL sync filter option.
func TestSyncDatabasesSQL_FilterSkipsOthers(t *testing.T) {
	townRoot := t.TempDir()
	dataDir := filepath.Join(townRoot, ".dolt-data")

	for _, db := range []string{"alpha", "beta"} {
		nomsDir := filepath.Join(dataDir, db, ".dolt", "noms")
		if err := os.MkdirAll(nomsDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nomsDir, "manifest"), []byte("x"), 0644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}

	opts := SyncOptions{Filter: "alpha", DryRun: true}
	results := SyncDatabasesSQL(townRoot, opts)

	for _, r := range results {
		if r.Database == "beta" {
			t.Errorf("filter=alpha but beta was included in results")
		}
	}
}

// TestValidSQLName verifies the defense-in-depth name validation.
func TestValidSQLName(t *testing.T) {
	valid := []string{"mydb", "beads_gastown", "my-db", "db.v2", "ABC123"}
	for _, name := range valid {
		if !validSQLName(name) {
			t.Errorf("validSQLName(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "my`db", "db; DROP TABLE", "name'quote", "has space", "db\nline"}
	for _, name := range invalid {
		if validSQLName(name) {
			t.Errorf("validSQLName(%q) = true, want false", name)
		}
	}
}

// initDoltDB runs "dolt init" in a directory. Returns error if dolt isn't available.
func initDoltDB(dir string) error {
	cmd := exec.Command("dolt", "init", "--name", "test", "--email", "test@test.com")
	cmd.Dir = dir
	return cmd.Run()
}
