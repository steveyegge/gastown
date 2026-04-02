package ingest

import (
	"encoding/json"
	"regexp"
	"strings"
)

// PII patterns to detect and redact.
var (
	emailRe      = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	creditCardRe = regexp.MustCompile(`\b(?:4\d{3}|5[1-5]\d{2}|3[47]\d{2}|6(?:011|5\d{2}))[ \-]?\d{4}[ \-]?\d{4}[ \-]?\d{1,7}\b`)
	ssnRe        = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	bearerRe     = regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-._~+/]+=*`)
	apiKeyRe     = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token)["\s:=]+["']?([a-zA-Z0-9\-._~+/]{16,})["']?`)
	passwordRe   = regexp.MustCompile(`(?i)(?:password|passwd|pwd)["\s:=]+["']?([^\s"',}{]{1,})["']?`)
)

const redacted = "[Filtered]"

// sensitiveKeys are JSON keys whose values should be fully redacted.
var sensitiveKeys = map[string]bool{
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
	"x-api-key":     true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"api_key":       true,
	"private_key":   true,
	"credit_card":   true,
	"card_number":   true,
	"cvv":           true,
	"ssn":           true,
}

// ScrubEvent applies PII scrubbing to a raw event JSON payload.
// It removes sensitive values from known keys and redacts patterns
// (emails, credit cards, SSNs, tokens) in string values.
func ScrubEvent(raw json.RawMessage) json.RawMessage {
	var data interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		// Can't parse — scrub as raw string.
		return json.RawMessage(scrubString(string(raw)))
	}

	scrubbed := scrubValue(data)

	out, err := json.Marshal(scrubbed)
	if err != nil {
		return raw
	}
	return out
}

func scrubValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return scrubMap(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = scrubValue(item)
		}
		return result
	case string:
		return scrubString(val)
	default:
		return v
	}
}

func scrubMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		lower := strings.ToLower(k)
		if sensitiveKeys[lower] {
			result[k] = redacted
			continue
		}
		result[k] = scrubValue(v)
	}
	return result
}

func scrubString(s string) string {
	s = emailRe.ReplaceAllString(s, redacted)
	s = creditCardRe.ReplaceAllStringFunc(s, func(match string) string {
		// Only redact if it has enough digits (Luhn-plausible).
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, match)
		if len(digits) >= 13 {
			return redacted
		}
		return match
	})
	s = ssnRe.ReplaceAllString(s, redacted)
	s = bearerRe.ReplaceAllString(s, "Bearer "+redacted)
	s = apiKeyRe.ReplaceAllString(s, redacted)
	s = passwordRe.ReplaceAllString(s, redacted)
	return s
}
