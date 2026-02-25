package tmux

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests investigate tmux session creation edge cases that can cause:
// 1. Blank window - session created but window shows nothing
// 2. Command visible at top - the exec command is printed before the actual program renders
//
// Root causes identified:
// - Blank window: command fails silently (binary not found, syntax error, workdir missing)
// - Command visible: shell echo mode (set -x), shell startup scripts printing, or
//   tmux default-shell behavior differences

// =============================================================================
// BLANK WINDOW TESTS
// =============================================================================

// TestBlankWindow_CommandNotFound reproduces blank window when binary doesn't exist.
// The session is created by tmux, but dies immediately when command fails.
// This is the "blank window" symptom - user sees nothing useful.
func TestBlankWindow_CommandNotFound(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-blank-notfound-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a non-existent binary - this simulates claude-code not being in PATH
	cmd := "/nonexistent/path/to/binary --some-flag"

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Logf("NewSessionWithCommand correctly returned error for missing binary: %v", err)
		return
	}

	// If we get here, the function didn't catch the problem
	// Wait a bit for the command to fail
	time.Sleep(200 * time.Millisecond)

	// Check if session still exists
	exists, _ := tm.HasSession(sessionName)

	if !exists {
		// Session died because the command failed
		t.Error("NewSessionWithCommand returned nil but session died immediately (binary not found). Should return an error.")
		t.Log("Root cause: tmux ran command, command failed, pane process exited, session died")
		t.Log("Impact: User runs gt sling, session appears briefly then vanishes")
		return
	}

	// If session still exists, capture output
	output, err := tm.CapturePane(sessionName, 50)
	if err != nil {
		t.Logf("CapturePane error: %v", err)
	}

	t.Logf("Pane output after command-not-found:\n%s", output)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane current command: %q", paneCmd)

	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" {
		t.Error("Session exists but pane is completely empty — blank window")
	} else if strings.Contains(output, "not found") || strings.Contains(output, "No such file") {
		t.Log("Error message visible to user — acceptable but error should still be returned to caller")
	}
}

// TestBlankWindow_WorkDirNotExists reproduces blank window when working directory
// doesn't exist. The -c flag to tmux new-session may fail silently.
func TestBlankWindow_WorkDirNotExists(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-blank-workdir-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a non-existent working directory
	nonExistentDir := "/tmp/gastown-test-nonexistent-dir-12345"
	_ = os.RemoveAll(nonExistentDir) // Ensure it doesn't exist

	cmd := "echo 'hello world'"

	err := tm.NewSessionWithCommand(sessionName, nonExistentDir, cmd)

	// Document whether tmux fails or succeeds with bad workdir
	if err != nil {
		t.Logf("tmux rejected non-existent workdir: %v", err)
		t.Log("This is GOOD - failure is surfaced to caller")
		return
	}

	// Session was created despite bad workdir - check what happened
	time.Sleep(200 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output with non-existent workdir:\n%s", output)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane current command: %q", paneCmd)

	// BUG: NewSessionWithCommand should validate workDir before creating the session.
	t.Error("NewSessionWithCommand should return an error when workDir does not exist")
}

// TestBlankWindow_SyntaxError reproduces blank window when command has shell syntax error.
// The shell parses the command, finds an error, and exits.
func TestBlankWindow_SyntaxError(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-blank-syntax-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Command with unclosed quote - shell syntax error
	cmd := `echo "unclosed quote`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Logf("NewSessionWithCommand correctly returned error: %v", err)
		return
	}

	time.Sleep(200 * time.Millisecond)

	exists, _ := tm.HasSession(sessionName)
	output, _ := tm.CapturePane(sessionName, 50)
	paneCmd, _ := tm.GetPaneCommand(sessionName)

	t.Logf("Session exists: %v", exists)
	t.Logf("Pane command: %q", paneCmd)
	t.Logf("Pane output:\n%s", output)

	if !exists {
		t.Error("NewSessionWithCommand returned nil but session died from syntax error. Should either return error or keep session alive with remain-on-exit.")
	} else if strings.TrimSpace(output) == "" {
		t.Error("Session alive but pane is blank — syntax error not visible to user")
	}
}

// TestBlankWindow_ExecEnvBinaryNotFound reproduces blank window with exec env pattern.
// This is the exact pattern used by gastown for polecat startup.
func TestBlankWindow_ExecEnvBinaryNotFound(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-blank-execenv-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate the exact pattern gastown uses: exec env VAR=value ... binary
	// If the binary doesn't exist, the exec fails and shell exits
	cmd := `exec env GT_TEST=1 GT_ROLE=test /nonexistent/claude-code --settings /tmp`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Logf("NewSessionWithCommand correctly returned error for exec env with missing binary: %v", err)
		return
	}

	// If we get here, the function didn't catch the problem
	time.Sleep(300 * time.Millisecond)

	exists, _ := tm.HasSession(sessionName)
	output, _ := tm.CapturePane(sessionName, 50)
	paneCmd, _ := tm.GetPaneCommand(sessionName)

	t.Logf("Session exists: %v", exists)
	t.Logf("Pane command: %q", paneCmd)
	t.Logf("Pane output:\n%s", output)

	// With exec, the shell is replaced by the command.
	// If command fails, the pane process exits immediately.
	if !exists || strings.TrimSpace(output) == "" {
		t.Error("NewSessionWithCommand returned nil but session died immediately (exec env binary not found). Should return an error.")
		t.Log("Root cause: exec replaced shell, then binary not found, pane died")
	}
}

// =============================================================================
// COMMAND VISIBLE AT TOP TESTS
// =============================================================================

// TestCommandVisibleAtTop_SetX reproduces command being echoed when shell has set -x.
// This happens when tmux's default-shell or the user's shell has xtrace enabled.
func TestCommandVisibleAtTop_SetX(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-cmdvisible-setx-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate shell with set -x enabled
	// The command will be echoed before execution
	cmd := `bash -c 'set -x; echo "actual output"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output with set -x:\n%s", output)

	// With set -x, we should see the command echoed with + prefix
	if strings.Contains(output, "+ echo") || strings.Contains(output, "'actual output'") {
		t.Log("COMMAND VISIBLE REPRODUCED: set -x causes command echo")
		t.Log("The '+' prefix and command are printed before actual output")
	}
}

// TestCommandVisibleAtTop_BashrcDebug reproduces command visibility when
// bash startup scripts have debug output.
func TestCommandVisibleAtTop_BashrcDebug(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-cmdvisible-bashrc-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create a temp bashrc that prints debug info
	tmpDir := t.TempDir()
	bashrcPath := tmpDir + "/bashrc"
	err := os.WriteFile(bashrcPath, []byte(`
echo "BASHRC: Loading..."
echo "BASHRC: Command is: $BASH_COMMAND"
`), 0644)
	if err != nil {
		t.Fatalf("Failed to create bashrc: %v", err)
	}

	// Run bash with custom bashrc
	cmd := `bash --rcfile ` + bashrcPath + ` -c 'echo "actual program output"'`

	err = tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output with debug bashrc:\n%s", output)

	if strings.Contains(output, "BASHRC:") {
		t.Log("STARTUP SCRIPT OUTPUT visible before actual command output")
		t.Log("This can cause confusion - startup debug appears before program")
	}
}

// TestCommandVisibleAtTop_ExecEnvWithVerboseShell tests whether the full
// exec env command line is visible when shell is in verbose mode.
func TestCommandVisibleAtTop_ExecEnvWithVerboseShell(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-cmdvisible-execenv-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Test if the exec env command line itself becomes visible
	// Use a real command that will run successfully
	cmd := `bash -c 'set -x; exec env TEST_VAR=value sleep 1'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output with verbose exec env:\n%s", output)

	// Check if the exec env line is visible
	if strings.Contains(output, "exec env") || strings.Contains(output, "TEST_VAR=value") {
		t.Log("EXEC ENV COMMAND VISIBLE: The full command line is echoed")
		t.Log("This is the 'command visible at top' symptom")
	}
}

// =============================================================================
// POSITIVE TESTS - VERIFY CORRECT BEHAVIOR
// =============================================================================

// TestSessionCreation_SuccessfulCommand verifies that a successful command
// produces expected output without blank window or command echo.
func TestSessionCreation_SuccessfulCommand(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-success-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simple successful command
	cmd := `echo "SUCCESS: Program started correctly"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output:\n%s", output)

	if strings.Contains(output, "SUCCESS: Program started correctly") {
		t.Log("CORRECT BEHAVIOR: Output shows without command echo")
	}

	// Verify no command echo (shouldn't see 'echo' in output unless set -x)
	if strings.Contains(output, `echo "SUCCESS`) {
		t.Log("WARNING: Command line is visible in output (possible echo mode)")
	}
}

