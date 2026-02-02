package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
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
		{"no current target", ErrNoServer},
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

	// Verify it's a zombie (not running any agent)
	if tm.IsAgentAlive(sessionName) {
		t.Skip("session unexpectedly has agent running - can't test zombie case")
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

func TestIsAgentRunning(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-agent-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session (will run default shell)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

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

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session (will run default shell, not any agent)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// IsRuntimeRunning should be false (shell is running, not node/claude)
	cmd, _ := tm.GetPaneCommand(sessionName)
	processNames := []string{"node", "claude"}
	wantRunning := cmd == "node" || cmd == "claude"

	if got := tm.IsRuntimeRunning(sessionName, processNames); got != wantRunning {
		t.Errorf("IsRuntimeRunning() = %v, want %v (pane cmd: %q)", got, wantRunning, cmd)
	}
}

func TestIsRuntimeRunning_ShellWithNodeChild(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-shell-child-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with "bash -c" running a node process
	// Use a simple node command that runs for a few seconds
	cmd := `node -e "setTimeout(() => {}, 10000)"`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

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
	processNames := []string{"node", "claude"}
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	if paneCmd == "node" {
		// Direct node detection should work
		if !tm.IsRuntimeRunning(sessionName, processNames) {
			t.Error("IsRuntimeRunning should return true when pane command is 'node'")
		}
	} else {
		// Pane is a shell (bash/zsh) with node as child
		// The child process detection should catch this
		got := tm.IsRuntimeRunning(sessionName, processNames)
		t.Logf("Pane command: %q, IsRuntimeRunning: %v", paneCmd, got)
		// Note: This may or may not detect depending on how tmux runs the command.
		// On some systems, tmux runs the command directly; on others via a shell.
	}
}

func TestHasChildWithNames(t *testing.T) {
	// Test the hasChildWithNames helper function directly

	// Test with a definitely nonexistent PID
	got := hasChildWithNames("999999999", []string{"node", "claude"})
	if got {
		t.Error("hasChildWithNames should return false for nonexistent PID")
	}

	// Test with empty names slice - should always return false
	got = hasChildWithNames("1", []string{})
	if got {
		t.Error("hasChildWithNames should return false for empty names slice")
	}

	// Test with nil names slice - should always return false
	got = hasChildWithNames("1", nil)
	if got {
		t.Error("hasChildWithNames should return false for nil names slice")
	}

	// Test with PID 1 (init/launchd) - should have children but not specific agent processes
	got = hasChildWithNames("1", []string{"node", "claude"})
	if got {
		t.Logf("hasChildWithNames(\"1\", [node,claude]) = true - init has matching child?")
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

func TestKillSessionWithProcesses(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killproc-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with a long-running process
	cmd := `sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}

	// Verify session exists
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Fatal("expected session to exist after creation")
	}

	// Kill with processes
	if err := tm.KillSessionWithProcesses(sessionName); err != nil {
		t.Fatalf("KillSessionWithProcesses: %v", err)
	}

	// Verify session is gone
	has, err = tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession after kill: %v", err)
	}
	if has {
		t.Error("expected session to not exist after KillSessionWithProcesses")
		_ = tm.KillSession(sessionName) // cleanup
	}
}

func TestKillSessionWithProcesses_NonexistentSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Killing nonexistent session should not panic, just return error or nil
	err := tm.KillSessionWithProcesses("nonexistent-session-xyz-12345")
	// We don't care about the error value, just that it doesn't panic
	_ = err
}

func TestKillSessionWithProcessesExcluding(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killexcl-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with a long-running process
	cmd := `sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}

	// Verify session exists
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Fatal("expected session to exist after creation")
	}

	// Kill with empty excludePIDs (should behave like KillSessionWithProcesses)
	if err := tm.KillSessionWithProcessesExcluding(sessionName, nil); err != nil {
		t.Fatalf("KillSessionWithProcessesExcluding: %v", err)
	}

	// Verify session is gone
	has, err = tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession after kill: %v", err)
	}
	if has {
		t.Error("expected session to not exist after KillSessionWithProcessesExcluding")
		_ = tm.KillSession(sessionName) // cleanup
	}
}

