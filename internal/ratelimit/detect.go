package ratelimit

import (
	"regexp"
	"strings"
)

// RateLimitIndicator describes a detected rate limit condition.
type RateLimitIndicator struct {
	// Detected is true if a rate limit was detected.
	Detected bool

	// Provider is the inferred provider from the error message.
	Provider string

	// StatusCode is the HTTP status code if found (0 if not).
	StatusCode int

	// IsGlobal indicates if this appears to be a global rate limit
	// (affects all accounts) vs account-specific.
	IsGlobal bool

	// Message is the matched error message.
	Message string

	// RetryAfter is the suggested retry delay in seconds, if found.
	RetryAfter int
}

// Patterns for detecting rate limits from different providers.
var (
	// HTTP 429 patterns (includes overloaded which indicates capacity limits)
	pattern429        = regexp.MustCompile(`(?i)(429|rate.?limit(ed)?|too.?many.?requests|quota.?exceeded|throttl(ed|ing)|overloaded|at.?capacity)`)
	patternRetryAfter = regexp.MustCompile(`(?i)retry.?after[:\s]+(\d+)`)

	// Anthropic-specific patterns
	patternAnthropic       = regexp.MustCompile(`(?i)(anthropic|claude|overloaded|rate_limit_error)`)
	patternAnthropicGlobal = regexp.MustCompile(`(?i)(service.?overloaded|capacity|all.?servers|global.?rate)`)

	// OpenAI-specific patterns
	patternOpenAI = regexp.MustCompile(`(?i)(openai|gpt|rate.?limit.?reached|tokens?.?per.?minute)`)

	// Generic stuck patterns (no output for extended time)
	patternStuck = regexp.MustCompile(`(?i)(stuck|no.?response|timeout|hung|unresponsive)`)
)

// DetectRateLimit analyzes output text for rate limit indicators.
// This is the primary detection mechanism for rate limiting.
func DetectRateLimit(output string) *RateLimitIndicator {
	indicator := &RateLimitIndicator{}

	// Check for 429 / rate limit patterns
	if !pattern429.MatchString(output) {
		return indicator
	}

	indicator.Detected = true
	indicator.Message = truncateMessage(output, 200)

	// Detect provider
	if patternAnthropic.MatchString(output) {
		indicator.Provider = "anthropic"
		indicator.IsGlobal = patternAnthropicGlobal.MatchString(output)
	} else if patternOpenAI.MatchString(output) {
		indicator.Provider = "openai"
	}

	// Extract status code if present
	if strings.Contains(output, "429") {
		indicator.StatusCode = 429
	}

	// Extract retry-after if present
	if matches := patternRetryAfter.FindStringSubmatch(output); len(matches) > 1 {
		// Parse retry-after value
		var retryAfter int
		if _, err := parseIntSafe(matches[1], &retryAfter); err == nil {
			indicator.RetryAfter = retryAfter
		}
	}

	return indicator
}

// DetectStuck analyzes output for stuck/unresponsive patterns.
// This is a secondary detection mechanism for agents that aren't making progress.
func DetectStuck(output string, lastActivityMinutes int) *RateLimitIndicator {
	indicator := &RateLimitIndicator{}

	// Check for explicit stuck messages
	if patternStuck.MatchString(output) {
		indicator.Detected = true
		indicator.Message = "explicit stuck indicator detected"
		return indicator
	}

	// Behavior-based detection: no productive output for extended time
	// Consider stuck if no activity for more than 15 minutes
	if lastActivityMinutes > 15 {
		// Look for signs of recent API errors in output
		if pattern429.MatchString(output) {
			indicator.Detected = true
			indicator.Message = "no activity with recent rate limit errors"
			return indicator
		}
	}

	return indicator
}

// AnalyzeExitCode checks if an exit code indicates rate limiting.
// Some runtimes exit with specific codes for rate limits.
func AnalyzeExitCode(exitCode int) *RateLimitIndicator {
	indicator := &RateLimitIndicator{}

	// Common exit codes that might indicate rate limiting:
	// - 137: Killed (could be due to resource limits)
	// - 124: Timeout
	// - 75: Temporary failure (common for network issues)
	switch exitCode {
	case 75:
		indicator.Detected = true
		indicator.Message = "exit code 75: temporary failure"
	case 124:
		indicator.Detected = true
		indicator.Message = "exit code 124: timeout (possible rate limit retry exhaustion)"
	}

	return indicator
}

// truncateMessage truncates a message to maxLen characters.
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}

// parseIntSafe safely parses an integer from a string.
func parseIntSafe(s string, result *int) (int, error) {
	var val int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int(c-'0')
		} else {
			break
		}
	}
	*result = val
	return val, nil
}
