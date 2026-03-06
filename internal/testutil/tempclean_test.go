package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanStaleTempDirs_RemovesStalePatterns(t *testing.T) {
	// Create a temp dir to act as our "system temp".
	sysTmp := t.TempDir()
	t.Setenv("TMPDIR", sysTmp)

	// Create a stale directory matching a known pattern.
	staleDir := filepath.Join(sysTmp, "gt-clone-abc123")
	if err := os.MkdirAll(staleDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Backdate modification time to make it stale.
	staleTime := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(staleDir, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	// Create a fresh directory matching the same pattern (should NOT be removed).
	freshDir := filepath.Join(sysTmp, "gt-clone-fresh999")
	if err := os.MkdirAll(freshDir, 0755); err != nil {
		t.Fatal(err)
	}

	CleanStaleTempDirs()

	// Stale dir should be gone.
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("stale dir %s should have been removed", staleDir)
	}

	// Fresh dir should still exist.
	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("fresh dir %s should still exist: %v", freshDir, err)
	}
}

func TestCleanStaleTempDirs_RemovesStaleFiles(t *testing.T) {
	sysTmp := t.TempDir()
	t.Setenv("TMPDIR", sysTmp)

	// Create a stale cached binary.
	staleBinary := filepath.Join(sysTmp, "gt-integration-test")
	if err := os.WriteFile(staleBinary, []byte("fake binary"), 0755); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(staleBinary, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	CleanStaleTempDirs()

	if _, err := os.Stat(staleBinary); !os.IsNotExist(err) {
		t.Errorf("stale binary %s should have been removed", staleBinary)
	}
}

func TestCleanStaleTempDirs_PreservesFreshArtifacts(t *testing.T) {
	sysTmp := t.TempDir()
	t.Setenv("TMPDIR", sysTmp)

	// Create a fresh directory matching a known pattern.
	freshDir := filepath.Join(sysTmp, "namepool-test-abc")
	if err := os.MkdirAll(freshDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fresh file matching a known name.
	freshBinary := filepath.Join(sysTmp, "gt-integration-test")
	if err := os.WriteFile(freshBinary, []byte("fresh binary"), 0755); err != nil {
		t.Fatal(err)
	}

	CleanStaleTempDirs()

	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("fresh dir %s should still exist: %v", freshDir, err)
	}
	if _, err := os.Stat(freshBinary); err != nil {
		t.Errorf("fresh binary %s should still exist: %v", freshBinary, err)
	}
}

func TestCleanStaleTempDirs_HandlesEmptyTempDir(t *testing.T) {
	sysTmp := t.TempDir()
	t.Setenv("TMPDIR", sysTmp)

	// Should not panic on empty temp dir.
	CleanStaleTempDirs()
}
