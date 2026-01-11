package protocol

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestParseMessageType(t *testing.T) {
	tests := []struct {
		subject  string
		expected MessageType
	}{
		{"MERGE_READY nux", TypeMergeReady},
		{"MERGED Toast", TypeMerged},
		{"MERGE_FAILED ace", TypeMergeFailed},
		{"REWORK_REQUEST valkyrie", TypeReworkRequest},
		{"MERGE_READY", TypeMergeReady}, // no polecat name
		{"Unknown subject", ""},
		{"", ""},
		{"  MERGE_READY nux  ", TypeMergeReady}, // with whitespace
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			result := ParseMessageType(tt.subject)
			if result != tt.expected {
				t.Errorf("ParseMessageType(%q) = %q, want %q", tt.subject, result, tt.expected)
			}
		})
	}
}

func TestExtractPolecat(t *testing.T) {
	tests := []struct {
		subject  string
		expected string
	}{
		{"MERGE_READY nux", "nux"},
		{"MERGED Toast", "Toast"},
		{"MERGE_FAILED ace", "ace"},
		{"REWORK_REQUEST valkyrie", "valkyrie"},
		{"MERGE_READY", ""},
		{"", ""},
		{"  MERGE_READY nux  ", "nux"},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			result := ExtractPolecat(tt.subject)
			if result != tt.expected {
				t.Errorf("ExtractPolecat(%q) = %q, want %q", tt.subject, result, tt.expected)
			}
		})
	}
}

func TestIsProtocolMessage(t *testing.T) {
	tests := []struct {
		subject  string
		expected bool
	}{
		{"MERGE_READY nux", true},
		{"MERGED Toast", true},
		{"MERGE_FAILED ace", true},
		{"REWORK_REQUEST valkyrie", true},
		{"Unknown subject", false},
		{"", false},
		{"Hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			result := IsProtocolMessage(tt.subject)
			if result != tt.expected {
				t.Errorf("IsProtocolMessage(%q) = %v, want %v", tt.subject, result, tt.expected)
			}
		})
	}
}

func TestNewMergeReadyMessage(t *testing.T) {
	msg := NewMergeReadyMessage("gastown", "nux", "polecat/nux/gt-abc", "gt-abc")

	if msg.Subject != "MERGE_READY nux" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "MERGE_READY nux")
	}
	if msg.From != "gastown/witness" {
		t.Errorf("From = %q, want %q", msg.From, "gastown/witness")
	}
	if msg.To != "gastown/refinery" {
		t.Errorf("To = %q, want %q", msg.To, "gastown/refinery")
	}
	if msg.Priority != mail.PriorityHigh {
		t.Errorf("Priority = %q, want %q", msg.Priority, mail.PriorityHigh)
	}
	if !strings.Contains(msg.Body, "Branch: polecat/nux/gt-abc") {
		t.Errorf("Body missing branch: %s", msg.Body)
	}
	if !strings.Contains(msg.Body, "Issue: gt-abc") {
		t.Errorf("Body missing issue: %s", msg.Body)
	}
}

func TestNewMergedMessage(t *testing.T) {
	msg := NewMergedMessage("gastown", "nux", "polecat/nux/gt-abc", "gt-abc", "main", "abc123")

	if msg.Subject != "MERGED nux" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "MERGED nux")
	}
	if msg.From != "gastown/refinery" {
		t.Errorf("From = %q, want %q", msg.From, "gastown/refinery")
	}
	if msg.To != "gastown/witness" {
		t.Errorf("To = %q, want %q", msg.To, "gastown/witness")
	}
	if !strings.Contains(msg.Body, "Merge-Commit: abc123") {
		t.Errorf("Body missing merge commit: %s", msg.Body)
	}
}

func TestNewMergeFailedMessage(t *testing.T) {
	msg := NewMergeFailedMessage("gastown", "nux", "polecat/nux/gt-abc", "gt-abc", "main", "tests", "Test failed")

	if msg.Subject != "MERGE_FAILED nux" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "MERGE_FAILED nux")
	}
	if !strings.Contains(msg.Body, "Failure-Type: tests") {
		t.Errorf("Body missing failure type: %s", msg.Body)
	}
	if !strings.Contains(msg.Body, "Error: Test failed") {
		t.Errorf("Body missing error: %s", msg.Body)
	}
}

