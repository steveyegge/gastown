package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWLCommonsFork_InTownRoot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	forkDir := filepath.Join(tmpDir, "wl-commons")
	doltDir := filepath.Join(forkDir, ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	got := findWLCommonsFork(tmpDir)
	if got != forkDir {
		t.Errorf("findWLCommonsFork() = %q, want %q", got, forkDir)
	}
}

func TestFindWLCommonsFork_InParent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create wl-commons as sibling
	parentDir := tmpDir
	forkDir := filepath.Join(parentDir, "wl-commons")
	doltDir := filepath.Join(forkDir, ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Town root is a subdirectory
	townRoot := filepath.Join(tmpDir, "mytown")
	if err := os.MkdirAll(townRoot, 0755); err != nil {
		t.Fatal(err)
	}

	got := findWLCommonsFork(townRoot)
	if got != forkDir {
		t.Errorf("findWLCommonsFork() = %q, want %q", got, forkDir)
	}
}

func TestFindWLCommonsFork_NotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	got := findWLCommonsFork(tmpDir)
	if got != "" {
		t.Errorf("findWLCommonsFork() = %q, want empty", got)
	}
}

func TestFindWLCommonsFork_NoDoltDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// Directory exists but has no .dolt subdirectory
	forkDir := filepath.Join(tmpDir, "wl-commons")
	if err := os.MkdirAll(forkDir, 0755); err != nil {
		t.Fatal(err)
	}

	got := findWLCommonsFork(tmpDir)
	if got != "" {
		t.Errorf("findWLCommonsFork() = %q, want empty (no .dolt dir)", got)
	}
}
