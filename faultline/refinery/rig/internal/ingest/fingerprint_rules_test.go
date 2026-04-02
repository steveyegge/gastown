package ingest

import (
	"encoding/json"
	"testing"

	"github.com/outdoorsea/faultline/internal/db"
)

func TestFingerprintWithRules_ExceptionTypeOverride(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "ValueError",
				"value": "bad input",
				"stacktrace": {
					"frames": [
						{"module": "main", "function": "run"},
						{"module": "app", "function": "start"}
					]
				}
			}]
		}
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "exception_type",
			Pattern:     "ValueError",
			Fingerprint: "custom-fp-for-valueerror",
			Priority:    10,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "custom-fp-for-valueerror" {
		t.Fatalf("expected custom fingerprint, got %q", fp)
	}

	// Verify the default fingerprint is different.
	defaultFP := Fingerprint(raw)
	if defaultFP == "custom-fp-for-valueerror" {
		t.Fatal("default fingerprint should differ from custom rule fingerprint")
	}
}

func TestFingerprintWithRules_PriorityOrder(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "ValueError",
				"value": "bad input"
			}]
		},
		"message": "something went wrong"
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-low",
			ProjectID:   1,
			MatchType:   "exception_type",
			Pattern:     "ValueError",
			Fingerprint: "low-priority-fp",
			Priority:    1,
		},
		{
			ID:          "rule-high",
			ProjectID:   1,
			MatchType:   "exception_type",
			Pattern:     "ValueError",
			Fingerprint: "high-priority-fp",
			Priority:    100,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "high-priority-fp" {
		t.Fatalf("expected high-priority rule to win, got %q", fp)
	}
}

func TestFingerprintWithRules_NoMatchFallthrough(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "TypeError",
				"value": "bad type"
			}]
		}
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "exception_type",
			Pattern:     "ValueError",
			Fingerprint: "custom-fp",
			Priority:    10,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	defaultFP := Fingerprint(raw)
	if fp != defaultFP {
		t.Fatalf("expected fallthrough to default fingerprint %q, got %q", defaultFP, fp)
	}
}

func TestFingerprintWithRules_MessageMatch(t *testing.T) {
	raw := json.RawMessage(`{"message": "connection timeout to database"}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "message",
			Pattern:     "connection timeout",
			Fingerprint: "timeout-group",
			Priority:    5,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "timeout-group" {
		t.Fatalf("expected message rule match, got %q", fp)
	}
}

func TestFingerprintWithRules_ModuleMatch(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "Error",
				"stacktrace": {
					"frames": [
						{"module": "base", "function": "run"},
						{"module": "payments.stripe", "function": "charge"}
					]
				}
			}]
		}
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "module",
			Pattern:     "payments\\.stripe",
			Fingerprint: "stripe-errors",
			Priority:    5,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "stripe-errors" {
		t.Fatalf("expected module rule match, got %q", fp)
	}
}

func TestFingerprintWithRules_TagMatch(t *testing.T) {
	raw := json.RawMessage(`{
		"message": "some error",
		"tags": {"environment": "staging"}
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "tag",
			Pattern:     "environment:staging",
			Fingerprint: "staging-errors",
			Priority:    5,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "staging-errors" {
		t.Fatalf("expected tag rule match, got %q", fp)
	}
}

func TestFingerprintWithRules_EmptyRules(t *testing.T) {
	raw := json.RawMessage(`{"message": "hello"}`)
	fp := FingerprintWithRules(raw, nil)
	defaultFP := Fingerprint(raw)
	if fp != defaultFP {
		t.Fatalf("empty rules should fall through to default: got %q, want %q", fp, defaultFP)
	}
}

func TestFingerprintWithRules_RegexPattern(t *testing.T) {
	raw := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "HttpError_404"
			}]
		}
	}`)

	rules := []db.FingerprintRule{
		{
			ID:          "rule-1",
			ProjectID:   1,
			MatchType:   "exception_type",
			Pattern:     "^HttpError_[0-9]+$",
			Fingerprint: "http-errors",
			Priority:    5,
		},
	}

	fp := FingerprintWithRules(raw, rules)
	if fp != "http-errors" {
		t.Fatalf("expected regex match, got %q", fp)
	}
}
