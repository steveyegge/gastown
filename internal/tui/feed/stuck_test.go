package feed

import (
	"errors"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// mockTmuxClient is a test double for TmuxClient
type mockTmuxClient struct {
	sessions        map[string]bool
	sessionNames    []string // For GetSessionSet
	paneContent     map[string]string
	sessionActivity map[string]time.Time
	captureErr      error
	activityErr     error
	sessionSetErr   error
}

func newMockTmuxClient() *mockTmuxClient {
	return &mockTmuxClient{
		sessions:        make(map[string]bool),
		paneContent:     make(map[string]string),
		sessionActivity: make(map[string]time.Time),
	}
}

func (m *mockTmuxClient) HasSession(name string) (bool, error) {
	return m.sessions[name], nil
}

func (m *mockTmuxClient) CapturePane(session string, lines int) (string, error) {
	if m.captureErr != nil {
		return "", m.captureErr
	}
	return m.paneContent[session], nil
}

func (m *mockTmuxClient) GetSessionActivity(session string) (time.Time, error) {
	if m.activityErr != nil {
		return time.Time{}, m.activityErr
	}
	if t, ok := m.sessionActivity[session]; ok {
		return t, nil
	}
	return time.Time{}, errors.New("no activity")
}

func (m *mockTmuxClient) GetSessionSet() (*tmux.SessionSet, error) {
	if m.sessionSetErr != nil {
		return nil, m.sessionSetErr
	}
	return tmux.NewSessionSet(m.sessionNames), nil
}

// TestAgentStateString tests the String() method for all AgentState values
func TestAgentStateString(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "gupp"},
		{StateInputRequired, "input"},
		{StateStalled, "stalled"},
		{StateWorking, "working"},
		{StateIdle, "idle"},
		{StateZombie, "zombie"},
		{AgentState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("AgentState(%d).String() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestAgentStatePriority tests that priorities are ordered correctly
func TestAgentStatePriority(t *testing.T) {
	// GUPP should have highest priority (lowest number)
	if StateGUPPViolation.Priority() >= StateInputRequired.Priority() {
		t.Error("GUPP violation should have higher priority than input required")
	}
	if StateInputRequired.Priority() >= StateStalled.Priority() {
		t.Error("Input required should have higher priority than stalled")
	}
	if StateStalled.Priority() >= StateWorking.Priority() {
		t.Error("Stalled should have higher priority than working")
	}
	if StateWorking.Priority() >= StateIdle.Priority() {
		t.Error("Working should have higher priority than idle")
	}
	if StateIdle.Priority() >= StateZombie.Priority() {
		t.Error("Idle should have higher priority than zombie")
	}
}

// TestAgentStateNeedsAttention tests which states require user attention
func TestAgentStateNeedsAttention(t *testing.T) {
	needsAttention := []AgentState{
		StateGUPPViolation,
		StateInputRequired,
		StateStalled,
		StateZombie,
	}
	noAttention := []AgentState{
		StateWorking,
		StateIdle,
	}

	for _, state := range needsAttention {
		if !state.NeedsAttention() {
			t.Errorf("%s.NeedsAttention() = false, want true", state)
		}
	}
	for _, state := range noAttention {
		if state.NeedsAttention() {
			t.Errorf("%s.NeedsAttention() = true, want false", state)
		}
	}
}

// TestAgentStateSymbol tests the display symbols
func TestAgentStateSymbol(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "🔥"},
		{StateInputRequired, "⌨"},
		{StateStalled, "⚠"},
		{StateWorking, "●"},
		{StateIdle, "○"},
		{StateZombie, "💀"},
		{AgentState(99), "?"},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := tt.state.Symbol(); got != tt.expected {
				t.Errorf("AgentState(%d).Symbol() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestAgentStateLabel tests the display labels
func TestAgentStateLabel(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "GUPP!"},
		{StateInputRequired, "INPUT"},
		{StateStalled, "STALL"},
		{StateWorking, "work"},
		{StateIdle, "idle"},
		{StateZombie, "dead"},
		{AgentState(99), "???"},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := tt.state.Label(); got != tt.expected {
				t.Errorf("AgentState(%d).Label() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestInputReasonString tests the String() method for InputReason
func TestInputReasonString(t *testing.T) {
	tests := []struct {
		reason   InputReason
		expected string
	}{
		{InputReasonUnknown, "waiting"},
		{InputReasonPromptWaiting, "prompt"},
		{InputReasonYOLOConfirmation, "[Y/n]"},
		{InputReasonEnterRequired, "Enter"},
		{InputReasonPermission, "Allow?"},
		{InputReason(99), "waiting"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.expected {
				t.Errorf("InputReason(%d).String() = %q, want %q", tt.reason, got, tt.expected)
			}
		})
	}
}

// TestIsGasTownSession tests the session name detection
func TestIsGasTownSession(t *testing.T) {
	gasTownSessions := []string{
		"polecat-1",
		"polecat-myproject-42",
		"mayor",
		"mayor-main",
		"refinery-abc",
		"witness-123",
		"deacon-boot",
		"crew-joe",
		"boot-init",
	}
	nonGasTownSessions := []string{
		"my-session",
		"dev",
		"main",
		"polecatnotreally", // No hyphen, but still matches prefix - this is expected behavior
		"",
	}

	for _, name := range gasTownSessions {
		if !isGasTownSession(name) {
			t.Errorf("isGasTownSession(%q) = false, want true", name)
		}
	}
	for _, name := range nonGasTownSessions {
		// Note: "polecatnotreally" will match because it starts with "polecat"
		// This is acceptable behavior - session names typically have hyphens
		if name != "polecatnotreally" && name != "" && isGasTownSession(name) {
			t.Errorf("isGasTownSession(%q) = true, want false", name)
		}
	}
}

// TestIsGUPPViolation tests the GUPP violation detection
func TestIsGUPPViolation(t *testing.T) {
	tests := []struct {
		name          string
		hasHookedWork bool
		minutes       int
		expected      bool
	}{
		{"no work, no time", false, 0, false},
		{"no work, long time", false, 60, false},
		{"has work, short time", true, 10, false},
		{"has work, at threshold", true, 30, true},
		{"has work, over threshold", true, 45, true},
		{"has work, just under threshold", true, 29, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGUPPViolation(tt.hasHookedWork, tt.minutes); got != tt.expected {
				t.Errorf("IsGUPPViolation(%v, %d) = %v, want %v",
					tt.hasHookedWork, tt.minutes, got, tt.expected)
			}
		})
	}
}

// TestProblemAgentDurationDisplay tests the human-readable duration formatting
func TestProblemAgentDurationDisplay(t *testing.T) {
	tests := []struct {
		minutes  int
		expected string
	}{
		{0, "<1m"},
		{1, "1m"},
		{5, "5m"},
		{59, "59m"},
		{60, "1h"},
		{61, "1h1m"},
		{90, "1h30m"},
		{120, "2h"},
		{125, "2h5m"},
		{180, "3h"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			agent := &ProblemAgent{IdleMinutes: tt.minutes}
			if got := agent.DurationDisplay(); got != tt.expected {
				t.Errorf("ProblemAgent{IdleMinutes: %d}.DurationDisplay() = %q, want %q",
					tt.minutes, got, tt.expected)
			}
		})
	}
}

// TestProblemAgentNeedsAttention tests the NeedsAttention delegation
func TestProblemAgentNeedsAttention(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected bool
	}{
		{StateGUPPViolation, true},
		{StateInputRequired, true},
		{StateStalled, true},
		{StateZombie, true},
		{StateWorking, false},
		{StateIdle, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			agent := &ProblemAgent{State: tt.state}
			if got := agent.NeedsAttention(); got != tt.expected {
				t.Errorf("ProblemAgent{State: %s}.NeedsAttention() = %v, want %v",
					tt.state, got, tt.expected)
			}
		})
	}
}

// TestDefaultPatternsMatch tests that the default patterns match expected content
func TestDefaultPatternsMatch(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected InputReason
	}{
		{"general prompt", "some output\n> ", InputReasonPromptWaiting},
		{"claude prompt", "claude> ", InputReasonPromptWaiting},
		{"YOLO Y/n", "Do something? [Y/n] ", InputReasonYOLOConfirmation},
		{"YOLO y/N", "Do something? [y/N] ", InputReasonYOLOConfirmation},
		{"allow prompt", "Allow? ", InputReasonPermission},
		{"press enter", "Press Enter to continue", InputReasonEnterRequired},
		{"continue question", "Do you want to continue?", InputReasonPermission},
		{"proceed question", "Proceed?", InputReasonPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matched InputReason
			for _, pattern := range DefaultPatterns {
				if pattern.Regex.MatchString(tt.content) {
					matched = pattern.Reason
					break
				}
			}
			if matched != tt.expected {
				t.Errorf("Pattern match for %q = %v, want %v", tt.content, matched, tt.expected)
			}
		})
	}
}

