package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/telemetry"
)

// ReviewFunc is the function signature for performing a model review.
// Production: calls bd subprocess. Tests: returns mock response.
type ReviewFunc func(ctx context.Context, model, prompt string) (string, error)

// Guardian performs quality reviews on merge diffs.
// It is called synchronously by the Refinery, not a long-running agent.
type Guardian struct {
	config   *Config
	stateDir string
	townRoot string
	rigPath  string

	// ReviewFunc is injectable for testing.
	ReviewFunc ReviewFunc
}

// GetConfig returns the Guardian's configuration.
func (g *Guardian) GetConfig() *Config {
	return g.config
}

// New creates a new Guardian with the given configuration.
// stateDir is the directory for persistent state (typically townRoot).
func New(cfg *Config, townRoot, rigPath string) *Guardian {
	if cfg == nil {
		cfg = &Config{ReviewDepth: "standard", TimeoutSecs: 120}
	}
	return &Guardian{
		config:   cfg,
		stateDir: townRoot,
		townRoot: townRoot,
		rigPath:  rigPath,
	}
}

// Review performs a quality review on a merge diff.
// Returns nil if Guardian is disabled or the diff should be skipped.
// On ReviewFunc error, returns the error (fail-open: caller should proceed with merge).
func (g *Guardian) Review(ctx context.Context, diff *MergeDiff) (*GuardianResult, error) {
	if !g.config.Enabled {
		return nil, nil
	}

	if diff == nil || diff.DiffText == "" {
		return nil, nil
	}

	// Classify risk and determine if we should skip
	if shouldSkip(diff.Stats) {
		result := &GuardianResult{
			BeadID:         diff.BeadID,
			Score:          1.0,
			Recommendation: "skip",
			DurationMs:     0,
			ReviewedAt:     time.Now(),
			Worker:         diff.Worker,
			Rig:            diff.Rig,
		}
		// Persist skip result (log-and-continue on error)
		if err := g.persistResult(result); err != nil {
			log.Printf("guardian: persist skip result: %v", err)
		}
		return result, nil
	}

	if g.ReviewFunc == nil {
		return nil, fmt.Errorf("guardian: ReviewFunc not configured")
	}

	// Build prompt
	model := g.resolveModel()
	prompt := buildReviewPrompt(diff, g.config.ReviewDepth)

	// Call model
	start := time.Now()

	timeout := time.Duration(g.config.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	reviewCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response, err := g.ReviewFunc(reviewCtx, model, prompt)
	durationMs := float64(time.Since(start).Milliseconds())

	if err != nil {
		return nil, fmt.Errorf("guardian review failed: %w", err)
	}

	// Parse response
	result, err := parseReviewResponse(response, diff, durationMs)
	if err != nil {
		return nil, fmt.Errorf("guardian: failed to parse review response: %w", err)
	}
	result.Model = model

	// Record telemetry
	telemetry.RecordGuardianResult(ctx, diff.Worker, diff.Rig, result.Recommendation, result.Score, durationMs)

	// Persist result to state (log-and-continue on error)
	if err := g.persistResult(result); err != nil {
		log.Printf("guardian: persist review result: %v", err)
	}

	return result, nil
}

// persistResult saves a review result to the Guardian state file.
// Returns any error from load/save for the caller to handle.
func (g *Guardian) persistResult(result *GuardianResult) error {
	state, err := LoadState(g.stateDir)
	if err != nil {
		log.Printf("guardian: failed to load state: %v (starting fresh)", err)
		state = NewGuardianState()
	}
	state.AddResult(result.Worker, result)
	if err := SaveState(g.stateDir, state); err != nil {
		log.Printf("guardian: failed to save state: %v", err)
		return fmt.Errorf("saving guardian state: %w", err)
	}
	return nil
}

// resolveModel returns the model name for Guardian reviews.
func (g *Guardian) resolveModel() string {
	switch g.config.ReviewDepth {
	case "quick":
		return "haiku"
	case "deep":
		return "opus"
	default:
		return "sonnet"
	}
}

// classifyRisk determines the risk level of a diff based on its stats.
func classifyRisk(stats DiffStats) string {
	totalLines := stats.LinesAdded + stats.LinesRemoved
	if stats.HasSecurityPaths || totalLines > 500 {
		return "high"
	}
	if stats.HasCorePaths || totalLines > 100 || stats.FilesChanged > 10 {
		return "medium"
	}
	return "low"
}

// shouldSkip returns true if a diff is trivial enough to skip review.
func shouldSkip(stats DiffStats) bool {
	return stats.DocsOnly || stats.ConfigOnly
}

// buildReviewPrompt creates the prompt for the model review.
func buildReviewPrompt(diff *MergeDiff, depth string) string {
	risk := classifyRisk(diff.Stats)

	var sb strings.Builder
	sb.WriteString("You are a code quality reviewer for Gas Town (an AI agent orchestration platform).\n")
	sb.WriteString("Review the following merge diff and provide a structured quality assessment.\n\n")

	sb.WriteString(fmt.Sprintf("Branch: %s -> %s\n", diff.Branch, diff.Target))
	sb.WriteString(fmt.Sprintf("Worker: %s\n", diff.Worker))
	sb.WriteString(fmt.Sprintf("Risk level: %s\n", risk))
	sb.WriteString(fmt.Sprintf("Files changed: %d, +%d/-%d lines\n\n",
		diff.Stats.FilesChanged, diff.Stats.LinesAdded, diff.Stats.LinesRemoved))

	if depth == "quick" {
		sb.WriteString("Perform a QUICK review focusing only on critical issues.\n\n")
	} else if depth == "deep" {
		sb.WriteString("Perform a THOROUGH review covering correctness, clarity, edge cases, and security.\n\n")
	} else {
		sb.WriteString("Perform a STANDARD review covering correctness and important issues.\n\n")
	}

	sb.WriteString("Respond with ONLY a JSON object (no markdown fencing) in this format:\n")
	sb.WriteString(`{
  "score": 0.85,
  "recommendation": "approve",
  "issues": [
    {
      "severity": "minor",
      "category": "correctness",
      "description": "Potential nil pointer on line 42",
      "file": "path/to/file.go",
      "line": 42
    }
  ]
}`)
	sb.WriteString("\n\nscore: 0.0 (terrible) to 1.0 (perfect)\n")
	sb.WriteString("recommendation: \"approve\", \"request_changes\"\n")
	sb.WriteString("severity: \"critical\", \"major\", \"minor\", \"info\"\n")
	sb.WriteString("category: \"correctness\", \"clarity\", \"edge_case\", \"security\", \"style\"\n\n")
	sb.WriteString("--- DIFF ---\n")

	// Truncate very large diffs
	diffText := diff.DiffText
	const maxDiffLen = 50000
	if len(diffText) > maxDiffLen {
		diffText = diffText[:maxDiffLen] + "\n... (truncated)"
	}
	sb.WriteString(diffText)

	return sb.String()
}

// parseReviewResponse extracts the GuardianResult from the model's JSON response.
func parseReviewResponse(response string, diff *MergeDiff, durationMs float64) (*GuardianResult, error) {
	// Try to find JSON in the response (model may include extra text)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var parsed struct {
		Score          float64         `json:"score"`
		Recommendation string          `json:"recommendation"`
		Issues         []GuardianIssue `json:"issues"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate score range
	if parsed.Score < 0 {
		parsed.Score = 0
	}
	if parsed.Score > 1 {
		parsed.Score = 1
	}

	// Validate recommendation
	switch parsed.Recommendation {
	case "approve", "request_changes":
		// valid
	default:
		if parsed.Score >= 0.6 {
			parsed.Recommendation = "approve"
		} else {
			parsed.Recommendation = "request_changes"
		}
	}

	return &GuardianResult{
		BeadID:         diff.BeadID,
		Score:          parsed.Score,
		Recommendation: parsed.Recommendation,
		Issues:         parsed.Issues,
		DurationMs:     durationMs,
		ReviewedAt:     time.Now(),
		Worker:         diff.Worker,
		Rig:            diff.Rig,
	}, nil
}

// extractJSON finds the first valid JSON object in a string using json.Decoder
// to correctly handle braces inside quoted strings.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}

	// Try decoding from each '{' position until we find valid JSON
	for start < len(s) {
		dec := json.NewDecoder(strings.NewReader(s[start:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == nil {
			return string(raw)
		}
		// Try the next '{' after this position
		next := strings.Index(s[start+1:], "{")
		if next == -1 {
			return ""
		}
		start = start + 1 + next
	}
	return ""
}

// ComputeDiffStats parses a unified diff for file count, line counts, and risk signals.
// Uses the provided config for security/core path patterns, falling back to defaults.
func ComputeDiffStats(diffText string, cfg *Config) DiffStats {
	var stats DiffStats

	securityPaths := DefaultSecurityPaths
	corePaths := DefaultCorePaths
	if cfg != nil && len(cfg.SecurityPaths) > 0 {
		securityPaths = cfg.SecurityPaths
	}
	if cfg != nil && len(cfg.CorePaths) > 0 {
		corePaths = cfg.CorePaths
	}

	files := make(map[string]bool)
	allDocs := true
	allConfig := true

	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			path := line[6:]
			files[path] = true

			if !isDocFile(path) {
				allDocs = false
			}
			if !isConfigFile(path) {
				allConfig = false
			}
			if matchesAnyPattern(path, securityPaths) {
				stats.HasSecurityPaths = true
			}
			if matchesAnyPattern(path, corePaths) {
				stats.HasCorePaths = true
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.LinesAdded++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.LinesRemoved++
		}
	}

	stats.FilesChanged = len(files)
	if stats.FilesChanged > 0 {
		stats.DocsOnly = allDocs
		stats.ConfigOnly = allConfig
	}

	return stats
}

func isDocFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".txt") ||
		strings.HasSuffix(lower, ".rst") ||
		strings.Contains(lower, "doc/") ||
		strings.Contains(lower, "docs/")
}

func isConfigFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".json") ||
		strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".toml") ||
		strings.HasSuffix(lower, ".ini")
}

func matchesAnyPattern(path string, patterns []string) bool {
	lower := strings.ToLower(path)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
