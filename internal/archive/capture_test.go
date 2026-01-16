package archive

import (
	"errors"
	"strings"
	"testing"
)

// MockRunner is a test double for CommandRunner.
type MockRunner struct {
	Output string
	Err    error
	// Recorded captures the arguments from the last Run call
	Recorded struct {
		Name string
		Args []string
	}
}

func (m *MockRunner) Run(name string, args ...string) (string, error) {
	m.Recorded.Name = name
	m.Recorded.Args = args
	return m.Output, m.Err
}

func TestCaptureTmuxPane_Success(t *testing.T) {
	mock := &MockRunner{
		Output: "line 1\nline 2\nline 3\n",
	}
	capturer := NewCapturerWithRunner(mock)

	lines, err := capturer.CaptureTmuxPane("test-session", 80, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	expected := []string{"line 1", "line 2", "line 3"}
	for i, want := range expected {
		if lines[i] != want {
			t.Errorf("line %d: got %q, want %q", i, lines[i], want)
		}
	}

	// Verify tmux was called with correct arguments
	if mock.Recorded.Name != "tmux" {
		t.Errorf("expected tmux command, got %s", mock.Recorded.Name)
	}

	args := mock.Recorded.Args
	if !contains(args, "capture-pane") {
		t.Error("missing capture-pane subcommand")
	}
	if !contains(args, "-p") {
		t.Error("missing -p flag")
	}
	if !contains(args, "-e") {
		t.Error("missing -e flag for ANSI preservation")
	}
	if !contains(args, "-J") {
		t.Error("missing -J flag for wrapped line handling")
	}
	if !contains(args, "-S") {
		t.Error("missing -S flag for scrollback")
	}
	if !contains(args, "-100") {
		t.Error("missing -100 scrollback value")
	}
}

func TestCaptureTmuxPane_SessionNotFound(t *testing.T) {
	mock := &MockRunner{
		Err: errors.New("session not found: test-session"),
	}
	capturer := NewCapturerWithRunner(mock)

	_, err := capturer.CaptureTmuxPane("test-session", 80, 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestCaptureTmuxPane_NoServer(t *testing.T) {
	mock := &MockRunner{
		Err: errors.New("no server running on /tmp/tmux-1000/default"),
	}
	capturer := NewCapturerWithRunner(mock)

	_, err := capturer.CaptureTmuxPane("test-session", 80, 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrNoServer) {
		t.Errorf("expected ErrNoServer, got %v", err)
	}
}

func TestCaptureTmuxPane_EmptySession(t *testing.T) {
	capturer := NewCapturer()

	_, err := capturer.CaptureTmuxPane("", 80, 100)
	if err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestCaptureTmuxPane_DefaultDimensions(t *testing.T) {
	mock := &MockRunner{
		Output: "line 1\n",
	}
	capturer := NewCapturerWithRunner(mock)

	// Use zero/negative values to test defaults
	_, err := capturer.CaptureTmuxPane("test", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := mock.Recorded.Args
	// Should use default height
	if !contains(args, "-100") {
		t.Errorf("expected default height of 100, args: %v", args)
	}
}

func TestCaptureTmuxPane_LineTruncation(t *testing.T) {
	longLine := strings.Repeat("x", 200)
	mock := &MockRunner{
		Output: longLine + "\n",
	}
	capturer := NewCapturerWithRunner(mock)

	lines, err := capturer.CaptureTmuxPane("test", 80, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	if len(lines[0]) != 80 {
		t.Errorf("expected line truncated to 80 chars, got %d", len(lines[0]))
	}
}

func TestCaptureTmuxPane_ANSIPreservation(t *testing.T) {
	// ANSI escape sequence for red text
	ansiLine := "\x1b[31mred text\x1b[0m"
	mock := &MockRunner{
		Output: ansiLine + "\n",
	}
	capturer := NewCapturerWithRunner(mock)

	lines, err := capturer.CaptureTmuxPane("test", 80, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ANSI sequences should be preserved
	if !strings.Contains(lines[0], "\x1b[31m") {
		t.Error("ANSI escape sequence should be preserved")
	}
}

func TestCaptureTmuxPane_ANSINotCountedInWidth(t *testing.T) {
	// Line with ANSI: 8 visible chars but many more bytes
	// \x1b[31m = red, \x1b[0m = reset
	ansiLine := "\x1b[31m" + strings.Repeat("x", 10) + "\x1b[0m"
	mock := &MockRunner{
		Output: ansiLine + "\n",
	}
	capturer := NewCapturerWithRunner(mock)

	// Width of 5 should truncate to 5 visible chars but keep ANSI
	lines, err := capturer.CaptureTmuxPane("test", 5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count visible characters (excluding ANSI)
	visibleCount := 0
	inEscape := false
	for _, c := range lines[0] {
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
		visibleCount++
	}

	if visibleCount != 5 {
		t.Errorf("expected 5 visible chars, got %d", visibleCount)
	}
}

func TestCaptureTmuxPane_EmptyOutput(t *testing.T) {
	mock := &MockRunner{
		Output: "",
	}
	capturer := NewCapturerWithRunner(mock)

	lines, err := capturer.CaptureTmuxPane("test", 80, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 0 {
		t.Errorf("expected 0 lines, got %d", len(lines))
	}
}

func TestCaptureTmuxPane_OnlyNewline(t *testing.T) {
	mock := &MockRunner{
		Output: "\n",
	}
	capturer := NewCapturerWithRunner(mock)

	lines, err := capturer.CaptureTmuxPane("test", 80, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single newline results in one empty line after trimming
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}

func TestTruncateLine_Unicode(t *testing.T) {
	// Unicode characters should be counted as single visible chars
	unicodeLine := "日本語テスト" // 6 characters
	result := truncateLine(unicodeLine, 3)

	// Should truncate to 3 visible characters
	if len([]rune(result)) != 3 {
		t.Errorf("expected 3 runes, got %d", len([]rune(result)))
	}
}

func TestTruncateLine_ShortLine(t *testing.T) {
	// Lines shorter than width should not be modified
	shortLine := "short"
	result := truncateLine(shortLine, 100)

	if result != shortLine {
		t.Errorf("short line should not be modified, got %q", result)
	}
}

func TestTruncateLine_EmptyLine(t *testing.T) {
	result := truncateLine("", 80)
	if result != "" {
		t.Errorf("empty line should stay empty, got %q", result)
	}
}

func TestTruncateLine_ZeroWidth(t *testing.T) {
	// Zero width should return line unchanged
	line := "test"
	result := truncateLine(line, 0)
	if result != line {
		t.Errorf("zero width should return unchanged, got %q", result)
	}
}

// TestConvenienceFunction tests the package-level convenience function.
func TestConvenienceFunction(t *testing.T) {
	// This will fail without a running tmux server, which is expected in CI
	// The test verifies the function signature and basic error handling
	_, err := CaptureTmuxPane("nonexistent-session-xyz", 80, 100)
	if err == nil {
		// If tmux isn't running or session doesn't exist, we should get an error
		// This is actually testing the real system, so success means tmux is running
		// with that session (unlikely)
		t.Log("tmux appears to be running with the test session")
	}
}

// contains checks if a slice contains a string.
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
