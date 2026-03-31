package db

import (
	"context"
	"database/sql"
	"time"
)

// BeadRecord tracks a filed Gas Town bead for an issue group.
type BeadRecord struct {
	GroupID    string     `json:"group_id"`
	ProjectID  int64     `json:"project_id"`
	BeadID     string    `json:"bead_id"`
	Rig        string    `json:"rig"`
	FiledAt    time.Time `json:"filed_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
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
	d.ExecContext(ctx, `UPDATE issue_groups SET bead_id = ? WHERE id = ? AND project_id = ?`,
		beadID, groupID, projectID)
	d.MarkDirty()
	return nil
}

// GetBeadForGroup returns the bead record for an issue group, if any.
func (d *DB) GetBeadForGroup(ctx context.Context, projectID int64, groupID string) (*BeadRecord, error) {
	var b BeadRecord
	err := d.QueryRowContext(ctx,
		`SELECT group_id, project_id, bead_id, rig, filed_at, resolved_at FROM beads WHERE project_id = ? AND group_id = ?`,
		projectID, groupID,
	).Scan(&b.GroupID, &b.ProjectID, &b.BeadID, &b.Rig, &b.FiledAt, &b.ResolvedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListOpenBeads returns all beads that haven't been resolved yet.
func (d *DB) ListOpenBeads(ctx context.Context) ([]BeadRecord, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT group_id, project_id, bead_id, rig, filed_at, resolved_at FROM beads WHERE resolved_at IS NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var beads []BeadRecord
	for rows.Next() {
		var b BeadRecord
		if err := rows.Scan(&b.GroupID, &b.ProjectID, &b.BeadID, &b.Rig, &b.FiledAt, &b.ResolvedAt); err != nil {
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
