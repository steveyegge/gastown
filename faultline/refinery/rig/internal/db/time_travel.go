package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IssueGroupSnapshot represents an issue group at a specific Dolt commit.
type IssueGroupSnapshot struct {
	IssueGroup
	CommitHash string    `json:"commit_hash"`
	Committer  string    `json:"committer"`
	CommitDate time.Time `json:"commit_date"`
	CommitMsg  string    `json:"commit_message"`
}

// IssueGroupAsOf returns the issue group state at a given point in time.
// Returns nil (without error) if the issue did not exist at that time.
func (d *DB) IssueGroupAsOf(ctx context.Context, projectID int64, groupID string, asOf time.Time) (*IssueGroup, error) {
	query := `SELECT id, project_id, fingerprint, title, culprit, level, first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, '')
		FROM issue_groups AS OF ?
		WHERE project_id = ? AND id = ?`

	var ig IssueGroup
	err := d.QueryRowContext(ctx, query, asOf.Format("2006-01-02T15:04:05.000000"), projectID, groupID).
		Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level,
			&ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("issue group as of: %w", err)
	}
	return &ig, nil
}

// IssueGroupHistory returns the commit-level history of changes to an issue group.
// It queries the dolt_history_issue_groups system table joined with dolt_log.
func (d *DB) IssueGroupHistory(ctx context.Context, projectID int64, groupID string, limit int) ([]IssueGroupSnapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT ig.id, ig.project_id, ig.fingerprint, ig.title, COALESCE(ig.culprit, ''), COALESCE(ig.level, ''),
			ig.first_seen, ig.last_seen, ig.event_count, COALESCE(ig.status, 'unresolved'), COALESCE(ig.bead_id, ''),
			ig.commit_hash, h.committer, h.date, h.message
		FROM dolt_history_issue_groups AS ig
		JOIN dolt_log AS h ON ig.commit_hash = h.commit_hash
		WHERE ig.project_id = ? AND ig.id = ?
		ORDER BY h.date DESC
		LIMIT %d`, limit)

	rows, err := d.QueryContext(ctx, query, projectID, groupID)
	if err != nil {
		return nil, fmt.Errorf("issue group history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var snapshots []IssueGroupSnapshot
	for rows.Next() {
		var s IssueGroupSnapshot
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Fingerprint, &s.Title, &s.Culprit, &s.Level,
			&s.FirstSeen, &s.LastSeen, &s.EventCount, &s.Status, &s.BeadID,
			&s.CommitHash, &s.Committer, &s.CommitDate, &s.CommitMsg,
		); err != nil {
			return nil, fmt.Errorf("scan issue group history: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// EventCountAsOf returns how many events existed for a group at a point in time.
func (d *DB) EventCountAsOf(ctx context.Context, projectID int64, groupID string, asOf time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM ft_events AS OF ?
		WHERE project_id = ? AND group_id = ?`

	var count int
	err := d.QueryRowContext(ctx, query, asOf.Format("2006-01-02T15:04:05.000000"), projectID, groupID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("event count as of: %w", err)
	}
	return count, nil
}
