// Package tmux provides integration tests for the Terminal interface implementation.
//
// These tests use real tmux commands and verify full adherence to the Terminal API.
// Run with: go test -tags=integration -v ./internal/tmux/
//
//go:build integration

package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/session"
)

// testPrefix is used to namespace test sessions and avoid conflicts.
const testPrefix = "gt-test-"

// TestMain ensures tmux is available and cleans up after all tests.
func TestMain(m *testing.M) {
	// Verify tmux is installed
	if err := exec.Command("tmux", "-V").Run(); err != nil {
		fmt.Println("SKIP: tmux not available")
		os.Exit(0)
	}

	code := m.Run()

	// Cleanup any leftover test sessions
	cleanupTestSessions()

	os.Exit(code)
}

// cleanupTestSessions removes all sessions starting with testPrefix.
func cleanupTestSessions() {
	t := NewTmux()
	sessions, _ := t.List()
	for _, id := range sessions {
		if strings.HasPrefix(string(id), testPrefix) {
			_ = t.Stop(id)
		}
	}
}

// uniqueSessionName generates a unique session name for each test.
func uniqueSessionName(t *testing.T) string {
	return fmt.Sprintf("%s%s-%d", testPrefix, t.Name(), time.Now().UnixNano())
}

// TestStart verifies the Start method creates a session running a command.
func TestStart(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	t.Run("creates session with command", func(t *testing.T) {
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if string(id) != name {
			t.Errorf("expected SessionID %q, got %q", name, id)
		}

		// Verify session exists
		exists, err := tmx.Exists(id)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("session should exist after Start")
		}
	})

	t.Run("creates session with workDir", func(t *testing.T) {
		name2 := name + "-workdir"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name2))
		})

		workDir := os.TempDir()
		id, err := tmx.Start(name2, workDir, "pwd && sleep 30")
		if err != nil {
			t.Fatalf("Start with workDir failed: %v", err)
		}

		// Give the command a moment to run
		time.Sleep(200 * time.Millisecond)

		// Capture output should show the working directory
		output, err := tmx.Capture(id, 10)
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}

		// Normalize paths for comparison (handle symlinks)
		expectedDir, _ := exec.Command("realpath", workDir).Output()
		expectedDirStr := strings.TrimSpace(string(expectedDir))

		if !strings.Contains(output, expectedDirStr) && !strings.Contains(output, workDir) {
			t.Errorf("expected workDir %q in output, got %q", workDir, output)
		}
	})

	t.Run("returns error for duplicate session", func(t *testing.T) {
		// Try to create same session again
		_, err := tmx.Start(name, "", "sleep 60")
		if err == nil {
			t.Error("expected error for duplicate session")
		}
	})
}

// TestStop verifies the Stop method terminates sessions and cleans up processes.
func TestStop(t *testing.T) {
	tmx := NewTmux()

	t.Run("terminates existing session", func(t *testing.T) {
		name := uniqueSessionName(t)
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		err = tmx.Stop(id)
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}

		// Verify session no longer exists
		exists, _ := tmx.Exists(id)
		if exists {
			t.Error("session should not exist after Stop")
		}
	})

	t.Run("handles non-existent session gracefully", func(t *testing.T) {
		// Create and keep a session alive to ensure server stays up
		keepAlive := uniqueSessionName(t) + "-keepalive"
		_, _ = tmx.Start(keepAlive, "", "sleep 60")
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(keepAlive))
		})

		id := session.SessionID(testPrefix + "nonexistent")
		err := tmx.Stop(id)
		// Stop should succeed for non-existent session (goal is to ensure it's stopped)
		if err != nil {
			t.Errorf("Stop on non-existent session should succeed (nothing to stop): %v", err)
		}
	})
}