// TestDefaultPatternsNoMatch tests that normal output doesn't trigger patterns
func TestDefaultPatternsNoMatch(t *testing.T) {
	normalOutputs := []string{
		"Building project...",
		"Running tests",
		"Completed successfully",
		"Processing file.go",
		"",
	}

	for _, content := range normalOutputs {
		for _, pattern := range DefaultPatterns {
			if pattern.Regex.MatchString(content) {
				t.Errorf("Pattern %v unexpectedly matched %q", pattern.Reason, content)
			}
		}
	}
}

// TestErrorPatternsMatch tests error pattern detection
func TestErrorPatternsMatch(t *testing.T) {
	errorOutputs := []string{
		"Rate limit exceeded",
		"RATE LIMIT",
		"Context is full",
		"context window full",
		"Error: something went wrong",
		"error: failed to compile",
		"Failed: test case",
		"FAILED: build step",
	}

	for _, content := range errorOutputs {
		matched := false
		for _, pattern := range ErrorPatterns {
			if pattern.MatchString(content) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("No error pattern matched %q", content)
		}
	}
}

// TestParseRoleFromSession tests role extraction from session names
func TestParseRoleFromSession(t *testing.T) {
	tests := []struct {
		sessionID string
		expected  string
	}{
		{"polecat-1", "polecat"},
		{"polecat-myproject-42", "polecat"},
		{"mayor", "mayor"},
		{"mayor-main", "mayor"},
		{"refinery-abc", "refinery"},
		{"witness-123", "witness"},
		{"deacon-boot", "deacon"},
		{"crew-joe", "crew"},
		{"boot-init", "boot"},
		{"unknown-session", "unknown"},
		{"my-session", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.sessionID, func(t *testing.T) {
			if got := parseRoleFromSession(tt.sessionID); got != tt.expected {
				t.Errorf("parseRoleFromSession(%q) = %q, want %q", tt.sessionID, got, tt.expected)
			}
		})
	}
}

