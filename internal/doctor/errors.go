package doctor

import "errors"

// Common errors
var (
	// ErrCannotFix is returned when a check does not support auto-fix.
	ErrCannotFix = errors.New("check does not support auto-fix")
)
