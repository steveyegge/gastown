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

	// Purge old health checks (>7 days).
	healthDeleted, err := w.purgeHealthChecks(ctx)
	if err != nil {
		w.log.Error("retention: purge health checks", "err", err)
	}

	// Purge old CI runs (>90 days).
	ciDeleted, err := w.purgeCIRuns(ctx)
	if err != nil {
		w.log.Error("retention: purge ci runs", "err", err)
	}

	if eventsDeleted > 0 || sessionsDeleted > 0 || authDeleted > 0 || tokensDeleted > 0 || healthDeleted > 0 || ciDeleted > 0 {
		w.log.Info("retention purge",
			"events_deleted", eventsDeleted,
			"sessions_deleted", sessionsDeleted,
			"auth_sessions_deleted", authDeleted,
			"api_tokens_deleted", tokensDeleted,
			"health_checks_deleted", healthDeleted,
			"ci_runs_deleted", ciDeleted,
		)
	}
}

func (w *RetentionWorker) purgeHealthChecks(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM health_checks WHERE checked_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete health checks: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}

func (w *RetentionWorker) purgeCIRuns(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM ci_runs WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete ci runs: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}

// purgeEvents deletes events older than the cutoff.
// Dolt has a column-resolution bug where unqualified column references in
// DELETE statements sometimes fail with "column could not be found in any
// table in scope" (Error 1105). Fully qualifying the column as
// table.column works around this.
func (w *RetentionWorker) purgeEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM ft_events WHERE ft_events.received_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete events: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}

// purgeSessions deletes SDK sessions older than the cutoff.
func (w *RetentionWorker) purgeSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	// Dolt workaround: fully qualify columns to avoid resolution bugs.
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE sessions.updated_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}

// purgeAuthSessions removes expired dashboard login tokens.
// The dashboard auth sessions are stored in the auth_sessions table
// (distinct from SDK sessions in the sessions table).
func (w *RetentionWorker) purgeAuthSessions(ctx context.Context) (int64, error) { //nolint:unparam // error is always nil but kept for interface consistency
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE auth_sessions.expires_at < ?`, time.Now().UTC())
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