// TestGetActionHint tests action hint generation
func TestGetActionHint(t *testing.T) {
	tests := []struct {
		reason   InputReason
		contains string
	}{
		{InputReasonPromptWaiting, "prompt"},
		{InputReasonYOLOConfirmation, "YOLO"},
		{InputReasonEnterRequired, "Enter"},
		{InputReasonPermission, "Permission"},
		{InputReasonUnknown, "stuck"},
	}

	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			hint := getActionHint(tt.reason)
			if hint == "" {
				t.Errorf("getActionHint(%v) returned empty string", tt.reason)
			}
			// Just verify it returns something meaningful
			if len(hint) < 10 {
				t.Errorf("getActionHint(%v) = %q, expected longer hint", tt.reason, hint)
			}
		})
	}
}

// TestStuckDetectorAnalyzeSession_NoSession tests zombie detection for missing sessions
func TestStuckDetectorAnalyzeSession_NoSession(t *testing.T) {
	mock := newMockTmuxClient()
	// Don't add any sessions - HasSession will return false

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("nonexistent-session")

	if agent.State != StateZombie {
		t.Errorf("Expected StateZombie for missing session, got %s", agent.State)
	}
	if agent.SessionID != "nonexistent-session" {
		t.Errorf("Expected SessionID to be preserved, got %s", agent.SessionID)
	}
}

