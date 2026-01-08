// Package doctor provides a framework for running health checks on Gas Town workspaces.
package doctor

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/style"
)

// CheckStatus represents the result status of a health check.
type CheckStatus int

const (
	// StatusOK indicates the check passed.
	StatusOK CheckStatus = iota
	// StatusWarning indicates a non-critical issue.
	StatusWarning
	// StatusError indicates a critical problem.
	StatusError
)

// String returns a human-readable status.
func (s CheckStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarning:
		return "Warning"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// CheckContext provides context for running checks.
type CheckContext struct {
	TownRoot        string // Root directory of the Gas Town workspace
	RigName         string // Rig name (empty for town-level checks)
	Verbose         bool   // Enable verbose output
	RestartSessions bool   // Restart patrol sessions when fixing (requires explicit --restart-sessions flag)
}

// RigPath returns the full path to the rig directory.
// Returns empty string if RigName is not set.
func (ctx *CheckContext) RigPath() string {
	if ctx.RigName == "" {
		return ""
	}
	return ctx.TownRoot + "/" + ctx.RigName
}

// CheckResult represents the outcome of a health check.
type CheckResult struct {
	Name    string      // Check name
	Status  CheckStatus // Result status
	Message string      // Primary result message
	Details []string    // Additional information
	FixHint string      // Suggestion if not auto-fixable
}

// Check defines the interface for a health check.
type Check interface {
	// Name returns the check identifier.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Run executes the check and returns a result.
	Run(ctx *CheckContext) *CheckResult

	// Fix attempts to automatically fix the issue.
	// Should only be called if CanFix() returns true.
	Fix(ctx *CheckContext) error

	// CanFix returns true if this check can automatically fix issues.
	CanFix() bool
}

// ReportSummary summarizes the results of all checks.
type ReportSummary struct {
	Total    int
	OK       int
	Warnings int
	Errors   int
}

// Report contains all check results and a summary.
type Report struct {
	Timestamp time.Time
	Checks    []*CheckResult
	Summary   ReportSummary
}

// NewReport creates an empty report with the current timestamp.
func NewReport() *Report {
	return &Report{
		Timestamp: time.Now(),
		Checks:    make([]*CheckResult, 0),
	}
}

// Add adds a check result to the report and updates the summary.
func (r *Report) Add(result *CheckResult) {
	r.Checks = append(r.Checks, result)
	r.Summary.Total++

	switch result.Status {
	case StatusOK:
		r.Summary.OK++
	case StatusWarning:
		r.Summary.Warnings++
	case StatusError:
		r.Summary.Errors++
	}
}

// HasErrors returns true if any check reported an error.
func (r *Report) HasErrors() bool {
	return r.Summary.Errors > 0
}

// HasWarnings returns true if any check reported a warning.
func (r *Report) HasWarnings() bool {
	return r.Summary.Warnings > 0
}

// IsHealthy returns true if all checks passed without errors or warnings.
func (r *Report) IsHealthy() bool {
	return r.Summary.Errors == 0 && r.Summary.Warnings == 0
}

// Print outputs the report to the given writer.
func (r *Report) Print(w io.Writer, verbose bool) {
	// Print individual check results
	for _, check := range r.Checks {
		r.printCheck(w, check, verbose)
	}

	// Print summary (output errors non-actionable)
	_, _ = fmt.Fprintln(w)
	r.printSummary(w)
}

// printCheck outputs a single check result (output errors non-actionable).
func (r *Report) printCheck(w io.Writer, check *CheckResult, verbose bool) {
	var prefix string
	switch check.Status {
	case StatusOK:
		prefix = style.SuccessPrefix
	case StatusWarning:
		prefix = style.WarningPrefix
	case StatusError:
		prefix = style.ErrorPrefix
	}

	_, _ = fmt.Fprintf(w, "%s %s: %s\n", prefix, check.Name, check.Message)

	// Print details in verbose mode or for non-OK results
	if len(check.Details) > 0 && (verbose || check.Status != StatusOK) {
		for _, detail := range check.Details {
			_, _ = fmt.Fprintf(w, "    %s\n", detail)
		}
	}

	// Print fix hint for errors/warnings
	if check.FixHint != "" && check.Status != StatusOK {
		_, _ = fmt.Fprintf(w, "    %s %s\n", style.ArrowPrefix, check.FixHint)
	}
}

// printSummary outputs the summary line (output errors non-actionable).
func (r *Report) printSummary(w io.Writer) {
	parts := []string{
		fmt.Sprintf("%d checks", r.Summary.Total),
	}

	if r.Summary.OK > 0 {
		parts = append(parts, style.Success.Render(fmt.Sprintf("%d passed", r.Summary.OK)))
	}
	if r.Summary.Warnings > 0 {
		parts = append(parts, style.Warning.Render(fmt.Sprintf("%d warnings", r.Summary.Warnings)))
	}
	if r.Summary.Errors > 0 {
		parts = append(parts, style.Error.Render(fmt.Sprintf("%d errors", r.Summary.Errors)))
	}

	_, _ = fmt.Fprintln(w, strings.Join(parts, ", "))
}
