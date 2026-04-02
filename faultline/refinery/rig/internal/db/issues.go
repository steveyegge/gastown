package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// UpsertIssueGroup creates or updates an issue group by fingerprint.
// On new group: inserts with event_count=1, returns the new group ID.
// On existing: bumps count + last_seen, returns the existing group ID.
func (d *DB) UpsertIssueGroup(ctx context.Context, fingerprint string, projectID int64, title, culprit, level, platform string, ts time.Time) (groupID string, created bool, err error) {
	newID := uuid.New().String()
	res, err := d.ExecContext(ctx, `
		INSERT INTO issue_groups (id, project_id, fingerprint, title, culprit, level, platform, first_seen, last_seen, event_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE
			last_seen = VALUES(last_seen),
			event_count = event_count + 1`,
		newID, projectID, fingerprint, title, culprit, level, platform, ts, ts,
	)
	if err != nil {
		return "", false, err
	}

	// MySQL: ON DUPLICATE KEY UPDATE returns 2 for updated row, 1 for inserted.
	n, _ := res.RowsAffected()
	d.MarkDirty()

	if n == 1 {
		// New group was inserted with our generated ID.
		return newID, true, nil
	}

	// Existing group was updated — fetch its actual ID.
	err = d.QueryRowContext(ctx,
		"SELECT id FROM issue_groups WHERE project_id = ? AND fingerprint = ?",
		projectID, fingerprint,
	).Scan(&groupID)
	if err == sql.ErrNoRows {
		// Shouldn't happen after successful upsert, but handle gracefully.
		return newID, true, nil
	}
	return groupID, false, err
}
