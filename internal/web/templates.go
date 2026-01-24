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
	Convoys      []ConvoyRow
	MergeQueue   []MergeQueueRow   // GitHub PRs
	InternalMRs  []InternalMRRow   // Internal beads MRs
	MergeHistory []MergeHistoryRow
	Polecats     []PolecatRow
	HQAgents     []HQAgentRow
	Activity     []ActivityEvent
}

// HQAgentRow represents a town-level agent (Mayor, Deacon) in the dashboard.
type HQAgentRow struct {
	Name         string        // "mayor" or "deacon"
	SessionID    string        // e.g., "hq-mayor"
	Status       string        // "running", "stopped"
	LastActivity activity.Info // Activity indicator
	StatusHint   string        // Last line from pane
}

// HQAgentDetail contains expanded information for an HQ agent.
type HQAgentDetail struct {
	HQAgentRow
	Uptime        string   // Session duration (e.g., "45m", "2h 15m")
	TerminalLines []string // Last N lines from tmux pane
}

// PolecatRow represents a polecat worker in the dashboard.
type PolecatRow struct {
	Name         string        // e.g., "dag", "nux"
	Rig          string        // e.g., "roxas", "gastown"
	SessionID    string        // e.g., "gt-roxas-dag"
	LastActivity activity.Info // Colored activity display
	StatusHint   string        // Last line from pane (optional)
}

// PolecatDetail contains expanded information for a polecat.
type PolecatDetail struct {
	PolecatRow
	HookBead      string   // Current work assignment bead ID
	HookTitle     string   // Title of hooked issue
	Uptime        string   // Session duration (e.g., "45m", "2h 15m")
	TerminalLines []string // Last N lines from tmux pane
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

// InternalMRRow represents an MR bead in the internal merge queue.
type InternalMRRow struct {
	ID          string // Bead ID (e.g., "mr-abc123")
	Rig         string // Rig name (e.g., "roxas")
	Title       string // MR title
	Branch      string // Source branch
	Target      string // Target branch (e.g., "main")
	Worker      string // Who did the work
	SourceIssue string // Source issue being merged (e.g., "gt-abc")
	Status      string // "open", "in_progress"
	ColorClass  string // "mq-green", "mq-yellow"
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

// TrackedIssueDetail contains expanded information for a tracked issue.
type TrackedIssueDetail struct {
	TrackedIssue
	AssigneeShort    string        // Just the polecat name (e.g., "chrome")
	AssigneeActivity activity.Info // Live tmux activity for assignee
	LastCommit       string        // Most recent commit message
	LastCommitAgo    string        // "2m ago"
}

// ConvoyDetail contains expanded information for a convoy.
type ConvoyDetail struct {
	ConvoyRow
	TrackedDetails []TrackedIssueDetail
	CreatedAt      string
	CreatedAgo     string
	Description    string // Bead description/notes
}

// MergeHistoryRow represents a recently merged or failed PR.
type MergeHistoryRow struct {
	Number     int
	Repo       string
	Title      string
	URL        string
	MergedAt   string // Timestamp
	MergedAgo  string // "3m ago"
	Success    bool
	FailReason string // Only if failed
	Hash       string // Short commit hash for ID
	Additions  int    // Lines added
	Deletions  int    // Lines removed
	FilesChanged []string // Top files changed (for expandable detail)
}

// MergeQueueStats contains 24h merge statistics.
type MergeQueueStats struct {
	MergedCount24h  int
	FailedCount24h  int
	AvgQueueMinutes int
}

// MergeQueueData contains all merge queue information.
type MergeQueueData struct {
	Stats        *MergeQueueStats
	PendingPRs   []MergeQueueRow
	RecentMerges []MergeHistoryRow
}

// ActivityEvent represents a single event in the activity feed.
type ActivityEvent struct {
	ID           string // Unique event ID
	Timestamp    string // RFC3339 timestamp
	TimestampAgo string // "2m ago"
	Type         string // Event type: commit, spawn, stop, dispatch, pr_merge, etc.
	Actor        string // Who caused it (polecat name, "refinery", etc.)
	ActorType    string // "polecat", "refinery", "witness", "mayor", "user"
	Summary      string // One-line description
	Details      string // Optional second line
	RelatedID    string // Convoy ID, Issue ID, PR number, etc.
	Rig          string // Which rig (for filtering)
}

// ActivityFeed contains activity events for the dashboard.
type ActivityFeed struct {
	Events []ActivityEvent
}

// LoadTemplates loads and parses all HTML templates.
func LoadTemplates() (*template.Template, error) {
	// Define template functions
	funcMap := template.FuncMap{
		"activityClass":   activityClass,
		"statusClass":     statusClass,
		"workStatusClass": workStatusClass,
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
