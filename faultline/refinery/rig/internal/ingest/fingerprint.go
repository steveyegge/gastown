package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/outdoorsea/faultline/internal/db"
)

// FingerprintWithRules checks custom fingerprint rules before falling back to
// the default fingerprint logic. Rules are checked in priority order (highest first).
func FingerprintWithRules(raw json.RawMessage, rules []db.FingerprintRule) string {
	if len(rules) == 0 {
		return Fingerprint(raw)
	}

	// Parse fields needed for rule matching.
	var evt struct {
		Exception *struct {
			Values []struct {
				Type       string `json:"type"`
				Value      string `json:"value"`
				Stacktrace *struct {
					Frames []struct {
						Module string `json:"module"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
		Message  string `json:"message"`
		Logentry *struct {
			Formatted string `json:"formatted"`
			Message   string `json:"message"`
		} `json:"logentry"`
		Tags map[string]string `json:"tags"`
	}
	_ = json.Unmarshal(raw, &evt)

	// Ensure rules are sorted by priority descending.
	sorted := make([]db.FingerprintRule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	// Separate merge rules from matching rules — merge rules act on the computed fingerprint.
	var mergeRules []db.FingerprintRule
	var matchRules []db.FingerprintRule
	for _, rule := range sorted {
		if rule.MatchType == "fingerprint_merge" {
			mergeRules = append(mergeRules, rule)
		} else {
			matchRules = append(matchRules, rule)
		}
	}

	for _, rule := range matchRules {
		var value string
		switch rule.MatchType {
		case "exception_type":
			if evt.Exception != nil && len(evt.Exception.Values) > 0 {
				value = evt.Exception.Values[len(evt.Exception.Values)-1].Type
			}
		case "message":
			value = evt.Message
			if value == "" && evt.Logentry != nil {
				value = evt.Logentry.Formatted
				if value == "" {
					value = evt.Logentry.Message
				}
			}
		case "module":
			if evt.Exception != nil {
				for _, ex := range evt.Exception.Values {
					if ex.Stacktrace != nil && len(ex.Stacktrace.Frames) > 0 {
						// Use the top frame's module.
						value = ex.Stacktrace.Frames[len(ex.Stacktrace.Frames)-1].Module
						break
					}
				}
			}
		case "tag":
			// Pattern format for tags: "tag_name:regex"
			// The tag name is everything before the first colon, regex is the rest.
			if idx := strings.Index(rule.Pattern, ":"); idx > 0 && evt.Tags != nil {
				tagName := rule.Pattern[:idx]
				tagVal, ok := evt.Tags[tagName]
				if !ok {
					continue
				}
				tagPattern := rule.Pattern[idx+1:]
				matched, err := regexp.MatchString(tagPattern, tagVal)
				if err != nil || !matched {
					continue
				}
				return rule.Fingerprint
			}
			continue
		default:
			continue
		}

		if value == "" {
			continue
		}

		matched, err := regexp.MatchString(rule.Pattern, value)
		if err != nil || !matched {
			continue
		}
		return rule.Fingerprint
	}

	// No matching rule found — compute default fingerprint, then check merge rules.
	fp := Fingerprint(raw)
	for _, rule := range mergeRules {
		if rule.Pattern == fp {
			return rule.Fingerprint
		}
	}
	return fp
}

// Fingerprint computes a SHA256 group hash from a Sentry event.
// Priority:
//  1. If the SDK sent a "fingerprint" field, join with "|" and hash.
//  2. Else: exception type + top 5 stack frames (module.function).
//  3. Fallback: hash the message.
func Fingerprint(raw json.RawMessage) string {
	var evt struct {
		Fingerprint []string `json:"fingerprint"`
		Exception   *struct {
			Values []struct {
				Type       string `json:"type"`
				Value      string `json:"value"`
				Stacktrace *struct {
					Frames []struct {
						Module   string `json:"module"`
						Function string `json:"function"`
						Filename string `json:"filename"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
		Message  string `json:"message"`
		Logentry *struct {
			Formatted string `json:"formatted"`
			Message   string `json:"message"`
		} `json:"logentry"`
	}
	if err := json.Unmarshal(raw, &evt); err != nil {
		return hashString("unparseable")
	}

	// 1. SDK-provided fingerprint.
	if len(evt.Fingerprint) > 0 {
		return hashString(strings.Join(evt.Fingerprint, "|"))
	}

	// 2. Exception-based fingerprint.
	if evt.Exception != nil && len(evt.Exception.Values) > 0 {
		var parts []string
		for _, ex := range evt.Exception.Values {
			parts = append(parts, ex.Type)
			if ex.Stacktrace != nil {
				frames := ex.Stacktrace.Frames
				// Sentry frames are bottom-up; take top 5 (last 5).
				start := 0
				if len(frames) > 5 {
					start = len(frames) - 5
				}
				for _, f := range frames[start:] {
					mod := f.Module
					if mod == "" {
						mod = f.Filename
					}
					parts = append(parts, mod+"."+f.Function)
				}
			}
		}
		if len(parts) > 0 {
			return hashString(strings.Join(parts, "|"))
		}
	}

	// 3. Message fallback.
	msg := evt.Message
	if msg == "" && evt.Logentry != nil {
		msg = evt.Logentry.Formatted
		if msg == "" {
			msg = evt.Logentry.Message
		}
	}
	if msg != "" {
		return hashString(msg)
	}

	return hashString("unknown")
}

// IssueTitle extracts a human-readable title from the event.
func IssueTitle(raw json.RawMessage) string {
	var evt struct {
		Exception *struct {
			Values []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"values"`
		} `json:"exception"`
		Message  string `json:"message"`
		Logentry *struct {
			Formatted string `json:"formatted"`
			Message   string `json:"message"`
		} `json:"logentry"`
	}
	if err := json.Unmarshal(raw, &evt); err != nil {
		return "Unparseable event"
	}
	if evt.Exception != nil && len(evt.Exception.Values) > 0 {
		ex := evt.Exception.Values[len(evt.Exception.Values)-1]
		if ex.Value != "" {
			return truncate(ex.Type + ": " + ex.Value)
		}
		return truncate(ex.Type)
	}
	if evt.Message != "" {
		return truncate(evt.Message)
	}
	if evt.Logentry != nil {
		msg := evt.Logentry.Formatted
		if msg == "" {
			msg = evt.Logentry.Message
		}
		return truncate(msg)
	}
	return "Unknown event"
}

// IssueCulprit extracts the culprit (top frame) from the event.
func IssueCulprit(raw json.RawMessage) string {
	var evt struct {
		Culprit   string `json:"culprit"`
		Exception *struct {
			Values []struct {
				Stacktrace *struct {
					Frames []struct {
						Module   string `json:"module"`
						Function string `json:"function"`
						Filename string `json:"filename"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
	}
	if err := json.Unmarshal(raw, &evt); err != nil {
		return ""
	}
	if evt.Culprit != "" {
		return evt.Culprit
	}
	if evt.Exception != nil {
		for _, ex := range evt.Exception.Values {
			if ex.Stacktrace != nil && len(ex.Stacktrace.Frames) > 0 {
				f := ex.Stacktrace.Frames[len(ex.Stacktrace.Frames)-1]
				mod := f.Module
				if mod == "" {
					mod = f.Filename
				}
				return mod + "." + f.Function
			}
		}
	}
	return ""
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

const maxTitleLen = 200

func truncate(s string) string {
	if len(s) <= maxTitleLen {
		return s
	}
	return s[:maxTitleLen]
}
