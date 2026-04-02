package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// BeadRecord tracks a filed Gas Town bead for an issue group.
type BeadRecord struct {
	GroupID      string     `json:"group_id"`
	ProjectID    int64      `json:"project_id"`
	BeadID       string     `json:"bead_id"`
	Rig          string     `json:"rig"`
	FiledAt      time.Time  `json:"filed_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CommitSHA    string     `json:"commit_sha,omitempty"`
	MergeRef     string     `json:"merge_ref,omitempty"`
	CIVerifiedAt *time.Time `json:"ci_verified_at,omitempty"`
}

// migrateGastown adds the bead tracking table.
func (d *DB) migrateGastown(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS beads (
			group_id     VARCHAR(36) NOT NULL,
			project_id   BIGINT NOT NULL,
			bead_id      VARCHAR(64) NOT NULL,
			rig          VARCHAR(128) NOT NULL,
			filed_at     DATETIME(6) NOT NULL,
			resolved_at  DATETIME(6),
			PRIMARY KEY (group_id, project_id),
			INDEX idx_bead_id (bead_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// migrateBeadsVerification adds verification columns to the beads table.
func (d *DB) migrateBeadsVerification(ctx context.Context) error {
	for _, col := range []struct{ name, ddl string }{
		{"commit_sha", "ALTER TABLE beads ADD COLUMN commit_sha VARCHAR(40)"},
		{"merge_ref", "ALTER TABLE beads ADD COLUMN merge_ref VARCHAR(256)"},
		{"ci_verified_at", "ALTER TABLE beads ADD COLUMN ci_verified_at DATETIME(6)"},
	} {
		var dummy interface{}
		err := d.QueryRowContext(ctx, "SELECT "+col.name+" FROM beads LIMIT 1").Scan(&dummy)
		if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
			if _, err := d.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
			d.MarkDirty()
		}
	}
	return nil
}

// InsertBead records that a Gas Town bead was filed for an issue group.
// Also stores the bead_id on the issue group itself.
func (d *DB) InsertBead(ctx context.Context, groupID string, projectID int64, beadID, rig string) error {
	_, err := d.ExecContext(ctx, `
		INSERT IGNORE INTO beads (group_id, project_id, bead_id, rig, filed_at)
		VALUES (?, ?, ?, ?, ?)`,
		groupID, projectID, beadID, rig, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	// Also store bead_id on the issue group.
	_, _ = d.ExecContext(ctx, `UPDATE issue_groups SET bead_id = ? WHERE id = ? AND project_id = ?`,
		beadID, groupID, projectID)
	d.MarkDirty()
	return nil
}

// GetBeadForGroup returns the bead record for an issue group, if any.
func (d *DB) GetBeadForGroup(ctx context.Context, projectID int64, groupID string) (*BeadRecord, error) {
	var b BeadRecord
	var commitSHA, mergeRef sql.NullString
	err := d.QueryRowContext(ctx,
		`SELECT group_id, project_id, bead_id, rig, filed_at, resolved_at,
		        COALESCE(commit_sha,''), COALESCE(merge_ref,''), ci_verified_at
		 FROM beads WHERE project_id = ? AND group_id = ?`,
		projectID, groupID,
	).Scan(&b.GroupID, &b.ProjectID, &b.BeadID, &b.Rig, &b.FiledAt, &b.ResolvedAt,
		&commitSHA, &mergeRef, &b.CIVerifiedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if commitSHA.Valid {
		b.CommitSHA = commitSHA.String
	}
	if mergeRef.Valid {
		b.MergeRef = mergeRef.String
	}
	return &b, nil
}

// ListOpenBeads returns all beads that haven't been resolved yet.
func (d *DB) ListOpenBeads(ctx context.Context) ([]BeadRecord, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT group_id, project_id, bead_id, rig, filed_at, resolved_at,
		        COALESCE(commit_sha,''), COALESCE(merge_ref,''), ci_verified_at
		 FROM beads WHERE resolved_at IS NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var beads []BeadRecord
	for rows.Next() {
		var b BeadRecord
		if err := rows.Scan(&b.GroupID, &b.ProjectID, &b.BeadID, &b.Rig, &b.FiledAt, &b.ResolvedAt,
			&b.CommitSHA, &b.MergeRef, &b.CIVerifiedAt); err != nil {
			return nil, err
		}
		beads = append(beads, b)
	}
	return beads, rows.Err()
}

// MarkBeadResolved records the bead as resolved and sets resolved_at on the issue group.
func (d *DB) MarkBeadResolved(ctx context.Context, projectID int64, groupID string) error {
	now := time.Now().UTC()
	if _, err := d.ExecContext(ctx,
		`UPDATE beads SET resolved_at = ? WHERE project_id = ? AND group_id = ?`,
		now, projectID, groupID,
	); err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE issue_groups SET status = 'resolved', resolved_at = ? WHERE project_id = ? AND id = ?`,
		now, projectID, groupID,
	); err != nil {
		return err
	}
	d.MarkDirty()
	return nil
}

