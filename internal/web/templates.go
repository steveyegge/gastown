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
	TaskQueue  []TaskRow
}

// TaskRow represents a standalone task in the task queue.
type TaskRow struct {
	ID         string
	Title      string
	Status     string        // "open", "in_progress", "closed"
	Priority   int           // 0-4
	Assignee   string
	ColorClass string        // Row highlight color
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
	ID          string
	Title       string
	Status      string
	Assignee    string
	Description string
	Priority    int
	Type        string
	CreatedAt   string
	UpdatedAt   string
}

// issuesByStatus filters tracked issues by status for kanban columns.
func issuesByStatus(issues []TrackedIssue, status string) []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range issues {
		if issue.Status == status {
			result = append(result, issue)
		}
	}
	return result
}

// LoadTemplates loads and parses all HTML templates.
func LoadTemplates() (*template.Template, error) {
	// Define template functions
	funcMap := template.FuncMap{
		"activityClass":   activityClass,
		"statusClass":     statusClass,
		"workStatusClass": workStatusClass,
		"priorityClass":   priorityClass,
		"priorityLabel":   priorityLabel,
		"progressPercent": progressPercent,
		"issuesByStatus":  issuesByStatus,
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

// priorityClass returns the CSS class for a priority level.
func priorityClass(priority int) string {
	switch priority {
	case 0:
		return "priority-critical"
	case 1:
		return "priority-high"
	case 2:
		return "priority-medium"
	case 3:
		return "priority-low"
	default:
		return "priority-minimal"
	}
}

// priorityLabel returns the display label for a priority level.
func priorityLabel(priority int) string {
	labels := []string{"P0", "P1", "P2", "P3", "P4"}
	if priority >= 0 && priority < len(labels) {
		return labels[priority]
	}
	return "P?"
}

// progressPercent calculates percentage as an integer for progress bars.
func progressPercent(completed, total int) int {
	if total == 0 {
		return 0
	}
	return (completed * 100) / total
}