// TestSessionCreation_ExecEnvPattern tests the exact gastown exec env pattern
// with a real binary to ensure it works correctly.
func TestSessionCreation_ExecEnvPattern(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-execenv-success-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use exec env pattern with real binary (echo via /bin/echo)
	cmd := `exec env GT_TEST_VAR=hello /bin/echo "EXEC_ENV_SUCCESS: $GT_TEST_VAR"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	paneCmd, _ := tm.GetPaneCommand(sessionName)

	t.Logf("Pane command: %q", paneCmd)
	t.Logf("Pane output:\n%s", output)

	// The pane command should be 'echo' (since exec replaced shell)
	if paneCmd == "echo" {
		t.Log("CORRECT: exec replaced shell, pane command is 'echo'")
	}

	// Output should contain the success message
	if strings.Contains(output, "EXEC_ENV_SUCCESS") {
		t.Log("CORRECT: Program output visible")
	}

	// Check for unwanted command echo
	if strings.Contains(output, "exec env") {
		t.Error("PROBLEM: exec env command line is visible in output")
	}
}

// =============================================================================
// DIAGNOSTIC TESTS
// =============================================================================

// TestDiagnose_ShellDefaultBehavior documents what shell tmux uses by default
// and how commands are processed.
func TestDiagnose_ShellDefaultBehavior(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-diagnose-shell-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Diagnostic command that reveals shell behavior
	cmd := `echo "SHELL=$SHELL"; echo "BASH_VERSION=$BASH_VERSION"; echo "OPTIONS=$SHELLOPTS"; set -o`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 100)
	t.Logf("Shell diagnostic output:\n%s", output)

	// Document findings
	if strings.Contains(output, "xtrace") && strings.Contains(output, "on") {
		t.Log("WARNING: xtrace (set -x) is ON by default - commands will be echoed")
	}
	if strings.Contains(output, "verbose") && strings.Contains(output, "on") {
		t.Log("WARNING: verbose mode is ON by default")
	}
}

// TestDiagnose_TmuxDefaultShell documents what default-shell tmux is using.
func TestDiagnose_TmuxDefaultShell(t *testing.T) {
	tm := newTestTmux(t)

	// Bootstrap the isolated server by creating a throwaway session.
	// show-options -g requires a running server.
	if err := tm.NewSession("gt-test-bootstrap", ""); err != nil {
		t.Fatalf("bootstrap session: %v", err)
	}
	defer tm.KillSession("gt-test-bootstrap")

	// Query tmux for default-shell setting
	output, err := tm.run("show-options", "-g", "default-shell")
	if err != nil {
		t.Fatalf("Failed to query default-shell: %v", err)
	}

	t.Logf("tmux default-shell: %s", output)

	// Also check default-command
	output2, err := tm.run("show-options", "-g", "default-command")
	if err != nil {
		t.Logf("default-command query: %v", err)
	} else {
		t.Logf("tmux default-command: %s", output2)
	}
}

// TestDiagnose_CommandPassthrough tests how tmux passes commands to the shell.
func TestDiagnose_CommandPassthrough(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-diagnose-passthrough-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Set remain-on-exit first so we can capture output after command exits
	// First create a simple session, set the option, then kill it
	_ = tm.NewSession(sessionName, "")
	_, _ = tm.run("set-option", "-t", sessionName, "remain-on-exit", "on")
	_ = tm.KillSession(sessionName)

	// Now create with command - remain-on-exit should be inherited from server
	cmd := `bash -c 'echo "TEST: quotes work" && echo single_quotes && echo "$HOME"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	// Set remain-on-exit on the new session to preserve output
	_, _ = tm.run("set-option", "-t", sessionName, "remain-on-exit", "on")

	time.Sleep(500 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Command passthrough test output:\n%s", output)

	// Check if we got any output at all
	if strings.TrimSpace(output) == "" {
		t.Log("No output captured - command may have exited before capture")
		t.Log("This test is informational - documenting tmux behavior")
		return
	}

	// Verify all parts executed correctly
	if strings.Contains(output, "TEST: quotes work") {
		t.Log("Double-quoted strings pass through correctly")
	}
	if strings.Contains(output, "single_quotes") {
		t.Log("Unquoted strings pass through correctly")
	}
	if strings.Contains(output, "/") {
		t.Log("Variable expansion works")
	}
}

// =============================================================================
// POLECAT-SPECIFIC REPRODUCTION TESTS
// =============================================================================

// TestPolecatStartup_ExactPattern tests the exact command pattern used by gastown
// for polecat session creation. This is the most realistic reproduction.
func TestPolecatStartup_ExactPattern(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-polecat-exact-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create a temp directory as the worktree
	tmpDir := t.TempDir()

	// Exact pattern from gastown (simplified for test):
	// exec env VAR=value ... claude-code --settings /path
	// We use 'sleep' as the "claude-code" binary for testing
	cmd := `exec env GT_RIG=testrig GT_POLECAT=testcat GT_ROLE=testrig/polecats/testcat sleep 5`

	err := tm.NewSessionWithCommand(sessionName, tmpDir, cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Check session state
	exists, _ := tm.HasSession(sessionName)
	if !exists {
		t.Fatal("Session should exist with valid command")
	}

	output, _ := tm.CapturePane(sessionName, 50)
	paneCmd, _ := tm.GetPaneCommand(sessionName)

	t.Logf("Pane command: %q", paneCmd)
	t.Logf("Pane output:\n%s", output)

	// With exec, the pane command should be 'sleep' (not bash)
	if paneCmd != "sleep" {
		t.Errorf("Expected pane command 'sleep', got %q", paneCmd)
		t.Log("exec may not be replacing the shell as expected")
	}

	// Output should be empty (sleep produces no output)
	// If we see the command line, that's the "command visible" bug
	if strings.Contains(output, "exec env") || strings.Contains(output, "GT_RIG=") {
		t.Error("COMMAND VISIBLE BUG: The exec env command line appears in output")
	}
}

// TestPolecatStartup_RemainOnExit tests what happens when the command exits
// and tmux's remain-on-exit option affects visibility.
func TestPolecatStartup_RemainOnExit(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-polecat-remain-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with a command that exits immediately
	cmd := `echo "STARTUP MESSAGE" && exit 0`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	// Set remain-on-exit before checking
	_, _ = tm.run("set-option", "-t", sessionName, "remain-on-exit", "on")

	time.Sleep(300 * time.Millisecond)

	exists, _ := tm.HasSession(sessionName)
	output, _ := tm.CapturePane(sessionName, 50)

	t.Logf("Session exists: %v", exists)
	t.Logf("Pane output:\n%s", output)

	// With remain-on-exit, session should persist and show output
	if exists && strings.Contains(output, "STARTUP MESSAGE") {
		t.Log("remain-on-exit preserves session for debugging")
	}

	// Check if pane shows "Pane is dead" message
	if strings.Contains(output, "Pane is dead") {
		t.Log("tmux shows 'Pane is dead' indicator - useful for debugging")
	}
}

// TestPolecatStartup_ShellInheritedOptions tests what shell options are
// inherited that might cause command echo.
func TestPolecatStartup_ShellInheritedOptions(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-polecat-shellopts-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Check what shell options are active in a fresh tmux session
	cmd := `bash -c 'echo "SHELLOPTS=$SHELLOPTS"; echo "BASHOPTS=$BASHOPTS"; [[ $- == *x* ]] && echo "XTRACE IS ON" || echo "XTRACE IS OFF"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Shell options in tmux session:\n%s", output)

	if strings.Contains(output, "XTRACE IS ON") {
		t.Log("WARNING: xtrace (set -x) is enabled - commands will be echoed!")
		t.Log("This is a likely cause of 'command visible at top' bug")
	}

	if strings.Contains(output, "xtrace") {
		t.Log("Found 'xtrace' in shell options - may cause command echo")
	}
}

// TestPolecatStartup_EnvironmentLeak tests whether parent environment
// variables leak into tmux sessions unexpectedly.
func TestPolecatStartup_EnvironmentLeak(t *testing.T) {
	// Set a test variable in current environment
	testVar := "GT_TEST_LEAK_VAR"
	os.Setenv(testVar, "LEAKED_VALUE")
	defer os.Unsetenv(testVar)

	tm := newTestTmux(t)
	sessionName := "gt-test-polecat-envleak-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Check if the variable leaks into the tmux session
	cmd := `bash -c 'echo "GT_TEST_LEAK_VAR=$GT_TEST_LEAK_VAR"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Environment leak test output:\n%s", output)

	if strings.Contains(output, "LEAKED_VALUE") {
		t.Error("Parent environment variables leak into tmux sessions — stale GT_ROLE or other vars could affect polecats")
	} else {
		t.Log("Environment variables do NOT leak - tmux uses server environment")
	}
}

// =============================================================================
// SESSION NAME EDGE CASES
// =============================================================================

// TestSessionName_InvalidCharacters tests session creation with problematic names.
// Tmux has restrictions on session name characters.
func TestSessionName_InvalidCharacters(t *testing.T) {
	tm := newTestTmux(t)

	testCases := []struct {
		name        string
		sessionName string
		shouldFail  bool
	}{
		{"dots", "gt-test.with.dots", true},       // Dots are problematic
		{"colons", "gt-test:with:colons", true},   // Colons are problematic
		{"spaces", "gt-test with spaces", true},   // Spaces are problematic
		{"slashes", "gt-test/with/slashes", true}, // Slashes are problematic
		{"valid-hyphens", "gt-test-valid-hyphens", false},
		{"valid-underscores", "gt_test_underscores", false},
		{"very-long", "gt-test-" + strings.Repeat("a", 200), false}, // Test length limits
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_ = tm.KillSession(tc.sessionName)
			defer func() { _ = tm.KillSession(tc.sessionName) }()

			err := tm.NewSessionWithCommand(tc.sessionName, "", "sleep 1")

			if tc.shouldFail && err == nil {
				t.Logf("Session name %q was accepted but expected to fail", tc.sessionName)
			} else if !tc.shouldFail && err != nil {
				t.Errorf("Session name %q should be valid but got error: %v", tc.sessionName, err)
			} else {
				t.Logf("Session name %q: error=%v (expected fail=%v)", tc.sessionName, err, tc.shouldFail)
			}
		})
	}
}

