// Package guardian provides quality review for internal polecat-to-Refinery merges.
// Phase 1 is measurement-only: reviews are recorded but do not gate merges.
package guardian

import "time"

// Config holds the Guardian configuration values.
// These are typically sourced from config.JudgmentConfig in TownSettings.
type Config struct {
	Enabled     bool
	ReviewDepth string
	TimeoutSecs int

	// SecurityPaths are path substrings that flag diffs as security-sensitive.
	// If empty, DefaultSecurityPaths is used.
	SecurityPaths []string

	// CorePaths are path substrings that flag diffs as touching core infrastructure.
	// If empty, DefaultCorePaths is used.
	CorePaths []string
}

// DefaultSecurityPaths are the built-in security-sensitive path patterns.
var DefaultSecurityPaths = []string{"auth", "crypto", "secret", "token", "credential"}

// DefaultCorePaths are the built-in core infrastructure path patterns.
var DefaultCorePaths = []string{"internal/beads", "internal/config", "internal/session", "internal/tmux"}

// GuardianResult is the structured output of a Guardian quality review.
type GuardianResult struct {
	// BeadID is the bead being reviewed.
	BeadID string `json:"bead_id"`

	// Score is the overall quality score (0.0 = terrible, 1.0 = perfect).
	Score float64 `json:"score"`

	// Recommendation is the review verdict: "approve", "request_changes", or "skip".
	Recommendation string `json:"recommendation"`

	// Issues is the list of specific quality issues found.
	Issues []GuardianIssue `json:"issues,omitempty"`

	// Model is which Claude model performed the review.
	Model string `json:"model,omitempty"`

	// DurationMs is how long the review took in milliseconds.
	DurationMs float64 `json:"duration_ms"`

	// ReviewedAt is when the review was performed.
	ReviewedAt time.Time `json:"reviewed_at"`

	// Worker is the polecat whose work was reviewed.
	Worker string `json:"worker"`

	// Rig is the rig context.
	Rig string `json:"rig"`
}

// GuardianIssue represents a single quality issue found during review.
type GuardianIssue struct {
	// Severity: "critical", "major", "minor", "info".
	Severity string `json:"severity"`

	// Category: "correctness", "clarity", "edge_case", "security", "style".
	Category string `json:"category"`

	// Description of the issue.
	Description string `json:"description"`

	// File path where the issue was found (optional).
	File string `json:"file,omitempty"`

	// Line number where the issue was found (optional).
	Line int `json:"line,omitempty"`
}

// MergeDiff is the input to a Guardian review.
type MergeDiff struct {
	// BeadID is the merge request bead ID.
	BeadID string

	// Branch is the source branch being merged.
	Branch string

	// Target is the target branch (e.g., "main").
	Target string

	// Worker is the polecat who did the work.
	Worker string

	// Rig is the rig context.
	Rig string

	// DiffText is the raw unified diff output.
	DiffText string

	// Stats are computed statistics about the diff.
	Stats DiffStats
}

// DiffStats holds computed statistics about a diff.
type DiffStats struct {
	// FilesChanged is the number of files modified.
	FilesChanged int `json:"files_changed"`

	// LinesAdded is the total lines added.
	LinesAdded int `json:"lines_added"`

	// LinesRemoved is the total lines removed.
	LinesRemoved int `json:"lines_removed"`

	// HasSecurityPaths is true if changes touch security-sensitive paths.
	HasSecurityPaths bool `json:"has_security_paths"`

	// HasCorePaths is true if changes touch core infrastructure paths.
	HasCorePaths bool `json:"has_core_paths"`

	// DocsOnly is true if only documentation files were changed.
	DocsOnly bool `json:"docs_only"`

	// ConfigOnly is true if only configuration files were changed.
	ConfigOnly bool `json:"config_only"`
}
