//go:build integration

package archive

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCaptureTmuxPane_Integration tests capture with a real tmux session.
// Run with: go test -tags=integration ./internal/archive/...
func TestCaptureTmuxPane_Integration(t *testing.T) {
	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	sessionName := "archive-test-" + time.Now().Format("150405")

	// Create a test session
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Ensure cleanup
	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	// Send some test content to the session
	testLines := []string{
		"echo 'Line 1: Hello from integration test'",
		"echo 'Line 2: Testing capture functionality'",
		"echo 'Line 3: With special chars: @#$%'",
	}

	for _, line := range testLines {
		cmd := exec.Command("tmux", "send-keys", "-t", sessionName, line, "Enter")
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to send keys: %v", err)
		}
		time.Sleep(100 * time.Millisecond) // Let commands execute
	}

	// Wait for output to appear
	time.Sleep(500 * time.Millisecond)

	// Capture the pane
	capturer := NewCapturer()
	lines, err := capturer.CaptureTmuxPane(sessionName, 120, 50)
	if err != nil {
		t.Fatalf("capture failed: %v", err)
	}

	// Verify we got some output
	if len(lines) == 0 {
		t.Fatal("expected non-empty capture")
	}

	// Join lines and check for expected content
	output := strings.Join(lines, "\n")

	expectedPhrases := []string{
		"Hello from integration test",
		"Testing capture functionality",
		"special chars",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(output, phrase) {
			t.Errorf("expected output to contain %q", phrase)
		}
	}

	t.Logf("Captured %d lines from session %s", len(lines), sessionName)
}

// TestCaptureTmuxPane_IntegrationSessionNotFound tests error handling for missing session.
func TestCaptureTmuxPane_IntegrationSessionNotFound(t *testing.T) {
	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	capturer := NewCapturer()
	_, err := capturer.CaptureTmuxPane("nonexistent-session-xyz-12345", 80, 100)

	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}

	// Error should be either ErrSessionNotFound or ErrNoServer (if no tmux server)
	if err != ErrSessionNotFound && err != ErrNoServer {
		t.Errorf("expected ErrSessionNotFound or ErrNoServer, got: %v", err)
	}
}

// TestCaptureTmuxPane_IntegrationLongLines tests truncation of long lines.
func TestCaptureTmuxPane_IntegrationLongLines(t *testing.T) {
	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	sessionName := "archive-long-" + time.Now().Format("150405")

	// Create a test session with a wide window
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-x", "200", "-y", "24")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	defer func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}()

	// Send a very long line (200 characters)
	longLine := strings.Repeat("x", 200)
	cmd = exec.Command("tmux", "send-keys", "-t", sessionName, "echo '"+longLine+"'", "Enter")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to send keys: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Capture with width limit of 80
	capturer := NewCapturer()
	lines, err := capturer.CaptureTmuxPane(sessionName, 80, 50)
	if err != nil {
		t.Fatalf("capture failed: %v", err)
	}

	// Check that captured lines are truncated to 80 visible chars
	for _, line := range lines {
		visibleLen := visibleLength(line)
		if visibleLen > 80 {
			t.Errorf("line exceeds width limit: %d visible chars", visibleLen)
		}
	}
}

// visibleLength counts visible characters (excluding ANSI escape sequences).
func visibleLength(s string) int {
	count := 0
	inEscape := false
	for _, c := range s {
		if c == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}
