package mail

import (
	"bytes"
	"os/exec"
	"strings"
)

// bdError represents an error from running a bd command.
// It wraps the underlying error and includes the stderr output for inspection.
type bdError struct {
	Err    error
	Stderr string
}

// Error implements the error interface.
func (e *bdError) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "unknown bd error"
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *bdError) Unwrap() error {
	return e.Err
}

// ContainsError checks if the stderr message contains the given substring.
func (e *bdError) ContainsError(substr string) bool {
	return strings.Contains(e.Stderr, substr)
}

// runBdCommand executes a bd command with proper environment setup.
// workDir is kept for API compatibility but ignored - we use current directory
// to avoid bd daemon health check timeout when running from town root (hq-33lwcx).
// beadsDir is the BEADS_DIR environment variable value.
// extraEnv contains additional environment variables to set (e.g., "BD_IDENTITY=...").
// Returns stdout bytes on success, or a *bdError on failure.
func runBdCommand(args []string, workDir, beadsDir string, extraEnv ...string) ([]byte, error) {
	_ = workDir // Intentionally unused - see comment above

	// Use the daemon for connection pooling. Previous --no-daemon was causing
	// massive connection churn (~17 connections/second with 32 agents).
	// See: hq-i97ri for the fix, hq-vvbubs/hq-33lwcx for original daemon issues.
	cmd := exec.Command("bd", args...) //nolint:gosec // G204: bd is a trusted internal tool
	// Don't set cmd.Dir - use current directory to avoid daemon timeout issue

	env := append(cmd.Environ(), "BEADS_DIR="+beadsDir)
	env = append(env, extraEnv...)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &bdError{
			Err:    err,
			Stderr: strings.TrimSpace(stderr.String()),
		}
	}

	return stdout.Bytes(), nil
}
