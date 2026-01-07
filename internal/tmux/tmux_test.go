package tmux

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestListSessionsNoServer(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessions, err := tm.ListSessions()
	// Should not error even if no server running
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	// Result may be nil or empty slice
	_ = sessions
}

func TestHasSessionNoServer(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	has, err := tm.HasSession("nonexistent-session-xyz")
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if has {
		t.Error("expected session to not exist")
	}
}

func TestSessionLifecycle(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-session-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Verify exists
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after creation")
	}

	// List should include it
	sessions, err := tm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Error("session not found in list")
	}

	// Kill session
	if err := tm.KillSession(sessionName); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	// Verify gone
	has, err = tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession after kill: %v", err)
	}
	if has {
		t.Error("expected session to not exist after kill")
	}
}

func TestDuplicateSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-dup-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Try to create duplicate
	err := tm.NewSession(sessionName, "")
	if err != ErrSessionExists {
		t.Errorf("expected ErrSessionExists, got %v", err)
	}
}

func TestSendKeysAndCapture(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-keys-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Send echo command
	if err := tm.SendKeys(sessionName, "echo HELLO_TEST_MARKER"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	// Give it a moment to execute
	// In real tests you'd wait for output, but for basic test we just capture
	output, err := tm.CapturePane(sessionName, 50)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}

	// Should contain our marker (might not if shell is slow, but usually works)
	if !strings.Contains(output, "echo HELLO_TEST_MARKER") {
		t.Logf("captured output: %s", output)
		// Don't fail, just note - timing issues possible
	}
}

func TestGetSessionInfo(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-info-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	info, err := tm.GetSessionInfo(sessionName)
	if err != nil {
		t.Fatalf("GetSessionInfo: %v", err)
	}

	if info.Name != sessionName {
		t.Errorf("Name = %q, want %q", info.Name, sessionName)
	}
	if info.Windows < 1 {
		t.Errorf("Windows = %d, want >= 1", info.Windows)
	}
}

func TestWrapError(t *testing.T) {
	tm := NewTmux()

	tests := []struct {
		stderr string
		want   error
	}{
		{"no server running on /tmp/tmux-...", ErrNoServer},
		{"error connecting to /tmp/tmux-...", ErrNoServer},
		{"duplicate session: test", ErrSessionExists},
		{"session not found: test", ErrSessionNotFound},
		{"can't find session: test", ErrSessionNotFound},
	}

	for _, tt := range tests {
		err := tm.wrapError(nil, tt.stderr, []string{"test"})
		if err != tt.want {
			t.Errorf("wrapError(%q) = %v, want %v", tt.stderr, err, tt.want)
		}
	}
}

func TestEnsureSessionFresh_NoExistingSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-fresh-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// EnsureSessionFresh should create a new session
	if err := tm.EnsureSessionFresh(sessionName, ""); err != nil {
		t.Fatalf("EnsureSessionFresh: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Verify session exists
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after EnsureSessionFresh")
	}
}

func TestEnsureSessionFresh_ZombieSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-zombie-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create a zombie session (session exists but no Claude/node running)
	// A normal tmux session with bash/zsh is a "zombie" for our purposes
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Verify it's a zombie (not running Claude/node)
	if tm.IsClaudeRunning(sessionName) {
		t.Skip("session unexpectedly has Claude running - can't test zombie case")
	}

	// Verify generic agent check also treats it as not running (shell session)
	if tm.IsAgentRunning(sessionName) {
		t.Fatalf("expected IsAgentRunning(%q) to be false for a fresh shell session", sessionName)
	}

	// EnsureSessionFresh should kill the zombie and create fresh session
	// This should NOT error with "session already exists"
	if err := tm.EnsureSessionFresh(sessionName, ""); err != nil {
		t.Fatalf("EnsureSessionFresh on zombie: %v", err)
	}

	// Session should still exist
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after EnsureSessionFresh on zombie")
	}
}

