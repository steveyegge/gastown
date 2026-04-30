package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/dog"
)

// initVerifyRepo creates a temp git repo with an initial commit on main, plus
// an origin remote pointing at a bare repo. Returns the worktree path and the
// bare remote path.
func initVerifyRepo(t *testing.T) (worktree, remote string) {
	t.Helper()
	tmp := t.TempDir()
	worktree = filepath.Join(tmp, "wt")
	remote = filepath.Join(tmp, "remote.git")

	mustGit(t, "", "init", "--bare", remote)
	mustGit(t, "", "init", "--initial-branch=main", worktree)
	mustGit(t, worktree, "config", "user.email", "test@test.com")
	mustGit(t, worktree, "config", "user.name", "Test User")
	mustGit(t, worktree, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	mustGit(t, worktree, "add", ".")
	mustGit(t, worktree, "commit", "-m", "initial")
	mustGit(t, worktree, "remote", "add", "origin", remote)
	mustGit(t, worktree, "push", "-u", "origin", "main")
	return worktree, remote
}

// mustGit runs a git command and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// makeFeatureCommit creates a new branch, commits a file, and returns the branch name.
func makeFeatureCommit(t *testing.T, worktree, branch, filename string) {
	t.Helper()
	mustGit(t, worktree, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(worktree, filename), []byte("feature\n"), 0644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	mustGit(t, worktree, "add", ".")
	mustGit(t, worktree, "commit", "-m", "feature")
}

func TestVerifyDogWork_CleanAlreadyPushed(t *testing.T) {
	wt, _ := initVerifyRepo(t)
	makeFeatureCommit(t, wt, "feature/x", "a.txt")
	mustGit(t, wt, "push", "-u", "origin", "feature/x")

	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": wt}}
	reports, err := verifyDogWork(d, true)
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("want 1 report, got %d", len(reports))
	}
	r := reports[0]
	if r.Failure != "" {
		t.Errorf("unexpected Failure: %s", r.Failure)
	}
	if !r.Pushed {
		t.Errorf("expected Pushed=true")
	}
	if r.Branch != "feature/x" {
		t.Errorf("Branch = %q, want feature/x", r.Branch)
	}
}

func TestVerifyDogWork_AutoPushesUnpushedCommit(t *testing.T) {
	wt, _ := initVerifyRepo(t)
	makeFeatureCommit(t, wt, "feature/y", "b.txt")

	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": wt}}
	reports, err := verifyDogWork(d, true)
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	if !reports[0].Pushed {
		t.Errorf("expected auto-push to succeed")
	}

	// Confirm origin now has the commit.
	remoteSha, _ := gitOutput(wt, "ls-remote", "--heads", "origin", "feature/y")
	if strings.TrimSpace(strings.SplitN(remoteSha, "\t", 2)[0]) != reports[0].Commit {
		t.Errorf("origin does not have expected commit after auto-push")
	}
}

func TestVerifyDogWork_NoPushFailsIfUnpushed(t *testing.T) {
	wt, _ := initVerifyRepo(t)
	makeFeatureCommit(t, wt, "feature/z", "c.txt")

	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": wt}}
	reports, err := verifyDogWork(d, false)
	if err == nil {
		t.Fatal("expected failure when --no-push set on unpushed commit")
	}
	if !strings.Contains(reports[0].Failure, "not on origin") {
		t.Errorf("Failure = %q, want contains 'not on origin'", reports[0].Failure)
	}
}

func TestVerifyDogWork_RejectsProtectedBranch(t *testing.T) {
	wt, _ := initVerifyRepo(t)
	if err := os.WriteFile(filepath.Join(wt, "extra.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, wt, "add", ".")
	mustGit(t, wt, "commit", "-m", "on main")

	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": wt}}
	_, err := verifyDogWork(d, true)
	if err == nil {
		t.Fatal("expected verify to refuse commits on main")
	}
	if !strings.Contains(err.Error(), "protected branch") {
		t.Errorf("error = %v, want contains 'protected branch'", err)
	}
}

func TestVerifyDogWork_RejectsDirtyTree(t *testing.T) {
	wt, _ := initVerifyRepo(t)
	makeFeatureCommit(t, wt, "feature/dirty", "d.txt")
	if err := os.WriteFile(filepath.Join(wt, "uncommitted.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": wt}}
	_, err := verifyDogWork(d, true)
	if err == nil {
		t.Fatal("expected verify to refuse dirty working tree")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error = %v, want contains 'uncommitted'", err)
	}
}

func TestVerifyDogWork_SkipsMissingWorktree(t *testing.T) {
	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"ghost": "/nonexistent/path"}}
	reports, err := verifyDogWork(d, true)
	if err != nil {
		t.Fatalf("unexpected error for missing worktree: %v", err)
	}
	if reports[0].Skipped == "" {
		t.Errorf("expected Skipped to be set for missing worktree")
	}
}

func TestVerifyDogWork_SkipsNonGitDir(t *testing.T) {
	tmp := t.TempDir()
	d := &dog.Dog{Name: "alpha", Worktrees: map[string]string{"testrig": tmp}}
	reports, err := verifyDogWork(d, true)
	if err != nil {
		t.Fatalf("unexpected error for non-git dir: %v", err)
	}
	if reports[0].Skipped == "" {
		t.Errorf("expected Skipped to be set for non-git directory")
	}
}

func TestVerifyDogWork_MultipleWorktreesMixedResult(t *testing.T) {
	wt1, _ := initVerifyRepo(t)
	makeFeatureCommit(t, wt1, "feature/ok", "ok.txt")
	mustGit(t, wt1, "push", "-u", "origin", "feature/ok")

	wt2, _ := initVerifyRepo(t)
	if err := os.WriteFile(filepath.Join(wt2, "on-main.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, wt2, "add", ".")
	mustGit(t, wt2, "commit", "-m", "on main")

	d := &dog.Dog{
		Name: "alpha",
		Worktrees: map[string]string{
			"good": wt1,
			"bad":  wt2,
		},
	}
	reports, err := verifyDogWork(d, true)
	if err == nil {
		t.Fatal("expected overall failure because one worktree is on main")
	}
	if len(reports) != 2 {
		t.Fatalf("want 2 reports, got %d", len(reports))
	}
	// Exactly one should have Failure and one should be clean.
	var failures, ok int
	for _, r := range reports {
		if r.Failure != "" {
			failures++
		} else if r.Pushed {
			ok++
		}
	}
	if failures != 1 || ok != 1 {
		t.Errorf("want 1 failure + 1 ok, got failures=%d ok=%d", failures, ok)
	}
}
