package doctor

import "errors"

// Common errors
var (
	// ErrCannotFix is returned when a check does not support auto-fix.
	ErrCannotFix = errors.New("check does not support auto-fix")

	// ErrSkippedNoStart is returned when a fix is skipped due to --no-start.
	ErrSkippedNoStart = errors.New("skipped: --no-start suppresses daemon/agent startup")
)
