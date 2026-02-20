package session

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTrackPID_WritesFile(t *testing.T) {
	townRoot := t.TempDir()
	originalStartFn := pidStartTimeFunc
	t.Cleanup(func() { pidStartTimeFunc = originalStartFn })
	pidStartTimeFunc = func(pid int) (string, error) {
		if pid != 12345 {
			t.Fatalf("unexpected PID: %d", pid)
		}
		return "Mon Jan  1 00:00:00 2026", nil
	}

	if err := TrackPID(townRoot, "gt-myrig-witness", 12345); err != nil {
		t.Fatalf("TrackPID() error = %v", err)
	}

	path := pidFile(townRoot, "gt-myrig-witness")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}

	if got := string(data); got != "12345|Mon Jan  1 00:00:00 2026\n" {
		t.Errorf("PID file content = %q, want start-time tracked format", got)
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

	record, err := parseTrackedPID(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parseTrackedPID() error = %v", err)
	}
	if record.PID != myPID {
		t.Errorf("PID = %d, want %d", record.PID, myPID)
	}

	// Clean up without killing ourselves
	UntrackPID(townRoot, "gt-self-test")
}

func TestKillTrackedPIDs_SkipsPidReuse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Signal(0) liveness check not supported on Windows")
	}
	townRoot := t.TempDir()
	dir := pidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use our own PID so Signal(0) succeeds — this guarantees the test
	// reaches the start-time comparison branch rather than exiting early
	// at the liveness check.
	myPID := os.Getpid()
	path := filepath.Join(dir, "gt-reused.pid")
	record := fmt.Sprintf("%d|old-start\n", myPID)
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	originalStartFn := pidStartTimeFunc
	t.Cleanup(func() { pidStartTimeFunc = originalStartFn })
	startTimeCalled := false
	pidStartTimeFunc = func(pid int) (string, error) {
		if pid == myPID {
			startTimeCalled = true
			return "new-start", nil
		}
		return "", os.ErrNotExist
	}

	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0 (pid reuse should be skipped)", killed)
	}
	if len(errs) != 0 {
		t.Errorf("errs = %v, want empty", errs)
	}
	if !startTimeCalled {
		t.Error("pidStartTimeFunc was not invoked — reuse guard not exercised")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should be removed when pid reuse is detected")
	}
}

func TestKillTrackedPIDs_PreservesFileOnLookupError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Signal(0) liveness check not supported on Windows")
	}
	townRoot := t.TempDir()
	dir := pidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use our own PID so Signal(0) succeeds and we reach the start-time check.
	myPID := os.Getpid()
	path := filepath.Join(dir, "gt-err-lookup.pid")
	record := fmt.Sprintf("%d|some-start-time\n", myPID)
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	originalStartFn := pidStartTimeFunc
	t.Cleanup(func() { pidStartTimeFunc = originalStartFn })
	pidStartTimeFunc = func(pid int) (string, error) {
		return "", fmt.Errorf("ps not available")
	}

	killed, errs := KillTrackedPIDs(townRoot)
	if killed != 0 {
		t.Errorf("killed = %d, want 0 (lookup error should skip kill)", killed)
	}
	// Should report the error via errSessions
	if len(errs) != 1 {
		t.Errorf("errs = %v, want 1 entry for lookup error", errs)
	}
	// PID file must be preserved for future retry
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("PID file should be preserved when start-time lookup fails")
	}
}

func TestPidFile_Path(t *testing.T) {
	got := pidFile("/home/user/gt", "gt-myrig-witness")
	want := filepath.Join("/home/user/gt", ".runtime", "pids", "gt-myrig-witness.pid")
	if got != want {
		t.Errorf("pidFile() = %q, want %q", got, want)
	}
}
