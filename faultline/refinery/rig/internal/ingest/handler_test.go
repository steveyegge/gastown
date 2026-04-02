package ingest

import (
	"context"
	"encoding/json"
	"fmt"
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
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)
	t.Cleanup(func() {
		_, _ = d.ExecContext(ctx, "DELETE FROM ft_events")
		_, _ = d.ExecContext(ctx, "DELETE FROM issue_groups")
		_ = d.Close()
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
	err = d.QueryRowContext(ctx, `SELECT raw_json FROM ft_events WHERE event_id = ?`, eventID).Scan(&rawJSON)
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
	err = d.QueryRowContext(ctx, `SELECT raw_json FROM ft_events WHERE event_id = ?`, eventID).Scan(&rawJSON)
	if err != nil {
		t.Fatalf("query stored event: %v", err)
	}

	// With ScrubPII=false, email should still be present.
	if !strings.Contains(rawJSON, "user@example.com") {
		t.Error("stored raw_json should contain email when ScrubPII=false")
	}
}

func TestProcessEvent_Regression_Integration(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:  d,
		Log: log,
	}

	// Step 1: Create an issue group by sending an event.
	payload1 := json.RawMessage(`{
		"event_id": "cccc1111dddd2222eeee3333ffff4444",
		"timestamp": 1704067200,
		"platform": "go",
		"level": "error",
		"message": "something broke",
		"exception": {"values": [{"type": "RuntimeError", "value": "regression test"}]}
	}`)
	eventID1 := "cccc1111-dddd-2222-eeee-3333ffff4444"
	if err := h.processEvent(ctx, 1, eventID1, payload1); err != nil {
		t.Fatalf("processEvent #1: %v", err)
	}

	// Find the group that was created.
	var groupID string
	if err := d.QueryRowContext(ctx, `SELECT group_id FROM ft_events WHERE event_id = ?`, eventID1).Scan(&groupID); err != nil {
		t.Fatalf("lookup group_id: %v", err)
	}

	// Step 2: Resolve the issue.
	if err := d.ResolveIssueGroup(ctx, 1, groupID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Confirm it is resolved.
	ig, err := d.GetIssueGroup(ctx, 1, groupID)
	if err != nil {
		t.Fatalf("get issue group: %v", err)
	}
	if ig.Status != "resolved" {
		t.Fatalf("expected status=resolved, got %s", ig.Status)
	}

	// Step 3: Send a second event with the same fingerprint (same exception).
	payload2 := json.RawMessage(`{
		"event_id": "dddd2222eeee3333ffff4444aaaa5555",
		"timestamp": 1704153600,
		"platform": "go",
		"level": "error",
		"message": "something broke again",
		"exception": {"values": [{"type": "RuntimeError", "value": "regression test"}]}
	}`)
	eventID2 := "dddd2222-eeee-3333-ffff-4444aaaa5555"
	if err := h.processEvent(ctx, 1, eventID2, payload2); err != nil {
		t.Fatalf("processEvent #2: %v", err)
	}

	// Step 4: Verify the issue is now regressed.
	ig, err = d.GetIssueGroup(ctx, 1, groupID)
	if err != nil {
		t.Fatalf("get issue group after regression: %v", err)
	}
	if ig.Status != "regressed" {
		t.Fatalf("expected status=regressed, got %s", ig.Status)
	}
	if ig.RegressionCount != 1 {
		t.Fatalf("expected regression_count=1, got %d", ig.RegressionCount)
	}
	if ig.RegressedAt == nil {
		t.Fatal("expected regressed_at to be set")
	}
}

func TestProcessEvent_RegressionCount_Increments_Integration(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:  d,
		Log: log,
	}

	// Create issue.
	payload := json.RawMessage(`{
		"event_id": "eeee1111ffff2222aaaa3333bbbb4444",
		"timestamp": 1704067200,
		"platform": "go",
		"level": "error",
		"message": "count test",
		"exception": {"values": [{"type": "CountError", "value": "count regression"}]}
	}`)
	if err := h.processEvent(ctx, 1, "eeee1111-ffff-2222-aaaa-3333bbbb4444", payload); err != nil {
		t.Fatalf("processEvent #1: %v", err)
	}
	var groupID string
	if err := d.QueryRowContext(ctx, `SELECT group_id FROM ft_events WHERE event_id = ?`, "eeee1111-ffff-2222-aaaa-3333bbbb4444").Scan(&groupID); err != nil {
		t.Fatalf("lookup group_id: %v", err)
	}

	// Resolve -> regress cycle twice.
	for i := 1; i <= 2; i++ {
		if err := d.ResolveIssueGroup(ctx, 1, groupID); err != nil {
			t.Fatalf("resolve #%d: %v", i, err)
		}
		eid := fmt.Sprintf("ffff%04d-aaaa-bbbb-cccc-dddd0000%04d", i, i)
		p := json.RawMessage(fmt.Sprintf(`{
			"event_id": "%s",
			"timestamp": %d,
			"platform": "go",
			"level": "error",
			"message": "count test again",
			"exception": {"values": [{"type": "CountError", "value": "count regression"}]}
		}`, strings.ReplaceAll(eid, "-", ""), 1704067200+i*86400))
		if err := h.processEvent(ctx, 1, eid, p); err != nil {
			t.Fatalf("processEvent regression #%d: %v", i, err)
		}
	}

	ig, err := d.GetIssueGroup(ctx, 1, groupID)
	if err != nil {
		t.Fatalf("get issue group: %v", err)
	}
	if ig.RegressionCount != 2 {
		t.Fatalf("expected regression_count=2, got %d", ig.RegressionCount)
	}
}

func TestProcessEvent_UnresolvedNoRegression_Integration(t *testing.T) {
	d := openHandlerTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := &Handler{
		DB:  d,
		Log: log,
	}

	// Create issue (unresolved by default).
	payload1 := json.RawMessage(`{
		"event_id": "aaaa9999bbbb8888cccc7777dddd6666",
		"timestamp": 1704067200,
		"platform": "go",
		"level": "error",
		"message": "no regression test",
		"exception": {"values": [{"type": "NoRegError", "value": "no regression"}]}
	}`)
	if err := h.processEvent(ctx, 1, "aaaa9999-bbbb-8888-cccc-7777dddd6666", payload1); err != nil {
		t.Fatalf("processEvent #1: %v", err)
	}
	var groupID string
	if err := d.QueryRowContext(ctx, `SELECT group_id FROM ft_events WHERE event_id = ?`, "aaaa9999-bbbb-8888-cccc-7777dddd6666").Scan(&groupID); err != nil {
		t.Fatalf("lookup group_id: %v", err)
	}

	// Send another event to an unresolved group — should NOT trigger regression.
	payload2 := json.RawMessage(`{
		"event_id": "bbbb8888cccc7777dddd6666eeee5555",
		"timestamp": 1704153600,
		"platform": "go",
		"level": "error",
		"message": "no regression test again",
		"exception": {"values": [{"type": "NoRegError", "value": "no regression"}]}
	}`)
	if err := h.processEvent(ctx, 1, "bbbb8888-cccc-7777-dddd-6666eeee5555", payload2); err != nil {
		t.Fatalf("processEvent #2: %v", err)
	}

	ig, err := d.GetIssueGroup(ctx, 1, groupID)
	if err != nil {
		t.Fatalf("get issue group: %v", err)
	}
	if ig.Status != "unresolved" {
		t.Fatalf("expected status=unresolved, got %s", ig.Status)
	}
	if ig.RegressionCount != 0 {
		t.Fatalf("expected regression_count=0, got %d", ig.RegressionCount)
	}
}