// TestStopOrphanPrevention tests that Stop kills descendant processes.
// This is a separate test to ensure proper isolation and server availability.
func TestStopOrphanPrevention(t *testing.T) {
	tmx := NewTmux()

	// Keep a session alive throughout the test to ensure server doesn't shut down
	keepAlive := uniqueSessionName(t) + "-keepalive"
	_, err := tmx.Start(keepAlive, "", "sleep 300")
	if err != nil {
		t.Fatalf("Failed to create keepalive session: %v", err)
	}
	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(keepAlive))
	})

	name := uniqueSessionName(t)

	// Start a session that spawns child processes
	// Use bash to create a process tree: bash -> sleep (child) -> (grandchild simulated)
	id, err := tmx.Start(name, "", "bash -c 'sleep 300 & sleep 300 & wait'")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for processes to spawn
	time.Sleep(500 * time.Millisecond)

	// Get the pane PID to find descendants
	panePID, err := tmx.GetPanePID(string(id))
	if err != nil {
		t.Fatalf("GetPanePID failed: %v", err)
	}

	// Verify children exist
	out, _ := exec.Command("pgrep", "-P", panePID).Output()
	childPIDs := strings.Fields(strings.TrimSpace(string(out)))
	if len(childPIDs) == 0 {
		_ = tmx.Stop(id)
		t.Skip("no child processes spawned - test environment may not support this")
	}

	// Stop the session (this kills descendants first, then the session)
	err = tmx.Stop(id)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Wait for processes to be cleaned up
	time.Sleep(300 * time.Millisecond)

	// Verify descendant processes are killed
	for _, pid := range childPIDs {
		// Check if process still exists
		err := exec.Command("kill", "-0", pid).Run()
		if err == nil {
			t.Errorf("child process %s still running after Stop (orphan!)", pid)
		}
	}
}

// TestExists verifies the Exists method with exact matching.
func TestExists(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	t.Run("returns false for non-existent session", func(t *testing.T) {
		exists, err := tmx.Exists(session.SessionID(name))
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Error("session should not exist before Start")
		}
	})

	t.Run("returns true for existing session", func(t *testing.T) {
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		exists, err := tmx.Exists(id)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("session should exist after Start")
		}
	})

	t.Run("uses exact match (no prefix matching)", func(t *testing.T) {
		// Create a session with a suffix
		nameSuffix := name + "-suffix"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(nameSuffix))
		})

		_, err := tmx.Start(nameSuffix, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Checking for the base name (prefix) should return false
		exists, _ := tmx.Exists(session.SessionID(name + "-suf"))
		if exists {
			t.Error("Exists should use exact match, not prefix match")
		}
	})
}

// TestSend verifies the Send method sends text with Enter.
func TestSend(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	// Start a session with cat to echo back input
	id, err := tmx.Start(name, "", "cat")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for cat to be ready
	time.Sleep(300 * time.Millisecond)

	t.Run("sends text and Enter", func(t *testing.T) {
		testMsg := "hello-from-test"
		err := tmx.Send(id, testMsg)
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		// Wait for cat to echo back
		time.Sleep(200 * time.Millisecond)

		output, err := tmx.Capture(id, 10)
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}

		if !strings.Contains(output, testMsg) {
			t.Errorf("expected %q in output, got %q", testMsg, output)
		}
	})

	t.Run("handles special characters", func(t *testing.T) {
		// Test that literal mode handles special chars
		testMsg := "test with $pecial chars: <>&|"
		err := tmx.Send(id, testMsg)
		if err != nil {
			t.Fatalf("Send with special chars failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		output, err := tmx.Capture(id, 10)
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}

		// Special chars should appear literally in output
		if !strings.Contains(output, "$pecial") {
			t.Errorf("special characters not preserved in output: %q", output)
		}
	})
}

