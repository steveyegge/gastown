package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestDefaultRetentionConfig(t *testing.T) {
	cfg := DefaultRetentionConfig()
	if cfg.EventTTL != 90*24*time.Hour {
		t.Errorf("EventTTL = %v, want 90 days", cfg.EventTTL)
	}
	if cfg.SessionTTL != 90*24*time.Hour {
		t.Errorf("SessionTTL = %v, want 90 days", cfg.SessionTTL)
	}
	if cfg.Interval != 1*time.Hour {
		t.Errorf("Interval = %v, want 1 hour", cfg.Interval)
	}
}

func TestRetentionWorkerRunCancellation(t *testing.T) {
	// Verify Run exits promptly when context is cancelled.
	// Requires a real DB because purge calls ExecContext on startup.
	d := openTestDB(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	w := NewRetentionWorker(d, log, RetentionConfig{
		EventTTL:   24 * time.Hour,
		SessionTTL: 24 * time.Hour,
		Interval:   50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — Run exited after ctx cancellation.
	case <-time.After(5 * time.Second):
		t.Fatal("RetentionWorker.Run did not exit after context cancellation")
	}
}

func TestNewRetentionWorker(t *testing.T) {
	d := &DB{}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := DefaultRetentionConfig()
	w := NewRetentionWorker(d, log, cfg)
	if w.db != d {
		t.Error("db not set")
	}
	if w.cfg != cfg {
		t.Error("cfg not set")
	}
}

// --- Integration tests (require Dolt server on localhost:3307) ---

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_retention_test"
	}

	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		// Clean up test data.
		_, _ = d.ExecContext(context.Background(), "DELETE FROM ft_events")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM sessions")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		_ = d.Close()
	})
	return d
}

