package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		check func(time.Time) bool
	}{
		{"unix float", float64(1704067200), func(ts time.Time) bool { return ts.Year() == 2024 }},
		{"rfc3339", "2024-01-01T00:00:00Z", func(ts time.Time) bool { return ts.Year() == 2024 }},
		{"iso no tz", "2024-06-15T12:30:00", func(ts time.Time) bool { return ts.Month() == 6 }},
		{"nil fallback", nil, func(ts time.Time) bool { return time.Since(ts) < 5*time.Second }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := parseTimestamp(tt.input)
			if !tt.check(ts) {
				t.Fatalf("parseTimestamp(%v) = %v", tt.input, ts)
			}
		})
	}
}

func TestNormalizeEventID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef1234567890abcdef1234567890", "abcdef12-3456-7890-abcd-ef1234567890"},
		{"abcdef12-3456-7890-abcd-ef1234567890", "abcdef12-3456-7890-abcd-ef1234567890"},
		{"short", "short"},
	}
	for _, tt := range tests {
		got := normalizeEventID(tt.input)
		if got != tt.want {
			t.Fatalf("normalizeEventID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func openHandlerTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_handler_test"
	}
	d, err := db.Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	ctx := context.Background()
	// Seed a project for FK-free inserts.
	d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)
	t.Cleanup(func() {
		d.ExecContext(ctx, "DELETE FROM events")
		d.ExecContext(ctx, "DELETE FROM issue_groups")
		d.Close()
	})
	return d
}

func TestProcessEvent_ScrubPII_Integration(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:       d,
		Log:      log,
		ScrubPII: true,
	}

	// Event payload containing PII.
	payload := json.RawMessage(`{
		"event_id": "aaaa1111bbbb2222cccc3333dddd4444",
		"timestamp": 1704067200,
		"platform": "go",
		"level": "error",
		"message": "contact user@example.com for help",
		"exception": {
			"values": [{"type": "RuntimeError", "value": "SSN is 123-45-6789"}]
		},
		"user": {
			"email": "jane.doe@corp.io",
			"password": "s3cret"
		},
		"request": {
			"headers": {"Authorization": "Bearer eyJhbGciOiJIUzI1NiJ9.dGVzdA.abc123"}
		}
	}`)

	eventID := "aaaa1111-bbbb-2222-cccc-3333dddd4444"
	err := h.processEvent(ctx, 1, eventID, payload)
	if err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	// Read back the stored raw_json.
	var rawJSON string
	err = d.QueryRowContext(ctx, `SELECT raw_json FROM events WHERE event_id = ?`, eventID).Scan(&rawJSON)
	if err != nil {
		t.Fatalf("query stored event: %v", err)
	}

	// Verify PII is scrubbed.
	if strings.Contains(rawJSON, "user@example.com") {
		t.Error("stored raw_json still contains email user@example.com")
	}
	if strings.Contains(rawJSON, "jane.doe@corp.io") {
		t.Error("stored raw_json still contains email jane.doe@corp.io")
	}
	if strings.Contains(rawJSON, "123-45-6789") {
		t.Error("stored raw_json still contains SSN 123-45-6789")
	}
	if strings.Contains(rawJSON, "s3cret") {
		t.Error("stored raw_json still contains password")
	}
	if strings.Contains(rawJSON, "eyJhbGciOiJIUzI1NiJ9") {
		t.Error("stored raw_json still contains bearer token")
	}

	// Verify [Filtered] replacements are present.
	if !strings.Contains(rawJSON, "[Filtered]") {
		t.Error("stored raw_json does not contain [Filtered] markers")
	}
}

func TestProcessEvent_InvalidJSON(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:  d,
		Log: log,
	}

	// Invalid JSON should be rejected, not stored as "Unparseable event".
	payload := json.RawMessage(`{not valid json`)
	err := h.processEvent(ctx, 1, "dead0000-0000-0000-0000-000000000000", payload)
	if err == nil {
		t.Fatal("processEvent should return error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no "Unparseable event" issue group was created.
	var count int
	d.QueryRowContext(ctx, `SELECT COUNT(*) FROM issue_groups WHERE title = 'Unparseable event'`).Scan(&count)
	if count > 0 {
		t.Error("invalid JSON payload should not create an 'Unparseable event' issue group")
	}
}

func TestProcessEvent_NoScrub_Integration(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:       d,
		Log:      log,
		ScrubPII: false,
	}

	payload := json.RawMessage(`{
		"event_id": "bbbb2222cccc3333dddd4444eeee5555",
		"timestamp": 1704067200,
		"platform": "go",
		"level": "error",
		"message": "contact user@example.com for help"
	}`)

	eventID := "bbbb2222-cccc-3333-dddd-4444eeee5555"
	err := h.processEvent(ctx, 1, eventID, payload)
	if err != nil {
		t.Fatalf("processEvent: %v", err)
	}

	var rawJSON string
	err = d.QueryRowContext(ctx, `SELECT raw_json FROM events WHERE event_id = ?`, eventID).Scan(&rawJSON)
	if err != nil {
		t.Fatalf("query stored event: %v", err)
	}

	// With ScrubPII=false, email should still be present.
	if !strings.Contains(rawJSON, "user@example.com") {
		t.Error("stored raw_json should contain email when ScrubPII=false")
	}
}