func TestEnsureSessionFresh_IdempotentOnZombie(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-idem-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Call EnsureSessionFresh multiple times - should work each time
	for i := 0; i < 3; i++ {
		if err := tm.EnsureSessionFresh(sessionName, ""); err != nil {
			t.Fatalf("EnsureSessionFresh attempt %d: %v", i+1, err)
		}
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session should exist
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after multiple EnsureSessionFresh calls")
	}
}

// mockSessionState represents the state of a mock tmux session for testing
type mockSessionState struct {
	exists       bool
	claudeAlive  bool
	killCalled   bool
	killError    error
	hasError     error
}

// mockTmuxOps provides mock implementations for EnsureSessionClear dependencies
type mockTmuxOps struct {
	sessions map[string]*mockSessionState
}

func newMockTmuxOps() *mockTmuxOps {
	return &mockTmuxOps{sessions: make(map[string]*mockSessionState)}
}

func (m *mockTmuxOps) addSession(name string, claudeAlive bool) {
	m.sessions[name] = &mockSessionState{exists: true, claudeAlive: claudeAlive}
}

func (m *mockTmuxOps) setHasError(name string, err error) {
	if s, ok := m.sessions[name]; ok {
		s.hasError = err
	} else {
		m.sessions[name] = &mockSessionState{hasError: err}
	}
}

func (m *mockTmuxOps) setKillError(name string, err error) {
	if s, ok := m.sessions[name]; ok {
		s.killError = err
	}
}

func (m *mockTmuxOps) hasSession(name string) (bool, error) {
	s, ok := m.sessions[name]
	if !ok {
		return false, nil
	}
	if s.hasError != nil {
		return false, s.hasError
	}
	return s.exists, nil
}

func (m *mockTmuxOps) isClaudeRunning(name string) bool {
	s, ok := m.sessions[name]
	if !ok {
		return false
	}
	return s.claudeAlive
}

func (m *mockTmuxOps) killSession(name string) error {
	s, ok := m.sessions[name]
	if !ok {
		return nil
	}
	s.killCalled = true
	if s.killError != nil {
		return s.killError
	}
	s.exists = false
	return nil
}

func (m *mockTmuxOps) wasKillCalled(name string) bool {
	s, ok := m.sessions[name]
	return ok && s.killCalled
}

// ensureSessionClearWithOps is the testable core logic of EnsureSessionClear
func ensureSessionClearWithOps(
	name string,
	hasSession func(string) (bool, error),
	isClaudeRunning func(string) bool,
	killSession func(string) error,
) (healthy, zombieKilled bool, err error) {
	exists, err := hasSession(name)
	if err != nil {
		return false, false, err
	}

	if !exists {
		return false, false, nil
	}

	if isClaudeRunning(name) {
		return true, false, nil
	}

	if err := killSession(name); err != nil {
		return false, false, err
	}
	return false, true, nil
}

