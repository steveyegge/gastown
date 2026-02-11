package session

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTrackPID_WritesFile(t *testing.T) {
	townRoot := t.TempDir()

	if err := TrackPID(townRoot, "gt-myrig-witness", 12345); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	path := pidFile(townRoot, "gt-myrig-witness")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}

	if got := string(data); got != "12345\n" {
		t.Errorf("PID file content = %q, want %q", got, "12345\n")
	}
}

func TestTrackPID_CreatesDirectory(t *testing.T) {
	townRoot := t.TempDir()

	if err := TrackPID(townRoot, "gt-test-session", 99); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	dir := pidsDir(townRoot)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("pids directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("pids path is not a directory")
	}
}

func TestUntrackPID_RemovesFile(t *testing.T) {
	townRoot := t.TempDir()

	if err := TrackPID(townRoot, "gt-test", 111); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	UntrackPID(townRoot, "gt-test")

	path := pidFile(townRoot, "gt-test")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should be removed after UntrackPID")
	}
}

func TestUntrackPID_NoopOnMissing(t *testing.T) {
	townRoot := t.TempDir()
	// Should not panic or error on missing file
	UntrackPID(townRoot, "nonexistent")
}

func TestKillTrackedPIDs_EmptyDir(t *testing.T) {
	townRoot := t.TempDir()
	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0", killed)
	}
	if len(errs) != 0 {
		t.Errorf("errs = %v, want empty", errs)
	}
}

func TestKillTrackedPIDs_DeadProcess(t *testing.T) {
	townRoot := t.TempDir()

	// Write a PID file for a process that definitely doesn't exist
	// (PID 2^22 + 1 is almost certainly not running)
	if err := TrackPID(townRoot, "gt-dead-session", 4194305); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0 (process should be dead)", killed)
	}
	if len(errs) != 0 {
		t.Errorf("errs = %v, want empty (dead process is not an error)", errs)
	}

	// PID file should be cleaned up
	path := pidFile(townRoot, "gt-dead-session")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should be cleaned up for dead process")
	}
}

func TestKillTrackedPIDs_CorruptFile(t *testing.T) {
	townRoot := t.TempDir()
	dir := pidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a corrupt PID file
	path := filepath.Join(dir, "gt-corrupt.pid")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatal(err)
	}

	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0", killed)
	}
	if len(errs) != 0 {
		t.Errorf("errs = %v, want empty (corrupt file should be silently removed)", errs)
	}

	// Corrupt file should be cleaned up
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("corrupt PID file should be removed")
	}
}

func TestKillTrackedPIDs_SkipsNonPidFiles(t *testing.T) {
	townRoot := t.TempDir()
	dir := pidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a non-.pid file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0", killed)
	}
	if len(errs) != 0 {
		t.Errorf("errs = %v, want empty", errs)
	}
}

func TestKillTrackedPIDs_KillsSelf(t *testing.T) {
	// Track our own PID — KillTrackedPIDs should find it alive.
	// We can't actually let it kill us, so just verify TrackPID + read round-trips.
	townRoot := t.TempDir()
	myPID := os.Getpid()

	if err := TrackPID(townRoot, "gt-self-test", myPID); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	// Verify the file contains our PID
	path := pidFile(townRoot, "gt-self-test")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}

	got, err := strconv.Atoi(string(data[:len(data)-1])) // trim newline
	if err != nil {
		t.Fatalf("parsing PID from file: %v", err)
	}
	if got != myPID {
		t.Errorf("PID = %d, want %d", got, myPID)
	}

	// Clean up without killing ourselves
	UntrackPID(townRoot, "gt-self-test")
}

func TestPidFile_Path(t *testing.T) {
	got := pidFile("/home/user/gt", "gt-myrig-witness")
	want := "/home/user/gt/.runtime/pids/gt-myrig-witness.pid"
	if got != want {
		t.Errorf("pidFile() = %q, want %q", got, want)
	}
}