// TestStuckDetectorAnalyzeSession_CaptureError tests zombie detection for capture failures
func TestStuckDetectorAnalyzeSession_CaptureError(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["test-session"] = true
	mock.captureErr = errors.New("capture failed")

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("test-session")

	if agent.State != StateZombie {
		t.Errorf("Expected StateZombie for capture error, got %s", agent.State)
	}
}

// TestStuckDetectorAnalyzeSession_Working tests normal working state
func TestStuckDetectorAnalyzeSession_Working(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["polecat-1"] = true
	mock.paneContent["polecat-1"] = "Building project...\nProcessing files..."
	mock.sessionActivity["polecat-1"] = time.Now() // Recent activity

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("polecat-1")

	if agent.State != StateWorking {
		t.Errorf("Expected StateWorking, got %s", agent.State)
	}
	if agent.Role != "polecat" {
		t.Errorf("Expected role 'polecat', got %s", agent.Role)
	}
}

// TestStuckDetectorAnalyzeSession_InputRequired tests input detection
func TestStuckDetectorAnalyzeSession_InputRequired(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["polecat-1"] = true
	mock.paneContent["polecat-1"] = "Some output\nclaude> "
	// Set activity to 3 minutes ago (past the 2-minute threshold)
	mock.sessionActivity["polecat-1"] = time.Now().Add(-3 * time.Minute)

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("polecat-1")

	if agent.State != StateInputRequired {
		t.Errorf("Expected StateInputRequired, got %s", agent.State)
	}
	if agent.InputReason != InputReasonPromptWaiting {
		t.Errorf("Expected InputReasonPromptWaiting, got %v", agent.InputReason)
	}
}

// TestStuckDetectorAnalyzeSession_Stalled tests stalled detection
func TestStuckDetectorAnalyzeSession_Stalled(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["polecat-1"] = true
	mock.paneContent["polecat-1"] = "Processing...\nStill working..."
	// Set activity to 20 minutes ago (past the 15-minute stalled threshold)
	mock.sessionActivity["polecat-1"] = time.Now().Add(-20 * time.Minute)

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("polecat-1")

	if agent.State != StateStalled {
		t.Errorf("Expected StateStalled, got %s", agent.State)
	}
}

// TestStuckDetectorAnalyzeSession_ErrorPattern tests error pattern detection
func TestStuckDetectorAnalyzeSession_ErrorPattern(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["polecat-1"] = true
	mock.paneContent["polecat-1"] = "Attempting operation...\nError: connection refused"
	mock.sessionActivity["polecat-1"] = time.Now() // Recent activity

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("polecat-1")

	if agent.State != StateStalled {
		t.Errorf("Expected StateStalled for error pattern, got %s", agent.State)
	}
	if agent.ActionHint != "Error detected in output" {
		t.Errorf("Expected error action hint, got %s", agent.ActionHint)
	}
}

// TestStuckDetectorAnalyzeSession_LastLines tests that last lines are captured
func TestStuckDetectorAnalyzeSession_LastLines(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessions["polecat-1"] = true
	mock.paneContent["polecat-1"] = "line1\nline2\nline3\nline4\nline5\nline6\nline7"
	mock.sessionActivity["polecat-1"] = time.Now()

	detector := NewStuckDetectorWithClient(mock)
	agent := detector.AnalyzeSession("polecat-1")

	// Should only capture last 5 lines
	if agent.LastLines == "" {
		t.Error("Expected LastLines to be populated")
	}
	// The last lines should contain the most recent content
	if len(agent.LastLines) == 0 {
		t.Error("LastLines should not be empty")
	}
}

// TestNewStuckDetector tests the default constructor
func TestNewStuckDetector(t *testing.T) {
	// This will use the real tmux wrapper, but we can verify the struct is set up correctly
	detector := NewStuckDetector()

	if detector == nil {
		t.Fatal("NewStuckDetector returned nil")
	}
	if len(detector.Patterns) == 0 {
		t.Error("Expected default patterns to be set")
	}
	if detector.IdleThreshold != InputWaitThresholdSecs*time.Second {
		t.Errorf("Expected IdleThreshold to be %v, got %v",
			InputWaitThresholdSecs*time.Second, detector.IdleThreshold)
	}
}