// TestSendControl verifies the SendControl method sends control sequences.
func TestSendControl(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	t.Run("sends C-c to interrupt", func(t *testing.T) {
		// Start a session running sleep
		id, err := tmx.Start(name, "", "sleep 300")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Wait for sleep to start
		time.Sleep(300 * time.Millisecond)

		// Send Ctrl-C
		err = tmx.SendControl(id, "C-c")
		if err != nil {
			t.Fatalf("SendControl C-c failed: %v", err)
		}

		// Wait for interrupt to take effect
		time.Sleep(500 * time.Millisecond)

		// The sleep command should have exited
		cmd, _ := tmx.GetPaneCommand(string(id))
		if cmd == "sleep" {
			t.Error("sleep should have been interrupted by C-c")
		}
	})

	t.Run("sends arrow keys", func(t *testing.T) {
		name2 := name + "-arrow"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name2))
		})

		// Start bash session
		id, err := tmx.Start(name2, "", "bash")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		time.Sleep(300 * time.Millisecond)

		// Send Down arrow (should not error)
		err = tmx.SendControl(id, "Down")
		if err != nil {
			t.Errorf("SendControl Down failed: %v", err)
		}

		// Send Escape (should not error)
		err = tmx.SendControl(id, "Escape")
		if err != nil {
			t.Errorf("SendControl Escape failed: %v", err)
		}
	})
}

// TestCapture verifies the Capture method returns pane content.
func TestCapture(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	// Start a session that outputs known text
	id, err := tmx.Start(name, "", "echo 'LINE1'; echo 'LINE2'; echo 'LINE3'; sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for output
	time.Sleep(300 * time.Millisecond)

	t.Run("captures specified number of lines", func(t *testing.T) {
		output, err := tmx.Capture(id, 10)
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}

		if !strings.Contains(output, "LINE1") || !strings.Contains(output, "LINE3") {
			t.Errorf("expected LINE1-3 in output, got %q", output)
		}
	})

	t.Run("handles small line count", func(t *testing.T) {
		output, err := tmx.Capture(id, 2)
		if err != nil {
			t.Fatalf("Capture failed: %v", err)
		}

		// Should get some output even with small line count
		if output == "" {
			t.Error("expected non-empty output with lines=2")
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := tmx.Capture(session.SessionID(testPrefix+"nonexistent"), 10)
		if err == nil {
			t.Error("expected error for non-existent session")
		}
	})
}

// TestIsRunning verifies the IsRunning method checks process names.
func TestIsRunning(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	// Start a session running sleep
	id, err := tmx.Start(name, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	t.Run("returns true for matching process", func(t *testing.T) {
		if !tmx.IsRunning(id, "sleep") {
			t.Error("IsRunning should return true for 'sleep'")
		}
	})

	t.Run("returns false for non-matching process", func(t *testing.T) {
		if tmx.IsRunning(id, "node", "python") {
			t.Error("IsRunning should return false for 'node', 'python'")
		}
	})

	t.Run("accepts multiple process names", func(t *testing.T) {
		if !tmx.IsRunning(id, "python", "sleep", "node") {
			t.Error("IsRunning should return true when any process matches")
		}
	})

	t.Run("returns false for empty process list", func(t *testing.T) {
		if tmx.IsRunning(id) {
			t.Error("IsRunning should return false for empty process list")
		}
	})
}

// TestWaitFor verifies the WaitFor method waits for process to start.
func TestWaitFor(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	t.Run("returns immediately when process already running", func(t *testing.T) {
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		start := time.Now()
		err = tmx.WaitFor(id, 5*time.Second, "sleep")
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("WaitFor failed: %v", err)
		}

		// Should return quickly since process is already running
		if elapsed > 2*time.Second {
			t.Errorf("WaitFor took too long: %v", elapsed)
		}
	})

	t.Run("times out when process not found", func(t *testing.T) {
		name2 := name + "-timeout"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name2))
		})

		id, err := tmx.Start(name2, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		start := time.Now()
		err = tmx.WaitFor(id, 500*time.Millisecond, "nonexistent")
		elapsed := time.Since(start)

		if err == nil {
			t.Error("expected timeout error")
		}

		// Should timeout around the specified duration
		if elapsed < 400*time.Millisecond {
			t.Errorf("WaitFor returned too quickly: %v", elapsed)
		}
	})

	t.Run("returns nil for empty process list", func(t *testing.T) {
		name3 := name + "-empty"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name3))
		})

		id, err := tmx.Start(name3, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		err = tmx.WaitFor(id, time.Second)
		if err != nil {
			t.Errorf("WaitFor with empty list should return nil: %v", err)
		}
	})
}

