package version

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestGetRepoRoot_PrefersSourceRepoEnv(t *testing.T) {
	repoDir := makeTestGTRepo(t)
	t.Setenv("GT_SOURCE_REPO", repoDir)
	t.Setenv("GT_ROOT", "")
	t.Setenv("HOME", t.TempDir())

	root, err := GetRepoRoot()
	if err != nil {
		t.Fatalf("GetRepoRoot() error = %v", err)
	}
	if root != repoDir {
		t.Fatalf("GetRepoRoot() = %q, want %q", root, repoDir)
	}
}

func TestGetRepoRoot_UsesInstallMetadata(t *testing.T) {
	repoDir := makeTestGTRepo(t)
	homeDir := t.TempDir()
	exePath := filepath.Join(homeDir, ".local", "bin", "gt")
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		t.Fatalf("mkdir exe dir: %v", err)
	}
	if err := os.WriteFile(exePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	if err := os.WriteFile(sourceRepoMetadataPath(exePath), []byte(repoDir+"\n"), 0o644); err != nil {
		t.Fatalf("write repo metadata: %v", err)
	}

	oldExecutablePath := executablePath
	executablePath = func() (string, error) { return exePath, nil }
	t.Cleanup(func() { executablePath = oldExecutablePath })

	t.Setenv("GT_SOURCE_REPO", "")
	t.Setenv("GT_ROOT", "")
	t.Setenv("HOME", homeDir)

	root, err := GetRepoRoot()
	if err != nil {
		t.Fatalf("GetRepoRoot() error = %v", err)
	}
	if root != repoDir {
		t.Fatalf("GetRepoRoot() = %q, want %q", root, repoDir)
	}
}

func makeTestGTRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "cmd", "gt"), 0o755); err != nil {
		t.Fatalf("mkdir repo source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "cmd", "gt", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return repoDir
}
