package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
)

func TestCleanupStalePolecatsForSlingRemovesClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	rigPath := t.TempDir()
	clonePath := filepath.Join(rigPath, "polecats", "Toast", "demo")
	if err := os.MkdirAll(clonePath, 0755); err != nil {
		t.Fatalf("mkdir clone path: %v", err)
	}
	if err := initGitRepo(clonePath); err != nil {
		t.Fatalf("init git repo: %v", err)
	}

	r := &rig.Rig{Name: "demo", Path: rigPath}
	mgr := polecat.NewManager(r, git.NewGit(rigPath), nil)

	cleanupStalePolecatsForSling(mgr, r)

	if _, err := os.Stat(filepath.Join(rigPath, "polecats", "Toast")); !os.IsNotExist(err) {
		t.Fatalf("expected polecat to be removed, stat err: %v", err)
	}
}

func TestCleanupStalePolecatsForSlingSkipsDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	rigPath := t.TempDir()
	clonePath := filepath.Join(rigPath, "polecats", "Toast", "demo")
	if err := os.MkdirAll(clonePath, 0755); err != nil {
		t.Fatalf("mkdir clone path: %v", err)
	}
	if err := initGitRepo(clonePath); err != nil {
		t.Fatalf("init git repo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(clonePath, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	r := &rig.Rig{Name: "demo", Path: rigPath}
	mgr := polecat.NewManager(r, git.NewGit(rigPath), nil)

	cleanupStalePolecatsForSling(mgr, r)

	if _, err := os.Stat(filepath.Join(rigPath, "polecats", "Toast")); err != nil {
		t.Fatalf("expected polecat to remain, stat err: %v", err)
	}
}
