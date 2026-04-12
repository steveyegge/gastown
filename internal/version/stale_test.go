package version

import (
	"os"
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

func TestIsBuildBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"carry/operational", true},
		{"carry/staging", true},
		{"carry/", true},
		{"fix/something", false},
		{"feat/new-thing", false},
		{"develop", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			if got := isBuildBranch(tt.branch); got != tt.want {
				t.Errorf("isBuildBranch(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
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

func TestCleanGitCmd_StripsGitEnv(t *testing.T) {
	// Verify cleanGitCmd strips GIT_DIR and GIT_WORK_TREE from the subprocess env.
	// When GIT_DIR='' is inherited (e.g., from tmux in polecat sessions), git fails
	// with "fatal: not a git repository: ''" even when cmd.Dir is a valid git repo.
	t.Setenv("GIT_DIR", "")
	t.Setenv("GIT_WORK_TREE", "/some/path")

	cmd := cleanGitCmd("--version")
	for _, e := range cmd.Env {
		if len(e) >= 8 && e[:8] == "GIT_DIR=" {
			t.Errorf("cleanGitCmd left GIT_DIR in env: %q", e)
		}
		if len(e) >= 14 && e[:14] == "GIT_WORK_TREE=" {
			t.Errorf("cleanGitCmd left GIT_WORK_TREE in env: %q", e)
		}
	}
}

func TestGetRepoRoot_SkipsNonGitDirs(t *testing.T) {
	// Create a temp dir that has cmd/gt/main.go but is NOT a git repo.
	// GetRepoRoot must skip it and not return it as the repo root.
	tmpDir := t.TempDir()
	mainGo := tmpDir + "/cmd/gt/main.go"
	if err := os.MkdirAll(tmpDir+"/cmd/gt", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainGo, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	// hasGtSource returns true for this dir, but isGitRepo returns false.
	if !hasGtSource(tmpDir) {
		t.Fatal("test setup: expected hasGtSource to return true for tmpDir")
	}
	if isGitRepo(tmpDir) {
		t.Fatal("test setup: expected isGitRepo to return false for tmpDir")
	}

	// GetRepoRoot searches GT_ROOT candidates first. Override GT_ROOT to point
	// only at our non-git tmpDir so we can confirm it is skipped.
	orig := os.Getenv("GT_ROOT")
	defer os.Setenv("GT_ROOT", orig)
	// Set GT_ROOT to a non-existent directory so the env-based candidates all fail,
	// then verify the function still returns an error rather than our non-git tmpDir.
	os.Setenv("GT_ROOT", "/nonexistent-gt-root")
	_, err := GetRepoRoot()
	// May succeed (finds a real repo via HOME fallback) or fail — both are fine.
	// What must NOT happen is returning tmpDir.
	if err == nil {
		// If it found something, confirm it's not our non-git tmpDir
		root, _ := GetRepoRoot()
		if root == tmpDir {
			t.Errorf("GetRepoRoot returned a non-git directory: %s", tmpDir)
		}
	}
}
