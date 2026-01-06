package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewBeadsDatabaseCheck(t *testing.T) {
	check := NewBeadsDatabaseCheck()

	if check.Name() != "beads-database" {
		t.Errorf("expected name 'beads-database', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBeadsDatabaseCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty database
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Create JSONL with content
	jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"id":"test-1","title":"Test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for empty db with content in jsonl, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_PopulatedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create database with content
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for populated db, got %v", result.Status)
	}
}

func TestEnsureBeadsPrefixSet_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// No database file exists - should return nil (no-op)
	err := ensureBeadsPrefixSet(beadsDir, "test")
	if err != nil {
		t.Errorf("expected nil error when database doesn't exist, got %v", err)
	}
}

func TestEnsureBeadsPrefixSet_DatabaseMissingPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty SQLite database with config table but no prefix
	dbPath := filepath.Join(beadsDir, "beads.db")
	createCmd := exec.Command("sqlite3", dbPath, "CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT);")
	if err := createCmd.Run(); err != nil {
		t.Skipf("sqlite3 not available: %v", err)
	}

	// Run ensureBeadsPrefixSet
	err := ensureBeadsPrefixSet(beadsDir, "test")
	if err != nil {
		t.Fatalf("ensureBeadsPrefixSet failed: %v", err)
	}

	// Verify prefix was inserted
	checkCmd := exec.Command("sqlite3", dbPath, "SELECT value FROM config WHERE key = 'issue_prefix';")
	out, err := checkCmd.Output()
	if err != nil {
		t.Fatalf("failed to check prefix: %v", err)
	}

	if got := strings.TrimSpace(string(out)); got != "test-" {
		t.Errorf("expected prefix 'test-', got %q", got)
	}
}

func TestEnsureBeadsPrefixSet_DatabaseHasPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create SQLite database with prefix already set
	dbPath := filepath.Join(beadsDir, "beads.db")
	createCmd := exec.Command("sqlite3", dbPath,
		"CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT); INSERT INTO config VALUES ('issue_prefix', 'existing-');")
	if err := createCmd.Run(); err != nil {
		t.Skipf("sqlite3 not available: %v", err)
	}

	// Run ensureBeadsPrefixSet - should be a no-op
	err := ensureBeadsPrefixSet(beadsDir, "new")
	if err != nil {
		t.Fatalf("ensureBeadsPrefixSet failed: %v", err)
	}

	// Verify prefix was NOT changed (should still be "existing-")
	checkCmd := exec.Command("sqlite3", dbPath, "SELECT value FROM config WHERE key = 'issue_prefix';")
	out, err := checkCmd.Output()
	if err != nil {
		t.Fatalf("failed to check prefix: %v", err)
	}

	if got := strings.TrimSpace(string(out)); got != "existing-" {
		t.Errorf("expected prefix 'existing-' (unchanged), got %q", got)
	}
}

func TestInsertBeadsPrefix_AddsHyphen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "beads.db")

	// Create empty database
	createCmd := exec.Command("sqlite3", dbPath, "SELECT 1;")
	if err := createCmd.Run(); err != nil {
		t.Skipf("sqlite3 not available: %v", err)
	}

	// Insert prefix without hyphen
	err := insertBeadsPrefix(dbPath, "test")
	if err != nil {
		t.Fatalf("insertBeadsPrefix failed: %v", err)
	}

	// Verify prefix has trailing hyphen
	checkCmd := exec.Command("sqlite3", dbPath, "SELECT value FROM config WHERE key = 'issue_prefix';")
	out, err := checkCmd.Output()
	if err != nil {
		t.Fatalf("failed to check prefix: %v", err)
	}

	if got := strings.TrimSpace(string(out)); got != "test-" {
		t.Errorf("expected prefix 'test-', got %q", got)
	}
}

func TestInsertBeadsPrefix_PreservesHyphen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "beads.db")

	// Create empty database
	createCmd := exec.Command("sqlite3", dbPath, "SELECT 1;")
	if err := createCmd.Run(); err != nil {
		t.Skipf("sqlite3 not available: %v", err)
	}

	// Insert prefix with hyphen already
	err := insertBeadsPrefix(dbPath, "test-")
	if err != nil {
		t.Fatalf("insertBeadsPrefix failed: %v", err)
	}

	// Verify prefix still has single hyphen
	checkCmd := exec.Command("sqlite3", dbPath, "SELECT value FROM config WHERE key = 'issue_prefix';")
	out, err := checkCmd.Output()
	if err != nil {
		t.Fatalf("failed to check prefix: %v", err)
	}

	if got := strings.TrimSpace(string(out)); got != "test-" {
		t.Errorf("expected prefix 'test-', got %q", got)
	}
}
