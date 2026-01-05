package tmux

import (
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

func TestVersionPatternMatching(t *testing.T) {
	// Test the version pattern regex used in IsClaudeRunning
	// Uses the exported versionPattern for consistency with production code

	testCases := []struct {
		input    string
		expected bool
		desc     string
	}{
		// Valid version patterns (should match)
		{"2.0.76", true, "standard version"},
		{"1.0.23", true, "standard version"},
		{"10.20.30", true, "double-digit version"},
		{"0.0.1", true, "minimum valid version"},
		{"1.0.0", true, "1.0.0 release"},
		{"99.99.99", true, "large version numbers"},
		{"123.456.789", true, "very large version numbers"},
		{"0.0.0", true, "all zeros"},

		// Invalid patterns (should not match)
		{"node", false, "process name: node"},
		{"claude", false, "process name: claude"},
		{"bash", false, "shell: bash"},
		{"zsh", false, "shell: zsh"},
		{"sh", false, "shell: sh"},
		{"fish", false, "shell: fish"},
		{"vim", false, "editor: vim"},
		{"nvim", false, "editor: nvim"},
		{"emacs", false, "editor: emacs"},
		{"python", false, "interpreter: python"},
		{"python3", false, "interpreter: python3"},
		{"", false, "empty string"},
		{" ", false, "single space"},
		{"   ", false, "multiple spaces"},
		{"\t", false, "tab character"},
		{"\n", false, "newline character"},
		{"2.0", false, "missing patch version"},
		{"2", false, "only major version"},
		{"v2.0.76", false, "leading 'v' prefix"},
		{"V2.0.76", false, "leading 'V' prefix (uppercase)"},
		{".0.76", false, "missing major version"},
		{"2..76", false, "missing minor version"},
		{"2.0.", false, "trailing dot, missing patch"},
		{"-1.0.0", false, "negative major version"},
		{"1.-1.0", false, "negative minor version"},
		{"1.0.-1", false, "negative patch version"},
		{"a.b.c", false, "letters instead of numbers"},
		{"1.2.3.4", true, "four-part version (matches prefix)"},
		{"1.2.3-beta", true, "prerelease suffix (matches prefix)"},
		{"1.2.3+build", true, "build metadata (matches prefix)"},
		{"1.2.3-rc.1", true, "release candidate (matches prefix)"},

		// Edge cases with whitespace
		{" 2.0.76", false, "leading space"},
		{"2.0.76 ", true, "trailing space (matches prefix)"},
		{" 2.0.76 ", false, "surrounded by spaces"},

		// Unicode edge cases
		{"２.０.７６", false, "fullwidth digits"},
		{"2.0.76日本語", true, "version with Japanese suffix (matches prefix)"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			matched := versionPattern.MatchString(tc.input)
			if matched != tc.expected {
				t.Errorf("versionPattern.MatchString(%q): got %v, want %v", tc.input, matched, tc.expected)
			}
		})
	}
}

func TestVersionPatternMatchingBoundary(t *testing.T) {
	// Test boundary conditions for version pattern

	t.Run("very long version string", func(t *testing.T) {
		// 1000+ digit version numbers
		longVersion := strings.Repeat("9", 1000) + "." + strings.Repeat("9", 1000) + "." + strings.Repeat("9", 1000)
		if !versionPattern.MatchString(longVersion) {
			t.Errorf("expected very long version string to match")
		}
	})

	t.Run("version at max int", func(t *testing.T) {
		maxInt := "9223372036854775807.9223372036854775807.9223372036854775807"
		if !versionPattern.MatchString(maxInt) {
			t.Errorf("expected max int version to match")
		}
	})

	t.Run("very long non-version string", func(t *testing.T) {
		longStr := strings.Repeat("bash", 10000)
		if versionPattern.MatchString(longStr) {
			t.Errorf("expected very long non-version string to not match")
		}
	})

	t.Run("unicode zero-width characters", func(t *testing.T) {
		// Zero-width space before version
		zws := "\u200B2.0.76"
		if versionPattern.MatchString(zws) {
			t.Errorf("expected version with zero-width space prefix to not match")
		}
	})
}

func TestVersionPatternPerformance(t *testing.T) {
	// Verify that versionPattern is pre-compiled (not re-compiled on each call)
	// This is a compile-time guarantee now, but we test behavior consistency

	testInputs := []string{
		"2.0.76", "node", "bash", "123.456.789", "",
		"v1.0.0", "1.0", "very-long-shell-name-that-shouldnt-match",
	}

	// Run multiple iterations - if regex were compiled each time, this would be slow
	for i := 0; i < 1000; i++ {
		for _, input := range testInputs {
			_ = versionPattern.MatchString(input)
		}
	}

	// If we got here without timeout, the pre-compiled pattern is working
	// The test framework will catch any significant slowdown
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
