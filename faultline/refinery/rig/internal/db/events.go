package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// InsertEvent stores a raw event in the events table. Returns true if inserted,
// false if the (project_id, event_id) already exists (idempotent).
func (d *DB) InsertEvent(ctx context.Context, eventID string, projectID int64, fingerprint, groupID string, ts time.Time, platform, level, culprit, message, environment, release, exceptionType string, raw json.RawMessage) (bool, error) {
	id := uuid.New().String()
	res, err := d.ExecContext(ctx, `
		INSERT IGNORE INTO ft_events (id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform, environment, release_name, exception_type, raw_json, timestamp, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, eventID, fingerprint, groupID, level, culprit, message, platform, environment, release, exceptionType, string(raw), ts, time.Now().UTC(),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n > 0, nil
}

// EventExists checks if an event_id is already stored for a project.
func (d *DB) EventExists(ctx context.Context, projectID int64, eventID string) (bool, error) {
	var x int
	err := d.QueryRowContext(ctx, `SELECT 1 FROM ft_events WHERE project_id = ? AND event_id = ?`, projectID, eventID).Scan(&x)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