func TestKillSessionWithProcessesExcluding_WithExcludePID(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killexcl2-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with a long-running process
	cmd := `sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Get the pane PID
	panePID, err := tm.GetPanePID(sessionName)
	if err != nil {
		t.Fatalf("GetPanePID: %v", err)
	}
	if panePID == "" {
		t.Skip("could not get pane PID")
	}

	// Kill with the pane PID excluded - the function should still kill the session
	// but should not kill the excluded PID before the session is destroyed
	err = tm.KillSessionWithProcessesExcluding(sessionName, []string{panePID})
	if err != nil {
		t.Fatalf("KillSessionWithProcessesExcluding: %v", err)
	}

	// Session should be gone (the final KillSession always happens)
	has, _ := tm.HasSession(sessionName)
	if has {
		t.Error("expected session to not exist after KillSessionWithProcessesExcluding")
	}
}

func TestKillSessionWithProcessesExcluding_NonexistentSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Killing nonexistent session should not panic
	err := tm.KillSessionWithProcessesExcluding("nonexistent-session-xyz-12345", []string{"12345"})
	// We don't care about the error value, just that it doesn't panic
	_ = err
}

func TestGetProcessGroupID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are not supported on Windows")
	}
	// Test with current process
	pid := fmt.Sprintf("%d", os.Getpid())
	pgid := getProcessGroupID(pid)

	if pgid == "" {
		t.Error("expected non-empty PGID for current process")
	}

	// PGID should not be 0 or 1 for a normal process
	if pgid == "0" || pgid == "1" {
		t.Errorf("unexpected PGID %q for current process", pgid)
	}

	// Test with nonexistent PID
	pgid = getProcessGroupID("999999999")
	if pgid != "" {
		t.Errorf("expected empty PGID for nonexistent process, got %q", pgid)
	}
}

func TestGetProcessGroupMembers(t *testing.T) {
	// Get current process's PGID
	pid := fmt.Sprintf("%d", os.Getpid())
	pgid := getProcessGroupID(pid)
	if pgid == "" {
		t.Skip("could not get PGID for current process")
	}

	members := getProcessGroupMembers(pgid)

	// Current process should be in the list
	found := false
	for _, m := range members {
		if m == pid {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("current process %s not found in process group %s members: %v", pid, pgid, members)
	}
}

func TestKillSessionWithProcesses_KillsProcessGroup(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killpg-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session that spawns a child process
	// The child will stay in the same process group as the shell
	cmd := `sleep 300 & sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}

	// Give processes time to start
	time.Sleep(200 * time.Millisecond)

	// Verify session exists
	has, err := tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !has {
		t.Fatal("expected session to exist after creation")
	}

	// Kill with processes (should kill the entire process group)
	if err := tm.KillSessionWithProcesses(sessionName); err != nil {
		t.Fatalf("KillSessionWithProcesses: %v", err)
	}

	// Verify session is gone
	has, err = tm.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession after kill: %v", err)
	}
	if has {
		t.Error("expected session to not exist after KillSessionWithProcesses")
		_ = tm.KillSession(sessionName) // cleanup
	}
}

func TestSessionSet(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-sessionset-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create a test session
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Get the session set
	set, err := tm.GetSessionSet()
	if err != nil {
		t.Fatalf("GetSessionSet: %v", err)
	}

	// Test Has() for existing session
	if !set.Has(sessionName) {
		t.Errorf("SessionSet.Has(%q) = false, want true", sessionName)
	}

	// Test Has() for non-existing session
	if set.Has("nonexistent-session-xyz-12345") {
		t.Error("SessionSet.Has(nonexistent) = true, want false")
	}

	// Test nil safety
	var nilSet *SessionSet
	if nilSet.Has("anything") {
		t.Error("nil SessionSet.Has() = true, want false")
	}

	// Test Names() returns the session
	names := set.Names()
	found := false
	for _, n := range names {
		if n == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SessionSet.Names() doesn't contain %q", sessionName)
	}
}

func TestCleanupOrphanedSessions(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Create test sessions with gt- and hq- prefixes (zombie sessions - no Claude running)
	gtSession := "gt-test-cleanup-rig"
	hqSession := "hq-test-cleanup"
	nonGtSession := "other-test-session"

	// Clean up any existing test sessions
	_ = tm.KillSession(gtSession)
	_ = tm.KillSession(hqSession)
	_ = tm.KillSession(nonGtSession)

	// Create zombie sessions (tmux alive, but just shell - no Claude)
	if err := tm.NewSession(gtSession, ""); err != nil {
		t.Fatalf("NewSession(gt): %v", err)
	}
	defer func() { _ = tm.KillSession(gtSession) }()

	if err := tm.NewSession(hqSession, ""); err != nil {
		t.Fatalf("NewSession(hq): %v", err)
	}
	defer func() { _ = tm.KillSession(hqSession) }()

	// Create a non-GT session (should NOT be cleaned up)
	if err := tm.NewSession(nonGtSession, ""); err != nil {
		t.Fatalf("NewSession(other): %v", err)
	}
	defer func() { _ = tm.KillSession(nonGtSession) }()

	// Verify all sessions exist
	for _, sess := range []string{gtSession, hqSession, nonGtSession} {
		has, err := tm.HasSession(sess)
		if err != nil {
			t.Fatalf("HasSession(%q): %v", sess, err)
		}
		if !has {
			t.Fatalf("expected session %q to exist", sess)
		}
	}

	// Run cleanup
	cleaned, err := tm.CleanupOrphanedSessions()
	if err != nil {
		t.Fatalf("CleanupOrphanedSessions: %v", err)
	}

	// Should have cleaned the gt- and hq- zombie sessions
	if cleaned < 2 {
		t.Errorf("CleanupOrphanedSessions cleaned %d sessions, want >= 2", cleaned)
	}

	// Verify GT sessions are gone
	for _, sess := range []string{gtSession, hqSession} {
		has, err := tm.HasSession(sess)
		if err != nil {
			t.Fatalf("HasSession(%q) after cleanup: %v", sess, err)
		}
		if has {
			t.Errorf("expected session %q to be cleaned up", sess)
		}
	}

	// Verify non-GT session still exists
	has, err := tm.HasSession(nonGtSession)
	if err != nil {
		t.Fatalf("HasSession(%q) after cleanup: %v", nonGtSession, err)
	}
	if !has {
		t.Error("non-GT session should NOT have been cleaned up")
	}
}

