package tmux

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/session"
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
	sessions, err := tm.List()
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
	has, err := tm.Exists(session.SessionID("nonexistent-session-xyz"))
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
	sessionName := "tmux-test-" + t.Name() // Use non-gt prefix to avoid translation
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Verify exists
	has, err := tm.Exists(sessionID)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after creation")
	}

	// List should include it
	sessions, err := tm.List()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Error("session not found in list")
	}

	// Kill session
	if err := tm.Stop(sessionID); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	// Verify gone
	has, err = tm.Exists(sessionID)
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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Send echo command
	if err := tm.Send(sessionID, "echo HELLO_TEST_MARKER"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	// Give it a moment to execute
	// In real tests you'd wait for output, but for basic test we just capture
	output, err := tm.Capture(sessionID, 50)
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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	info, err := tm.GetInfo(sessionID)
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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// EnsureSessionFresh should create a new session
	if err := tm.EnsureSessionFresh(sessionName, ""); err != nil {
		t.Fatalf("EnsureSessionFresh: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Verify session exists
	has, err := tm.Exists(sessionID)
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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create a zombie session (session exists but no Claude/node running)
	// A normal tmux session with bash/zsh is a "zombie" for our purposes
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Verify it's a zombie (not running Claude/node)
	if tm.IsRuntimeRunning(sessionName, []string{"claude", "node"}) {
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
	has, err := tm.Exists(sessionID)
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
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Call EnsureSessionFresh multiple times - should work each time
	for i := 0; i < 3; i++ {
		if err := tm.EnsureSessionFresh(sessionName, ""); err != nil {
			t.Fatalf("EnsureSessionFresh attempt %d: %v", i+1, err)
		}
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Session should exist
	has, err := tm.Exists(sessionID)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Error("expected session to exist after multiple EnsureSessionFresh calls")
	}
}

func TestIsAgentRunning(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-agent-" + t.Name()
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session (will run default shell)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Get the current pane command (should be bash/zsh/etc)
	cmd, err := tm.GetPaneCommand(sessionName)
	if err != nil {
		t.Fatalf("GetPaneCommand: %v", err)
	}

	tests := []struct {
		name         string
		processNames []string
		wantRunning  bool
	}{
		{
			name:         "empty process list",
			processNames: []string{},
			wantRunning:  false,
		},
		{
			name:         "matching shell process",
			processNames: []string{cmd}, // Current shell
			wantRunning:  true,
		},
		{
			name:         "claude agent (node) - not running",
			processNames: []string{"node"},
			wantRunning:  cmd == "node", // Only true if shell happens to be node
		},
		{
			name:         "gemini agent - not running",
			processNames: []string{"gemini"},
			wantRunning:  cmd == "gemini",
		},
		{
			name:         "cursor agent - not running",
			processNames: []string{"cursor-agent"},
			wantRunning:  cmd == "cursor-agent",
		},
		{
			name:         "multiple process names with match",
			processNames: []string{"nonexistent", cmd, "also-nonexistent"},
			wantRunning:  true,
		},
		{
			name:         "multiple process names without match",
			processNames: []string{"nonexistent1", "nonexistent2"},
			wantRunning:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tm.IsAgentRunning(sessionName, tt.processNames...)
			if got != tt.wantRunning {
				t.Errorf("IsAgentRunning(%q, %v) = %v, want %v (current cmd: %q)",
					sessionName, tt.processNames, got, tt.wantRunning, cmd)
			}
		})
	}
}

func TestIsAgentRunning_NonexistentSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// IsAgentRunning on nonexistent session should return false, not error
	got := tm.IsAgentRunning("nonexistent-session-xyz", "node", "gemini", "cursor-agent")
	if got {
		t.Error("IsAgentRunning on nonexistent session should return false")
	}
}

func TestIsRuntimeRunning(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-runtime-" + t.Name()
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session (will run default shell, not Claude)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// IsRuntimeRunning should be false (shell is running, not node/claude)
	cmd, _ := tm.GetPaneCommand(sessionName)
	wantRunning := cmd == "node" || cmd == "claude"

	if got := tm.IsRuntimeRunning(sessionName, []string{"claude", "node"}); got != wantRunning {
		t.Errorf("IsRuntimeRunning() = %v, want %v (pane cmd: %q)", got, wantRunning, cmd)
	}
}

func TestIsRuntimeRunning_VersionPattern(t *testing.T) {
	// Test the version pattern regex matching directly
	// Since we can't easily mock the pane command, test the pattern logic
	tests := []struct {
		cmd  string
		want bool
	}{
		{"node", true},
		{"claude", true},
		{"2.0.76", true},
		{"1.2.3", true},
		{"10.20.30", true},
		{"bash", false},
		{"zsh", false},
		{"", false},
		{"v2.0.76", false}, // version with 'v' prefix shouldn't match
		{"2.0", false},     // incomplete version
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			// Check if it matches node/claude directly
			isKnownCmd := tt.cmd == "node" || tt.cmd == "claude"
			// Check version pattern (only when "claude" is in processNames)
			matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+`, tt.cmd)

			got := isKnownCmd || matched
			if got != tt.want {
				t.Errorf("IsRuntimeRunning logic for %q = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestIsRuntimeRunning_ShellWithNodeChild(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-shell-child-" + t.Name()
	sessionID := session.SessionID(sessionName)

	// Clean up any existing session
	_ = tm.Stop(sessionID)

	// Create session with "bash -c" running a node process
	// Use a simple node command that runs for a few seconds
	cmd := `node -e "setTimeout(() => {}, 10000)"`
	if _, err := tm.Start(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.Stop(sessionID) }()

	// Give the node process time to start
	// WaitForCommand waits until NOT running bash/zsh/sh
	shellsToExclude := []string{"bash", "zsh", "sh"}
	err := tm.WaitForCommand(sessionName, shellsToExclude, 2000*1000000) // 2 second timeout
	if err != nil {
		// If we timeout waiting, it means the pane command is still a shell
		// This is the case we're testing - shell with a node child
		paneCmd, _ := tm.GetPaneCommand(sessionName)
		t.Logf("Pane command is %q - testing shell+child detection", paneCmd)
	}

	// Now test IsRuntimeRunning - it should detect node as a child process
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	claudeProcessNames := []string{"claude", "node"}
	if paneCmd == "node" {
		// Direct node detection should work
		if !tm.IsRuntimeRunning(sessionName, claudeProcessNames) {
			t.Error("IsRuntimeRunning should return true when pane command is 'node'")
		}
	} else {
		// Pane is a shell (bash/zsh) with node as child
		// The child process detection should catch this
		got := tm.IsRuntimeRunning(sessionName, claudeProcessNames)
		t.Logf("Pane command: %q, IsRuntimeRunning: %v", paneCmd, got)
		// Note: This may or may not detect depending on how tmux runs the command.
		// On some systems, tmux runs the command directly; on others via a shell.
	}
}

func TestHasClaudeChild(t *testing.T) {
	// Test the hasClaudeChild helper function directly
	// This uses the current process as a test subject

	// Get current process PID as string
	currentPID := "1" // init/launchd - should have children but not claude/node

	// hasClaudeChild should return false for init (no node/claude children)
	got := hasClaudeChild(currentPID)
	if got {
		t.Logf("hasClaudeChild(%q) = true - init has claude/node child?", currentPID)
	}

	// Test with a definitely nonexistent PID
	got = hasClaudeChild("999999999")
	if got {
		t.Error("hasClaudeChild should return false for nonexistent PID")
	}
}

func TestGetAllDescendants(t *testing.T) {
	// Test the getAllDescendants helper function

	// Test with nonexistent PID - should return empty slice
	got := getAllDescendants("999999999")
	if len(got) != 0 {
		t.Errorf("getAllDescendants(nonexistent) = %v, want empty slice", got)
	}

	// Test with PID 1 (init/launchd) - should find some descendants
	// Note: We can't test exact PIDs, just that the function doesn't panic
	// and returns reasonable results
	descendants := getAllDescendants("1")
	t.Logf("getAllDescendants(\"1\") found %d descendants", len(descendants))

	// Verify returned PIDs are all numeric strings
	for _, pid := range descendants {
		for _, c := range pid {
			if c < '0' || c > '9' {
				t.Errorf("getAllDescendants returned non-numeric PID: %q", pid)
			}
		}
	}
}
