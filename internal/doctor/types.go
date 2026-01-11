// Package doctor provides a framework for running health checks on Gas Town workspaces.
package doctor

import (
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/steveyegge/gastown/internal/ui"
)

// Category constants for grouping checks
const (
	CategoryCore          = "Core"
	CategoryInfrastructure = "Infrastructure"
	CategoryRig           = "Rig"
	CategoryPatrol        = "Patrol"
	CategoryConfig        = "Configuration"
	CategoryCleanup       = "Cleanup"
	CategoryHooks         = "Hooks"
)

// CategoryOrder defines the display order for categories
var CategoryOrder = []string{
	CategoryCore,
	CategoryInfrastructure,
	CategoryRig,
	CategoryPatrol,
	CategoryConfig,
	CategoryCleanup,
	CategoryHooks,
}

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
	Name     string      // Check name
	Status   CheckStatus // Result status
	Message  string      // Primary result message
	Details  []string    // Additional information
	FixHint  string      // Suggestion if not auto-fixable
	Category string      // Category for grouping (e.g., CategoryCore)
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
// Matches bd doctor UX: grouped by category, semantic icons, warnings section.
func (r *Report) Print(w io.Writer, verbose bool) {
	// Print header with version placeholder (caller should set via PrintWithVersion)
	_, _ = fmt.Fprintln(w)

	// Group checks by category
	checksByCategory := make(map[string][]*CheckResult)
	for _, check := range r.Checks {
		cat := check.Category
		if cat == "" {
			cat = "Other"
		}
		checksByCategory[cat] = append(checksByCategory[cat], check)
	}

	// Track warnings/errors for summary section
	var warnings []*CheckResult

	// Print checks by category in defined order
	for _, category := range CategoryOrder {
		checks, exists := checksByCategory[category]
		if !exists || len(checks) == 0 {
			continue
		}

		// Print category header
		_, _ = fmt.Fprintln(w, ui.RenderCategory(category))

		// Print each check in this category
		for _, check := range checks {
			r.printCheck(w, check, verbose)
			if check.Status != StatusOK {
				warnings = append(warnings, check)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Print any checks without a category
	if otherChecks, exists := checksByCategory["Other"]; exists && len(otherChecks) > 0 {
		_, _ = fmt.Fprintln(w, ui.RenderCategory("Other"))
		for _, check := range otherChecks {
			r.printCheck(w, check, verbose)
			if check.Status != StatusOK {
				warnings = append(warnings, check)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Print separator and summary
	_, _ = fmt.Fprintln(w, ui.RenderSeparator())
	r.printSummary(w)

	// Print warnings/errors section with fixes
	r.printWarningsSection(w, warnings)
}

// printCheck outputs a single check result with semantic styling.
func (r *Report) printCheck(w io.Writer, check *CheckResult, verbose bool) {
	var statusIcon string
	switch check.Status {
	case StatusOK:
		statusIcon = ui.RenderPassIcon()
	case StatusWarning:
		statusIcon = ui.RenderWarnIcon()
	case StatusError:
		statusIcon = ui.RenderFailIcon()
	}

	// Print check line: icon + name + muted message
	_, _ = fmt.Fprintf(w, "  %s  %s", statusIcon, check.Name)
	if check.Message != "" {
		_, _ = fmt.Fprintf(w, "%s", ui.RenderMuted(" "+check.Message))
	}
	_, _ = fmt.Fprintln(w)

	// Print details in verbose mode or for non-OK results (with tree connector)
	if len(check.Details) > 0 && (verbose || check.Status != StatusOK) {
		for _, detail := range check.Details {
			_, _ = fmt.Fprintf(w, "     %s%s\n", ui.MutedStyle.Render(ui.TreeLast), ui.RenderMuted(detail))
		}
	}
}

// printSummary outputs the summary line with semantic icons.
func (r *Report) printSummary(w io.Writer) {
	summary := fmt.Sprintf("%s %d passed  %s %d warnings  %s %d failed",
		ui.RenderPassIcon(), r.Summary.OK,
		ui.RenderWarnIcon(), r.Summary.Warnings,
		ui.RenderFailIcon(), r.Summary.Errors,
	)
	_, _ = fmt.Fprintln(w, summary)
}

// printWarningsSection outputs numbered warnings/errors sorted by severity.
func (r *Report) printWarningsSection(w io.Writer, warnings []*CheckResult) {
	if len(warnings) == 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, ui.RenderPass(ui.IconPass+" All checks passed"))
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, ui.RenderWarn(ui.IconWarn+"  WARNINGS"))

	// Sort by severity: errors first, then warnings
	slices.SortStableFunc(warnings, func(a, b *CheckResult) int {
		if a.Status == StatusError && b.Status != StatusError {
			return -1
		}
		if a.Status != StatusError && b.Status == StatusError {
			return 1
		}
		return 0
	})

	for i, check := range warnings {
		line := fmt.Sprintf("%s: %s", check.Name, check.Message)
		if check.Status == StatusError {
			_, _ = fmt.Fprintf(w, "  %s  %s %s\n", ui.RenderFailIcon(), ui.RenderFail(fmt.Sprintf("%d.", i+1)), ui.RenderFail(line))
		} else {
			_, _ = fmt.Fprintf(w, "  %s  %s %s\n", ui.RenderWarnIcon(), ui.RenderWarn(fmt.Sprintf("%d.", i+1)), line)
		}
		if check.FixHint != "" {
			_, _ = fmt.Fprintf(w, "        %s%s\n", ui.MutedStyle.Render(ui.TreeLast), check.FixHint)
		}
	}
}
