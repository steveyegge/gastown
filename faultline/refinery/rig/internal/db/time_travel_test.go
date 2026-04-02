package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func openTimeTravelTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_timetravel_test"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM ft_events")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM issue_groups")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM sessions")
		_, _ = d.ExecContext(context.Background(), "CALL dolt_commit('-Am', 'test cleanup')")
		_ = d.Close()
	})
	return d
}

func TestIssueGroupAsOf_BeforeExists(t *testing.T) {
	d := openTimeTravelTestDB(t)
	ctx := context.Background()

	// Ensure a project exists.
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)

	// Insert an issue group now.
	groupID := fmt.Sprintf("tt-grp-%d", time.Now().UnixNano())
	_, _ = d.ExecContext(ctx, `INSERT INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES (?, 1, ?, 'Time Travel Test', 'error', 'unresolved', NOW(), NOW())`,
		groupID, fmt.Sprintf("fp-tt-%d", time.Now().UnixNano()))

	// Commit so Dolt records the data.
	_, _ = d.ExecContext(ctx, `CALL dolt_commit('-Am', 'time travel test insert')`)

	// Query AS OF a time before the issue existed.
	past := time.Now().UTC().Add(-24 * time.Hour)
	ig, err := d.IssueGroupAsOf(ctx, 1, groupID, past)
	if err != nil {
		t.Fatalf("IssueGroupAsOf: %v", err)
	}
	if ig != nil {
		t.Errorf("expected nil for issue before it existed, got %+v", ig)
	}
}

func TestIssueGroupAsOf_AfterExists(t *testing.T) {
	d := openTimeTravelTestDB(t)
	ctx := context.Background()

	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)

	groupID := fmt.Sprintf("tt-grp2-%d", time.Now().UnixNano())
	fp := fmt.Sprintf("fp-tt2-%d", time.Now().UnixNano())
	_, _ = d.ExecContext(ctx, `INSERT INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES (?, 1, ?, 'Time Travel After', 'error', 'unresolved', NOW(), NOW())`,
		groupID, fp)

	_, _ = d.ExecContext(ctx, `CALL dolt_commit('-Am', 'time travel test after')`)

	// Query AS OF now — should find the issue.
	now := time.Now().UTC().Add(1 * time.Second)
	ig, err := d.IssueGroupAsOf(ctx, 1, groupID, now)
	if err != nil {
		t.Fatalf("IssueGroupAsOf: %v", err)
	}
	if ig == nil {
		t.Fatal("expected issue group, got nil")
	}
	if ig.ID != groupID {
		t.Errorf("ID = %q, want %q", ig.ID, groupID)
	}
	if ig.Title != "Time Travel After" {
		t.Errorf("Title = %q, want 'Time Travel After'", ig.Title)
	}
}

func TestIssueGroupHistory_ReturnsSnapshots(t *testing.T) {
	d := openTimeTravelTestDB(t)
	ctx := context.Background()

	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)

	groupID := fmt.Sprintf("tt-hist-%d", time.Now().UnixNano())
	fp := fmt.Sprintf("fp-hist-%d", time.Now().UnixNano())

	// Insert initial issue.
	_, _ = d.ExecContext(ctx, `INSERT INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES (?, 1, ?, 'History V1', 'error', 'unresolved', NOW(), NOW())`,
		groupID, fp)
	_, _ = d.ExecContext(ctx, `CALL dolt_commit('-Am', 'history test v1')`)

	// Update the issue to create a second commit.
	_, _ = d.ExecContext(ctx, `UPDATE issue_groups SET title = 'History V2' WHERE id = ?`, groupID)
	_, _ = d.ExecContext(ctx, `CALL dolt_commit('-Am', 'history test v2')`)

	snapshots, err := d.IssueGroupHistory(ctx, 1, groupID, 10)
	if err != nil {
		t.Fatalf("IssueGroupHistory: %v", err)
	}
	if len(snapshots) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(snapshots))
	}

	// Most recent should be V2.
	if snapshots[0].Title != "History V2" {
		t.Errorf("snapshots[0].Title = %q, want 'History V2'", snapshots[0].Title)
	}
	// Older should be V1.
	if snapshots[1].Title != "History V1" {
		t.Errorf("snapshots[1].Title = %q, want 'History V1'", snapshots[1].Title)
	}

	// Verify commit metadata is populated.
	if snapshots[0].CommitHash == "" {
		t.Error("expected non-empty commit hash")
	}
	if snapshots[0].Committer == "" {
		t.Error("expected non-empty committer")
	}
	if snapshots[0].CommitDate.IsZero() {
		t.Error("expected non-zero commit date")
	}
}

func TestEventCountAsOf(t *testing.T) {
	d := openTimeTravelTestDB(t)
	ctx := context.Background()

	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)

	groupID := fmt.Sprintf("tt-ec-%d", time.Now().UnixNano())
	fp := fmt.Sprintf("fp-ec-%d", time.Now().UnixNano())

	_, _ = d.ExecContext(ctx, `INSERT INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES (?, 1, ?, 'EventCount Test', 'error', 'unresolved', NOW(), NOW())`,
		groupID, fp)

	// Insert 3 events.
	for i := 0; i < 3; i++ {
		evtID := fmt.Sprintf("tt-evt-%d-%d", time.Now().UnixNano(), i)
		_, _ = d.ExecContext(ctx, `INSERT INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, raw_json, timestamp, received_at)
			VALUES (?, 1, ?, ?, ?, 'error', '{}', NOW(), NOW())`,
			evtID, evtID, fp, groupID)
	}
	_, _ = d.ExecContext(ctx, `CALL dolt_commit('-Am', 'event count test')`)

	// Count AS OF now should be 3.
	now := time.Now().UTC().Add(1 * time.Second)
	count, err := d.EventCountAsOf(ctx, 1, groupID, now)
	if err != nil {
		t.Fatalf("EventCountAsOf: %v", err)
	}
	if count != 3 {
		t.Errorf("EventCountAsOf = %d, want 3", count)
	}

	// Count AS OF past should be 0.
	past := time.Now().UTC().Add(-24 * time.Hour)
	count, err = d.EventCountAsOf(ctx, 1, groupID, past)
	if err != nil {
		t.Fatalf("EventCountAsOf past: %v", err)
	}
	if count != 0 {
		t.Errorf("EventCountAsOf past = %d, want 0", count)
	}
}