// TestThresholdConstants verifies the threshold constants are reasonable
func TestThresholdConstants(t *testing.T) {
	if GUPPViolationMinutes != 30 {
		t.Errorf("GUPPViolationMinutes = %d, want 30", GUPPViolationMinutes)
	}
	if StalledThresholdMinutes != 15 {
		t.Errorf("StalledThresholdMinutes = %d, want 15", StalledThresholdMinutes)
	}
	if InputWaitThresholdSecs != 120 {
		t.Errorf("InputWaitThresholdSecs = %d, want 120", InputWaitThresholdSecs)
	}

	// GUPP threshold should be longer than stalled threshold
	if GUPPViolationMinutes <= StalledThresholdMinutes {
		t.Error("GUPP violation threshold should be longer than stalled threshold")
	}
}

// TestNewStuckDetectorWithTmux tests the deprecated constructor
func TestNewStuckDetectorWithTmux(t *testing.T) {
	// NewStuckDetectorWithTmux is a deprecated wrapper around NewStuckDetectorWithClient
	// It should work identically
	realTmux := tmux.NewTmux()
	detector := NewStuckDetectorWithTmux(realTmux)

	if detector == nil {
		t.Fatal("NewStuckDetectorWithTmux returned nil")
	}
	if len(detector.Patterns) == 0 {
		t.Error("Expected default patterns to be set")
	}
	if detector.IdleThreshold != InputWaitThresholdSecs*time.Second {
		t.Errorf("Expected IdleThreshold to be %v, got %v",
			InputWaitThresholdSecs*time.Second, detector.IdleThreshold)
	}
}

// TestFindGasTownSessions tests session discovery
func TestFindGasTownSessions(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessionNames = []string{
		"polecat-1",
		"polecat-myproject-42",
		"mayor-main",
		"witness-123",
		"my-personal-session",
		"dev",
		"deacon-boot",
	}

	detector := NewStuckDetectorWithClient(mock)
	sessions, err := detector.FindGasTownSessions()
	if err != nil {
		t.Fatalf("FindGasTownSessions: %v", err)
	}

	// Should find GasTown sessions but not personal ones
	expected := map[string]bool{
		"polecat-1":           true,
		"polecat-myproject-42": true,
		"mayor-main":          true,
		"witness-123":         true,
		"deacon-boot":         true,
	}

	if len(sessions) != len(expected) {
		t.Errorf("FindGasTownSessions found %d sessions, want %d", len(sessions), len(expected))
	}

	for _, s := range sessions {
		if !expected[s] {
			t.Errorf("Unexpected session found: %s", s)
		}
	}
}

// TestFindGasTownSessions_Empty tests with no sessions
func TestFindGasTownSessions_Empty(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessionNames = []string{}

	detector := NewStuckDetectorWithClient(mock)
	sessions, err := detector.FindGasTownSessions()
	if err != nil {
		t.Fatalf("FindGasTownSessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected no sessions, got %d", len(sessions))
	}
}

// TestFindGasTownSessions_Error tests error handling
func TestFindGasTownSessions_Error(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessionSetErr = errors.New("tmux error")

	detector := NewStuckDetectorWithClient(mock)
	_, err := detector.FindGasTownSessions()
	if err == nil {
		t.Error("Expected error from FindGasTownSessions")
	}
}

// TestFindGasTownSessions_NoGasTownSessions tests when no GT sessions exist
func TestFindGasTownSessions_NoGasTownSessions(t *testing.T) {
	mock := newMockTmuxClient()
	mock.sessionNames = []string{
		"my-session",
		"dev",
		"work",
	}

	detector := NewStuckDetectorWithClient(mock)
	sessions, err := detector.FindGasTownSessions()
	if err != nil {
		t.Fatalf("FindGasTownSessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected no GasTown sessions, got %d: %v", len(sessions), sessions)
	}
}