// TestList verifies the List method returns all session IDs.
func TestList(t *testing.T) {
	tmx := NewTmux()
	name1 := uniqueSessionName(t) + "-1"
	name2 := uniqueSessionName(t) + "-2"

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name1))
		_ = tmx.Stop(session.SessionID(name2))
	})

	// Create two sessions
	_, err := tmx.Start(name1, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start 1 failed: %v", err)
	}
	_, err = tmx.Start(name2, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start 2 failed: %v", err)
	}

	t.Run("returns all sessions", func(t *testing.T) {
		sessions, err := tmx.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		// Find our test sessions
		found1, found2 := false, false
		for _, id := range sessions {
			if string(id) == name1 {
				found1 = true
			}
			if string(id) == name2 {
				found2 = true
			}
		}

		if !found1 {
			t.Errorf("List did not include session %s", name1)
		}
		if !found2 {
			t.Errorf("List did not include session %s", name2)
		}
	})

	t.Run("returns SessionID type", func(t *testing.T) {
		sessions, err := tmx.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		for _, id := range sessions {
			// Verify it's a SessionID (this is compile-time, but test the conversion)
			_ = session.SessionID(string(id))
		}
	})
}

// TestSetEnv verifies the SetEnv method sets environment variables.
func TestSetEnv(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	id, err := tmx.Start(name, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Run("sets environment variable", func(t *testing.T) {
		err := tmx.SetEnv(id, "TEST_VAR", "test_value_123")
		if err != nil {
			t.Fatalf("SetEnv failed: %v", err)
		}

		// Verify using GetEnvironment
		value, err := tmx.GetEnvironment(string(id), "TEST_VAR")
		if err != nil {
			t.Fatalf("GetEnvironment failed: %v", err)
		}

		if value != "test_value_123" {
			t.Errorf("expected 'test_value_123', got %q", value)
		}
	})

	t.Run("updates existing variable", func(t *testing.T) {
		err := tmx.SetEnv(id, "TEST_VAR", "new_value")
		if err != nil {
			t.Fatalf("SetEnv update failed: %v", err)
		}

		value, _ := tmx.GetEnvironment(string(id), "TEST_VAR")
		if value != "new_value" {
			t.Errorf("expected 'new_value', got %q", value)
		}
	})

	t.Run("handles special characters in value", func(t *testing.T) {
		err := tmx.SetEnv(id, "SPECIAL_VAR", "path=/foo/bar:baz")
		if err != nil {
			t.Fatalf("SetEnv with special chars failed: %v", err)
		}

		value, _ := tmx.GetEnvironment(string(id), "SPECIAL_VAR")
		if value != "path=/foo/bar:baz" {
			t.Errorf("expected 'path=/foo/bar:baz', got %q", value)
		}
	})
}

// TestGetInfo verifies the GetInfo method returns session information.
func TestGetInfo(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	id, err := tmx.Start(name, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Run("returns session info", func(t *testing.T) {
		info, err := tmx.GetInfo(id)
		if err != nil {
			t.Fatalf("GetInfo failed: %v", err)
		}

		if info.Name != name {
			t.Errorf("expected name %q, got %q", name, info.Name)
		}

		if info.Windows < 1 {
			t.Errorf("expected at least 1 window, got %d", info.Windows)
		}

		// Note: Created timestamp may be empty in some tmux versions (3.4+)
		// where #{session_created_string} returns empty. This is acceptable
		// as long as the session is queryable.
		t.Logf("Created timestamp: %q (may be empty in some tmux versions)", info.Created)
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := tmx.GetInfo(session.SessionID(testPrefix + "nonexistent"))
		if err == nil {
			t.Error("expected error for non-existent session")
		}
	})
}

// TestConfigureGasTownSession verifies the ConfigureGasTownSession method.
func TestConfigureGasTownSession(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	id, err := tmx.Start(name, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Run("applies theme and status bar", func(t *testing.T) {
		theme := session.Theme{
			Name: "test-theme",
			BG:   "#1e3a5f",
			FG:   "#e0e0e0",
		}

		err := tmx.ConfigureGasTownSession(id, theme, "testrig", "testworker", "crew")
		if err != nil {
			t.Fatalf("ConfigureGasTownSession failed: %v", err)
		}

		// Session should still be functional
		exists, _ := tmx.Exists(id)
		if !exists {
			t.Error("session should exist after ConfigureGasTownSession")
		}
	})

	t.Run("handles town-level agents (no rig)", func(t *testing.T) {
		name2 := name + "-town"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name2))
		})

		id2, _ := tmx.Start(name2, "", "sleep 60")
		theme := session.Theme{Name: "mayor", BG: "#2d2d2d", FG: "#ffffff"}

		err := tmx.ConfigureGasTownSession(id2, theme, "", "Mayor", "mayor")
		if err != nil {
			t.Fatalf("ConfigureGasTownSession for town agent failed: %v", err)
		}
	})
}

