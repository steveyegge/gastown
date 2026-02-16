package doctor

import (
	"fmt"
	"io"
	"time"

	"github.com/steveyegge/gastown/internal/ui"
)

// Doctor manages and executes health checks.
type Doctor struct {
	checks []Check
}

// NewDoctor creates a new Doctor with no registered checks.
func NewDoctor() *Doctor {
	return &Doctor{
		checks: make([]Check, 0),
	}
}

// Register adds a check to the doctor's check list.
func (d *Doctor) Register(check Check) {
	d.checks = append(d.checks, check)
}

// RegisterAll adds multiple checks to the doctor's check list.
func (d *Doctor) RegisterAll(checks ...Check) {
	d.checks = append(d.checks, checks...)
}

// Checks returns the list of registered checks.
func (d *Doctor) Checks() []Check {
	return d.checks
}


// Run executes all registered checks and returns a report.
func (d *Doctor) Run(ctx *CheckContext) *Report {
	return d.RunStreaming(ctx, nil, 0, true)
}

// RunStreaming executes all registered checks with optional real-time output.
// If w is non-nil, prints each check name as it starts and result when done.
// If slowThreshold > 0, shows hourglass icon for slow checks.
// If isTTY is false, output uses plain text prefixes (PASS/WARN/FAIL) instead
// of ANSI colors and icons, with no \r carriage return overwrites.
func (d *Doctor) RunStreaming(ctx *CheckContext, w io.Writer, slowThreshold time.Duration, isTTY bool) *Report {
	report := NewReport()

	for _, check := range d.checks {
		// Stream: print check name before running (TTY only — non-TTY skips intermediate output)
		if w != nil && isTTY {
			fmt.Fprintf(w, "  %s  %s...", ui.RenderMuted("○"), check.Name())
		}

		start := time.Now()
		result := check.Run(ctx)
		result.Elapsed = time.Since(start)

		// Ensure check name is populated
		if result.Name == "" {
			result.Name = check.Name()
		}
		// Set category from check if not already set by Run()
		if result.Category == "" {
			result.Category = check.Category()
		}

		// Stream: output result
		if w != nil {
			isSlow := slowThreshold > 0 && result.Elapsed >= slowThreshold
			if isSlow {
				report.Summary.Slow++
			}

			if isTTY {
				// TTY: overwrite the "checking..." line with colored result
				var statusIcon string
				switch result.Status {
				case StatusOK:
					statusIcon = ui.RenderPassIcon()
				case StatusWarning:
					statusIcon = ui.RenderWarnIcon()
				case StatusError:
					statusIcon = ui.RenderFailIcon()
				}
				slowIndicator := "  "
				if isSlow {
					slowIndicator = "⏳"
				}
				fmt.Fprintf(w, "\r  %s%s%s", statusIcon, slowIndicator, result.Name)
				if result.Message != "" {
					fmt.Fprintf(w, "%s", ui.RenderMuted(" "+result.Message))
				}
				if isSlow {
					fmt.Fprintf(w, "%s", ui.RenderMuted(" ("+formatDuration(result.Elapsed)+")"))
				}
			} else {
				// Non-TTY: clean line-by-line output with text prefixes
				var prefix string
				switch result.Status {
				case StatusOK:
					prefix = "PASS"
				case StatusWarning:
					prefix = "WARN"
				case StatusError:
					prefix = "FAIL"
				}
				fmt.Fprintf(w, "%s  %s", prefix, result.Name)
				if result.Message != "" {
					fmt.Fprintf(w, "  %s", result.Message)
				}
				if isSlow {
					fmt.Fprintf(w, "  (%s)", formatDuration(result.Elapsed))
				}
			}
			fmt.Fprintln(w)
		}

		report.Add(result)
	}

	return report
}

// Fix runs all checks with auto-fix enabled where possible.
// It first runs the check, then if it fails and can be fixed, attempts the fix.
func (d *Doctor) Fix(ctx *CheckContext) *Report {
	return d.FixStreaming(ctx, nil, 0, true)
}

