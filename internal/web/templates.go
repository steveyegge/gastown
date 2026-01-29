// Package web provides HTTP server and templates for the Gas Town dashboard.
package web

import (
	"embed"
	"html/template"
	"io/fs"

	"github.com/steveyegge/gastown/internal/activity"
)

//go:embed templates/*.html
var templateFS embed.FS

// ConvoyData represents data passed to the convoy template.
type ConvoyData struct {
	Convoys    []ConvoyRow
	MergeQueue []MergeQueueRow
	Polecats   []PolecatRow
	RigPath    string // e.g., "~/gt/atmosphere"
}

// PolecatRow represents a polecat worker in the dashboard.
type PolecatRow struct {
	Name         string        // e.g., "dag", "nux"
	Rig          string        // e.g., "roxas", "gastown"
	SessionID    string        // e.g., "gt-roxas-dag"
	LastActivity activity.Info // Colored activity display
	StatusHint   string        // Last line from pane (optional)
}

// MergeQueueRow represents a PR in the merge queue.
type MergeQueueRow struct {
	Number     int
	Repo       string // Short repo name (e.g., "roxas", "gastown")
	Title      string
	URL        string
	CIStatus   string // "pass", "fail", "pending"
	Mergeable  string // "ready", "conflict", "pending"
	ColorClass string // "mq-green", "mq-yellow", "mq-red"
}

// ConvoyRow represents a single convoy in the dashboard.
type ConvoyRow struct {
	ID            string
	Title         string
	Status        string // "open" or "closed" (raw beads status)
	WorkStatus    string // Computed: "complete", "active", "stale", "stuck", "waiting"
	Progress      string // e.g., "2/5"
	Completed     int
	Total         int
	LastActivity  activity.Info
	TrackedIssues []TrackedIssue
}

// TrackedIssue represents an issue tracked by a convoy.
type TrackedIssue struct {
	ID       string
	Title    string
	Status   string
	Assignee string
}

// LoadTemplates loads and parses all HTML templates.
func LoadTemplates() (*template.Template, error) {
	// Define template functions
	funcMap := template.FuncMap{
		"activityClass":     activityClass,
		"statusClass":       statusClass,
		"workStatusClass":   workStatusClass,
		"progressPercent":   progressPercent,
		"countComplete":     countComplete,
		"rowClass":          rowClass,
		"statusBadgeClass":  statusBadgeClass,
		"ringClass":         ringClass,
		"ringOffset":        ringOffset,
		"ringColor":         ringColor,
		"actionBtnClass":    actionBtnClass,
		"actionLabel":       actionLabel,
		"mergeStatusClass":  mergeStatusClass,
		"mergeStatusLabel":  mergeStatusLabel,
		"ciClass":           ciClass,
		"ciLabel":           ciLabel,
		"workerRowClass":    workerRowClass,
		"workerStatusClass": workerStatusClass,
		"workerStatusLabel": workerStatusLabel,
		"workerActionClass": workerActionClass,
		"workerActionLabel": workerActionLabel,
	}

	// Get the templates subdirectory
	subFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}

	// Parse all templates
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(subFS, "*.html")
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

// activityClass returns the CSS class for an activity color.
func activityClass(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorGreen:
		return "activity-green"
	case activity.ColorYellow:
		return "activity-yellow"
	case activity.ColorRed:
		return "activity-red"
	default:
		return "activity-unknown"
	}
}

// statusClass returns the CSS class for a convoy status.
func statusClass(status string) string {
	switch status {
	case "open":
		return "status-open"
	case "closed":
		return "status-closed"
	default:
		return "status-unknown"
	}
}

// workStatusClass returns the CSS class for a computed work status.
func workStatusClass(workStatus string) string {
	switch workStatus {
	case "complete":
		return "work-complete"
	case "active":
		return "work-active"
	case "stale":
		return "work-stale"
	case "stuck":
		return "work-stuck"
	case "waiting":
		return "work-waiting"
	default:
		return "work-unknown"
	}
}

// progressPercent calculates percentage as an integer for progress bars.
func progressPercent(completed, total int) int {
	if total == 0 {
		return 0
	}
	return (completed * 100) / total
}

// countComplete counts convoys with "complete" work status.
func countComplete(convoys []ConvoyRow) int {
	count := 0
	for _, c := range convoys {
		if c.WorkStatus == "complete" {
			count++
		}
	}
	return count
}

