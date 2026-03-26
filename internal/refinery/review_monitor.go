package refinery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	gh "github.com/steveyegge/gastown/internal/github"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/util"
)

// ConvoyReviewResult holds the outcome of checking a single convoy's PR review.
type ConvoyReviewResult struct {
	ConvoyID    string
	PRNumber    int
	ReviewState gh.ReviewState
	Error       string
}

// PollConvoyReviews checks all batch-pr convoys in awaiting_review state for
// PR review status updates. For each convoy:
//   - APPROVED: updates convoy status and nudges for final merge
//   - CHANGES_REQUESTED: creates a feedback bead and dispatches a polecat
//   - PENDING: no action (will be checked again next poll)
//
// This is designed to be called from the daemon's convoy manager or refinery
// patrol loop at the configured poll interval.
func PollConvoyReviews(ctx context.Context, townRoot string, logger func(format string, args ...interface{}), gtPath string) []ConvoyReviewResult {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}

	hqBeadsDir := filepath.Join(townRoot, ".beads")
	hqBeads := beads.New(hqBeadsDir)

	// List open convoys via bd subprocess (issue_type=convoy requires --type flag).
	convoys, err := listOpenConvoys(ctx, hqBeadsDir)
	if err != nil {
		logger("ReviewMonitor: failed to list convoys: %v", err)
		return nil
	}

	var results []ConvoyReviewResult
	for _, convoy := range convoys {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		// Process convoys in awaiting_review or approved state.
		// "approved" convoys need merge retry if a previous attempt failed.
		status := beads.GetConvoyStatusField(convoy.Description)
		if status != "awaiting_review" && status != "approved" {
			continue
		}

		result := checkConvoyReview(ctx, hqBeads, townRoot, convoy, logger, gtPath)
		if result != nil {
			results = append(results, *result)
		}
	}

	return results
}

// listOpenConvoys lists open convoys from the hq beads store.
// Uses the bd subprocess with --type=convoy to filter by issue_type.
func listOpenConvoys(ctx context.Context, hqBeadsDir string) ([]*beads.Issue, error) {
	cmd := exec.CommandContext(ctx, "bd", "list", "--type=convoy", "--status=open", "--json")
	cmd.Dir = hqBeadsDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bd list: %s", util.FirstLine(stderr.String()))
	}

	out := stdout.Bytes()
	if len(out) == 0 {
		return nil, nil
	}

	var issues []*beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse convoy list: %w", err)
	}
	return issues, nil
}

