package mail

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

const (
	// bdReadTimeout is the timeout for bd read operations (list, show, query).
	bdReadTimeout = 30 * time.Second
	// bdWriteTimeout is the timeout for bd write operations (create, close, label, reopen).
	bdWriteTimeout = 60 * time.Second
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

// runBdCommand executes a bd command with a context timeout and proper environment setup.
// ctx controls the deadline/timeout for the subprocess.
// workDir is the directory to run the command in.
// beadsDir is the BEADS_DIR environment variable value.
// extraEnv contains additional environment variables to set (e.g., "BD_IDENTITY=...").
// Returns stdout bytes on success, or a *bdError on failure.
func runBdCommand(ctx context.Context, args []string, workDir, beadsDir string, extraEnv ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "bd", args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = workDir

	env := append(cmd.Environ(), "BEADS_DIR="+beadsDir)
	env = append(env, extraEnv...)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if runErr != nil {
		return nil, &bdError{
			Err:    runErr,
			Stderr: strings.TrimSpace(stderr.String()),
		}
	}

	return stdout.Bytes(), nil
}

// bdReadCtx returns a context with the standard bd read timeout.
func bdReadCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), bdReadTimeout)
}

// bdWriteCtx returns a context with the standard bd write timeout.
func bdWriteCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), bdWriteTimeout)
}
