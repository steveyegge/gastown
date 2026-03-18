package checkpoint

import (
	"fmt"
	"os/exec"
	"strings"
)

// CountWIPCommits counts the number of WIP checkpoint commits between
// the merge-base with the given ref and HEAD.
func CountWIPCommits(workDir, baseRef string) (int, error) {
	commits, err := listCommitMessages(workDir, baseRef)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, msg := range commits {
		if strings.HasPrefix(msg, WIPCommitPrefix) {
			count++
		}
	}
	return count, nil
}

// SquashWIPCommits removes WIP checkpoint commits by soft-resetting to the
// merge-base and creating a single clean commit with all changes. Non-WIP
// commit messages are preserved in the new commit body.
//
// Returns the number of WIP commits squashed, or 0 if none were found.
// This is a no-op if there are no WIP commits.
func SquashWIPCommits(workDir, baseRef string) (int, error) {
	commits, err := listCommitSubjects(workDir, baseRef)
	if err != nil {
		return 0, fmt.Errorf("listing commits: %w", err)
	}

	// Count WIP commits.
	wipCount := 0
	var realMessages []string
	for _, subject := range commits {
		if strings.HasPrefix(subject, WIPCommitPrefix) {
			wipCount++
		} else {
			realMessages = append(realMessages, subject)
		}
	}

	if wipCount == 0 {
		return 0, nil
	}

	// Find the merge-base to reset to.
	mergeBase, err := getMergeBase(workDir, baseRef)
	if err != nil {
		return 0, fmt.Errorf("finding merge-base: %w", err)
	}

	// Soft reset to merge-base — keeps all changes staged.
	cmd := exec.Command("git", "reset", "--soft", mergeBase)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("soft reset: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Build a combined commit message from the real (non-WIP) commits.
	var commitMsg string
	switch len(realMessages) {
	case 0:
		// All commits were WIP — use a generic message.
		commitMsg = "feat: implementation work"
	case 1:
		commitMsg = realMessages[0]
	default:
		// Use the first real message as the subject, list the rest in the body.
		commitMsg = realMessages[0] + "\n"
		for _, msg := range realMessages[1:] {
			commitMsg += "\n- " + msg
		}
	}

	// Append squash note.
	commitMsg += fmt.Sprintf("\n\nSquashed %d WIP checkpoint commit(s)", wipCount)

	// Create the combined commit.
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("creating squashed commit: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return wipCount, nil
}

// listCommitMessages returns the full commit messages between baseRef and HEAD.
func listCommitMessages(workDir, baseRef string) ([]string, error) {
	mergeBase, err := getMergeBase(workDir, baseRef)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "log", "--format=%B---COMMIT-SEP---", mergeBase+"..HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var messages []string
	for _, msg := range strings.Split(string(out), "---COMMIT-SEP---") {
		msg = strings.TrimSpace(msg)
		if msg != "" {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// listCommitSubjects returns the subject lines of commits between baseRef and HEAD.
// Commits are returned in chronological order (oldest first).
func listCommitSubjects(workDir, baseRef string) ([]string, error) {
	mergeBase, err := getMergeBase(workDir, baseRef)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "log", "--format=%s", "--reverse", mergeBase+"..HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var subjects []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			subjects = append(subjects, line)
		}
	}
	return subjects, nil
}

// getMergeBase finds the merge-base between the given ref and HEAD.
func getMergeBase(workDir, ref string) (string, error) {
	cmd := exec.Command("git", "merge-base", ref, "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("finding merge-base between %s and HEAD: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}
