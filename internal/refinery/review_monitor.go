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

		// Only process convoys in awaiting_review state.
		if beads.GetConvoyStatusField(convoy.Description) != "awaiting_review" {
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

	// Check PR review status via GitHub API.
	client, err := gh.NewClient()
	if err != nil {
		result.Error = fmt.Sprintf("create github client: %v", err)
		logger("ReviewMonitor: convoy %s: %s", convoy.ID, result.Error)
		return result
	}

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
		handleReviewApproved(hqBeads, townRoot, convoy, prNumber, logger, gtPath)
	case gh.ReviewChangesRequired:
		handleChangesRequested(ctx, hqBeads, townRoot, convoy, prNumber, owner, repo, logger, gtPath)
	case gh.ReviewPending, gh.ReviewCommented, gh.ReviewDismissed:
		// No action — will be checked again next poll cycle.
	}

	return result
}

// handleReviewApproved processes a convoy whose PR has been approved.
// Updates the convoy status to approved and nudges downstream for final merge.
func handleReviewApproved(hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: APPROVED — triggering final merge", convoy.ID, prNumber)

	// Update convoy bead description with approved status.
	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status":      "approved",
		"review_approved_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("ReviewMonitor: convoy %s: failed to update bead: %v", convoy.ID, err)
	}

	// Nudge downstream agents that the PR is approved and ready for final merge.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	msg := fmt.Sprintf("PR_APPROVED: convoy=%s pr=#%d", convoy.ID, prNumber)
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)
}

// handleChangesRequested processes a convoy whose PR received change requests.
// Creates a feedback bead and dispatches a polecat to address the review comments.
func handleChangesRequested(ctx context.Context, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, owner, repo string, logger func(format string, args ...interface{}), gtPath string) {
	logger("ReviewMonitor: convoy %s PR #%d: CHANGES_REQUESTED — creating feedback bead", convoy.ID, prNumber)

	// Update convoy bead description with changes_requested status.
	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status":     "changes_requested",
		"review_changes_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("ReviewMonitor: convoy %s: failed to update bead: %v", convoy.ID, err)
	}

	// Fetch review comments for context in the feedback bead.
	var commentSummary string
	client, err := gh.NewClient()
	if err == nil {
		comments, cerr := client.GetPRReviewComments(ctx, owner, repo, prNumber)
		if cerr == nil && len(comments) > 0 {
			commentSummary = formatReviewComments(comments)
		}
	}

	// Determine the integration branch for the feedback polecat to work on.
	integrationBranch := beads.GetIntegrationBranchField(convoy.Description)
	prURL := beads.GetPRURLField(convoy.Description)

	// Create a feedback bead — a task for a polecat to address review comments.
	title := fmt.Sprintf("Address PR review feedback for %s", convoy.ID)
	desc := fmt.Sprintf("convoy_id: %s\npr_number: %d\npr_url: %s\nintegration_branch: %s\nmerge_strategy: batch-pr\n\n"+
		"## Context\nThe PR for convoy %s received CHANGES_REQUESTED from reviewers.\n"+
		"Address the review comments and push fixes to the integration branch.\n\n"+
		"## Review Comments\n%s",
		convoy.ID, prNumber, prURL, integrationBranch, convoy.ID, commentSummary)

	feedbackID, err := hqBeads.Create(beads.CreateOptions{
		Title:       title,
		Type:        "task",
		Priority:    2,
		Description: desc,
	})
	if err != nil {
		logger("ReviewMonitor: convoy %s: failed to create feedback bead: %v", convoy.ID, err)
		return
	}
	logger("ReviewMonitor: convoy %s: created feedback bead %s", convoy.ID, feedbackID)

	// Dispatch the feedback bead to a rig for a polecat to pick up.
	rigName := findRigNameWithRefinery(townRoot)
	if rigName == "" {
		logger("ReviewMonitor: convoy %s: cannot determine rig for dispatch", convoy.ID)
		return
	}

	slingArgs := []string{"sling", feedbackID, rigName, "--no-boot"}
	if baseBranch := beads.GetBaseBranchField(convoy.Description); baseBranch != "" {
		slingArgs = append(slingArgs, "--base-branch="+baseBranch)
	}
	slingCmd := exec.CommandContext(ctx, gtPath, slingArgs...)
	slingCmd.Dir = townRoot
	util.SetProcessGroup(slingCmd)
	if err := slingCmd.Run(); err != nil {
		logger("ReviewMonitor: convoy %s: failed to dispatch feedback bead %s: %v", convoy.ID, feedbackID, err)
	} else {
		logger("ReviewMonitor: convoy %s: dispatched feedback bead %s to %s", convoy.ID, feedbackID, rigName)
	}

	// Nudge convoy stakeholders about the changes requested.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	msg := fmt.Sprintf("CHANGES_REQUESTED: convoy=%s pr=#%d feedback=%s", convoy.ID, prNumber, feedbackID)
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)
}

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