// TestSetPaneDiedHook verifies the SetPaneDiedHook method.
func TestSetPaneDiedHook(t *testing.T) {
	tmx := NewTmux()
	name := uniqueSessionName(t)

	t.Cleanup(func() {
		_ = tmx.Stop(session.SessionID(name))
	})

	id, err := tmx.Start(name, "", "sleep 60")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Run("sets pane-died hook", func(t *testing.T) {
		err := tmx.SetPaneDiedHook(id, "testrig/testworker")
		if err != nil {
			t.Fatalf("SetPaneDiedHook failed: %v", err)
		}

		// Session should still be functional
		exists, _ := tmx.Exists(id)
		if !exists {
			t.Error("session should exist after SetPaneDiedHook")
		}
	})
}

// TestInterfaceCompliance verifies *Tmux implements session.Session.
func TestInterfaceCompliance(t *testing.T) {
	var _ session.Session = (*Tmux)(nil)
	var _ session.TmuxExtensions = (*Tmux)(nil)
}

// TestConcurrentOperations verifies thread-safety of concurrent operations.
func TestConcurrentOperations(t *testing.T) {
	tmx := NewTmux()
	sessions := make([]session.SessionID, 5)

	// Create multiple sessions
	for i := range sessions {
		name := uniqueSessionName(t) + fmt.Sprintf("-%d", i)
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start %d failed: %v", i, err)
		}
		sessions[i] = id
	}

	t.Cleanup(func() {
		for _, id := range sessions {
			_ = tmx.Stop(id)
		}
	})

	t.Run("concurrent Capture calls", func(t *testing.T) {
		done := make(chan error, len(sessions))

		for _, id := range sessions {
			go func(id session.SessionID) {
				_, err := tmx.Capture(id, 10)
				done <- err
			}(id)
		}

		for range sessions {
			if err := <-done; err != nil {
				t.Errorf("concurrent Capture failed: %v", err)
			}
		}
	})

	t.Run("concurrent Exists calls", func(t *testing.T) {
		done := make(chan error, len(sessions))

		for _, id := range sessions {
			go func(id session.SessionID) {
				_, err := tmx.Exists(id)
				done <- err
			}(id)
		}

		for range sessions {
			if err := <-done; err != nil {
				t.Errorf("concurrent Exists failed: %v", err)
			}
		}
	})

	t.Run("concurrent List calls", func(t *testing.T) {
		done := make(chan error, 10)

		for i := 0; i < 10; i++ {
			go func() {
				_, err := tmx.List()
				done <- err
			}()
		}

		for i := 0; i < 10; i++ {
			if err := <-done; err != nil {
				t.Errorf("concurrent List failed: %v", err)
			}
		}
	})
}