// FixStreaming runs all checks with auto-fix and optional real-time output.
// If w is non-nil, prints each check name as it starts and result when done.
// If slowThreshold > 0, shows hourglass icon for slow checks.
// If isTTY is false, output uses plain text prefixes (PASS/WARN/FAIL/FIXED) instead
// of ANSI colors and icons, with no \r carriage return overwrites.
func (d *Doctor) FixStreaming(ctx *CheckContext, w io.Writer, slowThreshold time.Duration, isTTY bool) *Report {
	report := NewReport()

	for _, check := range d.checks {
		// Stream: print check name before running (TTY only)
		if w != nil && isTTY {
			fmt.Fprintf(w, "  %s  %s...", ui.RenderMuted("○"), check.Name())
		}

		start := time.Now()
		result := check.Run(ctx)
		if result.Name == "" {
			result.Name = check.Name()
		}
		// Set category from check if not already set by Run()
		if result.Category == "" {
			result.Category = check.Category()
		}

		// Attempt fix if check failed and is fixable
		if result.Status != StatusOK && check.CanFix() {
			// Stream: show the problem with fixing indicator (TTY only)
			if w != nil && isTTY {
				var problemIcon string
				if result.Status == StatusError {
					problemIcon = ui.RenderFailIcon()
				} else {
					problemIcon = ui.RenderWarnIcon()
				}
				// Overwrite the "checking" line with problem status + fixing indicator
				fmt.Fprintf(w, "\r  %s  %s", problemIcon, check.Name())
				if result.Message != "" {
					fmt.Fprintf(w, "%s", ui.RenderMuted(" "+result.Message))
				}
				fmt.Fprintf(w, "%s", ui.RenderMuted(" (fixing)..."))
			}

			err := check.Fix(ctx)
			if err == nil {
				// Re-run check to verify fix worked
				result = check.Run(ctx)
				if result.Name == "" {
					result.Name = check.Name()
				}
				// Set category again after re-run
				if result.Category == "" {
					result.Category = check.Category()
				}
				// Update message to indicate fix was applied
				if result.Status == StatusOK {
					result.Message = result.Message + " (fixed)"
					result.Fixed = true
				}
			} else {
				// Fix failed, add error to details
				result.Details = append(result.Details, "Fix failed: "+err.Error())
			}
		}

		// Record total elapsed time including any fix attempts
		result.Elapsed = time.Since(start)

		// Stream: output final result
		if w != nil {
			isSlow := slowThreshold > 0 && result.Elapsed >= slowThreshold
			if isSlow {
				report.Summary.Slow++
			}

			if isTTY {
				// TTY: overwrite line with colored result and icons
				var statusIcon string
				if result.Fixed {
					statusIcon = ui.RenderFixIcon()
				} else {
					switch result.Status {
					case StatusOK:
						statusIcon = ui.RenderPassIcon()
					case StatusWarning:
						statusIcon = ui.RenderWarnIcon()
					case StatusError:
						statusIcon = ui.RenderFailIcon()
					}
				}
				// Fix icon (🔧) is double-width, so use one less padding space
				slowIndicator := "  "
				if result.Fixed {
					slowIndicator = " "
				}
				if isSlow {
					slowIndicator = "⏳"
				}
				fmt.Fprintf(w, "\r  %s%s%s", statusIcon, slowIndicator, result.Name)
				if result.Message != "" {
					fmt.Fprintf(w, "%s", ui.RenderMuted(" "+result.Message))
				}
				if isSlow {
					fmt.Fprintf(w, "%s", ui.RenderMuted(" ("+formatDuration(result.Elapsed)+")"))
				}
			} else {
				// Non-TTY: clean line-by-line output with text prefixes
				var prefix string
				if result.Fixed {
					prefix = "FIXED"
				} else {
					switch result.Status {
					case StatusOK:
						prefix = "PASS"
					case StatusWarning:
						prefix = "WARN"
					case StatusError:
						prefix = "FAIL"
					}
				}
				fmt.Fprintf(w, "%s  %s", prefix, result.Name)
				if result.Message != "" {
					fmt.Fprintf(w, "  %s", result.Message)
				}
				if isSlow {
					fmt.Fprintf(w, "  (%s)", formatDuration(result.Elapsed))
				}
			}
			fmt.Fprintln(w)
		}

		report.Add(result)
	}

	return report
}

// BaseCheck provides a base implementation for checks that don't support auto-fix.
// Embed this in custom checks to get default CanFix() and Fix() implementations.
type BaseCheck struct {
	CheckName        string
	CheckDescription string
	CheckCategory    string // Category for grouping (e.g., CategoryCore)
}

// Category returns the check's category for grouping in output.
func (b *BaseCheck) Category() string {
	return b.CheckCategory
}

// Name returns the check name.
func (b *BaseCheck) Name() string {
	return b.CheckName
}

// Description returns the check description.
func (b *BaseCheck) Description() string {
	return b.CheckDescription
}

// CanFix returns false by default.
func (b *BaseCheck) CanFix() bool {
	return false
}

// Fix returns an error indicating this check cannot be auto-fixed.
func (b *BaseCheck) Fix(ctx *CheckContext) error {
	return ErrCannotFix
}

// FixableCheck provides a base implementation for checks that support auto-fix.
// Embed this and override CanFix() to return true, and implement Fix().
type FixableCheck struct {
	BaseCheck
}

// CanFix returns true for fixable checks.
func (f *FixableCheck) CanFix() bool {
	return true
}
