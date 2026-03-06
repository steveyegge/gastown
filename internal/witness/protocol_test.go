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

// --- AssessHelp tests (gt-td6p) ---

func TestAssessHelp_Emergency(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Security vulnerability found",
		Problem: "Exposed secret in logs",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryEmergency {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryEmergency)
	}
	if assessment.Severity != HelpSeverityCritical {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityCritical)
	}
	if assessment.SuggestTo != "overseer" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "overseer")
	}
}

func TestAssessHelp_Failed(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Database error",
		Problem: "Connection refused on port 3307",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryFailed {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryFailed)
	}
	if assessment.Severity != HelpSeverityHigh {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityHigh)
	}
	if assessment.SuggestTo != "deacon" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "deacon")
	}
}

func TestAssessHelp_Blocked(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Merge conflict in main.go",
		Problem: "Cannot proceed with rebase",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryBlocked {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryBlocked)
	}
	if assessment.Severity != HelpSeverityHigh {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityHigh)
	}
	if assessment.SuggestTo != "mayor" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "mayor")
	}
}

func TestAssessHelp_Decision(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Which approach for caching?",
		Problem: "Multiple options available, need guidance",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryDecision {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryDecision)
	}
	if assessment.Severity != HelpSeverityMedium {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityMedium)
	}
	if assessment.SuggestTo != "deacon" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "deacon")
	}
}

func TestAssessHelp_Lifecycle(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Polecat zombie detected",
		Problem: "Session dead but bead still in_progress",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryLifecycle {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryLifecycle)
	}
	if assessment.Severity != HelpSeverityMedium {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityMedium)
	}
	if assessment.SuggestTo != "witness" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "witness")
	}
}

func TestAssessHelp_DefaultHelp(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "Need guidance on implementation",
		Problem: "Not sure how to approach this feature",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryHelp {
		t.Errorf("Category = %q, want %q", assessment.Category, HelpCategoryHelp)
	}
	if assessment.Severity != HelpSeverityMedium {
		t.Errorf("Severity = %q, want %q", assessment.Severity, HelpSeverityMedium)
	}
	if assessment.SuggestTo != "deacon" {
		t.Errorf("SuggestTo = %q, want %q", assessment.SuggestTo, "deacon")
	}
	if assessment.Rationale == "" {
		t.Error("Rationale should not be empty")
	}
}

func TestAssessHelp_CaseInsensitive(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Topic:   "SECURITY issue found",
		Problem: "Possible BREACH in auth",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryEmergency {
		t.Errorf("Category = %q, want %q (case-insensitive match expected)", assessment.Category, HelpCategoryEmergency)
	}
}

func TestAssessHelp_PriorityOrder(t *testing.T) {
	t.Parallel()
	// Emergency keywords should take priority over blocked keywords
	payload := &HelpPayload{
		Topic:   "Data corruption causing blocked state",
		Problem: "Cannot proceed due to corrupted data",
	}
	assessment := AssessHelp(payload)
	if assessment.Category != HelpCategoryEmergency {
		t.Errorf("Category = %q, want %q (emergency should take priority over blocked)", assessment.Category, HelpCategoryEmergency)
	}
}

func TestFormatHelpSummary_WithAssessment(t *testing.T) {
	t.Parallel()
	payload := &HelpPayload{
		Agent:   "gastown/polecats/nux",
		Topic:   "Merge conflict",
		Problem: "Cannot rebase",
		Assessment: &HelpAssessment{
			Category:  HelpCategoryBlocked,
			Severity:  HelpSeverityHigh,
			SuggestTo: "mayor",
			Rationale: "matched keyword \"merge conflict\"",
		},
	}
	summary := FormatHelpSummary(payload)
	if !strings.Contains(summary, "Assessment:") {
		t.Errorf("summary should contain assessment line, got: %s", summary)
	}
	if !strings.Contains(summary, "blocked") {
		t.Errorf("summary should contain category, got: %s", summary)
	}
	if !strings.Contains(summary, "high") {
		t.Errorf("summary should contain severity, got: %s", summary)
	}
	if !strings.Contains(summary, "mayor") {
		t.Errorf("summary should contain suggested target, got: %s", summary)
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