// TestEdgeCases tests edge cases and error conditions.
func TestEdgeCases(t *testing.T) {
	tmx := NewTmux()

	t.Run("operations on stopped session", func(t *testing.T) {
		name := uniqueSessionName(t)
		id, _ := tmx.Start(name, "", "sleep 60")
		_ = tmx.Stop(id)

		// All operations should error or return appropriate values
		_, err := tmx.Capture(id, 10)
		if err == nil {
			t.Error("Capture on stopped session should error")
		}

		err = tmx.Send(id, "test")
		if err == nil {
			t.Error("Send on stopped session should error")
		}

		exists, _ := tmx.Exists(id)
		if exists {
			t.Error("Exists should return false for stopped session")
		}
	})

	t.Run("empty session name", func(t *testing.T) {
		_, err := tmx.Start("", "", "sleep 60")
		if err == nil {
			// Clean up in case it somehow succeeded
			_ = tmx.Stop(session.SessionID(""))
			t.Error("Start with empty name should fail")
		}
	})

	t.Run("session name with special characters", func(t *testing.T) {
		// tmux allows alphanumeric, underscore, and hyphen
		name := testPrefix + "test_session-123"
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name))
		})

		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start with special chars failed: %v", err)
		}

		exists, _ := tmx.Exists(id)
		if !exists {
			t.Error("session with underscores and hyphens should exist")
		}
	})

	t.Run("very long capture", func(t *testing.T) {
		name := uniqueSessionName(t)
		t.Cleanup(func() {
			_ = tmx.Stop(session.SessionID(name))
		})

		id, _ := tmx.Start(name, "", "for i in $(seq 1 100); do echo \"line $i\"; done && sleep 30")
		time.Sleep(500 * time.Millisecond)

		output, err := tmx.Capture(id, 1000)
		if err != nil {
			t.Fatalf("Large capture failed: %v", err)
		}

		// Should have captured many lines
		lines := strings.Split(output, "\n")
		if len(lines) < 50 {
			t.Errorf("expected at least 50 lines, got %d", len(lines))
		}
	})
}

// TestNoServerHandling tests behavior when tmux server is not running.
// This test is skipped if there are existing sessions (to avoid killing the server).
func TestNoServerHandling(t *testing.T) {
	tmx := NewTmux()

	// Check if there are existing sessions
	sessions, _ := tmx.List()
	hasNonTestSessions := false
	for _, id := range sessions {
		if !strings.HasPrefix(string(id), testPrefix) {
			hasNonTestSessions = true
			break
		}
	}

	if hasNonTestSessions {
		t.Skip("skipping no-server tests: non-test sessions exist")
	}

	// Clean up all test sessions first
	cleanupTestSessions()

	// Kill the server if it's running (only test sessions should exist at this point)
	_ = tmx.KillServer()

	t.Cleanup(func() {
		// Tests will create new sessions, cleaning up
		cleanupTestSessions()
	})

	t.Run("List returns empty when no server", func(t *testing.T) {
		sessions, err := tmx.List()
		if err != nil {
			t.Errorf("List should not error when no server: %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("expected empty list, got %d sessions", len(sessions))
		}
	})

	t.Run("Exists returns false when no server", func(t *testing.T) {
		exists, err := tmx.Exists(session.SessionID("nonexistent"))
		if err != nil {
			t.Errorf("Exists should not error when no server: %v", err)
		}
		if exists {
			t.Error("Exists should return false when no server")
		}
	})

	t.Run("Start creates server automatically", func(t *testing.T) {
		name := uniqueSessionName(t)
		id, err := tmx.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start should create server: %v", err)
		}

		exists, _ := tmx.Exists(id)
		if !exists {
			t.Error("session should exist after Start creates server")
		}

		_ = tmx.Stop(id)
	})
}

// TestTmux_Conformance runs the conformance test suite against real tmux.
// This verifies tmux matches the Session contract that the Double also implements.
func TestTmux_Conformance(t *testing.T) {
	factory := func() session.Session {
		return NewTmux()
	}

	cfg := session.ConformanceConfig{
		StartupDelay: 200 * time.Millisecond,
	}

	session.RunConformanceTestsWithConfig(t, factory, cleanupTestSessions, cfg)
}
