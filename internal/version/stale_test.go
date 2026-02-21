package version

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestShortCommit(t *testing.T) {
	tests := []struct {
		name   string
		hash   string
		expect string
	}{
		{"full SHA", "abcdef1234567890abcdef1234567890abcdef12", "abcdef123456"},
		{"exactly 12", "abcdef123456", "abcdef123456"},
		{"short hash", "abcdef", "abcdef"},
		{"empty", "", ""},
		{"13 chars", "abcdef1234567", "abcdef123456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortCommit(tt.hash)
			if got != tt.expect {
				t.Errorf("ShortCommit(%q) = %q, want %q", tt.hash, got, tt.expect)
			}
		})
	}
}

func TestCommitsMatch(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		expect bool
	}{
		{"identical full", "abcdef1234567890", "abcdef1234567890", true},
		{"prefix match short-long", "abcdef1234567", "abcdef1234567890abcd", true},
		{"prefix match long-short", "abcdef1234567890abcd", "abcdef1234567", true},
		{"no match", "abcdef1234567", "1234567abcdef", false},
		{"too short a", "abc", "abcdef1234567", false},
		{"too short b", "abcdef1234567", "abc", false},
		{"both too short", "abc", "abc", false},
		{"exactly 7 chars match", "abcdefg", "abcdefg", true},
		{"exactly 7 chars no match", "abcdefg", "abcdefh", false},
		{"6 chars too short", "abcdef", "abcdef", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitsMatch(tt.a, tt.b)
			if got != tt.expect {
				t.Errorf("commitsMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expect)
			}
		})
	}
}

func TestSetCommit(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	SetCommit("abc123def456")
	if Commit != "abc123def456" {
		t.Errorf("SetCommit did not set Commit; got %q", Commit)
	}
}

func TestCheckStaleBinary_NoCommit(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	Commit = ""
	// Force resolveCommitHash to return empty by clearing Commit
	// (vcs.revision from build info may still be set, so this test
	// verifies the error path when no commit is available)
	info := CheckStaleBinary(t.TempDir())
	if info == nil {
		t.Fatal("CheckStaleBinary returned nil")
	}
	// Either we get an error (no commit) or we get a valid result from build info
	// Both are acceptable outcomes
	if info.BinaryCommit == "" && info.Error == nil {
		t.Error("expected error when binary commit is empty")
	}
}

func TestCheckStaleBinary_NotGitRepo(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	Commit = "abcdef1234567"
	info := CheckStaleBinary(t.TempDir()) // empty dir, not a git repo
	if info == nil {
		t.Fatal("CheckStaleBinary returned nil")
	}
	if info.Error == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestCheckStaleBinary_MatchingCommit(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	dir := initGitRepo(t)
	head := getGitHead(t, dir)
	Commit = head

	info := CheckStaleBinary(dir)
	if info == nil {
		t.Fatal("CheckStaleBinary returned nil")
	}
	if info.Error != nil {
		t.Fatalf("unexpected error: %v", info.Error)
	}
	if info.IsStale {
		t.Error("should not be stale when commits match")
	}
}

func TestCheckStaleBinary_StaleCommit(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	dir := initGitRepo(t)
	firstHead := getGitHead(t, dir)

	// Add another commit
	addGitCommit(t, dir, "second.txt", "second commit")

	Commit = firstHead
	info := CheckStaleBinary(dir)
	if info == nil {
		t.Fatal("CheckStaleBinary returned nil")
	}
	if info.Error != nil {
		t.Fatalf("unexpected error: %v", info.Error)
	}
	if !info.IsStale {
		t.Error("should be stale when commits differ")
	}
	if info.CommitsBehind < 1 {
		t.Errorf("CommitsBehind = %d, want >= 1", info.CommitsBehind)
	}
}

func TestStaleBinaryInfo_Fields(t *testing.T) {
	info := &StaleBinaryInfo{
		IsStale:       true,
		BinaryCommit:  "abc123",
		RepoCommit:    "def456",
		CommitsBehind: 5,
	}
	if !info.IsStale {
		t.Error("IsStale should be true")
	}
	if info.CommitsBehind != 5 {
		t.Errorf("CommitsBehind = %d", info.CommitsBehind)
	}
}

func TestResolveCommitHash_WithCommitVar(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	Commit = "explicit_commit_hash"
	got := resolveCommitHash()
	if got != "explicit_commit_hash" {
		t.Errorf("resolveCommitHash() = %q, want %q", got, "explicit_commit_hash")
	}
}

func TestHasGtSource(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		if hasGtSource(t.TempDir()) {
			t.Error("should be false for empty dir")
		}
	})

	t.Run("with marker file", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(dir+"/cmd/gt", 0755)
		os.WriteFile(dir+"/cmd/gt/main.go", []byte("package main"), 0644)
		if !hasGtSource(dir) {
			t.Error("should be true when cmd/gt/main.go exists")
		}
	})
}

func TestIsGitRepo(t *testing.T) {
	t.Run("git repo", func(t *testing.T) {
		dir := initGitRepo(t)
		if !isGitRepo(dir) {
			t.Error("should be true for git repo")
		}
	})

	t.Run("not git repo", func(t *testing.T) {
		if isGitRepo(t.TempDir()) {
			t.Error("should be false for non-git dir")
		}
	})
}

// helpers

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	addGitCommit(t, dir, "init.txt", "initial commit")
	return dir
}

func addGitCommit(t *testing.T, dir, filename, msg string) {
	t.Helper()
	os.WriteFile(dir+"/"+filename, []byte(msg), 0644)
	run(t, dir, "git", "add", filename)
	run(t, dir, "git", "commit", "-m", msg)
}

func getGitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
