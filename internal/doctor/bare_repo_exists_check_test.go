package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
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

// initBareRepoWithRemote creates a real git bare repo with an origin remote.
// Returns the bare repo path.
func initBareRepoWithRemote(t *testing.T, rigDir, fetchURL string) string {
	t.Helper()
	bareRepo := filepath.Join(rigDir, ".repo.git")
	cmd := exec.Command("git", "init", "--bare", bareRepo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", bareRepo, "remote", "add", "origin", fetchURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v\n%s", err, out)
	}
	return bareRepo
}

// setupWorktreeRef creates a refinery/rig directory with a .git file pointing to .repo.git.
func setupWorktreeRef(t *testing.T, rigDir, bareRepo string) {
	t.Helper()
	refineryRig := filepath.Join(rigDir, "refinery", "rig")
	if err := os.MkdirAll(refineryRig, 0755); err != nil {
		t.Fatal(err)
	}
	worktreeDir := filepath.Join(bareRepo, "worktrees", "rig")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitContent := "gitdir: " + filepath.Join(bareRepo, "worktrees", "rig") + "\n"
	if err := os.WriteFile(filepath.Join(refineryRig, ".git"), []byte(gitContent), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeConfigJSON writes a config.json with optional push_url.
func writeConfigJSON(t *testing.T, rigDir, gitURL, pushURL string) {
	t.Helper()
	cfg := map[string]string{"git_url": gitURL}
	if pushURL != "" {
		cfg["push_url"] = pushURL
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBareRepoExistsCheck_PushURLMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	fetchURL := "https://github.com/example/repo.git"
	bareRepo := initBareRepoWithRemote(t, rigDir, fetchURL)

	// Set a push URL on the bare repo that differs from config
	cmd := exec.Command("git", "-C", bareRepo, "remote", "set-url", "--push", "origin", "https://github.com/user/wrong-fork.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url --push failed: %v\n%s", err, out)
	}

	// Config says the push URL should be something else
	writeConfigJSON(t, rigDir, fetchURL, "https://github.com/user/correct-fork.git")
	setupWorktreeRef(t, rigDir, bareRepo)

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for push URL mismatch, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "push URL") {
		t.Errorf("expected message about push URL, got %q", result.Message)
	}
}

func TestBareRepoExistsCheck_LegacyConfigIgnoresPushURL(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	fetchURL := "https://github.com/example/repo.git"
	bareRepo := initBareRepoWithRemote(t, rigDir, fetchURL)

	// Set a push URL on the bare repo (may be from a pre-push_url-feature setup)
	cmd := exec.Command("git", "-C", bareRepo, "remote", "set-url", "--push", "origin", "https://github.com/user/old-fork.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url --push failed: %v\n%s", err, out)
	}

	// Config has NO push_url — legacy config that predates the push_url feature.
	// Doctor should NOT flag a mismatch; the existing push URL may be intentional.
	writeConfigJSON(t, rigDir, fetchURL, "")
	setupWorktreeRef(t, rigDir, bareRepo)

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for legacy config (no push_url field), got %v: %s", result.Status, result.Message)
	}
}

func TestBareRepoExistsCheck_PushURLMatchesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	fetchURL := "https://github.com/example/repo.git"
	pushURL := "https://github.com/user/fork.git"
	bareRepo := initBareRepoWithRemote(t, rigDir, fetchURL)

	// Set push URL matching config
	cmd := exec.Command("git", "-C", bareRepo, "remote", "set-url", "--push", "origin", pushURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url --push failed: %v\n%s", err, out)
	}

	writeConfigJSON(t, rigDir, fetchURL, pushURL)
	setupWorktreeRef(t, rigDir, bareRepo)

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when push URL matches config, got %v: %s", result.Status, result.Message)
	}
}

func TestBareRepoExistsCheck_FixPushURLMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	fetchURL := "https://github.com/example/repo.git"
	correctPushURL := "https://github.com/user/correct-fork.git"
	bareRepo := initBareRepoWithRemote(t, rigDir, fetchURL)

	// Set wrong push URL
	cmd := exec.Command("git", "-C", bareRepo, "remote", "set-url", "--push", "origin", "https://github.com/user/wrong-fork.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url --push failed: %v\n%s", err, out)
	}

	writeConfigJSON(t, rigDir, fetchURL, correctPushURL)
	setupWorktreeRef(t, rigDir, bareRepo)

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Run should detect mismatch
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}

	// Fix should correct it
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Re-run should show OK
	check2 := NewBareRepoExistsCheck()
	result2 := check2.Run(ctx)
	if result2.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result2.Status, result2.Message)
	}
}

func TestBareRepoExistsCheck_FixPreservesLegacyPushURL(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	fetchURL := "https://github.com/example/repo.git"
	legacyPushURL := "https://github.com/user/old-fork.git"
	bareRepo := initBareRepoWithRemote(t, rigDir, fetchURL)

	// Set a push URL (from a pre-push_url-feature setup)
	cmd := exec.Command("git", "-C", bareRepo, "remote", "set-url", "--push", "origin", legacyPushURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url --push failed: %v\n%s", err, out)
	}

	// Config has no push_url — legacy config
	writeConfigJSON(t, rigDir, fetchURL, "")
	setupWorktreeRef(t, rigDir, bareRepo)

	check := NewBareRepoExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Run should NOT flag a mismatch for legacy configs
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK for legacy config, got %v: %s", result.Status, result.Message)
	}

	// Verify the push URL is preserved (not cleared)
	bareGit := git.NewGitWithDir(bareRepo, "")
	actualPush, err := bareGit.GetPushURL("origin")
	if err != nil {
		t.Fatalf("GetPushURL failed: %v", err)
	}
	if actualPush != legacyPushURL {
		t.Errorf("expected push URL to be preserved as %q, got %q", legacyPushURL, actualPush)
	}
}
