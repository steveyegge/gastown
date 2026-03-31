package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

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
		Message string `json:"message"`
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
			return ex.Type + ": " + ex.Value
		}
		return ex.Type
	}
	if evt.Message != "" {
		return truncate(evt.Message, 200)
	}
	if evt.Logentry != nil {
		msg := evt.Logentry.Formatted
		if msg == "" {
			msg = evt.Logentry.Message
		}
		return truncate(msg, 200)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
