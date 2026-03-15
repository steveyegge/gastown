package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStaleSQLServerInfoCheck_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	setupMinimalTown(t, tmpDir)

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no sql-server.info exists, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleSQLServerInfoCheck_StaleFile(t *testing.T) {
	tmpDir := t.TempDir()
	setupMinimalTown(t, tmpDir)

	// Create stale sql-server.info in town .beads
	doltDir := filepath.Join(tmpDir, ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}
	// PID 999999999 is almost certainly dead
	if err := os.WriteFile(filepath.Join(doltDir, "sql-server.info"), []byte("999999999:3307:some-uuid"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for stale sql-server.info, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) == 0 {
		t.Error("expected details to describe the stale file")
	}
}

func TestStaleSQLServerInfoCheck_LiveProcess(t *testing.T) {
	tmpDir := t.TempDir()
	setupMinimalTown(t, tmpDir)

	// Create sql-server.info with our own PID (alive)
	doltDir := filepath.Join(tmpDir, ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("%d:3307:some-uuid", os.Getpid())
	if err := os.WriteFile(filepath.Join(doltDir, "sql-server.info"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for live process, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleSQLServerInfoCheck_FixRemovesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	setupMinimalTown(t, tmpDir)

	// Create stale sql-server.info
	doltDir := filepath.Join(tmpDir, ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}
	infoPath := filepath.Join(doltDir, "sql-server.info")
	if err := os.WriteFile(infoPath, []byte("999999999:3307:some-uuid"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v", result.Status)
	}

	// Fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify removed
	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Error("expected sql-server.info to be removed after fix")
	}

	// Re-run check should pass
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

// setupMinimalTown creates the minimum directory structure for a town.
func setupMinimalTown(t *testing.T, tmpDir string) {
	t.Helper()

	// Create mayor/rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigs := map[string]interface{}{"rigs": map[string]interface{}{}}
	rigsBytes, _ := json.Marshal(rigs)
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), rigsBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Create .beads directory
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
}
