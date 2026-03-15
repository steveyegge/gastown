package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanStaleSQLServerInfo_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Should not panic or error when file doesn't exist
	CleanStaleSQLServerInfo(tmpDir)
}

func TestCleanStaleSQLServerInfo_StaleProcess(t *testing.T) {
	tmpDir := t.TempDir()
	doltDir := filepath.Join(tmpDir, "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(doltDir, "sql-server.info")
	// PID 999999999 is almost certainly not running
	if err := os.WriteFile(infoPath, []byte("999999999:3307:some-uuid"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleSQLServerInfo(tmpDir)

	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Error("expected stale sql-server.info to be removed")
	}
}

func TestCleanStaleSQLServerInfo_LiveProcess(t *testing.T) {
	tmpDir := t.TempDir()
	doltDir := filepath.Join(tmpDir, "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(doltDir, "sql-server.info")
	// Use our own PID — guaranteed to be alive
	content := fmt.Sprintf("%d:3307:some-uuid", os.Getpid())
	if err := os.WriteFile(infoPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleSQLServerInfo(tmpDir)

	if _, err := os.Stat(infoPath); os.IsNotExist(err) {
		t.Error("expected sql-server.info for live process to be preserved")
	}
}

func TestCleanStaleSQLServerInfo_MalformedFile(t *testing.T) {
	tmpDir := t.TempDir()
	doltDir := filepath.Join(tmpDir, "dolt", ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(doltDir, "sql-server.info")
	if err := os.WriteFile(infoPath, []byte("not-a-pid"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleSQLServerInfo(tmpDir)

	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Error("expected malformed sql-server.info to be removed")
	}
}

func TestCleanStaleDoltServerPID_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Should not panic or error when file doesn't exist
	CleanStaleDoltServerPID(tmpDir)
}

func TestCleanStaleDoltServerPID_StaleProcess(t *testing.T) {
	tmpDir := t.TempDir()
	doltDir := filepath.Join(tmpDir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	pidPath := filepath.Join(doltDir, "dolt-server.pid")
	if err := os.WriteFile(pidPath, []byte("999999999"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleDoltServerPID(tmpDir)

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected stale dolt-server.pid to be removed")
	}
}
