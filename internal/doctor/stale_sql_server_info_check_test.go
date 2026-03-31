package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStaleSQLServerInfoCheck_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK with no sql-server.info files, got %s: %s", result.Status, result.Message)
	}
}

func TestStaleSQLServerInfoCheck_DetectsStaleFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix process signals")
	}

	tmpDir := t.TempDir()

	// Create a .dolt directory with a sql-server.info file referencing a dead PID
	doltDir := filepath.Join(tmpDir, "myrig", ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use PID 999999999 which is almost certainly not running
	infoPath := filepath.Join(doltDir, "sql-server.info")
	if err := os.WriteFile(infoPath, []byte("999999999:3307:some-uuid"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected Warning for stale sql-server.info, got %s: %s", result.Status, result.Message)
	}
	if len(check.staleFiles) != 1 {
		t.Errorf("expected 1 stale file, got %d", len(check.staleFiles))
	}
}

func TestStaleSQLServerInfoCheck_FixIsNoOp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix process signals")
	}

	tmpDir := t.TempDir()

	// Create stale sql-server.info files in two rigs
	for _, rig := range []string{"rig1", "rig2"} {
		doltDir := filepath.Join(tmpDir, rig, ".beads", "dolt", ".dolt")
		if err := os.MkdirAll(doltDir, 0755); err != nil {
			t.Fatal(err)
		}
		infoPath := filepath.Join(doltDir, "sql-server.info")
		if err := os.WriteFile(infoPath, []byte("999999999:3307:uuid"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected Warning, got %s", result.Status)
	}
	if len(check.staleFiles) != 2 {
		t.Fatalf("expected 2 stale files, got %d", len(check.staleFiles))
	}

	// Fix is a no-op — sql-server.info is a Dolt-internal file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify files are NOT removed (Fix no longer deletes Dolt-internal files)
	for _, path := range check.staleFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to still exist after no-op fix", path)
		}
	}
}

func TestStaleSQLServerInfoCheck_SkipsLiveProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix process signals")
	}

	tmpDir := t.TempDir()

	// Create a sql-server.info with our own PID (definitely alive)
	doltDir := filepath.Join(tmpDir, "myrig", ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(doltDir, "sql-server.info")
	content := fmt.Sprintf("%d:3307:some-uuid", os.Getpid())
	if err := os.WriteFile(infoPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for live process, got %s: %s", result.Status, result.Message)
	}
}

func TestStaleSQLServerInfoCheck_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	doltDir := filepath.Join(tmpDir, "myrig", ".beads", "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(doltDir, "sql-server.info")
	if err := os.WriteFile(infoPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleSQLServerInfoCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected Warning for empty sql-server.info, got %s: %s", result.Status, result.Message)
	}
}
