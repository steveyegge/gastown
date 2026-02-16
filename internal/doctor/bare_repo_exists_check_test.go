package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBareRepoExistsCheck_Name(t *testing.T) {
	check := NewBareRepoExistsCheck()
	if check.Name() != "bare-repo-exists" {
		t.Errorf("expected name 'bare-repo-exists', got %q", check.Name())
	}
	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBareRepoExistsCheck_NoRig(t *testing.T) {
	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: t.TempDir(), RigName: ""}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rig specified, got %v", result.Status)
	}
}

func TestBareRepoExistsCheck_BareRepoExists(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create .repo.git directory (bare repo)
	bareRepo := filepath.Join(rigDir, ".repo.git")
	if err := os.MkdirAll(bareRepo, 0755); err != nil {
		t.Fatal(err)
	}

	// Create refinery/rig with a .git file pointing to .repo.git
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .repo.git/worktrees/rig directory (so the target exists)
	worktreeDir := filepath.Join(bareRepo, "worktrees", "rig")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatal(err)
	}

	gitContent := "gitdir: " + filepath.Join(bareRepo, "worktrees", "rig") + "\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when .repo.git exists, got %v: %s", result.Status, result.Message)
	}
}

func TestBareRepoExistsCheck_NoBareRepoNoWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create refinery/rig with a .git directory (not a worktree)
	refineryRig := filepath.Join(rigDir, "refinery", "rig", ".git")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no worktrees depend on .repo.git, got %v", result.Status)
	}
}

func TestBareRepoExistsCheck_MissingBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create refinery/rig with a .git file pointing to missing .repo.git
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}

	gitContent := "gitdir: " + filepath.Join(rigDir, ".repo.git", "worktrees", "rig") + "\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError when .repo.git is missing, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "missing .repo.git") {
		t.Errorf("expected message about missing .repo.git, got %q", result.Message)
	}
	if len(result.Details) < 2 {
		t.Errorf("expected at least 2 details (bare repo path + worktree), got %d", len(result.Details))
	}
}

func TestBareRepoExistsCheck_MultipleWorktreesMissing(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	bareRepoTarget := filepath.Join(rigDir, ".repo.git")

	// Create refinery/rig worktree
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}
	gitContent := "gitdir: " + filepath.Join(bareRepoTarget, "worktrees", "rig") + "\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create polecat worktree
	polecatDir := filepath.Join(rigDir, "polecats", "worker1", rigName)
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}
	polecatGit := "gitdir: " + filepath.Join(bareRepoTarget, "worktrees", "worker1") + "\n"
	if err := os.WriteFile(filepath.Join(polecatDir, ".git"), []byte(polecatGit), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "2 worktree") {
		t.Errorf("expected message about 2 worktrees, got %q", result.Message)
	}
}

func TestBareRepoExistsCheck_RelativeGitdir(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create refinery/rig with a relative .git reference
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}

	// Relative path from refinery/rig/ to .repo.git/worktrees/rig
	gitContent := "gitdir: ../../.repo.git/worktrees/rig\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError for broken relative gitdir, got %v", result.Status)
	}
}

func TestBareRepoExistsCheck_NonRepoGitWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create refinery/rig with a .git file pointing to something other than .repo.git
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}

	gitContent := "gitdir: /some/other/path/worktrees/rig\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Worktree doesn't reference .repo.git, so this should pass
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when worktrees don't reference .repo.git, got %v", result.Status)
	}
}
