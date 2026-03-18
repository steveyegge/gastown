package checkpoint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("add", filename)
	run("commit", "-m", message)
}

func TestCountWIPCommits_None(t *testing.T) {
	dir := setupGitRepo(t)

	gitCommit(t, dir, "a.go", "package a\n", "feat: add feature A")
	gitCommit(t, dir, "b.go", "package b\n", "fix: fix bug B")

	count, err := CountWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 WIP commits, got %d", count)
	}
}

func TestCountWIPCommits_Mixed(t *testing.T) {
	dir := setupGitRepo(t)

	gitCommit(t, dir, "a.go", "package a\n", "feat: add feature A")
	gitCommit(t, dir, "b.go", "package b\n", WIPCommitPrefix)
	gitCommit(t, dir, "c.go", "package c\n", "fix: fix bug C")
	gitCommit(t, dir, "d.go", "package d\n", WIPCommitPrefix)

	count, err := CountWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 WIP commits, got %d", count)
	}
}

func TestSquashWIPCommits_NoWIP(t *testing.T) {
	dir := setupGitRepo(t)

	gitCommit(t, dir, "a.go", "package a\n", "feat: add feature A")

	count, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 squashed, got %d", count)
	}

	// Original commit should still be there.
	cmd := exec.Command("git", "log", "--format=%s", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	subjects := strings.TrimSpace(string(out))
	if subjects != "feat: add feature A" {
		t.Fatalf("expected original commit preserved, got: %q", subjects)
	}
}

func TestSquashWIPCommits_AllWIP(t *testing.T) {
	dir := setupGitRepo(t)

	gitCommit(t, dir, "a.go", "package a\n", WIPCommitPrefix)
	gitCommit(t, dir, "b.go", "package b\n", WIPCommitPrefix)

	count, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 squashed, got %d", count)
	}

	// Should have exactly one commit now.
	cmd := exec.Command("git", "rev-list", "--count", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "1" {
		t.Fatalf("expected 1 commit after squash, got: %s", out)
	}

	// Files should still exist.
	for _, f := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("file %s should exist after squash: %v", f, err)
		}
	}
}

func TestSquashWIPCommits_Mixed(t *testing.T) {
	dir := setupGitRepo(t)

	gitCommit(t, dir, "a.go", "package a\n", "feat: add feature A")
	gitCommit(t, dir, "b.go", "package b\n", WIPCommitPrefix)
	gitCommit(t, dir, "c.go", "package c\n", "fix: fix bug C")
	gitCommit(t, dir, "d.go", "package d\n", WIPCommitPrefix)

	count, err := SquashWIPCommits(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 squashed, got %d", count)
	}

	// Should have exactly one squashed commit.
	cmd := exec.Command("git", "rev-list", "--count", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "1" {
		t.Fatalf("expected 1 commit after squash, got: %s", out)
	}

	// Commit message should include the real commit subjects.
	cmd = exec.Command("git", "log", "--format=%B", "main..HEAD")
	cmd.Dir = dir
	out, _ = cmd.Output()
	msg := string(out)
	if !strings.Contains(msg, "feat: add feature A") {
		t.Fatalf("expected real commit message preserved, got: %s", msg)
	}
	if !strings.Contains(msg, "fix: fix bug C") {
		t.Fatalf("expected second real commit message preserved, got: %s", msg)
	}
	if strings.Contains(msg, WIPCommitPrefix) {
		t.Fatalf("WIP commit message should not appear in squashed commit")
	}

	// All files should exist.
	for _, f := range []string{"a.go", "b.go", "c.go", "d.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("file %s should exist after squash: %v", f, err)
		}
	}
}