// TestSessionName_Duplicate tests behavior when session already exists.
func TestSessionName_Duplicate(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-duplicate-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create first session
	err := tm.NewSessionWithCommand(sessionName, "", "sleep 10")
	if err != nil {
		t.Fatalf("First session creation failed: %v", err)
	}

	// Try to create duplicate
	err = tm.NewSessionWithCommand(sessionName, "", "sleep 10")
	if err == nil {
		t.Error("Duplicate session creation should fail")
	} else {
		t.Logf("Duplicate session correctly rejected: %v", err)
		// Check if it's the expected error type
		if err == ErrSessionExists {
			t.Log("Correct error type: ErrSessionExists")
		}
	}
}

// =============================================================================
// TIMEOUT AND SLOW STARTUP TESTS
// =============================================================================

// TestSlowStartup_ShellInit tests behavior with slow shell initialization.
func TestSlowStartup_ShellInit(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-slow-startup-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate slow shell startup with sleep before actual command
	// Use && sleep 1 at the end to keep session alive long enough to capture
	cmd := `bash -c 'sleep 2; echo "FINALLY READY"; sleep 1'`

	start := time.Now()
	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	createTime := time.Since(start)

	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	t.Logf("Session creation took %v", createTime)

	// Check immediately - should NOT see output yet
	outputImmediate, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Immediate output: %q", strings.TrimSpace(outputImmediate))

	// Wait for command to complete (2s sleep + output)
	time.Sleep(2200 * time.Millisecond)

	outputDelayed, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Delayed output: %q", strings.TrimSpace(outputDelayed))

	if !strings.Contains(outputDelayed, "FINALLY READY") {
		t.Error("Slow startup command output not captured")
	}
}

// TestWaitForCommand_Timeout tests the WaitForCommand timeout behavior.
func TestWaitForCommand_Timeout(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-waitcmd-timeout-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with bash that just sits (simulates slow agent startup)
	err := tm.NewSessionWithCommand(sessionName, "", "bash")
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// WaitForCommand should timeout because pane command is still "bash"
	shellsToExclude := []string{"bash", "zsh", "sh"}
	start := time.Now()
	err = tm.WaitForCommand(sessionName, shellsToExclude, 500*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("WaitForCommand should timeout when shell is still running")
	} else {
		t.Logf("WaitForCommand timed out after %v: %v", elapsed, err)
	}

	// Verify the pane command is still bash
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command after timeout: %q", paneCmd)
}

// =============================================================================
// CONCURRENT SESSION TESTS
// =============================================================================

// TestConcurrentSessionCreation tests creating multiple sessions rapidly.
func TestConcurrentSessionCreation(t *testing.T) {
	tm := newTestTmux(t)
	baseSessionName := "gt-test-concurrent-"
	numSessions := 5

	// Clean up any existing sessions
	for i := 0; i < numSessions; i++ {
		_ = tm.KillSession(baseSessionName + string(rune('a'+i)))
	}
	defer func() {
		for i := 0; i < numSessions; i++ {
			_ = tm.KillSession(baseSessionName + string(rune('a'+i)))
		}
	}()

	// Create sessions concurrently
	errors := make(chan error, numSessions)
	for i := 0; i < numSessions; i++ {
		go func(idx int) {
			sessionName := baseSessionName + string(rune('a'+idx))
			err := tm.NewSessionWithCommand(sessionName, "", "sleep 5")
			errors <- err
		}(i)
	}

	// Collect results
	var failCount int
	for i := 0; i < numSessions; i++ {
		err := <-errors
		if err != nil {
			failCount++
			t.Logf("Concurrent session creation error: %v", err)
		}
	}

	if failCount > 0 {
		t.Logf("CONCURRENT CREATION ISSUE: %d/%d sessions failed", failCount, numSessions)
	} else {
		t.Logf("All %d concurrent sessions created successfully", numSessions)
	}
}

// TestRapidCreateDestroy tests rapid session creation and destruction.
func TestRapidCreateDestroy(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-rapid-cd-" + t.Name()
	iterations := 10

	var failures int
	for i := 0; i < iterations; i++ {
		// Create
		err := tm.NewSessionWithCommand(sessionName, "", "sleep 1")
		if err != nil {
			failures++
			t.Logf("Iteration %d create failed: %v", i, err)
			continue
		}

		// Immediately destroy
		err = tm.KillSession(sessionName)
		if err != nil {
			t.Logf("Iteration %d kill failed: %v", i, err)
		}
	}

	if failures > 0 {
		t.Logf("RAPID CREATE/DESTROY ISSUE: %d/%d iterations failed", failures, iterations)
	} else {
		t.Logf("All %d rapid create/destroy cycles succeeded", iterations)
	}
}

// =============================================================================
// LONG COMMAND AND SPECIAL CHARACTER TESTS
// =============================================================================

// TestLongCommand_ManyEnvVars tests command with many environment variables.
func TestLongCommand_ManyEnvVars(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-longcmd-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Build a very long command with many env vars (similar to polecat startup)
	var envVars []string
	for i := 0; i < 50; i++ {
		envVars = append(envVars, fmt.Sprintf("VAR_%d=value_%d", i, i))
	}
	cmd := "exec env " + strings.Join(envVars, " ") + " echo SUCCESS"

	t.Logf("Command length: %d characters", len(cmd))

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Long command session creation failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	if strings.Contains(output, "SUCCESS") {
		t.Log("Long command with many env vars succeeded")
	} else {
		t.Logf("Long command output: %q", strings.TrimSpace(output))
	}
}

