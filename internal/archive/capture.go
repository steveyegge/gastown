package archive

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// Capture errors.
var (
	ErrSessionNotFound = errors.New("tmux session not found")
	ErrNoServer        = errors.New("no tmux server running")
)

// CommandRunner executes shell commands and returns their output.
// This interface allows for mocking in tests.
type CommandRunner interface {
	Run(name string, args ...string) (string, error)
}

// ExecRunner is the default CommandRunner using os/exec.
type ExecRunner struct{}

// Run executes a command and returns its stdout.
func (e *ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", wrapTmuxError(err, stderr.String())
	}

	return stdout.String(), nil
}

// wrapTmuxError converts tmux stderr messages into typed errors.
func wrapTmuxError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)

	// Detect specific error types from tmux stderr
	if strings.Contains(stderr, "session not found") ||
		strings.Contains(stderr, "can't find session") ||
		strings.Contains(stderr, "can't find pane") {
		return ErrSessionNotFound
	}
	if strings.Contains(stderr, "no server running") ||
		strings.Contains(stderr, "error connecting to") {
		return ErrNoServer
	}

	if stderr != "" {
		return fmt.Errorf("tmux: %s", stderr)
	}
	return fmt.Errorf("tmux: %w", err)
}

// classifyError attempts to classify an error based on its message.
// This is used to normalize errors from different sources (real tmux, mocks).
// If the error already matches a known type, it's returned as-is.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Already classified?
	if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrNoServer) {
		return err
	}

	// Try to classify based on error message
	msg := err.Error()
	if strings.Contains(msg, "session not found") ||
		strings.Contains(msg, "can't find session") ||
		strings.Contains(msg, "can't find pane") {
		return ErrSessionNotFound
	}
	if strings.Contains(msg, "no server running") ||
		strings.Contains(msg, "error connecting to") {
		return ErrNoServer
	}

	return err
}

// Capturer handles tmux pane capture operations.
type Capturer struct {
	runner CommandRunner
}

// NewCapturer creates a new Capturer with the default command runner.
func NewCapturer() *Capturer {
	return &Capturer{
		runner: &ExecRunner{},
	}
}

// NewCapturerWithRunner creates a Capturer with a custom command runner.
// This is useful for testing with mock tmux output.
func NewCapturerWithRunner(runner CommandRunner) *Capturer {
	return &Capturer{
		runner: runner,
	}
}

// CaptureTmuxPane captures the contents of a tmux pane.
//
// Parameters:
//   - sessionName: The tmux session name to capture from
//   - width: Maximum line width (lines longer than this are truncated)
//   - height: Number of lines to capture from scrollback
//
// Returns the captured lines as a slice, or an error if capture fails.
//
// Uses tmux capture-pane with the following options:
//   - -p: Output to stdout (not buffer)
//   - -S -<height>: Start N lines from bottom (captures scrollback)
//   - -e: Preserve ANSI escape sequences for color
//   - -J: Join wrapped lines (handles line wrapping at terminal boundary)
func (c *Capturer) CaptureTmuxPane(sessionName string, width, height int) ([]string, error) {
	if sessionName == "" {
		return nil, fmt.Errorf("session name cannot be empty")
	}
	if height <= 0 {
		height = DefaultHeight
	}
	if width <= 0 {
		width = DefaultWidth
	}

	// Build tmux capture-pane command
	// -p: print to stdout
	// -t: target session
	// -S: start line (negative for scrollback)
	// -e: preserve escape sequences
	// -J: join wrapped lines
	args := []string{
		"capture-pane",
		"-p",
		"-t", sessionName,
		"-S", fmt.Sprintf("-%d", height),
		"-e",
		"-J",
	}

	output, err := c.runner.Run("tmux", args...)
	if err != nil {
		// Normalize error to our typed errors if possible
		// This handles both real tmux errors and test mocks
		return nil, classifyError(err)
	}

	// Split into lines and process
	lines := strings.Split(output, "\n")

	// Remove trailing empty line if present (common with capture-pane output)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Truncate lines that exceed width
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = truncateLine(line, width)
	}

	return result, nil
}

// truncateLine truncates a line to the specified width.
// It handles ANSI escape sequences correctly by not counting them
// toward the visible width.
func truncateLine(line string, width int) string {
	if width <= 0 || line == "" {
		return line
	}

	// Fast path: if line is shorter than width in bytes, it's definitely shorter in runes
	if len(line) <= width {
		return line
	}

	// Count visible characters (excluding ANSI escape sequences)
	visibleCount := 0
	inEscape := false
	var result strings.Builder
	result.Grow(len(line))

	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			// Start of ANSI escape sequence
			inEscape = true
			result.WriteByte(line[i])
			i++
			continue
		}

		if inEscape {
			result.WriteByte(line[i])
			// End of escape sequence (letter character)
			if (line[i] >= 'A' && line[i] <= 'Z') || (line[i] >= 'a' && line[i] <= 'z') {
				inEscape = false
			}
			i++
			continue
		}

		// Regular character
		r, size := utf8.DecodeRuneInString(line[i:])
		if visibleCount >= width {
			// We've reached the width limit
			break
		}

		result.WriteRune(r)
		visibleCount++
		i += size
	}

	return result.String()
}

// CaptureTmuxPane is a convenience function that uses the default capturer.
// For production use, prefer creating a Capturer instance for better control.
func CaptureTmuxPane(sessionName string, width, height int) ([]string, error) {
	return NewCapturer().CaptureTmuxPane(sessionName, width, height)
}