func TestNewReworkRequestMessage(t *testing.T) {
	conflicts := []string{"file1.go", "file2.go"}
	msg := NewReworkRequestMessage("gastown", "nux", "polecat/nux/gt-abc", "gt-abc", "main", conflicts)

	if msg.Subject != "REWORK_REQUEST nux" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "REWORK_REQUEST nux")
	}
	if !strings.Contains(msg.Body, "Conflict-Files: file1.go, file2.go") {
		t.Errorf("Body missing conflict files: %s", msg.Body)
	}
	if !strings.Contains(msg.Body, "git rebase origin/main") {
		t.Errorf("Body missing rebase instructions: %s", msg.Body)
	}
}

func TestParseMergeReadyPayload(t *testing.T) {
	body := `Branch: polecat/nux/gt-abc
Issue: gt-abc
Polecat: nux
Rig: gastown
Verified: clean git state`

	payload := ParseMergeReadyPayload(body)

	if payload.Branch != "polecat/nux/gt-abc" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "polecat/nux/gt-abc")
	}
	if payload.Issue != "gt-abc" {
		t.Errorf("Issue = %q, want %q", payload.Issue, "gt-abc")
	}
	if payload.Polecat != "nux" {
		t.Errorf("Polecat = %q, want %q", payload.Polecat, "nux")
	}
	if payload.Rig != "gastown" {
		t.Errorf("Rig = %q, want %q", payload.Rig, "gastown")
	}
}

func TestParseMergedPayload(t *testing.T) {
	ts := time.Now().Format(time.RFC3339)
	body := `Branch: polecat/nux/gt-abc
Issue: gt-abc
Polecat: nux
Rig: gastown
Target: main
Merged-At: ` + ts + `
Merge-Commit: abc123`

	payload := ParseMergedPayload(body)

	if payload.Branch != "polecat/nux/gt-abc" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "polecat/nux/gt-abc")
	}
	if payload.MergeCommit != "abc123" {
		t.Errorf("MergeCommit = %q, want %q", payload.MergeCommit, "abc123")
	}
	if payload.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want %q", payload.TargetBranch, "main")
	}
}

func TestHandlerRegistry(t *testing.T) {
	registry := NewHandlerRegistry()

	handled := false
	registry.Register(TypeMergeReady, func(msg *mail.Message) error {
		handled = true
		return nil
	})

	msg := &mail.Message{Subject: "MERGE_READY nux"}

	if !registry.CanHandle(msg) {
		t.Error("Registry should be able to handle MERGE_READY message")
	}

	if err := registry.Handle(msg); err != nil {
		t.Errorf("Handle returned error: %v", err)
	}

	if !handled {
		t.Error("Handler was not called")
	}

	// Test unregistered message type
	unknownMsg := &mail.Message{Subject: "UNKNOWN message"}
	if registry.CanHandle(unknownMsg) {
		t.Error("Registry should not handle unknown message type")
	}
}

func TestWrapWitnessHandlers(t *testing.T) {
	handler := &mockWitnessHandler{}
	registry := WrapWitnessHandlers(handler)

	// Test MERGED
	mergedMsg := &mail.Message{
		Subject: "MERGED nux",
		Body:    "Branch: polecat/nux\nIssue: gt-abc\nPolecat: nux\nRig: gastown\nTarget: main",
	}
	if err := registry.Handle(mergedMsg); err != nil {
		t.Errorf("HandleMerged error: %v", err)
	}
	if !handler.mergedCalled {
		t.Error("HandleMerged was not called")
	}

	// Test MERGE_FAILED
	failedMsg := &mail.Message{
		Subject: "MERGE_FAILED nux",
		Body:    "Branch: polecat/nux\nIssue: gt-abc\nPolecat: nux\nRig: gastown\nTarget: main\nFailure-Type: tests\nError: failed",
	}
	if err := registry.Handle(failedMsg); err != nil {
		t.Errorf("HandleMergeFailed error: %v", err)
	}
	if !handler.failedCalled {
		t.Error("HandleMergeFailed was not called")
	}

	// Test REWORK_REQUEST
	reworkMsg := &mail.Message{
		Subject: "REWORK_REQUEST nux",
		Body:    "Branch: polecat/nux\nIssue: gt-abc\nPolecat: nux\nRig: gastown\nTarget: main",
	}
	if err := registry.Handle(reworkMsg); err != nil {
		t.Errorf("HandleReworkRequest error: %v", err)
	}
	if !handler.reworkCalled {
		t.Error("HandleReworkRequest was not called")
	}
}

