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
	Convoys []ConvoyRow
}

// ConvoyRow represents a single convoy in the dashboard.
type ConvoyRow struct {
	ID            string
	Title         string
	Status        string // "open" or "closed"
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
		"activityClass":   activityClass,
		"statusClass":     statusClass,
		"progressPercent": progressPercent,
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

// progressPercent calculates percentage as an integer for progress bars.
func progressPercent(completed, total int) int {
	if total == 0 {
		return 0
	}
	return (completed * 100) / total
}
