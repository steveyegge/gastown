package checkpoint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

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

	run("init")
	run("checkout", "-b", "main")

	// Create initial commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	// Create a feature branch.
	run("checkout", "-b", "polecat/test")

	return dir
}

func TestDoCheckpoint_NoChanges(t *testing.T) {
	dir := setupGitRepo(t)

	err := doCheckpoint(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not have created any new commits.
	cmd := exec.Command("git", "log", "--oneline", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected no commits, got: %s", out)
	}
}

func TestDoCheckpoint_WithChanges(t *testing.T) {
	dir := setupGitRepo(t)

	// Create a new file.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := doCheckpoint(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created a WIP commit.
	cmd := exec.Command("git", "log", "--format=%s", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if !strings.Contains(string(out), WIPCommitPrefix) {
		t.Fatalf("expected WIP commit, got: %s", out)
	}
}

func TestDoCheckpoint_SkipsRuntimeDirs(t *testing.T) {
	dir := setupGitRepo(t)

	// Create changes only in runtime dirs.
	runtimeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	err := doCheckpoint(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have created any commits (only runtime dir changes).
	cmd := exec.Command("git", "log", "--oneline", "main..HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected no commits for runtime-only changes, got: %s", out)
	}
}

func TestRunWatchdog_ContextCancellation(t *testing.T) {
	dir := setupGitRepo(t)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunWatchdog(ctx, WatchdogConfig{
			WorkDir:  dir,
			Interval: 100 * time.Millisecond,
		})
	}()

	// Cancel after a short delay.
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err != nil {
		t.Fatalf("expected nil error on context cancellation, got: %v", err)
	}
}

func TestHasSignificantChanges(t *testing.T) {
	dir := setupGitRepo(t)

	// No changes initially.
	has, err := hasSignificantChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected no significant changes")
	}

	// Add a real file.
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	has, err = hasSignificantChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected significant changes")
	}
}
