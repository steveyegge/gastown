package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDaemonStartupFailure(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	logData := "" +
		"2026/03/28 22:00:00 Daemon startup failed (PID 111): stale error\n" +
		"2026/03/28 22:00:01 Daemon startup failed (PID 222): incompatible beads workspace / gt binary combination\n"
	if err := os.WriteFile(filepath.Join(daemonDir, "daemon.log"), []byte(logData), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := readDaemonStartupFailure(townRoot, 222)
	want := "incompatible beads workspace / gt binary combination"
	if got != want {
		t.Fatalf("readDaemonStartupFailure() = %q, want %q", got, want)
	}
}

func TestReadDaemonStartupFailure_MissingPIDReturnsEmpty(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(daemonDir, "daemon.log"), []byte("2026/03/28 22:00:00 Daemon startup failed (PID 111): stale error\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got := readDaemonStartupFailure(townRoot, 222); got != "" {
		t.Fatalf("readDaemonStartupFailure() = %q, want empty string", got)
	}
}
