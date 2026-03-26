package refinery

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	gh "github.com/steveyegge/gastown/internal/github"
	"github.com/steveyegge/gastown/internal/util"
)

const maxReviewRounds = 3

// FeedbackResult holds the outcome of processing a CHANGES_REQUESTED review.
type FeedbackResult struct {
	ConvoyID    string
	PRNumber    int
	Round       int
	FeedbackID  string
	Escalated   bool
	Error       string
}

// HandleReviewFeedback processes a convoy whose PR received CHANGES_REQUESTED.
// It increments the review round counter, fetches all review comments, creates
// a single feedback bead with the comments as markdown, and dispatches one
// polecat to address them. If the round exceeds maxReviewRounds, it notifies
// the crew instead of dispatching.
func HandleReviewFeedback(ctx context.Context, hqBeads *beads.Beads, townRoot string, convoy *beads.Issue, prNumber int, owner, repo string, logger func(format string, args ...interface{}), gtPath string) *FeedbackResult {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}

	result := &FeedbackResult{
		ConvoyID: convoy.ID,
		PRNumber: prNumber,
	}

	// 1. Increment review round counter on the convoy bead.
	round := getReviewRound(convoy.Description) + 1
	result.Round = round

	newDesc := replaceMetadataFields(convoy.Description, map[string]string{
		"convoy_status":     "changes_requested",
		"review_round":      strconv.Itoa(round),
		"review_changes_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err := hqBeads.Update(convoy.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		logger("Feedback: convoy %s: failed to update round counter: %v", convoy.ID, err)
	}

	logger("Feedback: convoy %s PR #%d: round %d of %d", convoy.ID, prNumber, round, maxReviewRounds)

	// 2. Fetch all review comments via GitHub API.
	comments, err := fetchReviewComments(ctx, owner, repo, prNumber)
	if err != nil {
		logger("Feedback: convoy %s: failed to fetch review comments: %v", convoy.ID, err)
		// Continue — polecat can still address review without inline comments.
	}

	// 3. Check round limit — escalate to crew if exceeded.
	if round > maxReviewRounds {
		result.Escalated = true
		escalateToCrewOnRoundLimit(convoy, prNumber, round, comments, townRoot, logger, gtPath)
		return result
	}

	// 4. Create one feedback bead with all review comments as markdown.
	feedbackID, err := createFeedbackBead(hqBeads, convoy, prNumber, round, owner, repo, comments)
	if err != nil {
		result.Error = fmt.Sprintf("create feedback bead: %v", err)
		logger("Feedback: convoy %s: %s", convoy.ID, result.Error)
		return result
	}
	result.FeedbackID = feedbackID
	logger("Feedback: convoy %s: created feedback bead %s (round %d)", convoy.ID, feedbackID, round)

	// 5. Dispatch one polecat to address the review comments.
	if err := dispatchFeedbackPolecat(ctx, townRoot, convoy, feedbackID, gtPath, logger); err != nil {
		result.Error = fmt.Sprintf("dispatch polecat: %v", err)
		logger("Feedback: convoy %s: %s", convoy.ID, result.Error)
		return result
	}

	// 6. Nudge convoy stakeholders about the feedback cycle.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	msg := fmt.Sprintf("CHANGES_REQUESTED: convoy=%s pr=#%d round=%d feedback=%s",
		convoy.ID, prNumber, round, feedbackID)
	nudgeConvoyStakeholders(convoyFields, msg, townRoot, convoy.ID, logger, gtPath)

	return result
}

// getReviewRound extracts the review_round counter from a convoy description.
// Returns 0 if not set or not parseable.
func getReviewRound(description string) int {
	val := beads.GetReviewRoundField(description)
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return n
}

// fetchReviewComments retrieves all PR review comments from GitHub.
func fetchReviewComments(ctx context.Context, owner, repo string, prNumber int) ([]gh.ReviewComment, error) {
	client, err := gh.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}
	return client.GetPRReviewComments(ctx, owner, repo, prNumber)
}