// checkConvoyReview checks a single convoy's PR review status and takes action.
func checkConvoyReview(ctx context.Context, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, logger func(format string, args ...interface{}), gtPath string) *ConvoyReviewResult {
	result := &ConvoyReviewResult{ConvoyID: convoy.ID}

	// Extract PR number from convoy description.
	prNumStr := beads.GetPRNumberField(convoy.Description)
	if prNumStr == "" {
		result.Error = "no pr_number field"
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	prNumber, err := strconv.Atoi(prNumStr)
	if err != nil {
		result.Error = fmt.Sprintf("invalid pr_number %q: %v", prNumStr, err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	result.PRNumber = prNumber

	// Determine owner/repo from rig config.
	rigPath := findRigWithRefinery(townRoot)
	if rigPath == "" {
		result.Error = "cannot find rig with refinery"
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	rigCfg, err := rig.LoadRigConfig(rigPath)
	if err != nil {
		result.Error = fmt.Sprintf("load rig config: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	owner, repo, err := parseGitRemoteURL(rigCfg.GitURL)
	if err != nil {
		result.Error = fmt.Sprintf("parse git URL: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}

	// Create GitHub client for all API operations.
	client, err := gh.NewClient()
	if err != nil {
		result.Error = fmt.Sprintf("create github client: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}

	// Check if the PR is still open before checking review status.
	prState, err := client.GetPRState(ctx, owner, repo, prNumber)
	if err != nil {
		result.Error = fmt.Sprintf("get PR state: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}

	if prState.State == "closed" && prState.Merged {
		// PR was merged externally (e.g., manually via GitHub UI).
		logger("ReviewMonitor: convoy %s PR #%d: already merged externally", convoy.ID, prNumber)
		handleExternallyMerged(ctx, client, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
		return result
	}
	if prState.State == "closed" && !prState.Merged {
		// PR was closed without merge — abandoned.
		logger("ReviewMonitor: convoy %s PR #%d: closed without merge", convoy.ID, prNumber)
		handlePRClosedWithoutMerge(hqBeads, townRoot, convoy, prNumber, logger, gtPath)
		return result
	}

	// For convoys already in "approved" state, go straight to merge.
	convoyStatus := beads.GetConvoyStatusField(convoy.Description)
	if convoyStatus == "approved" {
		handleFinalMerge(ctx, client, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
		return result
	}

	// Check PR review status via GitHub API.
	reviewState, err := client.GetPRReviewStatus(ctx, owner, repo, prNumber)
	if err != nil {
		result.Error = fmt.Sprintf("get review status: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	result.ReviewState = reviewState

	logger("ReviewMonitor: convoy %s PR #%d: review state = %s", convoy.ID, prNumber, reviewState)

	switch reviewState {
	case gh.ReviewApproved:
		handleReviewApproved(ctx, client, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
	case gh.ReviewChangesRequired:
		HandleReviewFeedback(ctx, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
	case gh.ReviewPending, gh.ReviewCommented, gh.ReviewDismissed:
		// No action — will be checked again next poll cycle.
	}

	return result
}

// handleReviewApproved processes a convoy whose PR has been approved.
// Updates the convoy status to approved, then performs the final merge.
func handleReviewApproved(ctx context.Context, client *gh.Client, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, owner, repo string, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: APPROVED — triggering final merge", convoy.ID, prNumber)

	// Update convoy bead description with approved status.
	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status":      "approved",
		"review_approved_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("ReviewMonitor: convoy %s: failed to update bead: %v", convoy.ID, err)
	}

	handleFinalMerge(ctx, client, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
}

// handleFinalMerge merges an approved PR, cleans up the integration branch,
// closes the convoy bead, and notifies the convoy owner.
func handleFinalMerge(ctx context.Context, client *gh.Client, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, owner, repo string, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: performing final merge", convoy.ID, prNumber)

	// Step 1: Detect the repo's preferred merge method (fallback: squash).
	mergeMethod, err := client.GetRepoMergeMethod(ctx, owner, repo)
	if err != nil {
		logger("ReviewMonitor: convoy %s: failed to detect merge method: %v (falling back to squash)", convoy.ID, err)
		mergeMethod = "squash"
	}
	logger("ReviewMonitor: convoy %s: merge method = %s", convoy.ID, mergeMethod)

	// Step 2: Merge the PR.
	if err := client.MergePR(ctx, owner, repo, prNumber, mergeMethod); err != nil {
		logger("ReviewMonitor: convoy %s: failed to merge PR #%d: %v", convoy.ID, prNumber, err)
		// Update status to reflect merge failure for retry on next poll.
		failDesc := replaceMetadataFields(convoy.Description, map[string]string{
			"convoy_status": "approved",
			"merge_error":   err.Error(),
			"merge_failed_at": time.Now().UTC().Format(time.RFC3339),
		})
		_ = hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &failDesc})
		return
	}
	logger("ReviewMonitor: convoy %s: ✓ PR #%d merged via %s", convoy.ID, prNumber, mergeMethod)

	// Step 3: Delete the integration branch (remote via API, best-effort).
	integrationBranch := beads.GetIntegrationBranchField(convoy.Description)
	if integrationBranch != "" {
		if err := client.DeleteBranch(ctx, owner, repo, integrationBranch); err != nil {
			logger("ReviewMonitor: convoy %s: failed to delete remote branch %s: %v (non-fatal)", convoy.ID, integrationBranch, err)
		} else {
			logger("ReviewMonitor: convoy %s: ✓ Deleted remote branch %s", convoy.ID, integrationBranch)
		}
	}

	// Step 4: Close the convoy bead with reason: merged.
	closeConvoyAfterMerge(hqBeads, convoy, prNumber, mergeMethod, logger)

	// Step 5: Notify convoy owner.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	prURL := beads.GetPRURLField(convoy.Description)
	msg := fmt.Sprintf("PR_MERGED: convoy=%s pr=#%d method=%s", convoy.ID, prNumber, mergeMethod)
	if prURL != "" {
		msg += " url=" + prURL
	}
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)
}

// closeConvoyAfterMerge updates the convoy description with merge metadata
// and closes the bead.
func closeConvoyAfterMerge(hqBeads *beads.Beads, convoy *beads.Issue, prNumber int, mergeMethod string, logger func(format string, args ...interface{})) {
	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status": "merged",
		"merged_at":     time.Now().UTC().Format(time.RFC3339),
		"merge_method":  mergeMethod,
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("ReviewMonitor: convoy %s: failed to update bead with merge status: %v", convoy.ID, err)
	}

	reason := fmt.Sprintf("PR #%d merged via %s", prNumber, mergeMethod)
	if err := hqBeads.CloseWithReason(reason, convoy.ID); err != nil {
		logger("ReviewMonitor: convoy %s: failed to close bead: %v", convoy.ID, err)
	} else {
		logger("ReviewMonitor: convoy %s: ✓ Convoy closed (merged)", convoy.ID)
	}
}

// handlePRClosedWithoutMerge processes a convoy whose PR was closed without
// being merged. Sets convoy status to abandoned and notifies the owner.
// Leaves the integration branch intact for manual cleanup.
func handlePRClosedWithoutMerge(hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: closed without merge — marking abandoned", convoy.ID, prNumber)

	// Update convoy status to abandoned.
	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status": "abandoned",
		"abandoned_at":  time.Now().UTC().Format(time.RFC3339),
		"abandon_reason": fmt.Sprintf("PR #%d closed without merge", prNumber),
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("ReviewMonitor: convoy %s: failed to update bead: %v", convoy.ID, err)
	}

	// Close the convoy bead with abandon reason.
	reason := fmt.Sprintf("PR #%d closed without merge", prNumber)
	if err := hqBeads.CloseWithReason(reason, convoy.ID); err != nil {
		logger("ReviewMonitor: convoy %s: failed to close bead: %v", convoy.ID, err)
	}

	// Notify convoy owner about the abandonment.
	// Integration branch is left intact for manual cleanup.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	integrationBranch := beads.GetIntegrationBranchField(convoy.Description)
	msg := fmt.Sprintf("PR_ABANDONED: convoy=%s pr=#%d", convoy.ID, prNumber)
	if integrationBranch != "" {
		msg += " branch=" + integrationBranch + " (left for manual cleanup)"
	}
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)
}

// handleExternallyMerged processes a convoy whose PR was merged outside
// the normal review monitor flow (e.g., manually via GitHub UI).
// Cleans up the integration branch and closes the convoy.
func handleExternallyMerged(ctx context.Context, client *gh.Client, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, owner, repo string, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: merged externally — cleaning up", convoy.ID, prNumber)

	// Delete the integration branch (best-effort).
	integrationBranch := beads.GetIntegrationBranchField(convoy.Description)
	if integrationBranch != "" {
		if err := client.DeleteBranch(ctx, owner, repo, integrationBranch); err != nil {
			logger("ReviewMonitor: convoy %s: failed to delete remote branch %s: %v (non-fatal)", convoy.ID, integrationBranch, err)
		} else {
			logger("ReviewMonitor: convoy %s: ✓ Deleted remote branch %s", convoy.ID, integrationBranch)
		}
	}

	// Close the convoy bead.
	closeConvoyAfterMerge(hqBeads, convoy, prNumber, "external", logger)

	// Notify convoy owner.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	msg := fmt.Sprintf("PR_MERGED: convoy=%s pr=#%d method=external (merged outside review monitor)", convoy.ID, prNumber)
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)
}

// handleChangesRequested is now delegated to HandleReviewFeedback in feedback.go.

// replaceMetadataFields replaces or adds key: value fields in a description.
// Existing fields with matching keys are replaced; new fields are appended.
func replaceMetadataFields(description string, fields map[string]string) string {
	lines := strings.Split(description, "\n")
	remaining := make(map[string]string, len(fields))
	for k, v := range fields {
		remaining[k] = v
	}

	var result []string
	for _, line := range lines {
		key := extractFieldKey(line)
		if _, ok := remaining[key]; ok {
			result = append(result, key+": "+remaining[key])
			delete(remaining, key)
		} else {
			result = append(result, line)
		}
	}

	// Append any fields that weren't already present.
	for k, v := range remaining {
		result = append(result, k+": "+v)
	}

	return strings.Join(result, "\n")
}

// extractFieldKey extracts the lowercase key before ":" from a line.
// Returns empty string if the line doesn't contain a key: value pair.
func extractFieldKey(line string) string {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(line[:idx]))
}

// formatReviewComments formats review comments into a readable markdown summary.
func formatReviewComments(comments []gh.ReviewComment) string {
	if len(comments) == 0 {
		return "(no comments)"
	}
	var b strings.Builder
	for _, c := range comments {
		fmt.Fprintf(&b, "- **%s** on `%s` (line %d):\n  > %s\n\n", c.User, c.Path, c.Line, c.Body)
	}
	return b.String()
}

// nudgeConvoyStakeholders sends nudges to all convoy stakeholders.
func nudgeConvoyStakeholders(fields *beads.ConvoyFields, msg, townRoot, convoyID string, logger func(format string, args ...interface{}), gtPath string) {
	seen := make(map[string]bool)
	var addrs []string

	if fields != nil {
		for _, addr := range fields.NotificationAddresses() {
			if !seen[addr] {
				addrs = append(addrs, addr)
				seen[addr] = true
			}
		}
		for _, addr := range fields.NudgeNotificationAddresses() {
			if !seen[addr] {
				addrs = append(addrs, addr)
				seen[addr] = true
			}
		}
	}
	// Always include overseer as fallback.
	if !seen["crew/overseer"] {
		addrs = append(addrs, "crew/overseer")
	}

	for _, addr := range addrs {
		cmd := exec.Command(gtPath, "nudge", addr, msg)
		cmd.Dir = townRoot
		util.SetProcessGroup(cmd)
		if err := cmd.Run(); err != nil {
			logger("ReviewMonitor: convoy %s: failed to nudge %s: %v", convoyID, addr, err)
		}
	}
}

// findRigNameWithRefinery returns the rig name (not path) for the first rig
// that has a refinery worktree.
func findRigNameWithRefinery(townRoot string) string {
	path := findRigWithRefinery(townRoot)
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
