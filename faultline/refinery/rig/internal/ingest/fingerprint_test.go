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
