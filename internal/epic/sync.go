// Package epic provides upstream contribution workflow support.
package epic

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ConflictInfo describes a merge conflict.
type ConflictInfo struct {
	Branch    string   // Branch with conflict
	BaseBranch string  // Branch we're conflicting with
	Files     []string // Conflicting files
	PRNumber  int      // Associated PR number (0 if none)
}

// CIStatus describes CI status for a PR.
type CIStatus struct {
	PRNumber int    // PR number
	State    string // success, failure, pending, error
	Details  string // Additional details
	URL      string // URL to CI details
}

// SyncResult describes the result of a sync operation.
type SyncResult struct {
	Branch    string        // Branch that was synced
	Success   bool          // Whether sync succeeded
	Conflicts *ConflictInfo // Conflict info if sync failed
	Message   string        // Human-readable result message
}

// RebaseResult describes the result of a rebase operation.
type RebaseResult struct {
	Branch      string        // Branch that was rebased
	BaseBranch  string        // Branch we rebased onto
	Success     bool          // Whether rebase succeeded
	Conflicts   *ConflictInfo // Conflict info if rebase failed
	CommitCount int           // Number of commits rebased
	Message     string        // Human-readable result message
}

// FetchUpstream fetches updates from upstream remote.
func FetchUpstream(workDir, remote string) error {
	cmd := exec.Command("git", "fetch", remote)
	cmd.Dir = workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fetching %s: %w (%s)", remote, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// CheckConflicts checks if a branch has conflicts with another branch.
func CheckConflicts(workDir, branch, baseBranch string) (*ConflictInfo, error) {
	// Try a merge with --no-commit to check for conflicts
	// First, save current state
	origBranch, err := getCurrentBranch(workDir)
	if err != nil {
		return nil, err
	}

	// Checkout the branch to test
	checkoutCmd := exec.Command("git", "checkout", branch)
	checkoutCmd.Dir = workDir
	if err := checkoutCmd.Run(); err != nil {
		return nil, fmt.Errorf("checking out %s: %w", branch, err)
	}

	// Ensure we restore the original branch
	defer func() {
		restoreCmd := exec.Command("git", "checkout", origBranch)
		restoreCmd.Dir = workDir
		_ = restoreCmd.Run()
	}()

	// Try merge with no commit
	mergeCmd := exec.Command("git", "merge", "--no-commit", "--no-ff", baseBranch)
	mergeCmd.Dir = workDir
	var mergeStderr bytes.Buffer
	mergeCmd.Stderr = &mergeStderr

	mergeErr := mergeCmd.Run()

	// Abort the merge regardless of outcome
	abortCmd := exec.Command("git", "merge", "--abort")
	abortCmd.Dir = workDir
	_ = abortCmd.Run()

	if mergeErr == nil {
		// No conflicts
		return nil, nil
	}

	// There were conflicts - parse them
	conflictFiles, _ := getConflictingFiles(workDir)

	return &ConflictInfo{
		Branch:     branch,
		BaseBranch: baseBranch,
		Files:      conflictFiles,
	}, nil
}

// RebaseBranch rebases a branch onto another.
func RebaseBranch(workDir, branch, ontoBranch string) (*RebaseResult, error) {
	// Save current branch
	origBranch, err := getCurrentBranch(workDir)
	if err != nil {
		return nil, err
	}

	// Checkout the branch to rebase
	checkoutCmd := exec.Command("git", "checkout", branch)
	checkoutCmd.Dir = workDir
	if err := checkoutCmd.Run(); err != nil {
		return nil, fmt.Errorf("checking out %s: %w", branch, err)
	}

	// Ensure we restore the original branch
	defer func() {
		restoreCmd := exec.Command("git", "checkout", origBranch)
		restoreCmd.Dir = workDir
		_ = restoreCmd.Run()
	}()

	// Count commits before rebase
	commitCount, _ := countCommits(workDir, ontoBranch, branch)

	// Perform rebase
	rebaseCmd := exec.Command("git", "rebase", ontoBranch)
	rebaseCmd.Dir = workDir
	var rebaseStderr bytes.Buffer
	rebaseCmd.Stderr = &rebaseStderr

	if err := rebaseCmd.Run(); err != nil {
		// Rebase failed - check for conflicts
		conflictFiles, _ := getConflictingFiles(workDir)

		// Abort the rebase
		abortCmd := exec.Command("git", "rebase", "--abort")
		abortCmd.Dir = workDir
		_ = abortCmd.Run()

		return &RebaseResult{
			Branch:     branch,
			BaseBranch: ontoBranch,
			Success:    false,
			Conflicts: &ConflictInfo{
				Branch:     branch,
				BaseBranch: ontoBranch,
				Files:      conflictFiles,
			},
			Message: fmt.Sprintf("Rebase failed with conflicts in %d file(s)", len(conflictFiles)),
		}, nil
	}

	return &RebaseResult{
		Branch:      branch,
		BaseBranch:  ontoBranch,
		Success:     true,
		CommitCount: commitCount,
		Message:     fmt.Sprintf("Rebased %d commit(s) onto %s", commitCount, ontoBranch),
	}, nil
}

// ForcePushBranch force-pushes a branch to remote.
func ForcePushBranch(workDir, remote, branch string) error {
	cmd := exec.Command("git", "push", "--force-with-lease", remote, branch)
	cmd.Dir = workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("force-pushing %s: %w (%s)", branch, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// GetPRCIStatus gets CI status for a PR using gh CLI.
// This is a convenience function that uses the DefaultGHClient.
func GetPRCIStatus(workDir string, prNumber int) (*CIStatus, error) {
	return GetPRCIStatusWithClient(DefaultGHClient, workDir, prNumber)
}

// GetPRReviewStatus gets review status for a PR using gh CLI.
// This is a convenience function that uses the DefaultGHClient.
func GetPRReviewStatus(workDir string, prNumber int) (string, int, error) {
	return GetPRReviewStatusWithClient(DefaultGHClient, workDir, prNumber)
}

// Helper functions

func getCurrentBranch(workDir string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func getConflictingFiles(workDir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func countCommits(workDir, baseBranch, headBranch string) (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", fmt.Sprintf("%s..%s", baseBranch, headBranch))
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	return count, err
}
