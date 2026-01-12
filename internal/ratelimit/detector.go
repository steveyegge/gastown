package ratelimit

import (
	"regexp"
	"strings"
	"time"
)

// rateLimitPatterns are regex patterns that indicate a rate limit error in stderr.
var rateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)429`),
	regexp.MustCompile(`(?i)rate.?limit`),
	regexp.MustCompile(`(?i)too many requests`),
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)capacity`),
	regexp.MustCompile(`(?i)throttl`),
}

// Detector detects rate limit events from exit codes and stderr output.
type Detector interface {
	// Detect checks if the exit indicates a rate limit.
	// Returns the event and true if rate limit detected, nil and false otherwise.
	Detect(exitCode int, stderr string) (*RateLimitEvent, bool)
}

// DefaultDetector is the standard rate limit detector implementation.
type DefaultDetector struct {
	agentID string
	profile string
}

// NewDetector creates a new rate limit detector.
func NewDetector(agentID, profile string) *DefaultDetector {
	return &DefaultDetector{
		agentID: agentID,
		profile: profile,
	}
}

// Detect checks if the exit code or stderr indicates a rate limit.
func (d *DefaultDetector) Detect(exitCode int, stderr string) (*RateLimitEvent, bool) {
	// Check for rate limit exit code
	if exitCode == ExitCodeRateLimit {
		return d.createEvent(exitCode, stderr), true
	}

	// Check stderr for rate limit patterns
	for _, pattern := range rateLimitPatterns {
		if pattern.MatchString(stderr) {
			return d.createEvent(exitCode, stderr), true
		}
	}

	return nil, false
}

// createEvent builds a RateLimitEvent from the available information.
func (d *DefaultDetector) createEvent(exitCode int, stderr string) *RateLimitEvent {
	// Extract error snippet (first meaningful line)
	snippet := extractErrorSnippet(stderr)

	// Detect provider from error message
	provider := detectProvider(stderr)

	return &RateLimitEvent{
		AgentID:      d.agentID,
		Profile:      d.profile,
		Timestamp:    time.Now(),
		ExitCode:     exitCode,
		ErrorSnippet: snippet,
		Provider:     provider,
	}
}

// extractErrorSnippet pulls out a meaningful error snippet from stderr.
func extractErrorSnippet(stderr string) string {
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Look for lines that look like error messages
		if strings.Contains(strings.ToLower(line), "error") ||
			strings.Contains(strings.ToLower(line), "rate") ||
			strings.Contains(line, "429") {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}
	// Fallback to first non-empty line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}
	return ""
}

// detectProvider attempts to identify the API provider from the error message.
func detectProvider(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "gpt"):
		return "openai"
	default:
		return ""
	}
}
