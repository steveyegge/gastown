package rpcserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"connectrpc.com/connect"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/mail"
)

// Error helpers for consistent RPC error handling across all services.
//
// Error code guidelines:
//   - CodeInvalidArgument: Missing/invalid request parameters
//   - CodeNotFound: Requested resource doesn't exist
//   - CodeUnauthenticated: Missing or invalid credentials
//   - CodePermissionDenied: Valid credentials but insufficient permissions
//   - CodeUnavailable: Transient failure, client should retry (includes Retry-After)
//   - CodeInternal: Unexpected server error (bug or unrecoverable failure)

// withRetryAfter adds Retry-After metadata (in seconds) to a Connect error.
func withRetryAfter(err *connect.Error, seconds int) *connect.Error {
	if seconds > 0 {
		err.Meta().Set("Retry-After", fmt.Sprintf("%d", seconds))
	}
	return err
}

// unavailableErr creates a CodeUnavailable error with retry guidance.
// Use for transient failures (file I/O, external services) where the client
// should retry after a delay.
func unavailableErr(msg string, err error, retryAfterSecs int) *connect.Error {
	connErr := connect.NewError(connect.CodeUnavailable, fmt.Errorf("%s: %w", msg, err))
	return withRetryAfter(connErr, retryAfterSecs)
}

// cmdExecErr handles errors from external command execution (gt CLI, tmux, etc).
// It logs full details for server-side debugging but returns a sanitized error
// to the client, preventing internal file paths and stack traces from leaking.
func cmdExecErr(operation string, err error, output []byte) *connect.Error {
	// Log full output for server-side debugging
	if len(output) > 0 {
		log.Printf("RPC: %s failed: %v | output: %s", operation, err, truncateLog(string(output)))
	} else {
		log.Printf("RPC: %s failed: %v", operation, err)
	}

	// Check for context cancellation
	if err == context.Canceled || err == context.DeadlineExceeded {
		return connect.NewError(connect.CodeCanceled, fmt.Errorf("%s was cancelled", operation))
	}

	// Classify based on exit code and output patterns
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		outLower := strings.ToLower(string(output))

		if containsAny(outLower, "not found", "does not exist", "no such") {
			return connect.NewError(connect.CodeNotFound,
				fmt.Errorf("%s: resource not found", operation))
		}
		if containsAny(outLower, "permission denied", "unauthorized", "forbidden") {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("%s: permission denied", operation))
		}
		if containsAny(outLower, "already exists", "duplicate", "conflict") {
			return connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("%s: resource already exists", operation))
		}

		// Default: treat external process failures as unavailable (transient)
		connErr := connect.NewError(connect.CodeUnavailable,
			fmt.Errorf("%s failed (exit code %d)", operation, code))
		return withRetryAfter(connErr, 5)
	}

	// Non-exit errors (command not found, signal killed, etc.)
	connErr := connect.NewError(connect.CodeUnavailable,
		fmt.Errorf("%s: service temporarily unavailable", operation))
	return withRetryAfter(connErr, 5)
}

// invalidArg creates a CodeInvalidArgument error for missing/invalid request parameters.
func invalidArg(field, msg string) *connect.Error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", field, msg))
}

// internalErr creates a CodeInternal error for unexpected server failures.
func internalErr(msg string, err error) *connect.Error {
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", msg, err))
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// classifyErr classifies errors from internal packages (beads, mail, sling, etc.)
// into appropriate Connect error codes. It checks for known sentinel errors first,
// then falls back to error message pattern matching.
//
// Use this for errors returned by internal Go packages (not external commands).
// For external command errors, use cmdExecErr instead.
func classifyErr(operation string, err error) *connect.Error {
	if err == nil {
		return nil
	}

	// Check for context cancellation
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeCanceled, fmt.Errorf("%s was cancelled", operation))
	}

	// Check for known sentinel errors: not found
	if errors.Is(err, beads.ErrNotFound) || errors.Is(err, mail.ErrMessageNotFound) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s: resource not found", operation))
	}

	// Check for known sentinel errors: not installed / unavailable
	if errors.Is(err, beads.ErrNotInstalled) {
		return unavailableErr(operation+": beads backend not available", err, 10)
	}

	// Pattern-match on error message for common categories
	errMsg := strings.ToLower(err.Error())

	if containsAny(errMsg, "not found", "does not exist", "no such") {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s: resource not found", operation))
	}
	if containsAny(errMsg, "already exists", "duplicate", "conflict") {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("%s: resource already exists", operation))
	}
	if containsAny(errMsg, "permission denied", "unauthorized", "forbidden") {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s: permission denied", operation))
	}
	if containsAny(errMsg, "invalid", "bad request", "malformed") {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", operation, err.Error()))
	}

	// Default: unexpected internal error
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", operation, err))
}

// notFoundOrInternal returns CodeNotFound if the error matches a not-found pattern,
// otherwise CodeInternal. Use for operations where the primary expected error is
// a missing resource (e.g., Show, Get).
func notFoundOrInternal(operation string, err error) *connect.Error {
	if errors.Is(err, beads.ErrNotFound) || errors.Is(err, mail.ErrMessageNotFound) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s: resource not found", operation))
	}
	errMsg := strings.ToLower(err.Error())
	if containsAny(errMsg, "not found", "does not exist", "no such") {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s: resource not found", operation))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", operation, err))
}

// truncateLog truncates a string for logging to prevent log flooding.
func truncateLog(s string) string {
	if len(s) > 1000 {
		return s[:1000] + "...(truncated)"
	}
	return s
}
