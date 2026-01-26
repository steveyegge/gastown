// Package epic provides upstream contribution workflow support.
package epic

import (
	"bytes"
	"encoding/json"
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
func GetPRCIStatus(workDir string, prNumber int) (*CIStatus, error) {
	cmd := exec.Command("gh", "pr", "checks", fmt.Sprintf("%d", prNumber), "--json", "state,name,detailsUrl")
	cmd.Dir = workDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("getting PR checks: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	var checks []struct {
		State      string `json:"state"`
		Name       string `json:"name"`
		DetailsURL string `json:"detailsUrl"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &checks); err != nil {
		return nil, fmt.Errorf("parsing PR checks: %w", err)
	}

	// Aggregate check status
	status := &CIStatus{
		PRNumber: prNumber,
		State:    "success",
	}

	var failedChecks []string
	var pendingChecks []string

	for _, check := range checks {
		switch check.State {
		case "FAILURE", "ERROR":
			status.State = "failure"
			failedChecks = append(failedChecks, check.Name)
			if status.URL == "" {
				status.URL = check.DetailsURL
			}
		case "PENDING", "QUEUED", "IN_PROGRESS":
			if status.State != "failure" {
				status.State = "pending"
			}
			pendingChecks = append(pendingChecks, check.Name)
		}
	}

	if len(failedChecks) > 0 {
		status.Details = fmt.Sprintf("Failed: %s", strings.Join(failedChecks, ", "))
	} else if len(pendingChecks) > 0 {
		status.Details = fmt.Sprintf("Pending: %s", strings.Join(pendingChecks, ", "))
	} else {
		status.Details = "All checks passed"
	}

	return status, nil
}

// GetPRReviewStatus gets review status for a PR using gh CLI.
func GetPRReviewStatus(workDir string, prNumber int) (string, int, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "reviewDecision,reviews")
	cmd.Dir = workDir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("getting PR review status: %w", err)
	}

	var result struct {
		ReviewDecision string `json:"reviewDecision"`
		Reviews        []struct {
			State string `json:"state"`
		} `json:"reviews"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return "", 0, fmt.Errorf("parsing PR review status: %w", err)
	}

	// Count approvals
	approvals := 0
	for _, review := range result.Reviews {
		if review.State == "APPROVED" {
			approvals++
		}
	}

	// Map review decision to status
	status := "pending"
	switch result.ReviewDecision {
	case "APPROVED":
		status = PRStatusApproved
	case "CHANGES_REQUESTED":
		status = PRStatusChangesRequested
	case "REVIEW_REQUIRED":
		status = "review_required"
	}

	return status, approvals, nil
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