func TestPurgeEvents_Integration(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Ensure a project exists for FK-free inserts.
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)

	// Ensure an issue group exists for FK-free inserts.
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES ('grp-1', 1, 'fp-1', 'Test Issue', 'error', 'unresolved', NOW(), NOW())`)

	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)

	// Insert old and recent events.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("old-evt-%d", i)
		_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, raw_json, timestamp, received_at)
			VALUES (?, 1, ?, 'fp-1', 'grp-1', 'error', '{}', ?, ?)`,
			id, id, old, old)
	}
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("new-evt-%d", i)
		_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, raw_json, timestamp, received_at)
			VALUES (?, 1, ?, 'fp-1', 'grp-1', 'error', '{}', ?, ?)`,
			id, id, now, now)
	}

	w := NewRetentionWorker(d, log, RetentionConfig{
		EventTTL:   90 * 24 * time.Hour,
		SessionTTL: 90 * 24 * time.Hour,
		Interval:   time.Hour,
	})

	cutoff := now.Add(-90 * 24 * time.Hour)
	deleted, err := w.purgeEvents(ctx, cutoff)
	if err != nil {
		t.Fatalf("purgeEvents: %v", err)
	}
	if deleted != 3 {
		t.Errorf("purgeEvents deleted %d, want 3", deleted)
	}

	// Verify recent events survive.
	var count int
	_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM ft_events").Scan(&count)
	if count != 2 {
		t.Errorf("remaining events = %d, want 2", count)
	}
}

func TestPurgeSessions_Integration(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)

	// Insert old sessions.
	for i := 0; i < 3; i++ {
		sid := fmt.Sprintf("old-sess-%d", i)
		_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO sessions (session_id, project_id, status, started, updated_at)
			VALUES (?, 1, 'ok', ?, ?)`, sid, old, old)
	}
	// Insert recent sessions.
	for i := 0; i < 2; i++ {
		sid := fmt.Sprintf("new-sess-%d", i)
		_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO sessions (session_id, project_id, status, started, updated_at)
			VALUES (?, 1, 'ok', ?, ?)`, sid, now, now)
	}

	w := NewRetentionWorker(d, log, RetentionConfig{
		EventTTL:   90 * 24 * time.Hour,
		SessionTTL: 90 * 24 * time.Hour,
		Interval:   time.Hour,
	})

	cutoff := now.Add(-90 * 24 * time.Hour)
	deleted, err := w.purgeSessions(ctx, cutoff)
	if err != nil {
		t.Fatalf("purgeSessions: %v", err)
	}
	if deleted != 3 {
		t.Errorf("purgeSessions deleted %d, want 3", deleted)
	}

	var count int
	_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE session_id LIKE 'new-sess-%' OR session_id LIKE 'old-sess-%'").Scan(&count)
	if count != 2 {
		t.Errorf("remaining sessions = %d, want 2", count)
	}
}

func TestPurgeAuthSessions_Integration(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// auth_sessions table may not exist — purgeAuthSessions handles this gracefully.
	w := NewRetentionWorker(d, log, DefaultRetentionConfig())

	deleted, err := w.purgeAuthSessions(ctx)
	if err != nil {
		t.Fatalf("purgeAuthSessions: %v", err)
	}
	// With no auth_sessions table or no expired rows, expect 0.
	if deleted != 0 {
		t.Errorf("purgeAuthSessions deleted %d, want 0", deleted)
	}
}

func TestPurgeEvents_BatchDeletion(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES ('grp-batch', 1, 'fp-batch', 'Batch Test', 'error', 'unresolved', NOW(), NOW())`)

	old := time.Now().UTC().Add(-200 * 24 * time.Hour)

	// Insert >1000 events to test batch deletion logic.
	for i := 0; i < 1050; i++ {
		id := fmt.Sprintf("batch-evt-%04d", i)
		raw, _ := json.Marshal(map[string]string{"i": fmt.Sprintf("%d", i)})
		_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, raw_json, timestamp, received_at)
			VALUES (?, 1, ?, 'fp-batch', 'grp-batch', 'error', ?, ?, ?)`,
			id, id, string(raw), old, old)
	}

	w := NewRetentionWorker(d, log, RetentionConfig{
		EventTTL:   90 * 24 * time.Hour,
		SessionTTL: 90 * 24 * time.Hour,
		Interval:   time.Hour,
	})

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	deleted, err := w.purgeEvents(ctx, cutoff)
	if err != nil {
		t.Fatalf("purgeEvents batch: %v", err)
	}
	if deleted != 1050 {
		t.Errorf("purgeEvents batch deleted %d, want 1050", deleted)
	}
}

func TestRetentionWorkerPurge_Integration(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO projects (id, name, slug, dsn_public_key) VALUES (1, 'test', 'test', 'testkey000')`)
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO issue_groups (id, project_id, fingerprint, title, level, status, first_seen, last_seen)
		VALUES ('grp-purge', 1, 'fp-purge', 'Purge Test', 'error', 'unresolved', NOW(), NOW())`)

	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)

	// Insert old event and session.
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, raw_json, timestamp, received_at)
		VALUES ('purge-evt-1', 1, 'purge-evt-1', 'fp-purge', 'grp-purge', 'error', '{}', ?, ?)`, old, old)
	_, _ = d.ExecContext(ctx, `INSERT IGNORE INTO sessions (session_id, project_id, status, started, updated_at)
		VALUES ('purge-sess-1', 1, 'ok', ?, ?)`, old, old)

	w := NewRetentionWorker(d, log, RetentionConfig{
		EventTTL:   90 * 24 * time.Hour,
		SessionTTL: 90 * 24 * time.Hour,
		Interval:   time.Hour,
	})

	// Call purge directly (the method called by Run).
	w.purge(ctx)

	var eventCount, sessionCount int
	_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM ft_events WHERE id = 'purge-evt-1'").Scan(&eventCount)
	_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE session_id = 'purge-sess-1'").Scan(&sessionCount)

	if eventCount != 0 {
		t.Errorf("old event not purged")
	}
	if sessionCount != 0 {
		t.Errorf("old session not purged")
	}
}
