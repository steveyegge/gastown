package witness

import (
	"testing"
)

func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		subject  string
		expected ProtocolType
	}{
		{"POLECAT_DONE nux", ProtoPolecatDone},
		{"POLECAT_DONE ace", ProtoPolecatDone},
		{"LIFECYCLE:Shutdown nux", ProtoLifecycleShutdown},
		{"HELP: Tests failing", ProtoHelp},
		{"HELP: Git conflict", ProtoHelp},
		{"MERGED nux", ProtoMerged},
		{"MERGED valkyrie", ProtoMerged},
		{"MERGE_FAILED nux", ProtoMergeFailed},
		{"MERGE_FAILED ace", ProtoMergeFailed},
		{"MERGE_READY nux", ProtoMergeReady},
		{"MERGE_READY ace", ProtoMergeReady},
		{"🤝 HANDOFF: Patrol context", ProtoHandoff},
		{"🤝HANDOFF: No space", ProtoHandoff},
		{"SWARM_START", ProtoSwarmStart},
		{"Unknown message", ProtoUnknown},
		{"", ProtoUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			result := ClassifyMessage(tc.subject)
			if result != tc.expected {
				t.Errorf("ClassifyMessage(%q) = %v, want %v", tc.subject, result, tc.expected)
			}
		})
	}
}

func TestParsePolecatDone(t *testing.T) {
	subject := "POLECAT_DONE nux"
	body := `Exit: MERGED
Issue: gt-abc123
MR: gt-mr-xyz
Branch: feature-branch`

	payload, err := ParsePolecatDone(subject, body)
	if err != nil {
		t.Fatalf("ParsePolecatDone() error = %v", err)
	}

	if payload.PolecatName != "nux" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "nux")
	}
	if payload.Exit != "MERGED" {
		t.Errorf("Exit = %q, want %q", payload.Exit, "MERGED")
	}
	if payload.IssueID != "gt-abc123" {
		t.Errorf("IssueID = %q, want %q", payload.IssueID, "gt-abc123")
	}
	if payload.MRID != "gt-mr-xyz" {
		t.Errorf("MRID = %q, want %q", payload.MRID, "gt-mr-xyz")
	}
	if payload.Branch != "feature-branch" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "feature-branch")
	}
}

func TestParsePolecatDone_MinimalBody(t *testing.T) {
	subject := "POLECAT_DONE ace"
	body := "Exit: DEFERRED"

	payload, err := ParsePolecatDone(subject, body)
	if err != nil {
		t.Fatalf("ParsePolecatDone() error = %v", err)
	}

	if payload.PolecatName != "ace" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "ace")
	}
	if payload.Exit != "DEFERRED" {
		t.Errorf("Exit = %q, want %q", payload.Exit, "DEFERRED")
	}
	if payload.IssueID != "" {
		t.Errorf("IssueID = %q, want empty", payload.IssueID)
	}
}

func TestParsePolecatDone_InvalidSubject(t *testing.T) {
	_, err := ParsePolecatDone("Invalid subject", "body")
	if err == nil {
		t.Error("ParsePolecatDone() expected error for invalid subject")
	}
}

func TestParseHelp(t *testing.T) {
	subject := "HELP: Tests failing on CI"
	body := `Agent: gastown/polecats/nux
Issue: gt-abc123
Problem: Unit tests timeout after 30 seconds
Tried: Increased timeout, checked for deadlocks`

	payload, err := ParseHelp(subject, body)
	if err != nil {
		t.Fatalf("ParseHelp() error = %v", err)
	}

	if payload.Topic != "Tests failing on CI" {
		t.Errorf("Topic = %q, want %q", payload.Topic, "Tests failing on CI")
	}
	if payload.Agent != "gastown/polecats/nux" {
		t.Errorf("Agent = %q, want %q", payload.Agent, "gastown/polecats/nux")
	}
	if payload.IssueID != "gt-abc123" {
		t.Errorf("IssueID = %q, want %q", payload.IssueID, "gt-abc123")
	}
	if payload.Problem != "Unit tests timeout after 30 seconds" {
		t.Errorf("Problem = %q, want %q", payload.Problem, "Unit tests timeout after 30 seconds")
	}
	if payload.Tried != "Increased timeout, checked for deadlocks" {
		t.Errorf("Tried = %q, want %q", payload.Tried, "Increased timeout, checked for deadlocks")
	}
}