// createFeedbackBead creates a single bead containing all review comments
// formatted as markdown, with instructions for the polecat to address each one.
func createFeedbackBead(hqBeads *beads.Beads, convoy *beads.Issue, prNumber, round int, owner, repo string, comments []gh.ReviewComment) (string, error) {
	integrationBranch := beads.GetIntegrationBranchField(convoy.Description)
	prURL := beads.GetPRURLField(convoy.Description)

	title := fmt.Sprintf("Address PR review feedback (round %d) for %s", round, convoy.ID)

	var desc strings.Builder
	// Structured metadata fields.
	fmt.Fprintf(&desc, "convoy_id: %s\n", convoy.ID)
	fmt.Fprintf(&desc, "pr_number: %d\n", prNumber)
	fmt.Fprintf(&desc, "pr_url: %s\n", prURL)
	fmt.Fprintf(&desc, "integration_branch: %s\n", integrationBranch)
	fmt.Fprintf(&desc, "review_round: %d\n", round)
	fmt.Fprintf(&desc, "github_owner: %s\n", owner)
	fmt.Fprintf(&desc, "github_repo: %s\n", repo)
	fmt.Fprintf(&desc, "merge_strategy: batch-pr\n")

	// Instructions for the polecat.
	desc.WriteString("\n## Context\n")
	fmt.Fprintf(&desc, "The PR for convoy %s received CHANGES_REQUESTED from reviewers (round %d).\n", convoy.ID, round)
	desc.WriteString("Address each review comment and push fixes to the integration branch.\n")

	desc.WriteString("\n## Instructions\n")
	desc.WriteString("1. Check out the integration branch and pull latest\n")
	desc.WriteString("2. For EACH review comment below, make the requested change\n")
	desc.WriteString("3. Commit fixes with descriptive messages referencing the comment\n")
	desc.WriteString("4. Push fixes to the integration branch\n")
	desc.WriteString("5. Reply to EACH PR comment via GitHub API explaining the fix:\n")
	desc.WriteString("   Use `gh api` or the GitHub MCP tool to post reply comments\n")
	desc.WriteString("   Reference the commit SHA that addresses each comment\n")

	// Format review comments.
	desc.WriteString("\n## Review Comments\n")
	if len(comments) == 0 {
		desc.WriteString("(No inline comments found — check the PR for review-level feedback)\n")
	} else {
		for i, c := range comments {
			fmt.Fprintf(&desc, "\n### Comment %d (ID: %d)\n", i+1, c.ID)
			fmt.Fprintf(&desc, "- **Reviewer**: %s\n", c.User)
			fmt.Fprintf(&desc, "- **File**: `%s` (line %d)\n", c.Path, c.Line)
			if c.HTMLURL != "" {
				fmt.Fprintf(&desc, "- **Link**: %s\n", c.HTMLURL)
			}
			fmt.Fprintf(&desc, "- **Comment**:\n  > %s\n", c.Body)
		}
	}

	issue, err := hqBeads.Create(beads.CreateOptions{
		Title:       title,
		Type:        "task",
		Priority:    2,
		Description: desc.String(),
	})
	if err != nil {
		return "", err
	}
	return issue.ID, nil
}

// dispatchFeedbackPolecat slings a polecat to work on the feedback bead.
// The polecat works on the same rig and targets the integration branch.
func dispatchFeedbackPolecat(ctx context.Context, townRoot string, convoy *beads.Issue, feedbackID, gtPath string, logger func(format string, args ...interface{})) error {
	rigName := findRigNameWithRefinery(townRoot)
	if rigName == "" {
		return fmt.Errorf("cannot determine rig for dispatch")
	}

	slingArgs := []string{"sling", feedbackID, rigName, "--no-boot"}
	if baseBranch := beads.GetIntegrationBranchField(convoy.Description); baseBranch != "" {
		slingArgs = append(slingArgs, "--base-branch="+baseBranch)
	}

	slingCmd := exec.CommandContext(ctx, gtPath, slingArgs...)
	slingCmd.Dir = townRoot
	util.SetProcessGroup(slingCmd)
	if err := slingCmd.Run(); err != nil {
		return fmt.Errorf("gt sling %s %s: %w", feedbackID, rigName, err)
	}

	logger("Feedback: dispatched polecat for feedback bead %s to %s", feedbackID, rigName)
	return nil
}

// escalateToCrewOnRoundLimit notifies the crew when review rounds exceed the
// limit, instead of dispatching another polecat.
func escalateToCrewOnRoundLimit(convoy *beads.Issue, prNumber, round int, comments []gh.ReviewComment, townRoot string, logger func(format string, args ...interface{}), gtPath string) {
	logger("Feedback: convoy %s PR #%d: round %d exceeds limit (%d) — escalating to crew",
		convoy.ID, prNumber, round, maxReviewRounds)

	var commentSummary string
	if len(comments) > 0 {
		commentSummary = formatReviewComments(comments)
	} else {
		commentSummary = "(no inline comments)"
	}

	msg := fmt.Sprintf("REVIEW_ESCALATION: convoy=%s pr=#%d round=%d\n"+
		"Review round limit (%d) exceeded. Manual intervention needed.\n"+
		"Outstanding comments:\n%s",
		convoy.ID, prNumber, round, maxReviewRounds, commentSummary)

	// Nudge crew/overseer for escalation.
	nudgeCmd := exec.Command(gtPath, "nudge", "crew/overseer", msg, "--mode=immediate")
	nudgeCmd.Dir = townRoot
	util.SetProcessGroup(nudgeCmd)
	if err := nudgeCmd.Run(); err != nil {
		logger("Feedback: convoy %s: failed to escalate to crew: %v", convoy.ID, err)
	}

	// Also nudge convoy stakeholders.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoy.Description})
	if convoyFields != nil {
		for _, addr := range convoyFields.NotificationAddresses() {
			cmd := exec.Command(gtPath, "nudge", addr, msg)
			cmd.Dir = townRoot
			util.SetProcessGroup(cmd)
			_ = cmd.Run()
		}
	}
}
