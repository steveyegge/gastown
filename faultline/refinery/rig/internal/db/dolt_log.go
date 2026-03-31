package db

import (
	"context"
	"fmt"
	"time"
)

// DoltLogEntry represents a single Dolt commit from the log.
type DoltLogEntry struct {
	CommitHash string    `json:"commit_hash"`
	Committer  string    `json:"committer"`
	Message    string    `json:"message"`
	Date       time.Time `json:"date"`
}

// DoltLogForIssue returns Dolt commits that touched a specific issue group.
// It queries the dolt_diff table for changes to the issue_groups row and the
// events that reference this group.
func (d *DB) DoltLogForIssue(ctx context.Context, projectID int64, issueID string, limit int) ([]DoltLogEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Query Dolt commit log for commits that touched either the issue_groups
	// or events tables, filtered to those relevant to this issue group.
	// We use dolt_log() and cross-reference with dolt_diff to find relevant commits.
	query := fmt.Sprintf(`
		SELECT DISTINCT dl.commit_hash, dl.committer, dl.message, dl.date
		FROM dolt_log AS dl
		WHERE dl.commit_hash IN (
			SELECT to_commit
			FROM dolt_diff_issue_groups
			WHERE to_id = ? AND to_project_id = ?
			UNION
			SELECT to_commit
			FROM dolt_diff_events
			WHERE to_group_id = ? AND to_project_id = ?
		)
		ORDER BY dl.date DESC
		LIMIT %d`, limit)

	rows, err := d.QueryContext(ctx, query, issueID, projectID, issueID, projectID)
	if err != nil {
		return nil, fmt.Errorf("dolt log for issue: %w", err)
	}
	defer rows.Close()

	var entries []DoltLogEntry
	for rows.Next() {
		var e DoltLogEntry
		if err := rows.Scan(&e.CommitHash, &e.Committer, &e.Message, &e.Date); err != nil {
			return nil, fmt.Errorf("scan dolt log: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
