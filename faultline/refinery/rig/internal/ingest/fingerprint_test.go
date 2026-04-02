package ingest

import (
	"encoding/json"
	"testing"
)

func TestFingerprintSDKProvided(t *testing.T) {
	raw := json.RawMessage(`{"fingerprint":["custom","group"]}`)
	fp := Fingerprint(raw)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}
	// Same input should produce same hash.
	if fp2 := Fingerprint(raw); fp2 != fp {
		t.Fatalf("not deterministic: %q vs %q", fp, fp2)
	}
}

func TestFingerprintException(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "ValueError",
				"value": "invalid literal",
				"stacktrace": {
					"frames": [
						{"module": "main", "function": "run"},
						{"module": "app", "function": "start"},
						{"module": "app.core", "function": "process"},
						{"module": "app.core", "function": "validate"},
						{"module": "app.core", "function": "parse"},
						{"module": "app.core", "function": "convert"}
					]
				}
			}]
		}
	}`)

	fp := Fingerprint(raw)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}

	// Different exception type -> different fingerprint.
	raw2 := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "TypeError",
				"value": "invalid literal",
				"stacktrace": {
					"frames": [
						{"module": "main", "function": "run"},
						{"module": "app", "function": "start"},
						{"module": "app.core", "function": "process"},
						{"module": "app.core", "function": "validate"},
						{"module": "app.core", "function": "parse"},
						{"module": "app.core", "function": "convert"}
					]
				}
			}]
		}
	}`)
	fp2 := Fingerprint(raw2)
	if fp2 == fp {
		t.Fatal("different exception types should produce different fingerprints")
	}
}

func TestFingerprintMessageFallback(t *testing.T) {
	raw := json.RawMessage(`{"message": "something went wrong"}`)
	fp := Fingerprint(raw)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}

	raw2 := json.RawMessage(`{"message": "different error"}`)
	fp2 := Fingerprint(raw2)
	if fp2 == fp {
		t.Fatal("different messages should produce different fingerprints")
	}
}

func TestIssueTitleException(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {"values": [{"type": "RuntimeError", "value": "boom"}]}
	}`)
	title := IssueTitle(raw)
	if title != "RuntimeError: boom" {
		t.Fatalf("title = %q, want RuntimeError: boom", title)
	}
}

func TestIssueTitleMessage(t *testing.T) {
	raw := json.RawMessage(`{"message": "server error"}`)
	title := IssueTitle(raw)
	if title != "server error" {
		t.Fatalf("title = %q", title)
	}
}

func TestIssueCulprit(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {"values": [{"type": "E", "stacktrace": {"frames": [
			{"module": "a", "function": "b"},
			{"module": "x", "function": "y"}
		]}}]}
	}`)
	c := IssueCulprit(raw)
	if c != "x.y" {
		t.Fatalf("culprit = %q, want x.y", c)
	}
}

func TestIssueCulpritField(t *testing.T) {
	raw := json.RawMessage(`{"culprit": "myapp.views.handler"}`)
	c := IssueCulprit(raw)
	if c != "myapp.views.handler" {
		t.Fatalf("culprit = %q", c)
	}
}

// Regression tests for fingerprinting bugs fixed 2026-03-27.

func TestFingerprintSDKExplicitOverridesException(t *testing.T) {
	// When an SDK-provided fingerprint is present, it takes priority over
	// exception-based fingerprinting. CI events use this to separate repos.
	raw1 := json.RawMessage(`{
		"fingerprint": ["ci:outdoorsea/faultline:main:CI"],
		"exception": {"values": [{"type": "CIFailure", "value": "CI failed"}]}
	}`)
	raw2 := json.RawMessage(`{
		"fingerprint": ["ci:outdoorsea/gastown:main:CI"],
		"exception": {"values": [{"type": "CIFailure", "value": "CI failed"}]}
	}`)

	fp1 := Fingerprint(raw1)
	fp2 := Fingerprint(raw2)
	if fp1 == fp2 {
		t.Fatal("different SDK fingerprints should produce different hashes even with same exception type")
	}
}

func TestFingerprintSameExceptionTypeDifferentMessage(t *testing.T) {
	// Two events with the same exception type but no stacktrace should NOT
	// automatically get the same fingerprint — the type alone is used, which
	// WILL be the same. This test documents the behavior: without an explicit
	// fingerprint, exception-type-only events DO group together.
	raw1 := json.RawMessage(`{
		"exception": {"values": [{"type": "faultline.internal", "value": "retention failed"}]}
	}`)
	raw2 := json.RawMessage(`{
		"exception": {"values": [{"type": "faultline.internal", "value": "relay ingest failed"}]}
	}`)

	fp1 := Fingerprint(raw1)
	fp2 := Fingerprint(raw2)
	// Without stacktrace, fingerprint = hash(type) — they WILL be the same.
	// This is why selfmon and CI events MUST set explicit fingerprint fields.
	if fp1 != fp2 {
		t.Fatal("exception-type-only fingerprints should match (the fix is to use explicit fingerprints)")
	}
}

func TestFingerprintExplicitSeparatesSameExceptionType(t *testing.T) {
	// Selfmon events with the same exception type but different messages
	// must get different fingerprints when they include an explicit fingerprint.
	raw1 := json.RawMessage(`{
		"fingerprint": ["faultline.internal", "retention: purge events: delete error"],
		"exception": {"values": [{"type": "faultline.internal", "value": "retention failed"}]}
	}`)
	raw2 := json.RawMessage(`{
		"fingerprint": ["faultline.internal", "relay ingest failed: parse envelope"],
		"exception": {"values": [{"type": "faultline.internal", "value": "relay ingest failed"}]}
	}`)

	fp1 := Fingerprint(raw1)
	fp2 := Fingerprint(raw2)
	if fp1 == fp2 {
		t.Fatal("explicit fingerprints with different messages should produce different hashes")
	}
}

func TestFingerprintTruncateDoesNotPanic(t *testing.T) {
	// Ensure truncate handles edge cases.
	if truncate("") != "" {
		t.Fatal("empty string should return empty")
	}
	short := "hello"
	if truncate(short) != short {
		t.Fatal("short string should not be truncated")
	}
	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	if len(truncate(long)) != maxTitleLen {
		t.Fatalf("long string should be truncated to %d, got %d", maxTitleLen, len(truncate(long)))
	}
}
