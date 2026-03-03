package witness

import (
	"strings"
	"testing"
	"time"
)

func TestClassifyMessage(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	_, err := ParsePolecatDone("Invalid subject", "body")
	if err == nil {
		t.Error("ParsePolecatDone() expected error for invalid subject")
	}
}

func TestParsePolecatDone_MRFailed(t *testing.T) {
	t.Parallel()
	subject := "POLECAT_DONE nux"
	body := `Exit: COMPLETED
Issue: gt-abc123
Branch: polecat/nux-abc123
MRFailed: true
Errors: MR bead creation failed: connection refused`

	payload, err := ParsePolecatDone(subject, body)
	if err != nil {
		t.Fatalf("ParsePolecatDone() error = %v", err)
	}

	if !payload.MRFailed {
		t.Error("MRFailed = false, want true")
	}
	if payload.MRID != "" {
		t.Errorf("MRID = %q, want empty when MR failed", payload.MRID)
	}
	if payload.Exit != "COMPLETED" {
		t.Errorf("Exit = %q, want COMPLETED", payload.Exit)
	}
}

func TestParsePolecatDone_MRFailedAbsent(t *testing.T) {
	t.Parallel()
	// When MRFailed is not in the body, it should default to false
	subject := "POLECAT_DONE nux"
	body := `Exit: COMPLETED
Issue: gt-abc123
MR: gt-mr-xyz
Branch: polecat/nux-abc123`

	payload, err := ParsePolecatDone(subject, body)
	if err != nil {
		t.Fatalf("ParsePolecatDone() error = %v", err)
	}

	if payload.MRFailed {
		t.Error("MRFailed = true, want false when not in body")
	}
}

func TestParseHelp(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := ParseHelp("Not a help message", "body")
	if err == nil {
		t.Error("ParseHelp() expected error for invalid subject")
	}
}

func TestParseMerged(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := ParseMerged("Not merged", "body")
	if err == nil {
		t.Error("ParseMerged() expected error for invalid subject")
	}
}

func TestParseMergeFailed(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	_, err := ParseMergeFailed("Not a merge failed", "body")
	if err == nil {
		t.Error("ParseMergeFailed() expected error for invalid subject")
	}
}

func TestParseMergeReady(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	_, err := ParseMergeReady("Not a merge ready", "body")
	if err == nil {
		t.Error("ParseMergeReady() expected error for invalid subject")
	}
}

func TestParseSwarmStart(t *testing.T) {
	t.Parallel()
	body := `SwarmID: batch-123
Beads: bd-a, bd-b, bd-c
Total: 3`

	payload, err := ParseSwarmStart(body)
	if err != nil {
		t.Fatalf("ParseSwarmStart() error = %v", err)
	}

	if payload.SwarmID != "batch-123" {
		t.Errorf("SwarmID = %q, want %q", payload.SwarmID, "batch-123")
	}
	if payload.Total != 3 {
		t.Errorf("Total = %d, want %d", payload.Total, 3)
	}
	expectedBeads := []string{"bd-a", "bd-b", "bd-c"}
	if len(payload.BeadIDs) != len(expectedBeads) {
		t.Fatalf("BeadIDs has %d items, want %d", len(payload.BeadIDs), len(expectedBeads))
	}
	for i, b := range payload.BeadIDs {
		if b != expectedBeads[i] {
			t.Errorf("BeadIDs[%d] = %q, want %q", i, b, expectedBeads[i])
		}
	}
	if payload.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
}

func TestParseSwarmStart_MinimalBody(t *testing.T) {
	t.Parallel()
	body := "SwarmID: batch-456"

	payload, err := ParseSwarmStart(body)
	if err != nil {
		t.Fatalf("ParseSwarmStart() error = %v", err)
	}

	if payload.SwarmID != "batch-456" {
		t.Errorf("SwarmID = %q, want %q", payload.SwarmID, "batch-456")
	}
	if payload.Total != 0 {
		t.Errorf("Total = %d, want 0", payload.Total)
	}
	if len(payload.BeadIDs) != 0 {
		t.Errorf("BeadIDs = %v, want empty", payload.BeadIDs)
	}
}

