package tmux

import (
	"strings"
	"testing"
	"time"
)

// Tests for the two-step session creation (new-session + respawn-pane) and
// checkSessionAfterCreate health check introduced to eliminate blank windows.

// TestNewSessionWithCommand_BadBinary verifies that NewSessionWithCommand returns
// an error when the command binary doesn't exist, instead of leaving a dead session.
func TestNewSessionWithCommand_BadBinary(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-badbinary-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	err := tm.NewSessionWithCommand(session, "", "/nonexistent/binary --flag")
	if err == nil {
		// checkSessionAfterCreate should have caught this
		t.Error("NewSessionWithCommand should return error for missing binary")
	}
}

// TestNewSessionWithCommand_BadWorkDir verifies workDir validation rejects
// non-existent directories before creating the session.
func TestNewSessionWithCommand_BadWorkDir(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-badworkdir-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	err := tm.NewSessionWithCommand(session, "/tmp/gastown-nonexistent-dir-99999", "echo hello")
	if err == nil {
		t.Error("NewSessionWithCommand should return error for non-existent workDir")
	}
}

// TestNewSessionWithCommand_ExecEnvBadBinary verifies the exact gastown polecat
// startup pattern (exec env VAR=val binary) returns an error for missing binaries.
func TestNewSessionWithCommand_ExecEnvBadBinary(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-execenv-bad-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	cmd := `exec env GT_TEST=1 GT_ROLE=test /nonexistent/claude-code --settings /tmp`
	err := tm.NewSessionWithCommand(session, "", cmd)
	if err == nil {
		t.Error("NewSessionWithCommand should return error for exec env with missing binary")
	}
}

// TestNewSessionWithCommand_Success verifies a valid command runs and produces output.
func TestNewSessionWithCommand_Success(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-success-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	err := tm.NewSessionWithCommand(session, "", `sh -c 'echo "SESSION_OK"; sleep 10'`)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	output, _ := tm.CapturePane(session, 50)
	if !strings.Contains(output, "SESSION_OK") {
		t.Errorf("expected output to contain SESSION_OK, got: %q", strings.TrimSpace(output))
	}
}

// TestNewSessionWithCommand_ExecEnvSuccess verifies the exec env pattern works
// with a real binary.
func TestNewSessionWithCommand_ExecEnvSuccess(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-execenv-ok-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	cmd := `exec env GT_RIG=testrig GT_POLECAT=testcat sleep 5`
	err := tm.NewSessionWithCommand(session, t.TempDir(), cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	paneCmd, _ := tm.GetPaneCommand(session)
	if paneCmd != "sleep" {
		t.Errorf("expected pane command 'sleep' (exec replaced shell), got %q", paneCmd)
	}
}

// TestNewSessionWithCommand_Duplicate verifies duplicate session creation is rejected.
func TestNewSessionWithCommand_Duplicate(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-dup-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	if err := tm.NewSessionWithCommand(session, "", "sleep 10"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := tm.NewSessionWithCommand(session, "", "sleep 10")
	if err == nil {
		t.Error("duplicate session creation should fail")
	}
}

// TestNewSessionWithCommand_Concurrent verifies multiple sessions can be created
// concurrently without errors.
func TestNewSessionWithCommand_Concurrent(t *testing.T) {
	tm := newTestTmux(t)
	n := 5
	base := "gt-test-concurrent-"

	for i := 0; i < n; i++ {
		_ = tm.KillSession(base + string(rune('a'+i)))
	}
	defer func() {
		for i := 0; i < n; i++ {
			_ = tm.KillSession(base + string(rune('a'+i)))
		}
	}()

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			errs <- tm.NewSessionWithCommand(base+string(rune('a'+idx)), "", "sleep 5")
		}(i)
	}

	var failures int
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			failures++
			t.Logf("concurrent create %d: %v", i, err)
		}
	}
	if failures > 0 {
		t.Errorf("%d/%d concurrent session creations failed", failures, n)
	}
}

// TestWaitForCommand_Timeout verifies WaitForCommand returns an error when the
// pane command remains a shell (agent never started).
func TestWaitForCommand_Timeout(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-waitcmd-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	if err := tm.NewSessionWithCommand(session, "", "bash"); err != nil {
		t.Fatalf("session creation: %v", err)
	}

	err := tm.WaitForCommand(session, []string{"bash", "zsh", "sh"}, 500*time.Millisecond)
	if err == nil {
		t.Error("WaitForCommand should timeout when shell is still running")
	}
}

// TestSanitizeNudgeMessage verifies control character stripping.
func TestSanitizeNudgeMessage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"passthrough", "hello world", "hello world"},
		{"strips ESC", "hello\x1bworld", "helloworld"},
		{"strips CR", "hello\rworld", "helloworld"},
		{"tab to space", "hello\tworld", "hello world"},
		{"preserves newline", "hello\nworld", "hello\nworld"},
		{"preserves unicode", "hello 世界", "hello 世界"},
		{"strips BS", "hello\x08world", "helloworld"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeNudgeMessage(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeNudgeMessage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSendMessageToTarget_Chunking verifies that long messages are chunked.
func TestSendMessageToTarget_Chunking(t *testing.T) {
	tm := newTestTmux(t)
	session := "gt-test-chunk-" + t.Name()
	_ = tm.KillSession(session)
	defer func() { _ = tm.KillSession(session) }()

	// Use cat to receive input
	if err := tm.NewSessionWithCommand(session, "", "cat"); err != nil {
		t.Fatalf("session creation: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Send a message longer than typical chunk size
	msg := strings.Repeat("A", 600)
	err := tm.sendMessageToTarget(session, msg, 5*time.Second)
	if err != nil {
		t.Fatalf("sendMessageToTarget: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(session, 50)
	// Count A's in output (may be split across lines)
	count := strings.Count(output, "A")
	if count < 500 {
		t.Errorf("expected ~600 A's in output, got %d (message may have been truncated)", count)
	}
}
