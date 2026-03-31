package ingest

import (
	"testing"
)

func TestParseEnvelopeBytes(t *testing.T) {
	envelope := `{"event_id":"abc123","dsn":"https://key@sentry.io/1","sent_at":"2024-01-01T00:00:00Z"}
{"type":"event","length":0}
{"exception":{"values":[{"type":"RuntimeError","value":"boom"}]}}
{"type":"attachment","length":5}
hello`

	hdr, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if hdr.EventID != "abc123" {
		t.Fatalf("event_id = %q, want abc123", hdr.EventID)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Type != "event" {
		t.Fatalf("item[0].Type = %q, want event", items[0].Type)
	}
	if items[1].Type != "attachment" {
		t.Fatalf("item[1].Type = %q, want attachment", items[1].Type)
	}
}

func TestParseEnvelopeWithLength(t *testing.T) {
	payload := `{"message":"hello world"}`
	envelope := `{"event_id":"def456"}
{"type":"event","length":25}
` + payload

	hdr, items, err := parseEnvelopeBytes([]byte(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if hdr.EventID != "def456" {
		t.Fatalf("event_id = %q", hdr.EventID)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if string(items[0].Payload) != payload {
		t.Fatalf("payload = %q, want %q", items[0].Payload, payload)
	}
}

func TestParseEmptyEnvelope(t *testing.T) {
	_, _, err := parseEnvelopeBytes([]byte{})
	if err == nil {
		t.Fatal("expected error for empty envelope")
	}
}

func TestParseDSNProjectID(t *testing.T) {
	tests := []struct {
		dsn  string
		want int64
		err  bool
	}{
		{"https://key@sentry.io/42", 42, false},
		{"https://key@o123.ingest.sentry.io/99", 99, false},
		{"noslash", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseDSNProjectID(tt.dsn)
		if tt.err {
			if err == nil {
				t.Fatalf("ParseDSNProjectID(%q): expected error", tt.dsn)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseDSNProjectID(%q): %v", tt.dsn, err)
		}
		if got != tt.want {
			t.Fatalf("ParseDSNProjectID(%q) = %d, want %d", tt.dsn, got, tt.want)
		}
	}
}
