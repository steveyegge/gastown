package db

import (
	"context"
	"time"
)

// UpsertSession creates or updates a session record.
// Sentry sessions have a lifecycle: init → ok/errored/crashed/abnormal.
func (d *DB) UpsertSession(ctx context.Context, sessionID string, projectID int64, distinctID, status string, errors int, started time.Time, duration float64, release, environment, userAgent string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO sessions (session_id, project_id, distinct_id, status, errors, started, duration, release_name, environment, user_agent, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			errors = errors + VALUES(errors),
			duration = COALESCE(VALUES(duration), duration),
			updated_at = VALUES(updated_at)`,
		sessionID, projectID, distinctID, status, errors, started, duration, release, environment, userAgent, time.Now().UTC(),
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}