// rowClass returns row tint class based on work status.
func rowClass(workStatus string) string {
	switch workStatus {
	case "stuck":
		return "row-critical"
	case "stale":
		return "row-warning"
	default:
		return ""
	}
}

// statusBadgeClass returns the CSS class for status badge.
func statusBadgeClass(workStatus string) string {
	switch workStatus {
	case "complete":
		return "status-complete"
	case "active":
		return "status-active"
	case "stale":
		return "status-stale"
	case "stuck":
		return "status-stuck"
	case "waiting":
		return "status-waiting"
	default:
		return "status-active"
	}
}

// ringClass returns progress ring class based on status.
func ringClass(workStatus string) string {
	switch workStatus {
	case "stuck":
		return "ring-critical"
	case "stale":
		return "ring-warning"
	default:
		return ""
	}
}

// ringOffset calculates SVG stroke-dashoffset for progress ring.
// Circumference = 2 * π * r = 2 * 3.14159 * 13 ≈ 82
func ringOffset(completed, total int) int {
	if total == 0 {
		return 82 // Full offset = empty ring
	}
	pct := float64(completed) / float64(total)
	// offset = circumference * (1 - pct)
	return int(82 * (1 - pct))
}

// ringColor returns CSS color variable based on status.
func ringColor(workStatus string) string {
	switch workStatus {
	case "complete":
		return "var(--green)"
	case "active":
		return "var(--accent)"
	case "stale":
		return "var(--yellow)"
	case "stuck":
		return "var(--red)"
	default:
		return "var(--accent)"
	}
}

// actionBtnClass returns action button class based on status.
func actionBtnClass(workStatus string) string {
	switch workStatus {
	case "complete":
		return "muted"
	case "active":
		return "primary"
	case "stale":
		return "warning"
	case "stuck":
		return "danger"
	default:
		return "primary"
	}
}

// actionLabel returns action button label based on status.
func actionLabel(workStatus string) string {
	switch workStatus {
	case "complete":
		return "View"
	case "stuck":
		return "Retry"
	case "stale":
		return "Nudge"
	default:
		return "View"
	}
}

// mergeStatusClass returns merge status badge class.
func mergeStatusClass(mergeable string) string {
	switch mergeable {
	case "ready":
		return "status-ready"
	case "conflict":
		return "status-blocked"
	case "pending":
		return "status-pending"
	default:
		return "status-pending"
	}
}

// mergeStatusLabel returns merge status label.
func mergeStatusLabel(mergeable string) string {
	switch mergeable {
	case "ready":
		return "Ready"
	case "conflict":
		return "Blocked"
	case "pending":
		return "Pending"
	default:
		return "Pending"
	}
}

// ciClass returns CI badge class.
func ciClass(ciStatus string) string {
	switch ciStatus {
	case "pass":
		return "ci-pass"
	case "fail":
		return "ci-fail"
	default:
		return "ci-pending"
	}
}

// ciLabel returns CI badge label.
func ciLabel(ciStatus string) string {
	switch ciStatus {
	case "pass":
		return "Pass"
	case "fail":
		return "Fail"
	default:
		return "Pending"
	}
}

// workerRowClass returns worker row tint class based on activity.
func workerRowClass(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorRed:
		return "row-critical"
	case activity.ColorYellow:
		return "row-warning"
	default:
		return ""
	}
}

// workerStatusClass returns worker status badge class.
func workerStatusClass(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorGreen:
		return "status-healthy"
	case activity.ColorYellow:
		return "status-warning"
	case activity.ColorRed:
		return "status-offline"
	default:
		return "status-healthy"
	}
}

// workerStatusLabel returns worker status label.
func workerStatusLabel(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorGreen:
		return "Healthy"
	case activity.ColorYellow:
		return "Warning"
	case activity.ColorRed:
		return "Offline"
	default:
		return "Unknown"
	}
}

// workerActionClass returns worker action button class.
func workerActionClass(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorRed:
		return "danger"
	case activity.ColorYellow:
		return "warning"
	default:
		return "muted"
	}
}

// workerActionLabel returns worker action button label.
func workerActionLabel(info activity.Info) string {
	switch info.ColorClass {
	case activity.ColorRed:
		return "Restart"
	case activity.ColorYellow:
		return "Restart"
	default:
		return "View Logs"
	}
}