// ReopenBead clears the resolved_at on both the bead and issue group (regression).
func (d *DB) ReopenBead(ctx context.Context, projectID int64, groupID string) error {
	if _, err := d.ExecContext(ctx,
		`UPDATE beads SET resolved_at = NULL WHERE project_id = ? AND group_id = ?`,
		projectID, groupID,
	); err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE issue_groups SET status = 'unresolved', resolved_at = NULL WHERE project_id = ? AND id = ?`,
		projectID, groupID,
	); err != nil {
		return err
	}
	d.MarkDirty()
	return nil
}

// UpdateBeadID replaces the bead_id for a group (used when a regression bead supersedes the original).
func (d *DB) UpdateBeadID(ctx context.Context, projectID int64, groupID, newBeadID string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE beads SET bead_id = ?, resolved_at = NULL WHERE project_id = ? AND group_id = ?`,
		newBeadID, projectID, groupID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UnbeadedIssues returns issue groups that have no bead and were first seen before the given cutoff.
// Used by the slow-burn sweep to ensure all errors eventually get a bead.
func (d *DB) UnbeadedIssues(ctx context.Context, olderThan time.Time) ([]IssueGroup, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT ig.id, ig.project_id, ig.title, ig.culprit, ig.level, ig.status,
		       ig.first_seen, ig.last_seen, ig.event_count
		FROM issue_groups ig
		LEFT JOIN beads b ON ig.id = b.group_id AND ig.project_id = b.project_id
		WHERE b.group_id IS NULL
		  AND ig.first_seen < ?
		  AND ig.status = 'unresolved'
		ORDER BY ig.first_seen ASC
		LIMIT 50`,
		olderThan,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Title, &ig.Culprit, &ig.Level,
			&ig.Status, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount); err != nil {
			return nil, err
		}
		issues = append(issues, ig)
	}
	return issues, rows.Err()
}

// QuietUnresolvedIssues returns unresolved issues that haven't received events since the cutoff.
// Used to auto-resolve issues where the underlying bug was fixed (no new events = quiet).
func (d *DB) QuietUnresolvedIssues(ctx context.Context, quietSince time.Time) ([]IssueGroup, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, project_id, title, culprit, level, status,
		       first_seen, last_seen, event_count
		FROM issue_groups
		WHERE status IN ('unresolved', 'regressed')
		  AND last_seen < ?
		ORDER BY last_seen ASC
		LIMIT 50`,
		quietSince,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Title, &ig.Culprit, &ig.Level,
			&ig.Status, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount); err != nil {
			return nil, err
		}
		issues = append(issues, ig)
	}
	return issues, rows.Err()
}

// AutoResolveIssue marks an issue as resolved via auto-resolution (quiet period expired).
func (d *DB) AutoResolveIssue(ctx context.Context, projectID int64, groupID string) error {
	now := time.Now().UTC()
	_, err := d.ExecContext(ctx,
		`UPDATE issue_groups SET status = 'resolved', resolved_at = ? WHERE project_id = ? AND id = ?`,
		now, projectID, groupID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UpdateBeadCommitInfo stores commit/merge reference on a bead after the fix is detected.
func (d *DB) UpdateBeadCommitInfo(ctx context.Context, projectID int64, groupID, commitSHA, mergeRef string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE beads SET commit_sha = ?, merge_ref = ? WHERE project_id = ? AND group_id = ?`,
		commitSHA, mergeRef, projectID, groupID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// MarkBeadCIVerified records that CI passed for the fix commit.
func (d *DB) MarkBeadCIVerified(ctx context.Context, projectID int64, groupID string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE beads SET ci_verified_at = ? WHERE project_id = ? AND group_id = ?`,
		time.Now().UTC(), projectID, groupID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// IssueResolvedAt returns the resolved_at time for an issue group, or nil if not resolved.
func (d *DB) IssueResolvedAt(ctx context.Context, projectID int64, groupID string) (*time.Time, error) {
	var t sql.NullTime
	err := d.QueryRowContext(ctx,
		`SELECT resolved_at FROM issue_groups WHERE project_id = ? AND id = ?`,
		projectID, groupID,
	).Scan(&t)
	if err != nil {
		return nil, err
	}
	if t.Valid {
		return &t.Time, nil
	}
	return nil, nil
}