func TestParseHelp_InvalidSubject(t *testing.T) {
	_, err := ParseHelp("Not a help message", "body")
	if err == nil {
		t.Error("ParseHelp() expected error for invalid subject")
	}
}

func TestParseMerged(t *testing.T) {
	subject := "MERGED nux"
	body := `Branch: feature-nux
Issue: gt-abc123
Merged-At: 2025-12-30T10:30:00Z`

	payload, err := ParseMerged(subject, body)
	if err != nil {
		t.Fatalf("ParseMerged() error = %v", err)
	}

	if payload.PolecatName != "nux" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "nux")
	}
	if payload.Branch != "feature-nux" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "feature-nux")
	}
	if payload.IssueID != "gt-abc123" {
		t.Errorf("IssueID = %q, want %q", payload.IssueID, "gt-abc123")
	}
	if payload.MergedAt.IsZero() {
		t.Error("MergedAt should not be zero")
	}
}

func TestParseMerged_InvalidSubject(t *testing.T) {
	_, err := ParseMerged("Not merged", "body")
	if err == nil {
		t.Error("ParseMerged() expected error for invalid subject")
	}
}

func TestParseMergeFailed(t *testing.T) {
	subject := "MERGE_FAILED nux"
	body := `Branch: feature-nux
Issue: gt-abc123
FailureType: tests
Error: unit tests failed with 3 errors`

	payload, err := ParseMergeFailed(subject, body)
	if err != nil {
		t.Fatalf("ParseMergeFailed() error = %v", err)
	}

	if payload.PolecatName != "nux" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "nux")
	}
	if payload.Branch != "feature-nux" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "feature-nux")
	}
	if payload.IssueID != "gt-abc123" {
		t.Errorf("IssueID = %q, want %q", payload.IssueID, "gt-abc123")
	}
	if payload.FailureType != "tests" {
		t.Errorf("FailureType = %q, want %q", payload.FailureType, "tests")
	}
	if payload.Error != "unit tests failed with 3 errors" {
		t.Errorf("Error = %q, want %q", payload.Error, "unit tests failed with 3 errors")
	}
	if payload.FailedAt.IsZero() {
		t.Error("FailedAt should not be zero")
	}
}

func TestParseMergeFailed_MinimalBody(t *testing.T) {
	subject := "MERGE_FAILED ace"
	body := "FailureType: build"

	payload, err := ParseMergeFailed(subject, body)
	if err != nil {
		t.Fatalf("ParseMergeFailed() error = %v", err)
	}

	if payload.PolecatName != "ace" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "ace")
	}
	if payload.FailureType != "build" {
		t.Errorf("FailureType = %q, want %q", payload.FailureType, "build")
	}
	if payload.Branch != "" {
		t.Errorf("Branch = %q, want empty", payload.Branch)
	}
}

func TestParseMergeFailed_InvalidSubject(t *testing.T) {
	_, err := ParseMergeFailed("Not a merge failed", "body")
	if err == nil {
		t.Error("ParseMergeFailed() expected error for invalid subject")
	}
}

