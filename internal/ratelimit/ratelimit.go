// Package ratelimit provides rate limit detection and backoff for polecats.
//
// When polecats hit Claude API rate limits, this package:
// - Detects rate limit errors in session output
// - Tracks rate limit state persistently
// - Computes exponential backoff periods
// - Provides pre-flight checks before spawning new work
package ratelimit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Common rate limit patterns in Claude Code output.
var rateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rate.?limit`),
	regexp.MustCompile(`(?i)too many requests`),
	regexp.MustCompile(`(?i)429`),
	regexp.MustCompile(`(?i)please wait.*before`),
	regexp.MustCompile(`(?i)exceeded.*quota`),
	regexp.MustCompile(`(?i)temporarily unavailable`),
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)capacity`),
	regexp.MustCompile(`(?i)throttl`),
}

// State represents the current rate limit status.
type State struct {
	// Limited indicates if we're currently rate limited.
	Limited bool `json:"limited"`

	// DetectedAt is when the rate limit was first detected.
	DetectedAt time.Time `json:"detected_at,omitempty"`

	// LastAttempt is when we last tried to spawn work.
	LastAttempt time.Time `json:"last_attempt,omitempty"`

	// ConsecutiveHits is the number of consecutive rate limit detections.
	// Used for exponential backoff calculation.
	ConsecutiveHits int `json:"consecutive_hits"`

	// BackoffUntil is when the backoff period ends.
	BackoffUntil time.Time `json:"backoff_until,omitempty"`

	// Account is the account that was rate limited (if known).
	Account string `json:"account,omitempty"`

	// Source describes where the rate limit was detected.
	Source string `json:"source,omitempty"`
}

// Tracker manages rate limit state for a rig.
type Tracker struct {
	statePath string
	mu        sync.Mutex
	state     *State
}

// NewTracker creates a rate limit tracker for a rig.
func NewTracker(rigPath string) *Tracker {
	runtimeDir := filepath.Join(rigPath, ".runtime")
	return &Tracker{
		statePath: filepath.Join(runtimeDir, "rate-limit.json"),
		state:     &State{},
	}
}

// Load reads rate limit state from disk.
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.statePath)
	if os.IsNotExist(err) {
		t.state = &State{}
		return nil
	}
	if err != nil {
		return err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	t.state = &state
	return nil
}

// Save writes rate limit state to disk.
func (t *Tracker) Save() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(t.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(t.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(t.statePath, data, 0644)
}

// State returns a copy of the current rate limit state.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return *t.state
}

// RecordRateLimit records a rate limit detection.
func (t *Tracker) RecordRateLimit(source, account string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	if !t.state.Limited {
		t.state.DetectedAt = now
	}

	t.state.Limited = true
	t.state.ConsecutiveHits++
	t.state.Source = source
	t.state.Account = account

	// Calculate backoff: starts at 30s, doubles each time, max 30 minutes
	backoffSeconds := 30 * (1 << min(t.state.ConsecutiveHits-1, 6))
	t.state.BackoffUntil = now.Add(time.Duration(backoffSeconds) * time.Second)
}

// RecordSuccess records a successful operation (no rate limit).
func (t *Tracker) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Reset state on success
	t.state.Limited = false
	t.state.ConsecutiveHits = 0
	t.state.BackoffUntil = time.Time{}
}

// ShouldDefer returns true if we should wait before spawning new work.
func (t *Tracker) ShouldDefer() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.state.Limited {
		return false
	}

	return time.Now().Before(t.state.BackoffUntil)
}

// TimeUntilReady returns how long to wait before spawning is allowed.
// Returns 0 if ready now.
func (t *Tracker) TimeUntilReady() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.state.Limited {
		return 0
	}

	remaining := time.Until(t.state.BackoffUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Clear manually clears the rate limit state.
func (t *Tracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = &State{}
}

// DetectRateLimit checks if output contains rate limit indicators.
func DetectRateLimit(output string) bool {
	if output == "" {
		return false
	}

	// Normalize for matching
	lower := strings.ToLower(output)

	for _, pattern := range rateLimitPatterns {
		if pattern.MatchString(lower) {
			return true
		}
	}

	return false
}

// ExtractRetryAfter attempts to extract a Retry-After value from output.
// Returns the duration to wait, or 0 if not found.
func ExtractRetryAfter(output string) time.Duration {
	// Look for patterns like "retry after 30 seconds" or "wait 2 minutes"
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)retry.?after\s+(\d+)\s*s`),
		regexp.MustCompile(`(?i)wait\s+(\d+)\s*s`),
		regexp.MustCompile(`(?i)retry.?after\s+(\d+)\s*m`),
		regexp.MustCompile(`(?i)wait\s+(\d+)\s*m`),
		regexp.MustCompile(`(?i)(\d+)\s*seconds?.*retry`),
		regexp.MustCompile(`(?i)(\d+)\s*minutes?.*retry`),
	}

	for i, pattern := range patterns {
		matches := pattern.FindStringSubmatch(output)
		if len(matches) > 1 {
			var value int
			parseIntFromString(matches[1], &value)
			// Patterns 2,3,5 are minutes, others are seconds
			if i == 2 || i == 3 || i == 5 {
				return time.Duration(value) * time.Minute
			}
			return time.Duration(value) * time.Second
		}
	}

	return 0
}

func parseIntFromString(s string, result *int) int {
	n := parseInt(s)
	*result = n
	return n
}

func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
