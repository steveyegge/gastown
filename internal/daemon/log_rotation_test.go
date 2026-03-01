package daemon

import (
	"os"
	"path/filepath"
	"testing"
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
