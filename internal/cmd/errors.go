package cmd

import (
	"errors"
	"fmt"
)

// SilentExitError signals that the command should exit with a specific code
// without printing an error message. This is used for scripting purposes
// where exit codes convey status (e.g., "no mail" = exit 1).
type SilentExitError struct {
	Code int
}

func (e *SilentExitError) Error() string {
	return fmt.Sprintf("exit %d", e.Code)
}

// NewSilentExit creates a SilentExitError with the given exit code.
func NewSilentExit(code int) *SilentExitError {
	return &SilentExitError{Code: code}
}

// IsSilentExit checks if an error is a SilentExitError and returns its code.
// Uses errors.As to properly handle wrapped errors.
// Returns 0 and false if err is nil or not a SilentExitError.
func IsSilentExit(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var se *SilentExitError
	if errors.As(err, &se) {
		return se.Code, true
	}
	return 0, false
}
