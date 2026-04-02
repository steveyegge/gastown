package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// LifecycleEventType enumerates the stages of an error's lifecycle.
type LifecycleEventType string

const (
	LifecycleDetection      LifecycleEventType = "detection"
	LifecycleBeadFiled      LifecycleEventType = "bead_filed"
	LifecycleSlowBurnBead   LifecycleEventType = "slow_burn_bead"
	LifecycleDispatched     LifecycleEventType = "dispatched"
	LifecycleNotified       LifecycleEventType = "notified"
	LifecycleRegression     LifecycleEventType = "regression"
	LifecycleResolved       LifecycleEventType = "resolved"
	LifecycleEscalation     LifecycleEventType = "escalation"
	LifecycleFixFailed      LifecycleEventType = "fix_failed"
	LifecycleCommitDetected LifecycleEventType = "commit_detected"
	LifecycleCIGreen        LifecycleEventType = "ci_green"
	LifecycleIgnored        LifecycleEventType = "ignored"
	LifecycleAssigned       LifecycleEventType = "assigned"
	LifecycleSnoozed        LifecycleEventType = "snoozed"
	LifecycleUnsnoozed      LifecycleEventType = "unsnoozed"
)

// LifecycleEntry represents a single audit log entry for an error lifecycle event.
type LifecycleEntry struct {
	ID        int64              `json:"id"`
	ProjectID int64              `json:"project_id"`
	GroupID   string             `json:"group_id"`
	EventType LifecycleEventType `json:"event_type"`
	BeadID    *string            `json:"bead_id,omitempty"`
	Rig       *string            `json:"rig,omitempty"`
	Context   json.RawMessage    `json:"context,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}

// migrateAuditLog creates the ft_error_lifecycle table.
func (d *DB) migrateAuditLog(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ft_error_lifecycle (
		id          BIGINT AUTO_INCREMENT PRIMARY KEY,
		project_id  BIGINT NOT NULL,
		group_id    VARCHAR(36) NOT NULL,
		event_type  VARCHAR(32) NOT NULL,
		bead_id     VARCHAR(64),
		rig         VARCHAR(128),
		context     JSON,
		timestamp   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_lifecycle_group (project_id, group_id),
		INDEX idx_lifecycle_type (event_type),
		INDEX idx_lifecycle_time (timestamp)
	)`)
	return err
}

// InsertLifecycleEvent records a lifecycle event in the audit log.
func (d *DB) InsertLifecycleEvent(ctx context.Context, projectID int64, groupID string, eventType LifecycleEventType, beadID, rig *string, eventCtx map[string]interface{}) error {
	var ctxJSON []byte
	if eventCtx != nil {
		var err error
		ctxJSON, err = json.Marshal(eventCtx)
		if err != nil {
			ctxJSON = nil
		}
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO ft_error_lifecycle (project_id, group_id, event_type, bead_id, rig, context, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, groupID, string(eventType), beadID, rig, ctxJSON, time.Now().UTC(),
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// ListLifecycleEvents returns the audit log entries for an issue group, newest first.
func (d *DB) ListLifecycleEvents(ctx context.Context, projectID int64, groupID string, limit, offset int) ([]LifecycleEntry, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, project_id, group_id, event_type, bead_id, rig, context, timestamp
		FROM ft_error_lifecycle
		WHERE project_id = ? AND group_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		projectID, groupID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []LifecycleEntry
	for rows.Next() {
		var e LifecycleEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.GroupID, &e.EventType, &e.BeadID, &e.Rig, &e.Context, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// LatestLifecycleByGroups returns the most recent lifecycle event for each of the given group IDs.
// This is used to show a compact lifecycle stage on the issue list page.
func (d *DB) LatestLifecycleByGroups(ctx context.Context, projectID int64, groupIDs []string) (map[string]LifecycleEntry, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(groupIDs))
	args := make([]interface{}, 0, len(groupIDs)+1)
	args = append(args, projectID)
	for i, id := range groupIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT l.id, l.project_id, l.group_id, l.event_type, l.bead_id, l.rig, l.context, l.timestamp
		FROM ft_error_lifecycle l
		INNER JOIN (
			SELECT group_id, MAX(timestamp) AS max_ts
			FROM ft_error_lifecycle
			WHERE project_id = ? AND group_id IN (%s)
			GROUP BY group_id
		) latest ON l.group_id = latest.group_id AND l.timestamp = latest.max_ts
		WHERE l.project_id = ?`,
		strings.Join(placeholders, ","),
	)
	args = append(args, projectID)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]LifecycleEntry, len(groupIDs))
	for rows.Next() {
		var e LifecycleEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.GroupID, &e.EventType, &e.BeadID, &e.Rig, &e.Context, &e.Timestamp); err != nil {
			return nil, err
		}
		result[e.GroupID] = e
	}
	return result, rows.Err()
}

// ListLifecycleEventsByProject returns recent lifecycle events across all issues in a project.
func (d *DB) ListLifecycleEventsByProject(ctx context.Context, projectID int64, limit, offset int) ([]LifecycleEntry, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, project_id, group_id, event_type, bead_id, rig, context, timestamp
		FROM ft_error_lifecycle
		WHERE project_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []LifecycleEntry
	for rows.Next() {
		var e LifecycleEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.GroupID, &e.EventType, &e.BeadID, &e.Rig, &e.Context, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
