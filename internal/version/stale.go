// Package version provides version information and staleness checking for gt.
package version

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
)

// These variables are set at build time via ldflags in cmd package.
// We provide fallback methods to read from build info.
var (
	// Commit can be set from cmd package or read from build info
	Commit = ""
)

// StaleBinaryInfo contains information about binary staleness.
type StaleBinaryInfo struct {
	IsStale       bool   // True if binary commit doesn't match repo HEAD
	BinaryCommit  string // Commit hash the binary was built from
	RepoCommit    string // Current repo HEAD commit
	CommitsBehind int    // Number of commits binary is behind (0 if unknown)
	Error         error  // Any error encountered during check
}

// resolveCommitHash gets the commit hash from build info or the Commit variable.
func resolveCommitHash() string {
	if Commit != "" {
		return Commit
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}

	return ""
}

// ShortCommit returns first 12 characters of a hash.
func ShortCommit(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// commitsMatch compares two commit hashes, handling different lengths.
// Returns true if one is a prefix of the other (minimum 7 chars to avoid false positives).
func commitsMatch(a, b string) bool {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	// Need at least 7 chars for a reasonable comparison
	if minLen < 7 {
		return false
	}
	return strings.HasPrefix(a, b[:minLen]) || strings.HasPrefix(b, a[:minLen])
}

// CheckStaleBinary compares the binary's embedded commit with the repo HEAD.
// It returns staleness info including whether the binary needs rebuilding.
// This check is designed to be fast and non-blocking - errors are captured
// but don't interrupt normal operation.
func CheckStaleBinary(repoDir string) *StaleBinaryInfo {
	info := &StaleBinaryInfo{}

	// Get binary commit
	info.BinaryCommit = resolveCommitHash()
	if info.BinaryCommit == "" {
		info.Error = fmt.Errorf("cannot determine binary commit (dev build?)")
		return info
	}

	// Get repo HEAD
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		info.Error = fmt.Errorf("cannot get repo HEAD: %w", err)
		return info
	}
	info.RepoCommit = strings.TrimSpace(string(output))

	// Compare commits using prefix matching (handles short vs full hash)
	// Use the shorter of the two commit lengths for comparison
	if !commitsMatch(info.BinaryCommit, info.RepoCommit) {
		info.IsStale = true

		// Try to count commits between binary and HEAD
		countCmd := exec.Command("git", "rev-list", "--count", info.BinaryCommit+"..HEAD")
		countCmd.Dir = repoDir
		if countOutput, err := countCmd.Output(); err == nil {
			if count, parseErr := fmt.Sscanf(strings.TrimSpace(string(countOutput)), "%d", &info.CommitsBehind); parseErr != nil || count != 1 {
				info.CommitsBehind = 0
			}
		}
	}

	return info
}

// GetRepoRoot returns the git repository root for the gt source code.
// The gt source lives in the gastown rig at $GT_ROOT/gastown/mayor/rig.
func GetRepoRoot() (string, error) {
	// Check if GT_ROOT environment variable is set (agents always have this)
	if gtRoot := os.Getenv("GT_ROOT"); gtRoot != "" {
		rigPath := gtRoot + "/gastown/mayor/rig"
		if hasGtSource(rigPath) {
			return rigPath, nil
		}
	}

	// Try common development paths relative to home
	home := os.Getenv("HOME")
	if home != "" {
		candidates := []string{
			home + "/gt/gastown/mayor/rig",
			home + "/gastown/mayor/rig",
			home + "/src/gastown/mayor/rig",
		}
		for _, candidate := range candidates {
			if hasGtSource(candidate) {
				return candidate, nil
			}
		}
	}

	// Check if current directory is in the gt source repo
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if output, err := cmd.Output(); err == nil {
		root := strings.TrimSpace(string(output))
		if hasGtSource(root) {
			return root, nil
		}
	}

	return "", fmt.Errorf("cannot locate gt source repository")
}

// isGitRepo checks if a directory is a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// hasGtSource checks if a directory contains the gt source code.
// We look for cmd/gt/main.go as the definitive marker.
func hasGtSource(dir string) bool {
	_, err := os.Stat(dir + "/cmd/gt/main.go")
	return err == nil
}

// SetCommit allows the cmd package to pass in the build-time commit.
func SetCommit(commit string) {
	Commit = commit
}