func TestParseMergeReady(t *testing.T) {
	subject := "MERGE_READY nux"
	body := `Branch: polecat/nux/gt-abc123
Issue: gt-abc123
MR: mr-xyz789
Polecat: nux
Verified: clean git state`

	payload, err := ParseMergeReady(subject, body)
	if err != nil {
		t.Fatalf("ParseMergeReady() error = %v", err)
	}

	if payload.PolecatName != "nux" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "nux")
	}
	if payload.Branch != "polecat/nux/gt-abc123" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "polecat/nux/gt-abc123")
	}
	if payload.IssueID != "gt-abc123" {
		t.Errorf("IssueID = %q, want %q", payload.IssueID, "gt-abc123")
	}
	if payload.MRID != "mr-xyz789" {
		t.Errorf("MRID = %q, want %q", payload.MRID, "mr-xyz789")
	}
	if payload.ReadyAt.IsZero() {
		t.Error("ReadyAt should not be zero")
	}
}

func TestParseMergeReady_MinimalBody(t *testing.T) {
	subject := "MERGE_READY ace"
	body := "Branch: feature-ace"

	payload, err := ParseMergeReady(subject, body)
	if err != nil {
		t.Fatalf("ParseMergeReady() error = %v", err)
	}

	if payload.PolecatName != "ace" {
		t.Errorf("PolecatName = %q, want %q", payload.PolecatName, "ace")
	}
	if payload.Branch != "feature-ace" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "feature-ace")
	}
	if payload.IssueID != "" {
		t.Errorf("IssueID = %q, want empty", payload.IssueID)
	}
}

func TestParseMergeReady_InvalidSubject(t *testing.T) {
	_, err := ParseMergeReady("Not a merge ready", "body")
	if err == nil {
		t.Error("ParseMergeReady() expected error for invalid subject")
	}
}

func TestCleanupWispLabels(t *testing.T) {
	labels := CleanupWispLabels("nux", "pending")

	expected := []string{"cleanup", "polecat:nux", "state:pending"}
	if len(labels) != len(expected) {
		t.Fatalf("CleanupWispLabels() returned %d labels, want %d", len(labels), len(expected))
	}

	for i, label := range labels {
		if label != expected[i] {
			t.Errorf("labels[%d] = %q, want %q", i, label, expected[i])
		}
	}
}

func TestAssessHelpRequest_GitConflict(t *testing.T) {
	payload := &HelpPayload{
		Topic:   "Git issue",
		Problem: "Merge conflict in main.go",
	}

	assessment := AssessHelpRequest(payload)

	if assessment.CanHelp {
		t.Error("Should not be able to help with git conflicts")
	}
	if !assessment.NeedsEscalation {
		t.Error("Git conflicts should need escalation")
	}
}

func TestAssessHelpRequest_GitPush(t *testing.T) {
	payload := &HelpPayload{
		Topic:   "Git push failing",
		Problem: "Cannot push to remote",
	}

	assessment := AssessHelpRequest(payload)

	if !assessment.CanHelp {
		t.Error("Should be able to help with git push issues")
	}
	if assessment.HelpAction == "" {
		t.Error("HelpAction should not be empty")
	}
}

func TestAssessHelpRequest_TestFailures(t *testing.T) {
	payload := &HelpPayload{
		Topic:   "Test failures",
		Problem: "Tests fail on CI",
	}

	assessment := AssessHelpRequest(payload)

	if assessment.CanHelp {
		t.Error("Should not be able to help with test failures")
	}
	if !assessment.NeedsEscalation {
		t.Error("Test failures should need escalation")
	}
}

func TestAssessHelpRequest_RequirementsUnclear(t *testing.T) {
	payload := &HelpPayload{
		Topic:   "Requirements unclear",
		Problem: "Don't understand the requirements for this task",
	}

	assessment := AssessHelpRequest(payload)

	if assessment.CanHelp {
		t.Error("Should not be able to help with unclear requirements")
	}
	if !assessment.NeedsEscalation {
		t.Error("Unclear requirements should need escalation")
	}
}

func TestAssessHelpRequest_BuildIssues(t *testing.T) {
	payload := &HelpPayload{
		Topic:   "Build failing",
		Problem: "Cannot compile the project",
	}

	assessment := AssessHelpRequest(payload)

	if !assessment.CanHelp {
		t.Error("Should be able to help with build issues")
	}
}
