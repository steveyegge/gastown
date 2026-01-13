// Package exitcode defines structured exit codes for gt commands.
// These codes allow AI agents and scripts to handle specific error
// conditions programmatically without parsing error messages.
//
// # Exit Code Ranges
//
// Codes are grouped by category for easy identification:
//   - 0: Success
//   - 1-9: General errors (usage, internal)
//   - 10-19: Resource not found (bead, agent, rig, file)
//   - 20-29: Permission/access errors
//   - 30-39: Network/connectivity errors
//   - 40-49: Timeout errors
//   - 50-59: Conflict/state errors
//
// # Usage
//
// Create errors with specific codes:
//
//	return exitcode.BeadNotFound("gt-abc")        // Exit code 10
//	return exitcode.Newf(exitcode.ErrUsage, "invalid flag: %s", flag)
//
// Extract codes from errors (works with wrapped errors):
//
//	if exitcode.Is(err, exitcode.ErrBeadNotFound) {
//	    // Handle bead not found
//	}
//	code := exitcode.Code(err)  // Returns ErrGeneral for non-coded errors
package exitcode

import (
	"errors"
	"fmt"
)

// Exit codes for gt commands.
// Codes are grouped by category for easier identification:
//   - 0: Success
//   - 1-9: General errors
//   - 10-19: Resource not found
//   - 20-29: Permission/access errors
//   - 30-39: Network/connectivity
//   - 40-49: Timeout errors
//   - 50-59: Conflict/state errors
const (
	// Success indicates the command completed successfully.
	Success = 0

	// General errors (1-9)
	ErrGeneral  = 1 // General/unknown error
	ErrUsage    = 2 // Invalid arguments or usage
	ErrInternal = 3 // Internal error (bug)

	// Resource not found (10-19)
	ErrBeadNotFound  = 10 // Bead/issue not found
	ErrAgentNotFound = 11 // Agent not found
	ErrRigNotFound   = 12 // Rig not found
	ErrFileNotFound  = 13 // File or path not found

	// Permission/access errors (20-29)
	ErrPermission  = 20 // Permission denied
	ErrHookOccupied = 21 // Hook already occupied

	// Network/connectivity (30-39)
	ErrNetwork = 30 // Network/connectivity error

	// Timeout errors (40-49)
	ErrTimeout = 40 // Operation timed out

	// Conflict/state errors (50-59)
	ErrConflict      = 50 // Resource conflict (e.g., already exists)
	ErrAlreadyExists = 51 // Resource already exists
	ErrBusy          = 52 // Resource is busy
)

// Error wraps an error with a specific exit code.
type Error struct {
	Code    int
	Message string
	Cause   error
}

// Error returns the error message.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.Cause
}

// New creates a new coded error.
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap wraps an existing error with a code and message.
func Wrap(code int, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// Wrapf wraps an existing error with a code and printf-style message.
func Wrapf(code int, cause error, format string, args ...interface{}) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// Code extracts the exit code from an error.
// Returns ErrGeneral (1) if the error doesn't have a code.
func Code(err error) int {
	if err == nil {
		return Success
	}
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ErrGeneral
}

// Is checks if an error has a specific exit code.
func Is(err error, code int) bool {
	return Code(err) == code
}

// Newf creates a new coded error with printf-style formatting.
func Newf(code int, format string, args ...interface{}) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Convenience constructors for common error types.
// These make error creation more readable and ensure correct codes.

// BeadNotFound returns an error for a missing bead.
func BeadNotFound(id string) *Error {
	return Newf(ErrBeadNotFound, "bead not found: %s", id)
}

// AgentNotFound returns an error for a missing agent.
func AgentNotFound(name string) *Error {
	return Newf(ErrAgentNotFound, "agent not found: %s", name)
}

// RigNotFound returns an error for a missing rig.
func RigNotFound(name string) *Error {
	return Newf(ErrRigNotFound, "rig not found: %s", name)
}

// FileNotFound returns an error for a missing file.
func FileNotFound(path string) *Error {
	return Newf(ErrFileNotFound, "file not found: %s", path)
}

// PermissionDenied returns a permission error.
func PermissionDenied(msg string) *Error {
	return New(ErrPermission, msg)
}

// HookOccupied returns an error when a hook is already taken.
func HookOccupied(hook string) *Error {
	return Newf(ErrHookOccupied, "hook already occupied: %s", hook)
}

// Timeout returns a timeout error.
func Timeout(operation string) *Error {
	return Newf(ErrTimeout, "operation timed out: %s", operation)
}

// AlreadyExists returns an error when a resource already exists.
func AlreadyExists(resource string) *Error {
	return Newf(ErrAlreadyExists, "%s already exists", resource)
}
