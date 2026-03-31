package ingest

import (
	"testing"
)

func TestParseEnvelopeWithSession(t *testing.T) {
	envelope := `{"event_id":"abc123","sent_at":"2024-01-01T00:00:00Z"}
{"type":"session"}
{"sid":"sess-001","status":"ok","init":true,"started":"2024-01-01T00:00:00Z","attrs":{"release":"1.0.0","environment":"production"}}
{"type":"event"}
{"message":"test error","level":"error"}`

	hdr, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if hdr.EventID != "abc123" {
		t.Fatalf("event_id = %q", hdr.EventID)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Type != "session" {
		t.Fatalf("item[0].Type = %q, want session", items[0].Type)
	}
	if items[1].Type != "event" {
		t.Fatalf("item[1].Type = %q, want event", items[1].Type)
	}
}

func TestParseEnvelopeWithSessions(t *testing.T) {
	envelope := `{"sent_at":"2024-01-01T00:00:00Z"}
{"type":"sessions"}
{"aggregates":[{"started":"2024-01-01T00:00:00Z","exited":100,"errored":5,"crashed":1}],"attrs":{"release":"1.0.0","environment":"production"}}`

	_, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Type != "sessions" {
		t.Fatalf("item[0].Type = %q, want sessions", items[0].Type)
	}
}

func TestParseEnvelopeWithClientReport(t *testing.T) {
	// client_report should be parsed but silently dropped by handler.
	envelope := `{"sent_at":"2024-01-01T00:00:00Z"}
{"type":"client_report"}
{"timestamp":"2024-01-01T00:00:00Z","discarded_events":[{"reason":"sample_rate","category":"error","quantity":5}]}`

	_, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Type != "client_report" {
		t.Fatalf("item[0].Type = %q, want client_report", items[0].Type)
	}
}

func TestParseEnvelopeMixedItems(t *testing.T) {
	// Real-world envelope: session + event + client_report in one envelope.
	envelope := `{"event_id":"mixed-001","sent_at":"2024-01-01T00:00:00Z"}
{"type":"session"}
{"sid":"sess-mixed","status":"ok","init":true,"started":"2024-01-01T00:00:00Z"}
{"type":"event"}
{"message":"something broke","level":"error"}
{"type":"client_report"}
{"timestamp":"2024-01-01T00:00:00Z","discarded_events":[]}`

	_, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	types := make(map[string]bool)
	for _, item := range items {
		types[item.Type] = true
	}
	for _, want := range []string{"session", "event", "client_report"} {
		if !types[want] {
			t.Fatalf("missing item type %q", want)
		}
	}
}