func TestCleanupWispLabels(t *testing.T) {
	t.Parallel()
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

func TestFormatHelpSummary_FullPayload(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	payload := &HelpPayload{
		Agent:       "gastown/polecats/nux",
		IssueID:     "gt-1234",
		Topic:       "Git conflict",
		Problem:     "Merge conflict in main.go",
		Tried:       "Attempted rebase, still conflicts",
		RequestedAt: ts,
	}

	summary := FormatHelpSummary(payload)

	if !strings.Contains(summary, "HELP REQUEST from gastown/polecats/nux") {
		t.Errorf("summary should contain agent name, got: %s", summary)
	}
	if !strings.Contains(summary, "(issue: gt-1234)") {
		t.Errorf("summary should contain issue ID, got: %s", summary)
	}
	if !strings.Contains(summary, "Topic: Git conflict") {
		t.Errorf("summary should contain topic, got: %s", summary)
	}
	if !strings.Contains(summary, "Problem: Merge conflict in main.go") {
		t.Errorf("summary should contain problem, got: %s", summary)
	}
	if !strings.Contains(summary, "Tried: Attempted rebase") {
		t.Errorf("summary should contain tried, got: %s", summary)
	}
	if !strings.Contains(summary, "Requested: 2026-02-28") {
		t.Errorf("summary should contain timestamp, got: %s", summary)
	}
}

func TestFormatHelpSummary_MinimalPayload(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Agent:   "gastown/polecats/furiosa",
		Problem: "Tests fail on CI",
	}

	summary := FormatHelpSummary(payload)

	if !strings.Contains(summary, "HELP REQUEST from gastown/polecats/furiosa") {
		t.Errorf("summary should contain agent name, got: %s", summary)
	}
	if strings.Contains(summary, "issue:") {
		t.Errorf("summary should not contain issue line when empty, got: %s", summary)
	}
	if strings.Contains(summary, "Topic:") {
		t.Errorf("summary should not contain topic line when empty, got: %s", summary)
	}
	if !strings.Contains(summary, "Problem: Tests fail on CI") {
		t.Errorf("summary should contain problem, got: %s", summary)
	}
	if strings.Contains(summary, "Tried:") {
		t.Errorf("summary should not contain tried line when empty, got: %s", summary)
	}
	if strings.Contains(summary, "Requested:") {
		t.Errorf("summary should not contain timestamp when zero, got: %s", summary)
	}
}

// --- Agent state and exit type constants (gt-x7t9) ---

func TestAgentStateConstants(t *testing.T) {
	t.Parallel()
	// Verify all expected agent states are defined
	states := map[AgentState]string{
		AgentStateRunning:   "running",
		AgentStateIdle:      "idle",
		AgentStateDone:      "done",
		AgentStateStuck:     "stuck",
		AgentStateEscalated: "escalated",
		AgentStateSpawning:  "spawning",
		AgentStateWorking:   "working",
		AgentStateNuked:     "nuked",
	}
	for state, expected := range states {
		if string(state) != expected {
			t.Errorf("AgentState %q = %q, want %q", expected, string(state), expected)
		}
	}
}

func TestExitTypeConstants(t *testing.T) {
	t.Parallel()
	// Verify all expected exit types are defined and match PolecatDonePayload.Exit values
	types := map[ExitType]string{
		ExitTypeCompleted:     "COMPLETED",
		ExitTypeEscalated:     "ESCALATED",
		ExitTypeDeferred:      "DEFERRED",
		ExitTypePhaseComplete: "PHASE_COMPLETE",
	}
	for exitType, expected := range types {
		if string(exitType) != expected {
			t.Errorf("ExitType %q = %q, want %q", expected, string(exitType), expected)
		}
	}
}

func TestExitTypeMatchesPolecatDonePayload(t *testing.T) {
	t.Parallel()
	// The ExitType constants must match values parsed by ParsePolecatDone
	subject := "POLECAT_DONE nux"

	for _, exit := range []ExitType{ExitTypeCompleted, ExitTypeEscalated, ExitTypeDeferred, ExitTypePhaseComplete} {
		body := "Exit: " + string(exit)
		payload, err := ParsePolecatDone(subject, body)
		if err != nil {
			t.Fatalf("ParsePolecatDone() for exit %q: %v", exit, err)
		}
		if payload.Exit != string(exit) {
			t.Errorf("ParsePolecatDone Exit = %q, want %q", payload.Exit, string(exit))
		}
	}
}
