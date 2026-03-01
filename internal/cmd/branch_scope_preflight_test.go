package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
)

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func initScopeTestRepo(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	local := filepath.Join(tmp, "local")

	runGitCmd(t, tmp, "init", "--bare", remote)
	runGitCmd(t, tmp, "init", local)
	runGitCmd(t, local, "config", "user.email", "test@test.com")
	runGitCmd(t, local, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitCmd(t, local, "add", ".")
	runGitCmd(t, local, "commit", "-m", "initial")
	runGitCmd(t, local, "remote", "add", "origin", remote)

	mainBranchCmd := exec.Command("git", "branch", "--show-current")
	mainBranchCmd.Dir = local
	out, err := mainBranchCmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	mainBranch := strings.TrimSpace(string(out))
	runGitCmd(t, local, "push", "-u", "origin", mainBranch)
	return local, mainBranch
}

func TestRunBranchScopePreflight_PassesWhenChangedFilesAreInScope(t *testing.T) {
	local, mainBranch := initScopeTestRepo(t)
	g := git.NewGit(local)

	runGitCmd(t, local, "checkout", "-b", "feature/in-scope")
	if err := os.MkdirAll(filepath.Join(local, "src"), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(local, "src", "worker.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write src file: %v", err)
	}
	runGitCmd(t, local, "add", ".")
	runGitCmd(t, local, "commit", "-m", "in scope change")

	t.Setenv(branchScopeEnvVar, "src")
	if err := runBranchScopePreflight(g, "origin/"+mainBranch); err != nil {
		t.Fatalf("runBranchScopePreflight() expected success, got: %v", err)
	}
}

func TestRunBranchScopePreflight_FailsWhenChangedFilesAreOutOfScope(t *testing.T) {
	local, mainBranch := initScopeTestRepo(t)
	g := git.NewGit(local)

	runGitCmd(t, local, "checkout", "-b", "feature/out-of-scope")
	if err := os.MkdirAll(filepath.Join(local, "src"), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(local, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(local, "src", "worker.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write src file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(local, "docs", "note.md"), []byte("out of scope\n"), 0644); err != nil {
		t.Fatalf("write docs file: %v", err)
	}
	runGitCmd(t, local, "add", ".")
	runGitCmd(t, local, "commit", "-m", "mixed scope change")

	t.Setenv(branchScopeEnvVar, "src")
	err := runBranchScopePreflight(g, "origin/"+mainBranch)
	if err == nil {
		t.Fatal("runBranchScopePreflight() expected contamination error, got nil")
	}
	if !strings.Contains(err.Error(), "branch scope preflight failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "docs/note.md") {
		t.Fatalf("expected out-of-scope file in diagnostics, got: %v", err)
	}
}