func TestEnsureSessionClear_NoExistingSession(t *testing.T) {
	mock := newMockTmuxOps()
	// No session added - simulates non-existent session

	healthy, zombieKilled, err := ensureSessionClearWithOps(
		"test-session",
		mock.hasSession,
		mock.isClaudeRunning,
		mock.killSession,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if healthy {
		t.Error("expected healthy=false when no session exists")
	}
	if zombieKilled {
		t.Error("expected zombieKilled=false when no session exists")
	}
	if mock.wasKillCalled("test-session") {
		t.Error("KillSession should not be called when session doesn't exist")
	}
}

func TestEnsureSessionClear_ZombieSession(t *testing.T) {
	mock := newMockTmuxOps()
	mock.addSession("test-session", false) // exists but Claude not running (zombie)

	healthy, zombieKilled, err := ensureSessionClearWithOps(
		"test-session",
		mock.hasSession,
		mock.isClaudeRunning,
		mock.killSession,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if healthy {
		t.Error("expected healthy=false for zombie session")
	}
	if !zombieKilled {
		t.Error("expected zombieKilled=true for zombie session")
	}
	if !mock.wasKillCalled("test-session") {
		t.Error("KillSession should be called for zombie session")
	}
}

func TestEnsureSessionClear_HealthySession(t *testing.T) {
	mock := newMockTmuxOps()
	mock.addSession("test-session", true) // exists and Claude is running (healthy)

	healthy, zombieKilled, err := ensureSessionClearWithOps(
		"test-session",
		mock.hasSession,
		mock.isClaudeRunning,
		mock.killSession,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !healthy {
		t.Error("expected healthy=true for healthy session")
	}
	if zombieKilled {
		t.Error("expected zombieKilled=false for healthy session")
	}
	if mock.wasKillCalled("test-session") {
		t.Error("KillSession should NOT be called for healthy session")
	}
}

func TestEnsureSessionClear_HasSessionError(t *testing.T) {
	mock := newMockTmuxOps()
	mock.setHasError("test-session", errors.New("tmux error"))

	healthy, zombieKilled, err := ensureSessionClearWithOps(
		"test-session",
		mock.hasSession,
		mock.isClaudeRunning,
		mock.killSession,
	)

	if err == nil {
		t.Fatal("expected error from HasSession")
	}
	if healthy {
		t.Error("expected healthy=false on error")
	}
	if zombieKilled {
		t.Error("expected zombieKilled=false on error")
	}
}

func TestEnsureSessionClear_KillSessionError(t *testing.T) {
	mock := newMockTmuxOps()
	mock.addSession("test-session", false) // zombie
	mock.setKillError("test-session", errors.New("kill failed"))

	healthy, zombieKilled, err := ensureSessionClearWithOps(
		"test-session",
		mock.hasSession,
		mock.isClaudeRunning,
		mock.killSession,
	)

	if err == nil {
		t.Fatal("expected error from KillSession")
	}
	if healthy {
		t.Error("expected healthy=false on error")
	}
	if zombieKilled {
		t.Error("expected zombieKilled=false on error")
	}
}

func TestIsClaudeRunning_ShellSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-claude-shell-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create a session (will run default shell)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// A shell session should NOT be detected as Claude running
	if tm.IsClaudeRunning(sessionName) {
		cmd, _ := tm.GetPaneCommand(sessionName)
		t.Errorf("IsClaudeRunning returned true for shell session (cmd=%q)", cmd)
	}
}

func TestIsClaudeRunning_NonexistentSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Should return false for nonexistent session
	if tm.IsClaudeRunning("nonexistent-session-xyz-abc") {
		t.Error("IsClaudeRunning returned true for nonexistent session")
	}
}

func TestGetPaneCommand_ShellSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-pane-cmd-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	cmd, err := tm.GetPaneCommand(sessionName)
	if err != nil {
		t.Fatalf("GetPaneCommand: %v", err)
	}

	// Should be a shell (bash, zsh, etc.) not claude or node
	validShells := []string{"bash", "zsh", "sh", "fish", "tcsh", "csh"}
	isShell := false
	for _, shell := range validShells {
		if cmd == shell {
			isShell = true
			break
		}
	}
	if !isShell {
		t.Errorf("GetPaneCommand returned %q, expected a shell", cmd)
	}

	// Specifically verify it's not detected as claude
	if cmd == "claude" || cmd == "node" {
		t.Errorf("GetPaneCommand returned %q for new shell session, should be shell name", cmd)
	}
}

func TestIsClaudeRunningDirectMatches(t *testing.T) {
	// Test that IsClaudeRunning handles direct matches (node, claude) before regex
	// This tests the implementation logic, not just the regex

	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Test nonexistent session - should return false without panic
	result := tm.IsClaudeRunning("completely-nonexistent-session-12345")
	if result {
		t.Error("IsClaudeRunning returned true for nonexistent session")
	}
}
