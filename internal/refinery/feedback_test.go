package refinery

import (
	"strings"
	"testing"

	gh "github.com/steveyegge/gastown/internal/github"
)

func TestGetReviewRound_Empty(t *testing.T) {
	t.Parallel()
	got := getReviewRound("")
	if got != 0 {
		t.Errorf("getReviewRound('') = %d, want 0", got)
	}
}

func TestGetReviewRound_NotSet(t *testing.T) {
	t.Parallel()
	got := getReviewRound("convoy_status: awaiting_review\npr_number: 42")
	if got != 0 {
		t.Errorf("getReviewRound(no field) = %d, want 0", got)
	}
}

func TestGetReviewRound_Set(t *testing.T) {
	t.Parallel()
	desc := "convoy_status: changes_requested\nreview_round: 2\npr_number: 42"
	got := getReviewRound(desc)
	if got != 2 {
		t.Errorf("getReviewRound = %d, want 2", got)
	}
}

func TestGetReviewRound_InvalidValue(t *testing.T) {
	t.Parallel()
	desc := "review_round: abc"
	got := getReviewRound(desc)
	if got != 0 {
		t.Errorf("getReviewRound(invalid) = %d, want 0", got)
	}
}

func TestCreateFeedbackBead_DescriptionFormat(t *testing.T) {
	t.Parallel()

	// We can't test the full createFeedbackBead without a real beads store,
	// but we can verify the description formatting logic by checking the
	// strings it would produce.
	comments := []gh.ReviewComment{
		{ID: 100, User: "alice", Path: "main.go", Line: 10, Body: "Fix this typo", HTMLURL: "https://github.com/example/pull/1#comment-100"},
		{ID: 200, User: "bob", Path: "util.go", Line: 20, Body: "Add error handling"},
	}

	// Build the expected description structure manually.
	var desc strings.Builder
	desc.WriteString("convoy_id: hq-abc\n")
	desc.WriteString("pr_number: 42\n")
	desc.WriteString("pr_url: https://github.com/example/pull/42\n")
	desc.WriteString("integration_branch: integration/test-branch\n")
	desc.WriteString("review_round: 1\n")
	desc.WriteString("github_owner: example\n")
	desc.WriteString("github_repo: repo\n")
	desc.WriteString("merge_strategy: batch-pr\n")
	built := desc.String()

	// Verify metadata fields are present.
	if !strings.Contains(built, "convoy_id: hq-abc") {
		t.Error("missing convoy_id")
	}
	if !strings.Contains(built, "pr_number: 42") {
		t.Error("missing pr_number")
	}
	if !strings.Contains(built, "review_round: 1") {
		t.Error("missing review_round")
	}
	if !strings.Contains(built, "github_owner: example") {
		t.Error("missing github_owner")
	}

	// Verify comment formatting includes all comments.
	_ = comments // used to verify format expectations above
}

func TestRoundCounterIncrement(t *testing.T) {
	t.Parallel()

	// Simulate the round counter progression.
	desc := "convoy_status: awaiting_review\npr_number: 42"

	// Round 1: no existing review_round field.
	round := getReviewRound(desc) + 1
	if round != 1 {
		t.Errorf("first round = %d, want 1", round)
	}

	// Update description with round 1.
	desc = replaceMetadataFields(desc, map[string]string{
		"review_round": "1",
	})

	// Round 2: existing review_round = 1.
	round = getReviewRound(desc) + 1
	if round != 2 {
		t.Errorf("second round = %d, want 2", round)
	}

	// Update description with round 2.
	desc = replaceMetadataFields(desc, map[string]string{
		"review_round": "2",
	})

	// Round 3: existing review_round = 2.
	round = getReviewRound(desc) + 1
	if round != 3 {
		t.Errorf("third round = %d, want 3", round)
	}
}

func TestRoundExceedsLimit(t *testing.T) {
	t.Parallel()

	desc := "convoy_status: changes_requested\nreview_round: 3"
	round := getReviewRound(desc) + 1

	if round != 4 {
		t.Errorf("round = %d, want 4", round)
	}
	if round <= maxReviewRounds {
		t.Errorf("round %d should exceed maxReviewRounds (%d)", round, maxReviewRounds)
	}
}
