// Package observe provides runtime signal observation for Gas Town.
//
// It reads from configured sources (log files, metric streams) and converts
// raw lines into events that flow through the standard feed pipeline.
package observe

import (
	"regexp"
	"strings"
)

// Redaction policy levels.
const (
	RedactNone     = "none"
	RedactStandard = "standard"
	RedactStrict   = "strict"
)

// Redactor applies PII redaction based on a policy level.
type Redactor struct {
	policy   string
	patterns []*redactPattern
}

type redactPattern struct {
	re          *regexp.Regexp
	replacement string
}

// Standard patterns: emails, IPv4 addresses, bearer tokens, API keys.
var standardPatterns = []*redactPattern{
	{regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`), "[REDACTED_EMAIL]"},
	{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "[REDACTED_IP]"},
	{regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-._~+/]+=*`), "Bearer [REDACTED_TOKEN]"},
	{regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key)[=:]\s*["']?[a-zA-Z0-9\-._~+/]{16,}["']?`), "[REDACTED_API_KEY]"},
}

// Strict patterns: standard + UUIDs and long numeric sequences.
var strictPatterns = []*redactPattern{
	{regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`), "[REDACTED_UUID]"},
	{regexp.MustCompile(`\b\d{10,}\b`), "[REDACTED_NUM]"},
}

// NewRedactor creates a Redactor for the given policy.
func NewRedactor(policy string) *Redactor {
	r := &Redactor{policy: policy}
	switch strings.ToLower(policy) {
	case RedactStrict:
		r.patterns = append(r.patterns, standardPatterns...)
		r.patterns = append(r.patterns, strictPatterns...)
	case RedactStandard:
		r.patterns = append(r.patterns, standardPatterns...)
	default:
		// "none" or unknown — no patterns
	}
	return r
}

// Redact applies the configured redaction patterns to the input string.
func (r *Redactor) Redact(s string) string {
	for _, p := range r.patterns {
		s = p.re.ReplaceAllString(s, p.replacement)
	}
	return s
}