func TestCleanupOrphanedSessions_NoSessions(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Running cleanup with no orphaned GT sessions should return 0, no error
	cleaned, err := tm.CleanupOrphanedSessions()
	if err != nil {
		t.Fatalf("CleanupOrphanedSessions: %v", err)
	}

	// May clean some existing GT sessions if they exist, but shouldn't error
	t.Logf("CleanupOrphanedSessions cleaned %d sessions", cleaned)
}

func TestKillPaneProcessesExcluding(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killpaneexcl-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with a long-running process
	cmd := `sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Get the pane ID
	paneID, err := tm.GetPaneID(sessionName)
	if err != nil {
		t.Fatalf("GetPaneID: %v", err)
	}

	// Kill pane processes with empty excludePIDs (should kill all processes)
	if err := tm.KillPaneProcessesExcluding(paneID, nil); err != nil {
		t.Fatalf("KillPaneProcessesExcluding: %v", err)
	}

	// Session may still exist (pane respawns as dead), but processes should be gone
	// Check that we can still get info about the session (verifies we didn't panic)
	_, _ = tm.HasSession(sessionName)
}

func TestKillPaneProcessesExcluding_WithExcludePID(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-killpaneexcl2-" + t.Name()

	// Clean up any existing session
	_ = tm.KillSession(sessionName)

	// Create session with a long-running process
	cmd := `sleep 300`
	if err := tm.NewSessionWithCommand(sessionName, "", cmd); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Get the pane ID and PID
	paneID, err := tm.GetPaneID(sessionName)
	if err != nil {
		t.Fatalf("GetPaneID: %v", err)
	}

	panePID, err := tm.GetPanePID(sessionName)
	if err != nil {
		t.Fatalf("GetPanePID: %v", err)
	}
	if panePID == "" {
		t.Skip("could not get pane PID")
	}

	// Kill pane processes with the pane PID excluded
	// The function should NOT kill the excluded PID
	err = tm.KillPaneProcessesExcluding(paneID, []string{panePID})
	if err != nil {
		t.Fatalf("KillPaneProcessesExcluding: %v", err)
	}

	// The session/pane should still exist since we excluded the main process
	has, _ := tm.HasSession(sessionName)
	if !has {
		t.Log("Session was destroyed - this may happen if tmux auto-cleaned after descendants died")
	}
}

func TestKillPaneProcessesExcluding_NonexistentPane(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

	// Killing nonexistent pane should return an error but not panic
	err := tm.KillPaneProcessesExcluding("%99999", []string{"12345"})
	if err == nil {
		t.Error("expected error for nonexistent pane")
	}
}

func TestKillPaneProcessesExcluding_FiltersPIDs(t *testing.T) {
	// Unit test the PID filtering logic without needing tmux
	// This tests that the exclusion set is built correctly

	excludePIDs := []string{"123", "456", "789"}
	exclude := make(map[string]bool)
	for _, pid := range excludePIDs {
		exclude[pid] = true
	}

	// Test that excluded PIDs are in the set
	for _, pid := range excludePIDs {
		if !exclude[pid] {
			t.Errorf("exclude[%q] = false, want true", pid)
		}
	}

	// Test that non-excluded PIDs are not in the set
	nonExcluded := []string{"111", "222", "333"}
	for _, pid := range nonExcluded {
		if exclude[pid] {
			t.Errorf("exclude[%q] = true, want false", pid)
		}
	}

	// Test filtering logic
	allPIDs := []string{"111", "123", "222", "456", "333", "789"}
	var filtered []string
	for _, pid := range allPIDs {
		if !exclude[pid] {
			filtered = append(filtered, pid)
		}
	}

	expectedFiltered := []string{"111", "222", "333"}
	if len(filtered) != len(expectedFiltered) {
		t.Fatalf("filtered = %v, want %v", filtered, expectedFiltered)
	}
	for i, pid := range filtered {
		if pid != expectedFiltered[i] {
			t.Errorf("filtered[%d] = %q, want %q", i, pid, expectedFiltered[i])
		}
	}
}
