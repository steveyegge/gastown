package beads

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCleanStaleDoltServerPID_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	// Should be a no-op when no PID file exists
	CleanStaleDoltServerPID(dir)
}

func TestCleanStaleDoltServerPID_StalePID(t *testing.T) {
	dir := t.TempDir()
	doltDir := filepath.Join(dir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	pidPath := filepath.Join(doltDir, "dolt-server.pid")
	// PID 1 is init/launchd — always alive but never a dolt server.
	// Use a very high PID that doesn't exist.
	if err := os.WriteFile(pidPath, []byte("999999999"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleDoltServerPID(dir)

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected stale PID file to be removed")
	}
}

func TestCleanStaleDoltServerPID_AlivePID(t *testing.T) {
	dir := t.TempDir()
	doltDir := filepath.Join(dir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use our own PID (guaranteed alive)
	pidPath := filepath.Join(doltDir, "dolt-server.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleDoltServerPID(dir)

	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("expected alive PID file to be kept")
	}
}

func TestCleanStaleDoltServerPID_CorruptPID(t *testing.T) {
	dir := t.TempDir()
	doltDir := filepath.Join(dir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	pidPath := filepath.Join(doltDir, "dolt-server.pid")
	if err := os.WriteFile(pidPath, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatal(err)
	}

	CleanStaleDoltServerPID(dir)

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected corrupt PID file to be removed")
	}
}
