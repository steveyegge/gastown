package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFindStrandedWorktrees_EmptyPolecatsDir(t *testing.T) {
	// Create a temp rig with empty polecats directory
	rigPath := t.TempDir()
	polecatsDir := filepath.Join(rigPath, "polecats")
	if err := os.MkdirAll(polecatsDir, 0755); err != nil {
		t.Fatalf("mkdir polecats: %v", err)
	}

	stranded, err := findStrandedWorktrees(rigPath)
	if err != nil {
		t.Fatalf("findStrandedWorktrees: %v", err)
	}
	if len(stranded) != 0 {
		t.Errorf("expected 0 stranded worktrees, got %d", len(stranded))
	}
}

func TestFindStrandedWorktrees_NoPolecatsDir(t *testing.T) {
	// Create a temp rig without polecats directory
	rigPath := t.TempDir()

	stranded, err := findStrandedWorktrees(rigPath)
	if err != nil {
		t.Fatalf("findStrandedWorktrees: %v", err)
	}
	if len(stranded) != 0 {
		t.Errorf("expected 0 stranded worktrees, got %d", len(stranded))
	}
}

func TestFindStrandedWorktrees_NonGitDirectory(t *testing.T) {
	// Create a temp rig with a polecat directory that's not a git repo
	rigPath := t.TempDir()
	polecatDir := filepath.Join(rigPath, "polecats", "toast")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecat: %v", err)
	}

	stranded, err := findStrandedWorktrees(rigPath)
	if err != nil {
		t.Fatalf("findStrandedWorktrees: %v", err)
	}
	if len(stranded) != 0 {
		t.Errorf("expected 0 stranded worktrees (non-git dir should be skipped), got %d", len(stranded))
	}
}

func TestFindStrandedWorktrees_WithStrandedCommits(t *testing.T) {
	// Skip if git not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a mock rig structure with a polecat worktree that has stranded commits
	rigPath := t.TempDir()
	rigName := filepath.Base(rigPath)

	// Create origin repo (bare) with main as default branch
	originPath := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", "--initial-branch=main", originPath)

	// Create initial commit in a temp clone, then push to origin
	tempClone := filepath.Join(t.TempDir(), "temp")
	runGit(t, "", "clone", originPath, tempClone)
	runGit(t, tempClone, "config", "user.email", "test@test.com")
	runGit(t, tempClone, "config", "user.name", "Test")
	runGit(t, tempClone, "checkout", "-b", "main") // Ensure we're on main
	writeFile(t, filepath.Join(tempClone, "README.md"), "# Test")
	runGit(t, tempClone, "add", "README.md")
	runGit(t, tempClone, "commit", "-m", "Initial commit")
	runGit(t, tempClone, "push", "-u", "origin", "main")

	// Create polecat worktree (new structure: polecats/<name>/<rigname>/)
	polecatDir := filepath.Join(rigPath, "polecats", "toast")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecat: %v", err)
	}
	clonePath := filepath.Join(polecatDir, rigName)

	// Clone the origin
	runGit(t, "", "clone", originPath, clonePath)
	runGit(t, clonePath, "config", "user.email", "test@test.com")
	runGit(t, clonePath, "config", "user.name", "Test")

	// Create a local commit that's not pushed (stranded work)
	writeFile(t, filepath.Join(clonePath, "stranded.txt"), "stranded work")
	runGit(t, clonePath, "add", "stranded.txt")
	runGit(t, clonePath, "commit", "-m", "Stranded commit")

	// Now find stranded worktrees
	stranded, err := findStrandedWorktrees(rigPath)
	if err != nil {
		t.Fatalf("findStrandedWorktrees: %v", err)
	}
	if len(stranded) != 1 {
		t.Errorf("expected 1 stranded worktree, got %d", len(stranded))
	}
	if len(stranded) > 0 {
		if stranded[0].Name != "toast" {
			t.Errorf("expected polecat name 'toast', got %q", stranded[0].Name)
		}
		if stranded[0].Commits != 1 {
			t.Errorf("expected 1 stranded commit, got %d", stranded[0].Commits)
		}
	}
}

func TestFindStrandedWorktrees_NoStrandedCommits(t *testing.T) {
	// Skip if git not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a mock rig with a polecat that has no stranded commits
	rigPath := t.TempDir()
	rigName := filepath.Base(rigPath)

	// Create origin repo (bare) with main as default branch
	originPath := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", "--initial-branch=main", originPath)

	// Create initial commit
	tempClone := filepath.Join(t.TempDir(), "temp")
	runGit(t, "", "clone", originPath, tempClone)
	runGit(t, tempClone, "config", "user.email", "test@test.com")
	runGit(t, tempClone, "config", "user.name", "Test")
	runGit(t, tempClone, "checkout", "-b", "main") // Ensure we're on main
	writeFile(t, filepath.Join(tempClone, "README.md"), "# Test")
	runGit(t, tempClone, "add", "README.md")
	runGit(t, tempClone, "commit", "-m", "Initial commit")
	runGit(t, tempClone, "push", "-u", "origin", "main")

	// Create polecat worktree
	polecatDir := filepath.Join(rigPath, "polecats", "toast")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecat: %v", err)
	}
	clonePath := filepath.Join(polecatDir, rigName)

	// Clone the origin (HEAD matches origin/main - no stranded commits)
	runGit(t, "", "clone", originPath, clonePath)

	// Find stranded worktrees - should be empty since HEAD == origin/main
	stranded, err := findStrandedWorktrees(rigPath)
	if err != nil {
		t.Fatalf("findStrandedWorktrees: %v", err)
	}
	if len(stranded) != 0 {
		t.Errorf("expected 0 stranded worktrees (no commits ahead), got %d", len(stranded))
	}
}

// Helper functions

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
