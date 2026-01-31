package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPreCommitHook(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a minimal git structure
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("creating .git dir: %v", err)
	}

	// Install the hook
	if err := InstallPreCommitHook(tempDir); err != nil {
		t.Fatalf("InstallPreCommitHook: %v", err)
	}

	// Verify hook was created
	hookPath := filepath.Join(tempDir, ".git", "hooks", "pre-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}

	// Verify it contains expected content
	if !strings.Contains(string(content), "Gas Town pre-commit hook") {
		t.Error("hook missing identifier comment")
	}
	if !strings.Contains(string(content), ".repo.git") {
		t.Error("hook missing .repo.git protection")
	}
	if !strings.Contains(string(content), "gastown/") {
		t.Error("hook missing rig path protection")
	}

	// Verify it's executable
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("hook is not executable")
	}
}

func TestInstallPreCommitHook_Idempotent(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a minimal git structure
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("creating .git dir: %v", err)
	}

	// Install twice - should not error
	if err := InstallPreCommitHook(tempDir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := InstallPreCommitHook(tempDir); err != nil {
		t.Fatalf("second install: %v", err)
	}

	// Verify content is still correct
	hookPath := filepath.Join(tempDir, ".git", "hooks", "pre-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}

	if !strings.Contains(string(content), "Gas Town pre-commit hook") {
		t.Error("hook content corrupted after second install")
	}
}

func TestInstallPreCommitHook_NoOverwriteForeign(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a minimal git structure with hooks directory
	hooksDir := filepath.Join(tempDir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("creating hooks dir: %v", err)
	}

	// Create an existing foreign hook
	hookPath := filepath.Join(hooksDir, "pre-commit")
	foreignContent := "#!/bin/bash\n# Foreign hook content\necho 'foreign'\n"
	if err := os.WriteFile(hookPath, []byte(foreignContent), 0755); err != nil {
		t.Fatalf("writing foreign hook: %v", err)
	}

	// Install should not overwrite
	if err := InstallPreCommitHook(tempDir); err != nil {
		t.Fatalf("InstallPreCommitHook: %v", err)
	}

	// Verify foreign content is preserved
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}

	if !strings.Contains(string(content), "Foreign hook content") {
		t.Error("foreign hook was overwritten")
	}
}

func TestIsPreCommitHookInstalled(t *testing.T) {
	tempDir := t.TempDir()

	// No .git directory
	if IsPreCommitHookInstalled(tempDir) {
		t.Error("expected false when no .git exists")
	}

	// Create .git but no hook
	gitDir := filepath.Join(tempDir, ".git", "hooks")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("creating hooks dir: %v", err)
	}
	if IsPreCommitHookInstalled(tempDir) {
		t.Error("expected false when no hook exists")
	}

	// Install hook
	if err := InstallPreCommitHook(tempDir); err != nil {
		t.Fatalf("InstallPreCommitHook: %v", err)
	}
	if !IsPreCommitHookInstalled(tempDir) {
		t.Error("expected true after installation")
	}
}

func TestHQGitignore_ContainsRepoGit(t *testing.T) {
	if !strings.Contains(HQGitignore, ".repo.git") {
		t.Error("HQGitignore missing .repo.git pattern")
	}
	if !strings.Contains(HQGitignore, "**/.repo.git/") {
		t.Error("HQGitignore should have **/.repo.git/ pattern to match at any depth")
	}
}
