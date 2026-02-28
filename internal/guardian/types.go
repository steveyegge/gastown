// Package guardian provides types and state persistence for quality review judgments.
//
// The guardian system is agent-driven: the Refinery agent (Claude) makes quality
// decisions, and this package provides the Go transport/persistence layer.
// No Go code in this package makes quality decisions.
package guardian

import "time"

// Config holds guardian configuration for a rig.
type Config struct {
	Enabled       bool     `json:"enabled"`
	ReviewDepth   string   `json:"review_depth,omitempty"`   // "quick", "standard", "deep"
	TimeoutSecs   int      `json:"timeout_secs,omitempty"`   // max seconds for review
	SecurityPaths []string `json:"security_paths,omitempty"` // paths requiring security review
	CorePaths     []string `json:"core_paths,omitempty"`     // paths requiring careful review
}

// GuardianResult represents the outcome of a quality review.
// Populated by the Refinery agent and persisted via `gt judgment record`.
type GuardianResult struct {
	BeadID         string          `json:"bead_id"`
	Score          float64         `json:"score"`          // 0.0–1.0
	Recommendation string          `json:"recommendation"` // "approve", "request_changes", "skip"
	Issues         []GuardianIssue `json:"issues"`
	Model          string          `json:"model,omitempty"`       // review depth used
	DurationMs     int64           `json:"duration_ms,omitempty"` // review wall time
	ReviewedAt     time.Time       `json:"reviewed_at,omitempty"`
	Worker         string          `json:"worker"` // polecat that submitted the work
	Rig            string          `json:"rig"`    // rig name
}

// GuardianIssue represents a single issue found during quality review.
type GuardianIssue struct {
	Severity    string `json:"severity"`              // "critical", "major", "minor", "style"
	Category    string `json:"category"`              // "correctness", "security", "clarity", "style"
	Description string `json:"description"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
}

// MergeDiff represents the diff context for a quality review.
type MergeDiff struct {
	BeadID   string    `json:"bead_id"`
	Branch   string    `json:"branch"`
	Target   string    `json:"target"`
	Worker   string    `json:"worker"`
	Rig      string    `json:"rig"`
	DiffText string    `json:"diff_text"`
	Stats    DiffStats `json:"stats"`
}

// DiffStats summarizes the changes in a merge diff.
type DiffStats struct {
	FilesChanged    int  `json:"files_changed"`
	LinesAdded      int  `json:"lines_added"`
	LinesRemoved    int  `json:"lines_removed"`
	HasSecurityPaths bool `json:"has_security_paths"`
	HasCorePaths    bool `json:"has_core_paths"`
	DocsOnly        bool `json:"docs_only"`
	ConfigOnly      bool `json:"config_only"`
}

// Recommendation constants.
const (
	RecommendApprove        = "approve"
	RecommendRequestChanges = "request_changes"
	RecommendSkip           = "skip"
)

// Review depth constants.
const (
	DepthQuick    = "quick"
	DepthStandard = "standard"
	DepthDeep     = "deep"
)

// Status thresholds for judgment scoring.
const (
	StatusOK     = "OK"
	StatusWarn   = "WARN"
	StatusBreach = "BREACH"

	ThresholdWarn   = 0.60 // Below this: WARN
	ThresholdBreach = 0.45 // Below this: BREACH
)

// StatusForScore returns the status label for a given average score.
func StatusForScore(score float64) string {
	switch {
	case score < ThresholdBreach:
		return StatusBreach
	case score < ThresholdWarn:
		return StatusWarn
	default:
		return StatusOK
	}
}

// DefaultSecurityPaths are file patterns that warrant security-focused review.
var DefaultSecurityPaths = []string{
	"**/auth/**",
	"**/security/**",
	"**/crypto/**",
	"**/*secret*",
	"**/*credential*",
	"**/*token*",
	"**/.env*",
}

// DefaultCorePaths are file patterns that warrant careful review.
var DefaultCorePaths = []string{
	"**/cmd/**",
	"**/config/**",
	"**/internal/**",
	"go.mod",
	"go.sum",
	"Makefile",
}