func TestWrapRefineryHandlers(t *testing.T) {
	handler := &mockRefineryHandler{}
	registry := WrapRefineryHandlers(handler)

	msg := &mail.Message{
		Subject: "MERGE_READY nux",
		Body:    "Branch: polecat/nux\nIssue: gt-abc\nPolecat: nux\nRig: gastown",
	}

	if err := registry.Handle(msg); err != nil {
		t.Errorf("HandleMergeReady error: %v", err)
	}
	if !handler.readyCalled {
		t.Error("HandleMergeReady was not called")
	}
}

func TestDefaultWitnessHandler(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewWitnessHandler("gastown", tmpDir)

	// Capture output
	var buf bytes.Buffer
	handler.SetOutput(&buf)

	// Test HandleMerged
	mergedPayload := &MergedPayload{
		Branch:       "polecat/nux/gt-abc",
		Issue:        "gt-abc",
		Polecat:      "nux",
		Rig:          "gastown",
		TargetBranch: "main",
		MergeCommit:  "abc123",
	}
	if err := handler.HandleMerged(mergedPayload); err != nil {
		t.Errorf("HandleMerged error: %v", err)
	}
	if !strings.Contains(buf.String(), "MERGED received") {
		t.Errorf("Output missing expected text: %s", buf.String())
	}

	// Test HandleMergeFailed
	buf.Reset()
	failedPayload := &MergeFailedPayload{
		Branch:       "polecat/nux/gt-abc",
		Issue:        "gt-abc",
		Polecat:      "nux",
		Rig:          "gastown",
		TargetBranch: "main",
		FailureType:  "tests",
		Error:        "Test failed",
	}
	if err := handler.HandleMergeFailed(failedPayload); err != nil {
		t.Errorf("HandleMergeFailed error: %v", err)
	}
	if !strings.Contains(buf.String(), "MERGE_FAILED received") {
		t.Errorf("Output missing expected text: %s", buf.String())
	}

	// Test HandleReworkRequest
	buf.Reset()
	reworkPayload := &ReworkRequestPayload{
		Branch:        "polecat/nux/gt-abc",
		Issue:         "gt-abc",
		Polecat:       "nux",
		Rig:           "gastown",
		TargetBranch:  "main",
		ConflictFiles: []string{"file1.go"},
	}
	if err := handler.HandleReworkRequest(reworkPayload); err != nil {
		t.Errorf("HandleReworkRequest error: %v", err)
	}
	if !strings.Contains(buf.String(), "REWORK_REQUEST received") {
		t.Errorf("Output missing expected text: %s", buf.String())
	}
}

// Mock handlers for testing

type mockWitnessHandler struct {
	mergedCalled bool
	failedCalled bool
	reworkCalled bool
}

func (m *mockWitnessHandler) HandleMerged(payload *MergedPayload) error {
	m.mergedCalled = true
	return nil
}

func (m *mockWitnessHandler) HandleMergeFailed(payload *MergeFailedPayload) error {
	m.failedCalled = true
	return nil
}

func (m *mockWitnessHandler) HandleReworkRequest(payload *ReworkRequestPayload) error {
	m.reworkCalled = true
	return nil
}

type mockRefineryHandler struct {
	readyCalled bool
}

func (m *mockRefineryHandler) HandleMergeReady(payload *MergeReadyPayload) error {
	m.readyCalled = true
	return nil
}
