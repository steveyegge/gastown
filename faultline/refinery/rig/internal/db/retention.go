package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// RetentionConfig controls data retention policies.
type RetentionConfig struct {
	// EventTTL is how long to keep raw events (default 90 days).
	EventTTL time.Duration
	// SessionTTL is how long to keep session records (default 90 days).
	SessionTTL time.Duration
	// Interval is how often the retention job runs (default 1 hour).
	Interval time.Duration
}

// DefaultRetentionConfig returns sensible defaults.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		EventTTL:   90 * 24 * time.Hour,
		SessionTTL: 90 * 24 * time.Hour,
		Interval:   1 * time.Hour,
	}
}

// RetentionWorker runs periodic purges of expired data.
type RetentionWorker struct {
	db  *DB
	cfg RetentionConfig
	log *slog.Logger
}

// NewRetentionWorker creates a retention worker.
func NewRetentionWorker(d *DB, log *slog.Logger, cfg RetentionConfig) *RetentionWorker {
	return &RetentionWorker{db: d, cfg: cfg, log: log}
}

// Run starts the retention loop and blocks until ctx is cancelled.
func (w *RetentionWorker) Run(ctx context.Context) {
	// Run once on startup then on interval.
	w.purge(ctx)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.purge(ctx)
		}
	}
}

func (w *RetentionWorker) purge(ctx context.Context) {
	eventCutoff := time.Now().UTC().Add(-w.cfg.EventTTL)
	sessionCutoff := time.Now().UTC().Add(-w.cfg.SessionTTL)

	eventsDeleted, err := w.purgeEvents(ctx, eventCutoff)
	if err != nil {
		w.log.Error("retention: purge events", "err", err)
	}

	sessionsDeleted, err := w.purgeSessions(ctx, sessionCutoff)
	if err != nil {
		w.log.Error("retention: purge sessions", "err", err)
	}

	// Purge expired auth sessions.
	authDeleted, err := w.purgeAuthSessions(ctx)
	if err != nil {
		w.log.Error("retention: purge auth sessions", "err", err)
	}

	// Purge revoked API tokens (>90 days old).
	tokensDeleted, err := w.purgeRevokedAPITokens(ctx)
	if err != nil {
		w.log.Error("retention: purge revoked api tokens", "err", err)
	}

	if eventsDeleted > 0 || sessionsDeleted > 0 || authDeleted > 0 || tokensDeleted > 0 {
		w.log.Info("retention purge",
			"events_deleted", eventsDeleted,
			"sessions_deleted", sessionsDeleted,
			"auth_sessions_deleted", authDeleted,
			"api_tokens_deleted", tokensDeleted,
		)
	}
}

// purgeEvents deletes events older than the cutoff. Deletes in batches to
// avoid holding long locks.
func (w *RetentionWorker) purgeEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		res, err := w.db.ExecContext(ctx,
			`DELETE FROM events WHERE received_at < ? LIMIT 1000`, cutoff)
		if err != nil {
			return total, fmt.Errorf("delete events: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n > 0 {
			w.db.MarkDirty()
		}
		if n < 1000 {
			break
		}
	}
	return total, nil
}

// purgeSessions deletes SDK sessions older than the cutoff.
func (w *RetentionWorker) purgeSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		res, err := w.db.ExecContext(ctx,
			`DELETE FROM sessions WHERE updated_at < ? LIMIT 1000`, cutoff)
		if err != nil {
			return total, fmt.Errorf("delete sessions: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n > 0 {
			w.db.MarkDirty()
		}
		if n < 1000 {
			break
		}
	}
	return total, nil
}

// purgeAuthSessions removes expired dashboard login tokens.
// The dashboard auth sessions are stored in the auth_sessions table
// (distinct from SDK sessions in the sessions table).
func (w *RetentionWorker) purgeAuthSessions(ctx context.Context) (int64, error) {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		// Table may not exist yet if dashboard auth hasn't been initialized.
		// Silently return 0 rather than logging an error every cycle.
		return 0, nil
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}
