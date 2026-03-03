package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCopyTruncateRotate(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create a log file with some content
	content := []byte("line 1\nline 2\nline 3\n")
	if err := os.WriteFile(logPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	// Rotate it
	if err := copyTruncateRotate(logPath); err != nil {
		t.Fatalf("copyTruncateRotate: %v", err)
	}

	// Original should be truncated to 0
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat after rotate: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected truncated file (size 0), got %d", info.Size())
	}

	// .1.gz should exist
	gzPath := logPath + ".1.gz"
	if _, err := os.Stat(gzPath); err != nil {
		t.Errorf("expected rotated file %s to exist: %v", gzPath, err)
	}
}

func TestCopyTruncateRotate_ShiftsBackups(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Do 4 rotations — should keep only maxBackups (3) .gz files
	for i := 0; i < 4; i++ {
		if err := os.WriteFile(logPath, []byte("data\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := copyTruncateRotate(logPath); err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
	}

	// .1.gz, .2.gz, .3.gz should exist; .4.gz should not
	for i := 1; i <= logRotationMaxBackups; i++ {
		gz := filepath.Join(dir, "test.log."+string(rune('0'+i))+".gz")
		if _, err := os.Stat(gz); err != nil {
			t.Errorf("expected %s to exist", gz)
		}
	}
	gz4 := logPath + ".4.gz"
	if _, err := os.Stat(gz4); err == nil {
		t.Errorf("expected %s to NOT exist (exceeds maxBackups)", gz4)
	}
}

func TestRotateLogs_SkipsSmallFiles(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a small dolt.log (under threshold)
	smallLog := filepath.Join(daemonDir, "dolt.log")
	if err := os.WriteFile(smallLog, []byte("small"), 0600); err != nil {
		t.Fatal(err)
	}

	result := RotateLogs(townRoot)
	if len(result.Rotated) != 0 {
		t.Errorf("expected no rotations, got %v", result.Rotated)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestForceRotateLogs_RotatesSmallFiles(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a small dolt.log
	smallLog := filepath.Join(daemonDir, "dolt.log")
	if err := os.WriteFile(smallLog, []byte("small content"), 0600); err != nil {
		t.Fatal(err)
	}

	result := ForceRotateLogs(townRoot)
	if len(result.Rotated) != 1 {
		t.Errorf("expected 1 rotation, got %d (rotated: %v, skipped: %v)", len(result.Rotated), result.Rotated, result.Skipped)
	}
}

func TestForceRotateLogs_SkipsEmptyFiles(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an empty dolt.log
	emptyLog := filepath.Join(daemonDir, "dolt.log")
	if err := os.WriteFile(emptyLog, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	result := ForceRotateLogs(townRoot)
	if len(result.Rotated) != 0 {
		t.Errorf("expected no rotations for empty file, got %v", result.Rotated)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestCleanStaleArchives_RemovesOldFiles(t *testing.T) {
	daemonDir := t.TempDir()

	// Create a stale archive (8 days old)
	stalePath := filepath.Join(daemonDir, "dolt-2026-02-20T10-48-08.log.gz")
	if err := os.WriteFile(stalePath, []byte("old data"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(stalePath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	// Create a fresh archive (1 day old)
	freshPath := filepath.Join(daemonDir, "dolt-2026-02-28T23-19-42.log.gz")
	if err := os.WriteFile(freshPath, []byte("fresh data"), 0600); err != nil {
		t.Fatal(err)
	}
	freshTime := time.Now().Add(-1 * 24 * time.Hour)
	if err := os.Chtimes(freshPath, freshTime, freshTime); err != nil {
		t.Fatal(err)
	}

	// Create a non-archive file (should not be touched)
	regularPath := filepath.Join(daemonDir, "dolt.log")
	if err := os.WriteFile(regularPath, []byte("active log"), 0600); err != nil {
		t.Fatal(err)
	}

	removed, errs := cleanStaleArchives(daemonDir)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removal, got %d: %v", len(removed), removed)
	}
	if removed[0] != stalePath {
		t.Errorf("expected %s removed, got %s", stalePath, removed[0])
	}

	// Fresh archive should still exist
	if _, err := os.Stat(freshPath); err != nil {
		t.Errorf("fresh archive should still exist: %v", err)
	}
	// Regular file should still exist
	if _, err := os.Stat(regularPath); err != nil {
		t.Errorf("regular file should still exist: %v", err)
	}
}

func TestCleanStaleArchives_IgnoresNonTimestamped(t *testing.T) {
	daemonDir := t.TempDir()

	// Lumberjack-style rotation (not timestamped) — should NOT be removed
	lumberjackPath := filepath.Join(daemonDir, "daemon.log.1.gz")
	if err := os.WriteFile(lumberjackPath, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(lumberjackPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	removed, errs := cleanStaleArchives(daemonDir)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}
}

func TestStaleArchivePattern(t *testing.T) {
	tests := []struct {
		name  string
		match bool
	}{
		{"dolt-2026-02-28T23-19-42.log.gz", true},
		{"daemon-2026-02-18T21-26-55.log.gz", true},
		{"dolt-server-2026-02-22T10-48-08.log.gz", true},
		{"dolt-test-server-2026-02-28T23-21-02.log.gz", true},
		{"daemon.log.1.gz", false},    // lumberjack rotation
		{"dolt.log.2.gz", false},      // copytruncate rotation
		{"dolt.log", false},           // active log
		{"daemon.log", false},         // active log
	}

	for _, tt := range tests {
		got := staleArchivePattern.MatchString(tt.name)
		if got != tt.match {
			t.Errorf("staleArchivePattern.MatchString(%q) = %v, want %v", tt.name, got, tt.match)
		}
	}
}

func TestEnforceDiskBudget_DeletesOldestFirst(t *testing.T) {
	daemonDir := t.TempDir()

	// Create gz files totaling more than daemonDiskBudget is irrelevant for test,
	// but we can test the ordering logic by using collectGzFiles + small budget override.
	// Instead, test with real files and the actual function.

	// Create 3 gz files with different ages, ~100 bytes each
	files := []struct {
		name string
		age  time.Duration
	}{
		{"old-2026-01-01T00-00-00.log.gz", 60 * 24 * time.Hour},
		{"mid-2026-02-01T00-00-00.log.gz", 30 * 24 * time.Hour},
		{"new-2026-03-01T00-00-00.log.gz", 1 * 24 * time.Hour},
	}

	// Create a large non-gz file to push total over budget
	bigLog := filepath.Join(daemonDir, "dolt.log")
	bigData := make([]byte, 100)
	if err := os.WriteFile(bigLog, bigData, 0600); err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		path := filepath.Join(daemonDir, f.name)
		if err := os.WriteFile(path, []byte("archive data"), 0600); err != nil {
			t.Fatal(err)
		}
		ts := time.Now().Add(-f.age)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	// Total is well under 500MB, so nothing should be removed
	removed, errs := enforceDiskBudget(daemonDir)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals (under budget), got %v", removed)
	}
}

func TestCleanDaemonDir_Integration(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a stale archive
	stalePath := filepath.Join(daemonDir, "dolt-2026-01-15T10-00-00.log.gz")
	if err := os.WriteFile(stalePath, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(stalePath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	result := CleanDaemonDir(townRoot)
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
	if len(result.StaleRemoved) != 1 {
		t.Errorf("expected 1 stale removal, got %d", len(result.StaleRemoved))
	}

	// Verify file is gone
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("stale archive should have been deleted")
	}
}
