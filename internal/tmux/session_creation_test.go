package tmux

import (
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-blank-notfound-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Use a non-existent binary - this simulates claude-code not being in PATH
	cmd := "/nonexistent/path/to/binary --some-flag"

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed (tmux error): %v", err)
	}

	// Wait a bit for the command to fail
	time.Sleep(200 * time.Millisecond)

	// Check if session still exists
	exists, _ := tm.HasSession(sessionName)

	if !exists {
		// Session died because the command failed
		t.Log("BLANK WINDOW SCENARIO: Session died immediately when binary not found")
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
		t.Log("BLANK WINDOW REPRODUCED: Pane is completely empty")
	} else if strings.Contains(output, "not found") || strings.Contains(output, "No such file") {
		t.Log("Error message visible: User can see what went wrong")
	}
}

// TestBlankWindow_WorkDirNotExists reproduces blank window when working directory
// doesn't exist. The -c flag to tmux new-session may fail silently.
func TestBlankWindow_WorkDirNotExists(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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

	if strings.TrimSpace(output) == "" {
		t.Log("BLANK WINDOW: Session created but pane empty due to workdir issue")
	}
}

// TestBlankWindow_SyntaxError reproduces blank window when command has shell syntax error.
// The shell parses the command, finds an error, and exits.
func TestBlankWindow_SyntaxError(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-blank-syntax-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Command with unclosed quote - shell syntax error
	cmd := `echo "unclosed quote`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	output, _ := tm.CapturePane(sessionName, 50)
	t.Logf("Pane output with syntax error:\n%s", output)

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	t.Logf("Pane current command: %q", paneCmd)

	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" {
		t.Log("BLANK WINDOW REPRODUCED: Shell exited due to syntax error")
	} else if strings.Contains(output, "unexpected") || strings.Contains(output, "syntax") {
		t.Log("Syntax error message visible - user can see what went wrong")
	}
}

// TestBlankWindow_ExecEnvBinaryNotFound reproduces blank window with exec env pattern.
// This is the exact pattern used by gastown for polecat startup.
func TestBlankWindow_ExecEnvBinaryNotFound(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
	sessionName := "gt-test-blank-execenv-" + t.Name()

	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate the exact pattern gastown uses: exec env VAR=value ... binary
	// If the binary doesn't exist, the exec fails and shell exits
	cmd := `exec env GT_TEST=1 GT_ROLE=test /nonexistent/claude-code --settings /tmp`

	err := tm.NewSessionWithCommand(sessionName, "", cmd)
	if err != nil {
		t.Fatalf("NewSessionWithCommand failed: %v", err)
	}

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
		t.Log("BLANK WINDOW REPRODUCED with exec env pattern")
		t.Log("Root cause: exec replaced shell, then binary not found, pane died")
	}
}

// =============================================================================
// COMMAND VISIBLE AT TOP TESTS
// =============================================================================

// TestCommandVisibleAtTop_SetX reproduces command being echoed when shell has set -x.
// This happens when tmux's default-shell or the user's shell has xtrace enabled.
func TestCommandVisibleAtTop_SetX(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()

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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	tm := NewTmux()
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
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	// Set a test variable in current environment
	testVar := "GT_TEST_LEAK_VAR"
	os.Setenv(testVar, "LEAKED_VALUE")
	defer os.Unsetenv(testVar)

	tm := NewTmux()
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
		t.Log("Environment variables from parent process leak into tmux sessions")
		t.Log("This could cause stale GT_ROLE or other vars to affect polecats")
	} else {
		t.Log("Environment variables do NOT leak - tmux uses server environment")
	}
}