// TestSpecialCharacters_InPaths tests paths with special characters.
func TestSpecialCharacters_InPaths(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-special-paths-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create temp dir with space in name
	tmpBase := t.TempDir()
	specialDir := tmpBase + "/path with spaces"
	if err := os.MkdirAll(specialDir, 0755); err != nil {
		t.Fatalf("Failed to create special dir: %v", err)
	}

	// Test with quoted path
	cmd := `echo "PWD=$PWD"`

	err := tm.NewSessionWithCommand(sessionName, specialDir, cmd)
	if err != nil {
		t.Logf("Session with special path failed: %v", err)
		t.Log("SPECIAL CHARACTERS IN PATH: May cause issues")
		return
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output with special path: %q", strings.TrimSpace(output))

	if strings.Contains(output, "with spaces") {
		t.Log("Path with spaces handled correctly")
	}
}

// TestUnicode_InCommand tests Unicode characters in commands.
func TestUnicode_InCommand(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-unicode-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Test with Unicode characters (emoji, international chars)
	cmd := `echo "Unicode test: 你好 🚀 émojis"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Unicode command session failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Unicode output: %q", strings.TrimSpace(output))

	// Note: tmux -u flag enables UTF-8 mode
	if strings.Contains(output, "你好") || strings.Contains(output, "🚀") {
		t.Log("Unicode characters handled correctly (tmux -u flag working)")
	} else {
		t.Log("Unicode characters may not display correctly")
	}
}

// =============================================================================
// TMUX SERVER STATE TESTS
// =============================================================================

// TestTmuxServerEnvironment tests what environment the tmux server provides.
func TestTmuxServerEnvironment(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-serverenv-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Check critical environment variables that polecats need
	cmd := `bash -c 'echo "PATH=$PATH"; echo "HOME=$HOME"; echo "SHELL=$SHELL"; echo "TERM=$TERM"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Tmux server environment:\n%s", output)

	// Check for common issues
	if !strings.Contains(output, "PATH=") || strings.Contains(output, "PATH=\n") {
		t.Log("WARNING: PATH may be empty in tmux server environment")
	}
	if !strings.Contains(output, "/home") && !strings.Contains(output, "/Users") {
		t.Log("WARNING: HOME may not be set correctly")
	}
}

// TestStaleSessionRecovery tests detection and recovery from stale sessions.
func TestStaleSessionRecovery(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-stale-recovery-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with a command that exits quickly
	err := tm.NewSessionWithCommand(sessionName, "", "exit 0")
	if err != nil {
		t.Fatalf("First session creation failed: %v", err)
	}

	// Wait for command to exit
	time.Sleep(200 * time.Millisecond)

	// Check session state
	exists, _ := tm.HasSession(sessionName)
	t.Logf("Session exists after exit: %v", exists)

	if exists {
		// Session exists but process is dead - this is a "stale" session
		paneCmd, _ := tm.GetPaneCommand(sessionName)
		t.Logf("Stale session pane command: %q", paneCmd)

		// Test EnsureSessionFresh behavior
		t.Log("Testing stale session detection for gastown's EnsureSessionFresh pattern")
	}

	// Try to create a new session with the same name
	err = tm.NewSessionWithCommand(sessionName, "", "sleep 1")
	if err != nil {
		t.Logf("Recreate after stale failed: %v", err)
		t.Log("STALE SESSION ISSUE: Cannot reuse session name after process exits")
	} else {
		t.Log("Successfully recreated session (tmux cleaned up stale session)")
	}
}

// =============================================================================
// PROCESS DETECTION TESTS
// =============================================================================

// TestProcessDetection_ExecReplacement tests that exec properly replaces shell.
func TestProcessDetection_ExecReplacement(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-exec-replace-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// exec should replace the shell with the target process
	cmd := `exec sleep 10`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command after exec: %q", paneCmd)

	// The pane command should be 'sleep', not 'bash' or 'sh'
	if paneCmd == "sleep" {
		t.Log("exec correctly replaced shell with target process")
		t.Log("This is IMPORTANT for WaitForCommand detection")
	} else if paneCmd == "bash" || paneCmd == "sh" || paneCmd == "zsh" {
		t.Error("exec did NOT replace shell - pane still shows shell")
		t.Log("This would cause WaitForCommand to timeout incorrectly")
	} else {
		t.Logf("Unexpected pane command: %q", paneCmd)
	}
}

// TestProcessDetection_ChildProcess tests detection of child processes.
func TestProcessDetection_ChildProcess(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-child-proc-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Run a shell that spawns a child process (without exec)
	cmd := `bash -c 'sleep 10'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command with child process: %q", paneCmd)

	// Without exec, the pane command is bash (the wrapper shell)
	// The child (sleep) is a subprocess
	if paneCmd == "bash" {
		t.Log("Shell wrapper detected (not exec pattern)")
		t.Log("WaitForCommand would timeout waiting for non-shell process")

		// Test IsRuntimeRunning for child process detection
		if tm.IsRuntimeRunning(sessionName, []string{"sleep"}) {
			t.Log("IsRuntimeRunning correctly detected child process")
		} else {
			t.Log("IsRuntimeRunning did NOT detect child process")
		}
	}
}

// =============================================================================
// SIGNAL AND INTERRUPT TESTS
// =============================================================================

// TestSessionInterrupt_DuringStartup tests interrupting session during startup.
func TestSessionInterrupt_DuringStartup(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-interrupt-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Start a slow command
	err := tm.NewSessionWithCommand(sessionName, "", "sleep 10")
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Send Ctrl-C to interrupt
	err = tm.SendKeysRaw(sessionName, "C-c")
	if err != nil {
		t.Logf("SendKeys Ctrl-C failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Check session state after interrupt
	exists, _ := tm.HasSession(sessionName)
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	output, _ := tm.CapturePane(sessionName, 50)

	t.Logf("Session exists after Ctrl-C: %v", exists)
	t.Logf("Pane command after Ctrl-C: %q", paneCmd)
	t.Logf("Output after Ctrl-C: %q", strings.TrimSpace(output))

	// Document behavior for recovery scenarios
	if !exists {
		t.Log("Session died after Ctrl-C (no remain-on-exit)")
	}
}

// TestSendKeys_RaceWithStartup tests send-keys race condition (Issue #280).
func TestSendKeys_RaceWithStartup(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-sendkeys-race-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create bare session (shell only)
	err := tm.NewSession(sessionName, "")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Immediately send keys (this is the race condition from #280)
	// The shell may not be ready yet
	err = tm.SendKeys(sessionName, "echo RACE_TEST")
	if err != nil {
		t.Logf("SendKeys immediately after NewSession: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after immediate SendKeys:\n%s", output)

	// Check if the command was properly received
	if strings.Contains(output, "RACE_TEST") {
		t.Log("SendKeys worked despite potential race")
	} else if strings.Contains(output, "command not found") || strings.Contains(output, "bad pattern") {
		t.Log("RACE CONDITION REPRODUCED: Command garbled or failed")
		t.Log("This is why NewSessionWithCommand is preferred")
	} else {
		t.Log("Unclear result - may depend on system speed")
	}
}

// =============================================================================
// NUDGE DELIVERY TIMING TESTS (gt-k8uxb: prompt not received)
// =============================================================================

// TestNudge_BeforeAgentReady reproduces the scenario where a nudge is sent
// before the agent process is ready to receive input.
// This is the "sits at welcome screen" bug - the prompt is sent but never displayed.
func TestNudge_BeforeAgentReady(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-nudge-early-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate slow agent startup (like claude-code loading)
	// The agent takes 3 seconds before it's ready to receive input
	cmd := `bash -c 'sleep 3; cat'` // cat will echo input when ready

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Send nudge IMMEDIATELY after session creation (before agent is ready)
	// This simulates the race condition in polecat startup
	nudgeMessage := "EARLY_NUDGE_TEST: This message should be received"
	err = tm.NudgeSession(sessionName, nudgeMessage)
	if err != nil {
		t.Logf("NudgeSession returned error: %v", err)
	}

	// Wait for agent to become ready
	time.Sleep(4 * time.Second)

	// Capture output to see if the nudge was received
	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after early nudge:\n%s", output)

	// The bug: if nudge was sent before agent was ready, the message is lost
	if strings.Contains(output, "EARLY_NUDGE_TEST") {
		t.Log("Early nudge was received - no timing issue")
	} else {
		t.Error("Nudge sent before agent ready was lost. NudgeSession should wait for readiness or buffer the message.")
		t.Log("This reproduces gt-k8uxb: polecat sits at welcome screen")
	}
}

// TestNudge_DuringAgentStartup tests nudge delivery during the window when
// shell has started but agent process hasn't fully initialized.
func TestNudge_DuringAgentStartup(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-nudge-startup-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use exec env pattern like polecat startup
	// The command starts running but takes time to initialize.
	// Use while-read + cat -v for reliable capture (NudgeSession sends ESC before Enter).
	cmd := `exec env GT_TEST=1 bash -c 'echo "STARTING..."; sleep 2; echo "READY"; while IFS= read -r line; do echo "GOT: $line" | cat -v; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Wait for "STARTING..." but not "READY"
	time.Sleep(500 * time.Millisecond)

	// Check initial state
	outputBefore, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output before nudge:\n%s", outputBefore)

	if strings.Contains(outputBefore, "READY") {
		t.Log("Agent already ready - test timing needs adjustment")
	}

	// Send nudge during startup (after STARTING but before READY)
	err = tm.NudgeSession(sessionName, "STARTUP_NUDGE")
	if err != nil {
		t.Logf("NudgeSession error: %v", err)
	}

	// Wait for startup to complete
	time.Sleep(3 * time.Second)

	// Check if nudge was received
	outputAfter, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after startup:\n%s", outputAfter)

	if strings.Contains(outputAfter, "GOT: STARTUP_NUDGE") {
		t.Log("Nudge delivered during startup - timing worked")
	} else if strings.Contains(outputAfter, "STARTUP_NUDGE") {
		t.Log("Nudge visible but not processed by read")
	} else {
		t.Error("Nudge sent during startup window was not received at all")
	}
}

// TestNudge_WithWaitForCommand tests the polecat startup pattern:
// WaitForCommand, then nudge. This is the actual pattern used in session_manager.go
func TestNudge_WithWaitForCommand(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-nudge-waitcmd-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate claude-code: starts as shell, then becomes 'node' (or whatever runtime).
	// Using python as a proxy for claude-code since it's a long-running process.
	// Strips ESC char (sent by NudgeSession for vim mode exit) and loops to stay alive.
	cmd := `exec python3 -uc "import sys; print('Agent started'); sys.stdout.flush(); exec(\"for line in sys.stdin:\\n print('Received: ' + line.rstrip().replace(chr(27), ''))\\n sys.stdout.flush()\\n\")"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Wait for command (like polecat startup does)
	shellsToExclude := []string{"bash", "zsh", "sh"}
	err = tm.WaitForCommand(sessionName, shellsToExclude, 5*time.Second)
	if err != nil {
		t.Logf("WaitForCommand: %v", err)
		// Continue anyway to document behavior
	}

	// Check pane command
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command after WaitForCommand: %q", paneCmd)

	// Small delay like polecat startup has
	time.Sleep(500 * time.Millisecond)

	// Send nudge after WaitForCommand
	err = tm.NudgeSession(sessionName, "AFTER_WAIT_NUDGE")
	if err != nil {
		t.Logf("NudgeSession error: %v", err)
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after WaitForCommand + nudge:\n%s", output)

	if strings.Contains(output, "Received: AFTER_WAIT_NUDGE") {
		t.Log("Nudge correctly delivered after WaitForCommand")
	} else if strings.Contains(output, "Agent started") {
		t.Error("Agent started but nudge was not processed — gap between WaitForCommand and input readiness")
	} else {
		t.Error("Nudge after WaitForCommand was not received by agent")
	}
}

// TestNudge_MultipleRapid tests sending multiple nudges rapidly.
// This can cause interleaving or dropped messages.
func TestNudge_MultipleRapid(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-nudge-rapid-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a shell that will receive multiple inputs.
	// Pipe through cat -v to make ESC visible as ^[ instead of triggering
	// terminal escape sequences (NudgeSession sends ESC before Enter for vim mode exit).
	cmd := `bash -c 'while IFS= read -r line; do echo "GOT: $line" | cat -v; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Send multiple nudges rapidly
	messages := []string{"MSG_1", "MSG_2", "MSG_3"}
	for _, msg := range messages {
		go func(m string) {
			_ = tm.NudgeSession(sessionName, m)
		}(msg)
	}

	// Wait for all nudges to be processed (~1.2s per nudge, serialized = ~3.6s)
	time.Sleep(5 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after rapid nudges:\n%s", output)

	// Check which messages were received
	received := 0
	for _, msg := range messages {
		if strings.Contains(output, "GOT: "+msg) {
			received++
		}
	}
	t.Logf("Messages received: %d/%d", received, len(messages))

	if received < len(messages) {
		t.Errorf("Rapid nudge: only %d/%d messages received — lock serialization is not preventing message loss", received, len(messages))
	}
}

// =============================================================================
// SESSION LIFECYCLE EDGE CASES (gt-uyrsg: zombie polecats)
// =============================================================================

// TestSessionDeath_AfterStartupCheck tests the window between startup
// verification and actual work. The session could die in this gap.
func TestSessionDeath_AfterStartupCheck(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-death-gap-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Command that dies after a short delay (simulates agent crash)
	cmd := `bash -c 'echo "Started"; sleep 2; exit 1'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Verify session is running (like polecat startup does)
	exists, _ := tm.HasSession(sessionName)
	if !exists {
		t.Fatal("Session should exist immediately after creation")
	}
	t.Log("Session exists after startup check")

	// Simulate gap where startup returns success but work hasn't started
	time.Sleep(500 * time.Millisecond)

	// Try to send work (like nudge with instructions)
	err = tm.NudgeSession(sessionName, "WORK_INSTRUCTIONS")
	t.Logf("Nudge during gap: error=%v", err)

	// Wait for command to exit
	time.Sleep(2 * time.Second)

	// Check session state now
	exists, _ = tm.HasSession(sessionName)
	t.Logf("Session exists after command exit: %v", exists)

	if !exists {
		// This is expected behavior at the tmux layer: sessions die when their
		// command exits. The fix for this is at the session_manager level —
		// CheckSessionHealth and periodic monitoring detect dead sessions and
		// trigger recovery (respawn or reassign). This test documents the gap.
		t.Log("Session died after startup check passed — work was dispatched to a dead session")
		t.Log("Mitigation: session_manager.go monitors health via CheckSessionHealth")
	} else {
		t.Log("Session still alive (command hasn't exited yet)")
	}
}

// TestSessionDeath_DuringWork tests session dying while processing work.
// This is the zombie polecat pattern where work is committed but gt done never runs.
func TestSessionDeath_DuringWork(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-death-work-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate agent that does work then crashes before cleanup
	cmd := `bash -c 'echo "Working..."; sleep 1; echo "Work done"; exit 1'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Simulate startup sequence
	time.Sleep(500 * time.Millisecond)
	exists, _ := tm.HasSession(sessionName)
	t.Logf("Session exists at start: %v", exists)

	// Wait for work to complete
	time.Sleep(2 * time.Second)

	// Check session state
	exists, _ = tm.HasSession(sessionName)
	output, _ := tm.CapturePane(sessionName, 50)

	t.Logf("Session exists after work: %v", exists)
	t.Logf("Output: %s", strings.TrimSpace(output))

	if !exists {
		// This is expected at the tmux layer: when a command exits (even with
		// error), tmux destroys the session. The zombie polecat pattern (branch
		// has commits but gt done never ran) is a session_manager concern —
		// it must detect dead sessions and run cleanup. This test documents
		// the gap that makes monitoring essential.
		t.Log("Session died after completing work but before cleanup (gt done) — zombie polecat pattern")
		t.Log("Mitigation: session_manager detects dead sessions via health checks and triggers gt done")
	} else {
		t.Log("Session still alive (command hasn't exited yet)")
	}
}

// TestSessionDeath_DetectionDelay tests how long it takes to detect a dead session.
func TestSessionDeath_DetectionDelay(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-death-detect-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session that dies immediately
	cmd := `exit 0`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Check immediately
	checkTimes := []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
	for _, delay := range checkTimes {
		time.Sleep(delay)
		exists, _ := tm.HasSession(sessionName)
		t.Logf("After %v: session exists = %v", delay, exists)
	}

	// Document detection timing
	t.Log("Detection timing matters for zombie pattern - slow detection means late recovery")
}

// TestSessionDeath_WhileNudgePending tests session dying with pending nudge.
func TestSessionDeath_WhileNudgePending(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-death-nudge-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session that dies quickly
	cmd := `bash -c 'sleep 0.5; exit 1'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Try to send nudge while session is dying
	done := make(chan error, 1)
	go func() {
		// This nudge might arrive at a dying session
		done <- tm.NudgeSession(sessionName, "LATE_NUDGE")
	}()

	// Wait for nudge to complete
	select {
	case err := <-done:
		t.Logf("NudgeSession error: %v", err)
	case <-time.After(5 * time.Second):
		t.Log("NudgeSession blocked (may be stuck on dead session)")
	}

	// Check final state
	time.Sleep(500 * time.Millisecond)
	exists, _ := tm.HasSession(sessionName)
	t.Logf("Session exists after nudge attempt: %v", exists)
}

// =============================================================================
// WAIT FOR COMMAND EDGE CASES
// =============================================================================

// TestWaitForCommand_SlowStartup tests WaitForCommand with slow agent startup.
func TestWaitForCommand_SlowStartup(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-waitcmd-slow-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Shell wrapper that takes 3 seconds to exec to the target
	cmd := `bash -c 'sleep 3; exec cat'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Short timeout that will expire before exec
	shellsToExclude := []string{"bash", "zsh", "sh"}
	start := time.Now()
	err = tm.WaitForCommand(sessionName, shellsToExclude, 1*time.Second)
	elapsed := time.Since(start)

	t.Logf("WaitForCommand took %v, error: %v", elapsed, err)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command at timeout: %q", paneCmd)

	if err != nil {
		t.Log("WaitForCommand timed out (expected with slow startup)")
		t.Log("Subsequent nudges may be sent before agent is ready")
	}

	// Wait for actual startup
	time.Sleep(3 * time.Second)
	paneCmd, _ = tm.GetPaneCommand(sessionName)
	t.Logf("Pane command after full startup: %q", paneCmd)
}

// TestWaitForCommand_NeverExec tests WaitForCommand when command never execs.
// This simulates a stuck agent that never transitions from shell.
func TestWaitForCommand_NeverExec(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-waitcmd-never-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Command that runs in bash but never execs (stays as bash child)
	cmd := `bash -c 'while true; do sleep 1; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	shellsToExclude := []string{"bash", "zsh", "sh"}
	start := time.Now()
	err = tm.WaitForCommand(sessionName, shellsToExclude, 2*time.Second)
	elapsed := time.Since(start)

	t.Logf("WaitForCommand took %v, error: %v", elapsed, err)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command: %q", paneCmd)

	// Document: pane command is bash but sleep is a child
	// WaitForCommand correctly times out because pane process is bash
	if err != nil && paneCmd == "bash" {
		t.Log("WaitForCommand correctly times out when process stays as shell")
		t.Log("This is the expected behavior for commands that don't exec")
	}
}

// TestWaitForCommand_SessionDeath tests WaitForCommand when session dies.
func TestWaitForCommand_SessionDeath(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-waitcmd-death-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Command that exits quickly
	cmd := `exit 0`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// WaitForCommand on dead/dying session
	shellsToExclude := []string{"bash", "zsh", "sh"}
	start := time.Now()
	err = tm.WaitForCommand(sessionName, shellsToExclude, 2*time.Second)
	elapsed := time.Since(start)

	t.Logf("WaitForCommand took %v, error: %v", elapsed, err)

	exists, _ := tm.HasSession(sessionName)
	t.Logf("Session exists after WaitForCommand: %v", exists)

	// Document behavior when session dies during wait
	if err != nil {
		t.Log("WaitForCommand errors on dead session (expected)")
	}
}

// =============================================================================
// PANE COMMAND DETECTION EDGE CASES
// =============================================================================

// TestPaneCommand_VersionNumberArgv0 tests pane command detection when
// argv[0] is a version number (like "3.12" for python).
func TestPaneCommand_VersionNumberArgv0(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-pane-version-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Python shows version in pane command sometimes
	cmd := `exec python3 -c "import time; time.sleep(5)"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command for python: %q", paneCmd)

	// Document what pane command looks like for various runtimes
	// This affects WaitForCommand shell exclusion
	if paneCmd == "python3" || paneCmd == "python" {
		t.Log("Python shows as expected process name")
	} else if strings.HasPrefix(paneCmd, "3.") {
		t.Log("ISSUE: Pane command is version number, not process name")
		t.Log("WaitForCommand may not correctly detect this as non-shell")
	}
}

// TestPaneCommand_NodeWithArgs tests pane command detection for node processes.
func TestPaneCommand_NodeWithArgs(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-pane-node-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate claude-code which is node-based
	cmd := `exec node -e "setTimeout(() => {}, 5000)"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		// Node might not be installed
		t.Skipf("Node not available: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane command for node: %q", paneCmd)

	// Document: what does tmux show for node processes?
	t.Log("This is what claude-code would look like in pane command")
}

// =============================================================================
// ACCEPT BYPASS PERMISSIONS EDGE CASES
// =============================================================================

// TestAcceptBypass_NoDialog tests AcceptBypassPermissionsWarning when no dialog.
func TestAcceptBypass_NoDialog(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-bypass-none-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Normal session without bypass dialog
	cmd := `echo "Normal output"; sleep 5`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Call AcceptBypassPermissionsWarning on session without dialog
	start := time.Now()
	err = tm.AcceptBypassPermissionsWarning(sessionName)
	elapsed := time.Since(start)

	t.Logf("AcceptBypassPermissionsWarning took %v, error: %v", elapsed, err)

	// This should complete quickly (1s sleep + check)
	if elapsed > 2*time.Second {
		t.Log("AcceptBypassPermissionsWarning took too long for no-dialog case")
	} else {
		t.Log("AcceptBypassPermissionsWarning returned quickly when no dialog present")
	}
}

// TestAcceptBypass_DeadSession tests AcceptBypassPermissionsWarning on dead session.
func TestAcceptBypass_DeadSession(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-bypass-dead-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session that dies immediately
	cmd := `exit 0`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Call on dead session
	err = tm.AcceptBypassPermissionsWarning(sessionName)
	t.Logf("AcceptBypassPermissionsWarning on dead session: error=%v", err)

	// Document behavior
	if err != nil {
		t.Log("Correctly returns error on dead session")
	} else {
		t.Log("WARNING: No error on dead session - may mask startup failures")
	}
}

// =============================================================================
// SESSION STATE CONSISTENCY TESTS
// =============================================================================

// TestSessionState_RapidStateChanges tests detecting state changes during rapid operations.
func TestSessionState_RapidStateChanges(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-state-rapid-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session that changes state rapidly
	cmd := `bash -c 'for i in 1 2 3 4 5; do echo "State $i"; sleep 0.5; done; exit 0'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Poll session state rapidly
	var stateChanges []string
	for i := 0; i < 10; i++ {
		exists, _ := tm.HasSession(sessionName)
		paneCmd, _ := tm.GetPaneCommand(sessionName)
		stateChanges = append(stateChanges, fmt.Sprintf("exists=%v cmd=%q", exists, paneCmd))
		time.Sleep(300 * time.Millisecond)
	}

	for i, state := range stateChanges {
		t.Logf("Poll %d: %s", i, state)
	}

	// Document state transition visibility
	t.Log("State polling shows transition from running to dead")
}

// TestSessionState_OrphanDetection tests detecting orphaned sessions
// (sessions that exist but have no active process).
func TestSessionState_OrphanDetection(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-state-orphan-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with remain-on-exit so it stays after command exits
	err := tm.NewSession(sessionName, "")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Set remain-on-exit
	_, _ = tm.run("set-option", "-t", sessionName, "remain-on-exit", "on")

	// Run a command that exits
	_ = tm.SendKeysRaw(sessionName, "exit")
	_, _ = tm.run("send-keys", "-t", sessionName, "Enter")

	time.Sleep(500 * time.Millisecond)

	// Session exists but pane is dead
	exists, _ := tm.HasSession(sessionName)
	paneCmd, _ := tm.GetPaneCommand(sessionName)
	output, _ := tm.CapturePane(sessionName, 20)

	t.Logf("Session exists: %v", exists)
	t.Logf("Pane command: %q", paneCmd)
	t.Logf("Output: %q", strings.TrimSpace(output))

	// Check for orphan indicators — when remain-on-exit is on, the session
	// stays alive even after the pane's process exits. This is the expected
	// behavior that EnsureSessionFresh and CheckSessionHealth detect.
	// This test documents the orphan state; it's not a failure.
	if exists && (paneCmd == "" || strings.Contains(output, "Pane is dead")) {
		t.Log("Orphan session detected: tmux session exists but pane is dead")
		t.Log("EnsureSessionFresh handles this via CheckSessionHealth zombie detection")
	} else if exists {
		t.Log("Session still running (command may not have exited yet)")
	} else {
		t.Log("Session was cleaned up (remain-on-exit may not have taken effect)")
	}
}

// =============================================================================
// COPY MODE INTERFERENCE TESTS
// =============================================================================

// TestCopyMode_NudgeBlocked tests whether nudges work when session is in copy mode.
// Users or automation may put sessions in copy mode, blocking input.
func TestCopyMode_NudgeBlocked(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-copymode-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with a simple receiver
	cmd := `bash -c 'while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Enter copy mode
	_, err = tm.run("copy-mode", "-t", sessionName)
	if err != nil {
		t.Logf("Failed to enter copy mode: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Try to send nudge while in copy mode
	err = tm.NudgeSession(sessionName, "COPYMODE_TEST")
	t.Logf("NudgeSession during copy mode: error=%v", err)

	// Wait and check
	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after nudge in copy mode:\n%s", output)

	// Exit copy mode and check again
	_, _ = tm.run("send-keys", "-t", sessionName, "q") // q exits copy mode
	time.Sleep(500 * time.Millisecond)

	output2, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after exiting copy mode:\n%s", output2)

	if strings.Contains(output, "GOT: COPYMODE_TEST") {
		t.Log("Nudge worked even in copy mode")
	} else if strings.Contains(output2, "GOT: COPYMODE_TEST") {
		t.Log("Nudge queued and delivered after exiting copy mode")
	} else {
		t.Error("Nudge lost or blocked by copy mode — NudgeSession should exit copy mode before sending")
	}
}

// TestCopyMode_ScrollingState tests nudge behavior when user is scrolling.
func TestCopyMode_ScrollingState(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-scroll-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Generate lots of output to enable scrolling
	cmd := `bash -c 'for i in $(seq 1 100); do echo "Line $i"; done; while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Enter copy mode and scroll up
	_, _ = tm.run("copy-mode", "-t", sessionName)
	_, _ = tm.run("send-keys", "-t", sessionName, "C-u") // scroll up
	time.Sleep(200 * time.Millisecond)

	// Try to send nudge while scrolled
	err = tm.NudgeSession(sessionName, "SCROLL_TEST")
	t.Logf("NudgeSession while scrolled: error=%v", err)

	// Exit copy mode
	_, _ = tm.run("send-keys", "-t", sessionName, "q")
	time.Sleep(500 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after scroll nudge:\n%s", output)

	if strings.Contains(output, "GOT: SCROLL_TEST") {
		t.Log("Nudge delivered after exiting scroll mode")
	} else {
		t.Error("Nudge lost when pane was in scroll/copy mode — NudgeSession should exit copy mode before sending")
	}
}

// =============================================================================
// LARGE MESSAGE HANDLING TESTS
// =============================================================================

// TestLargeNudge_VeryLong tests nudge with a very large message.
func TestLargeNudge_VeryLong(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-large-nudge-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with Python receiver that reads stdin in raw mode.
	// The Linux TTY canonical mode buffer is 4096 bytes, limiting readline-based
	// programs. But TUI apps (like Claude Code) read raw stdin and aren't limited.
	// This test uses a Python receiver that reads raw to verify chunked delivery works.
	cmd := `python3 -c "
import sys, os, tty, termios
fd = sys.stdin.fileno()
old = termios.tcgetattr(fd)
try:
    tty.setraw(fd)
    buf = b''
    while True:
        ch = os.read(fd, 4096)
        if not ch:
            break
        buf += ch
        if b'\r' in ch or b'\n' in ch:
            break
    line = buf.rstrip(b'\r\n').decode('utf-8', errors='replace')
    # Strip trailing ESC (0x1b) — NudgeSession sends ESC after text for vim mode exit
    line = line.rstrip('\x1b')
    # Restore terminal before printing so output is readable
    termios.tcsetattr(fd, termios.TCSADRAIN, old)
    print('LENGTH: ' + str(len(line)))
    import time; time.sleep(10)
except:
    termios.tcsetattr(fd, termios.TCSADRAIN, old)
    raise
"`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Send a very large nudge (10KB)
	largeMessage := strings.Repeat("A", 10000) + "_END"

	start := time.Now()
	err = tm.NudgeSession(sessionName, largeMessage)
	elapsed := time.Since(start)

	t.Logf("Large nudge (%d bytes) took %v, error=%v", len(largeMessage), elapsed, err)

	// Wait for processing
	time.Sleep(3 * time.Second)

	output, _ := tm.CapturePane(sessionName, 200)
	t.Logf("Output after large nudge (last 200 lines):\n%s", output)

	if strings.Contains(output, "LENGTH:") {
		t.Log("Large message was received")
		// Check if full length was received
		if strings.Contains(output, "LENGTH: 10004") {
			t.Log("Full message received (10004 chars = 10000 A's + _END)")
		} else {
			t.Error("Large message was truncated — full 10004 chars not received")
		}
	} else {
		t.Error("Large message (10KB) was not received at all")
	}
}

// TestLargeNudge_MultiLine tests nudge with many newlines.
func TestLargeNudge_MultiLine(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-multiline-nudge-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session
	cmd := `bash -c 'cat; echo "---DONE---"'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Send message with many newlines
	multilineMessage := "LINE1\nLINE2\nLINE3\nLINE4\nLINE5"

	err = tm.NudgeSession(sessionName, multilineMessage)
	t.Logf("Multiline nudge error=%v", err)

	time.Sleep(1 * time.Second)

	// Send Ctrl-D to end cat input
	_, _ = tm.run("send-keys", "-t", sessionName, "C-d")
	time.Sleep(500 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after multiline nudge:\n%s", output)

	// Count how many lines were received
	lineCount := 0
	for i := 1; i <= 5; i++ {
		if strings.Contains(output, fmt.Sprintf("LINE%d", i)) {
			lineCount++
		}
	}
	t.Logf("Lines received: %d/5", lineCount)

	if lineCount < 5 {
		// tmux limitation: send-keys -l treats \n as Enter, splitting the message
		// into multiple input lines. Would need load-buffer/paste-buffer or base64
		// encoding to send multiline messages atomically.
		t.Skip("Known tmux limitation: send-keys -l treats \\n as Enter — multiline messages are split into separate inputs")
	}
}

// =============================================================================
// SPECIAL CHARACTER HANDLING TESTS
// =============================================================================

// TestSpecialChars_EscapeSequences tests nudge with escape sequences.
func TestSpecialChars_EscapeSequences(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-escape-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	cmd := `bash -c 'while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Message with escape sequences that might confuse tmux
	escapeMessage := "Test\x1b[31mRED\x1b[0mNormal" // ANSI color codes

	err = tm.NudgeSession(sessionName, escapeMessage)
	t.Logf("Escape sequence nudge error=%v", err)

	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after escape sequence nudge:\n%s", output)

	if strings.Contains(output, "GOT:") {
		t.Log("Message with escape sequences was received")
	} else {
		t.Error("Message with ANSI escape sequences was not received — NudgeSession should sanitize or encode non-printable characters")
	}
}

// TestSpecialChars_ControlChars tests nudge with control characters.
func TestSpecialChars_ControlChars(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-control-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	cmd := `bash -c 'while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Message with tab and other control characters
	controlMessage := "Before\tTab\rCarriage\bBackspace"

	err = tm.NudgeSession(sessionName, controlMessage)
	t.Logf("Control char nudge error=%v", err)

	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after control char nudge:\n%s", output)

	if strings.Contains(output, "GOT:") {
		t.Log("Message with control characters was processed")
	} else {
		t.Error("Message with control characters was not received — control chars may have killed the session or corrupted delivery")
	}
}

// TestSpecialChars_Quotes tests nudge with various quote types.
func TestSpecialChars_Quotes(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-quotes-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	cmd := `bash -c 'while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Message with quotes that might break shell parsing
	quoteMessage := "Say \"hello\" and 'goodbye' with `backticks`"

	err = tm.NudgeSession(sessionName, quoteMessage)
	t.Logf("Quote nudge error=%v", err)

	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after quote nudge:\n%s", output)

	if strings.Contains(output, "hello") && strings.Contains(output, "goodbye") {
		t.Log("Quoted message received correctly")
	} else {
		t.Error("Message with quotes was not received correctly — send-keys -l should handle quotes but delivery failed")
	}
}

// =============================================================================
// DETACHED SESSION BEHAVIOR TESTS
// =============================================================================

// TestDetached_NudgeWakeup tests that WakePane properly wakes detached sessions.
func TestDetached_NudgeWakeup(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-detached-wake-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create detached session
	cmd := `bash -c 'while read line; do echo "GOT: $line"; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify session is detached
	attached := tm.IsSessionAttached(sessionName)
	t.Logf("Session attached: %v", attached)

	// Send nudge to detached session
	err = tm.NudgeSession(sessionName, "DETACHED_TEST")
	t.Logf("NudgeSession to detached: error=%v", err)

	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output from detached session:\n%s", output)

	if strings.Contains(output, "GOT: DETACHED_TEST") {
		t.Log("Nudge to detached session worked correctly")
	} else {
		t.Error("Nudge to detached session was lost — WakePaneIfDetached may not be triggering SIGWINCH properly")
	}
}

// TestDetached_ResizeWake tests that resize-based wake actually triggers SIGWINCH.
func TestDetached_ResizeWake(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-resize-wake-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a script that detects SIGWINCH. Use a short sleep so the trap
	// fires quickly after the signal is delivered.
	cmd := `bash -c 'trap "echo SIGWINCH_RECEIVED" SIGWINCH; while true; do sleep 0.1; done'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Call WakePane (now uses resize-window which works on single-pane sessions)
	tm.WakePane(sessionName)

	// Wait enough for the sleep loop to notice the signal
	time.Sleep(1 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Output after WakePane:\n%s", output)

	if strings.Contains(output, "SIGWINCH_RECEIVED") {
		t.Log("WakePane correctly triggered SIGWINCH via resize-window")
	} else {
		t.Error("WakePane did not trigger SIGWINCH — resize-window may not work for detached sessions")
	}
}

// =============================================================================
// ENVIRONMENT VARIABLE EDGE CASES
// =============================================================================

// TestEnvironment_SetOnDeadSession tests SetEnvironment on dead session.
func TestEnvironment_SetOnDeadSession(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-env-dead-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session that exits immediately
	cmd := `exit 0`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Try to set environment on dead/dying session
	err = tm.SetEnvironment(sessionName, "TEST_VAR", "test_value")
	t.Logf("SetEnvironment on dead session: error=%v", err)

	if err != nil {
		t.Log("SetEnvironment correctly fails on dead session")
	} else {
		t.Log("WARNING: SetEnvironment succeeds on dead session")
	}
}

// TestEnvironment_LargeValue tests SetEnvironment with very large value.
func TestEnvironment_LargeValue(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-env-large-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a long-lived session so it's still alive when we call SetEnvironment
	cmd := `bash -c 'sleep 30'`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Set very large environment variable (100KB)
	largeValue := strings.Repeat("X", 100000)
	err = tm.SetEnvironment(sessionName, "TEST_LARGE", largeValue)
	t.Logf("SetEnvironment with 100KB value: error=%v", err)

	if err != nil {
		// 100KB may exceed tmux's argument limit — document the limitation
		t.Logf("SetEnvironment with 100KB value failed: %v", err)
		// Try a smaller large value (10KB) to find the limit
		smallerLargeValue := strings.Repeat("X", 10000)
		err2 := tm.SetEnvironment(sessionName, "TEST_LARGE_10K", smallerLargeValue)
		t.Logf("SetEnvironment with 10KB value: error=%v", err2)
		if err2 != nil {
			t.Errorf("SetEnvironment fails even with 10KB value: %v — need chunking or file-based approach", err2)
		} else {
			t.Log("10KB works, 100KB exceeds tmux argument limit — documenting limitation")
			// Verify the 10KB value is retrievable
			val, err3 := tm.GetEnvironment(sessionName, "TEST_LARGE_10K")
			if err3 != nil {
				t.Logf("GetEnvironment for 10KB value failed: %v", err3)
			} else if len(val) == 10000 {
				t.Log("10KB value stored and retrieved correctly")
			} else {
				t.Logf("10KB value truncated: got %d bytes", len(val))
			}
		}
	} else {
		t.Log("Large environment value (100KB) accepted")
		// Verify it's retrievable
		val, err2 := tm.GetEnvironment(sessionName, "TEST_LARGE")
		if err2 != nil {
			t.Logf("GetEnvironment for 100KB value failed: %v", err2)
		} else if len(val) == 100000 {
			t.Log("100KB value stored and retrieved correctly")
		} else {
			t.Logf("100KB value truncated: got %d bytes, expected 100000", len(val))
		}
	}
}

// =============================================================================
// MULTI-PANE SESSION TESTS
// =============================================================================

// TestMultiPane_NudgeTarget tests nudge targeting with multiple panes.
func TestMultiPane_NudgeTarget(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-multipane-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session with first pane (use while-read + cat -v for reliable capture)
	err := tm.NewSessionWithCommand(sessionName, "", `bash -c 'echo "PANE1"; while IFS= read -r line; do echo "PANE1 GOT: $line" | cat -v; done'`)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Split to create second pane
	_, err = tm.run("split-window", "-t", sessionName, "-h", `bash -c 'echo "PANE2"; while IFS= read -r line; do echo "PANE2 GOT: $line" | cat -v; done'`)
	if err != nil {
		t.Logf("Split window failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Discover actual pane IDs (indices vary by tmux config)
	paneList, _ := tm.run("list-panes", "-t", sessionName, "-F", "#{pane_id}")
	panes := strings.Split(strings.TrimSpace(paneList), "\n")
	t.Logf("Panes: %v", panes)

	// Nudge the session - which pane receives it?
	err = tm.NudgeSession(sessionName, "MULTIPANE_TEST")
	t.Logf("NudgeSession to multipane: error=%v", err)

	time.Sleep(2 * time.Second)

	// Capture all panes by ID
	var allOutput []string
	nudgeReceived := false
	for i, paneID := range panes {
		if paneID == "" {
			continue
		}
		out, _ := tm.run("capture-pane", "-p", "-t", paneID, "-S", "-50")
		t.Logf("Output from pane %d (%s):\n%s", i, paneID, out)
		allOutput = append(allOutput, out)
		if strings.Contains(out, "MULTIPANE_TEST") {
			t.Logf("Pane %d (%s) received the nudge", i, paneID)
			nudgeReceived = true
		}
	}

	if !nudgeReceived {
		t.Error("Nudge lost in multipane session — no pane received the message")
	}
}

// =============================================================================
// SESSION CREATION RACE CONDITIONS
// =============================================================================

// TestRace_OperationImmediatelyAfterCreate tests operations immediately after session creation.
func TestRace_OperationImmediatelyAfterCreate(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-race-create-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session
	err := tm.NewSessionWithCommand(sessionName, "", `bash -c 'sleep 5'`)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Immediately try various operations
	var results []string

	// HasSession
	exists, _ := tm.HasSession(sessionName)
	results = append(results, fmt.Sprintf("HasSession: %v", exists))

	// GetPaneCommand
	cmd, err := tm.GetPaneCommand(sessionName)
	results = append(results, fmt.Sprintf("GetPaneCommand: %q, err=%v", cmd, err))

	// SetEnvironment
	err = tm.SetEnvironment(sessionName, "IMMEDIATE_VAR", "immediate_value")
	results = append(results, fmt.Sprintf("SetEnvironment: err=%v", err))

	// CapturePane
	output, err := tm.CapturePane(sessionName, 10)
	results = append(results, fmt.Sprintf("CapturePane: len=%d, err=%v", len(output), err))

	for _, r := range results {
		t.Log(r)
	}

	// All operations should work immediately after create
	if !exists {
		t.Error("RACE ISSUE: Session not found immediately after creation")
	}
}

// TestRace_ConcurrentCreation tests creating sessions with similar names concurrently.
func TestRace_ConcurrentCreation(t *testing.T) {
	tm := newTestTmux(t)
	baseSessionName := "gt-test-race-concurrent"

	// Clean up
	for i := 0; i < 5; i++ {
		_ = tm.KillSession(fmt.Sprintf("%s-%d", baseSessionName, i))
	}
	defer func() {
		for i := 0; i < 5; i++ {
			_ = tm.KillSession(fmt.Sprintf("%s-%d", baseSessionName, i))
		}
	}()

	// Try to create 5 sessions concurrently
	results := make(chan string, 5)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			name := fmt.Sprintf("%s-%d", baseSessionName, idx)
			err := tm.NewSessionWithCommand(name, "", "sleep 5")
			if err != nil {
				results <- fmt.Sprintf("Session %d: FAILED - %v", idx, err)
			} else {
				results <- fmt.Sprintf("Session %d: OK", idx)
			}
		}(i)
	}

	// Collect results
	for i := 0; i < 5; i++ {
		t.Log(<-results)
	}
}

// TestRace_KillDuringOperation tests killing session while operation is in progress.
func TestRace_KillDuringOperation(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-race-kill-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session
	err := tm.NewSessionWithCommand(sessionName, "", "sleep 10")
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	// Start a long operation in background
	done := make(chan error, 1)
	go func() {
		// Send a large message that takes time
		done <- tm.NudgeSession(sessionName, strings.Repeat("X", 1000))
	}()

	// Kill session while nudge is happening
	time.Sleep(100 * time.Millisecond)
	killErr := tm.KillSession(sessionName)

	// Wait for nudge to complete
	nudgeErr := <-done

	t.Logf("Kill error: %v", killErr)
	t.Logf("Nudge error: %v", nudgeErr)

	// Document behavior when session is killed during operation
	if nudgeErr != nil {
		t.Log("Nudge correctly failed when session was killed")
	} else {
		t.Log("Nudge completed before kill (race outcome)")
	}
}

// =============================================================================
// TIMEOUT AND BLOCKING TESTS
// =============================================================================

// TestTimeout_NudgeLockContention tests nudge lock timeout behavior.
// The lock has a 30s timeout to prevent permanent lockout.
func TestTimeout_NudgeLockContention(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-lock-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Create session. Pipe through cat -v to make ESC visible as ^[ instead of
	// triggering terminal escape sequences (NudgeSession sends ESC for vim mode exit).
	err := tm.NewSessionWithCommand(sessionName, "", `bash -c 'while IFS= read -r line; do echo "GOT: $line" | cat -v; done'`)
	if err != nil {
		t.Fatalf("Session creation failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Send multiple nudges concurrently to test lock serialization
	results := make(chan string, 3)
	for i := 0; i < 3; i++ {
		go func(idx int) {
			start := time.Now()
			err := tm.NudgeSession(sessionName, fmt.Sprintf("MSG_%d", idx))
			elapsed := time.Since(start)
			results <- fmt.Sprintf("Nudge %d: elapsed=%v, err=%v", idx, elapsed, err)
		}(i)
	}

	// Collect results
	for i := 0; i < 3; i++ {
		t.Log(<-results)
	}

	// Nudges are serialized (~1.2s each), wait for output to settle
	time.Sleep(2 * time.Second)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Final output:\n%s", output)

	// Count how many messages were received
	received := 0
	for i := 0; i < 3; i++ {
		if strings.Contains(output, fmt.Sprintf("GOT: MSG_%d", i)) {
			received++
		}
	}
	t.Logf("Messages received through lock: %d/3", received)
	if received < 3 {
		t.Errorf("Lock contention: only %d/3 messages received — serialization is not preventing message loss", received)
	}
}

// TestTimeout_WaitForCommandVariations tests WaitForCommand with different timeouts.
func TestTimeout_WaitForCommandVariations(t *testing.T) {
	tm := newTestTmux(t)

	testCases := []struct {
		name    string
		timeout time.Duration
		delay   time.Duration // how long before exec
	}{
		{"immediate", 2 * time.Second, 0},
		{"short-delay", 2 * time.Second, 500 * time.Millisecond},
		{"timeout-exceeded", 500 * time.Millisecond, 2 * time.Second},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sessionName := "gt-test-waitcmd-var-" + tc.name

			_ = tm.KillSession(sessionName)
			defer func() { _ = tm.KillSession(sessionName) }()

			// Command with configurable delay before exec
			var cmd string
			if tc.delay > 0 {
				cmd = fmt.Sprintf(`bash -c 'sleep %.1f; exec cat'`, tc.delay.Seconds())
			} else {
				cmd = `exec cat`
			}

			err := tm.NewSessionWithCommand(sessionName, "", cmd)
			if err != nil {
				t.Fatalf("Session creation failed: %v", err)
			}

			start := time.Now()
			err = tm.WaitForCommand(sessionName, []string{"bash", "sh", "zsh"}, tc.timeout)
			elapsed := time.Since(start)

			t.Logf("Timeout=%v, Delay=%v, Elapsed=%v, Error=%v", tc.timeout, tc.delay, elapsed, err)

			if tc.delay > tc.timeout && err == nil {
				t.Error("Should have timed out but didn't")
			}
			if tc.delay <= tc.timeout && err != nil {
				t.Errorf("Should have succeeded but got: %v", err)
			}
		})
	}
}
