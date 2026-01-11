package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTownGitCheck(t *testing.T) {
	check := NewTownGitCheck()

	if check.Name() != "town-git" {
		t.Errorf("expected name 'town-git', got %q", check.Name())
	}

	if check.Description() == "" {
		t.Error("expected non-empty description")
	}

	if check.CanFix() {
		t.Error("expected CanFix() to return false")
	}
}

func TestTownGitCheck_NoGitDir(t *testing.T) {
	// Create temp directory without .git
	tmpDir, err := os.MkdirTemp("", "town-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewTownGitCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}

	if result.FixHint == "" {
		t.Error("expected non-empty FixHint")
	}
}

func TestTownGitCheck_WithGitDir(t *testing.T) {
	// Create temp directory with .git
	tmpDir, err := os.MkdirTemp("", "town-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewTownGitCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
}

func TestTownGitCheck_GitIsFile(t *testing.T) {
	// Create temp directory with .git as a file (worktree case)
	tmpDir, err := os.MkdirTemp("", "town-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitFile := filepath.Join(tmpDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/path"), 0644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}

	ctx := &CheckContext{TownRoot: tmpDir}
	check := NewTownGitCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for .git file, got %v", result.Status)
	}
}
