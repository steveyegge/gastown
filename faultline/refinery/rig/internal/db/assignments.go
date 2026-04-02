package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// IssueAssignment represents a row from the issue_assignments table.
type IssueAssignment struct {
	GroupID    string    `json:"group_id"`
	ProjectID int64     `json:"project_id"`
	AssignedTo string   `json:"assigned_to"`
	AssignedBy string   `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
}

// AssignIssue assigns (or reassigns) an issue group to a user.
func (d *DB) AssignIssue(ctx context.Context, groupID string, projectID int64, assignedTo, assignedBy string) error {
	_, err := d.ExecContext(ctx, `
		REPLACE INTO issue_assignments (group_id, project_id, assigned_to, assigned_by, assigned_at)
		VALUES (?, ?, ?, ?, NOW(6))`,
		groupID, projectID, assignedTo, assignedBy,
	)
	if err != nil {
		return fmt.Errorf("assign issue: %w", err)
	}
	d.MarkDirty()
	return nil
}

// UnassignIssue removes the assignment for an issue group.
func (d *DB) UnassignIssue(ctx context.Context, groupID string, projectID int64) error {
	res, err := d.ExecContext(ctx,
		"DELETE FROM issue_assignments WHERE group_id = ? AND project_id = ?",
		groupID, projectID,
	)
	if err != nil {
		return fmt.Errorf("unassign issue: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return nil
}

// GetIssueAssignment returns the assignment for a single issue group, or nil if unassigned.
func (d *DB) GetIssueAssignment(ctx context.Context, groupID string, projectID int64) (*IssueAssignment, error) {
	var a IssueAssignment
	err := d.QueryRowContext(ctx,
		"SELECT group_id, project_id, assigned_to, assigned_by, assigned_at FROM issue_assignments WHERE group_id = ? AND project_id = ?",
		groupID, projectID,
	).Scan(&a.GroupID, &a.ProjectID, &a.AssignedTo, &a.AssignedBy, &a.AssignedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get issue assignment: %w", err)
	}
	return &a, nil
}

// AssignmentsForIssues returns assignments for a batch of issue group IDs within a project.
// Returns a map from group_id to IssueAssignment.
func (d *DB) AssignmentsForIssues(ctx context.Context, projectID int64, groupIDs []string) (map[string]IssueAssignment, error) {
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

	query := fmt.Sprintf(
		"SELECT group_id, project_id, assigned_to, assigned_by, assigned_at FROM issue_assignments WHERE project_id = ? AND group_id IN (%s)",
		strings.Join(placeholders, ", "),
	)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("assignments for issues: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]IssueAssignment)
	for rows.Next() {
		var a IssueAssignment
		if err := rows.Scan(&a.GroupID, &a.ProjectID, &a.AssignedTo, &a.AssignedBy, &a.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		result[a.GroupID] = a
	}
	return result, rows.Err()
}

// migrateAssignments creates the issue_assignments table if it doesn't exist.
func (d *DB) migrateAssignments(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS issue_assignments (
			group_id    VARCHAR(36) NOT NULL,
			project_id  BIGINT NOT NULL,
			assigned_to VARCHAR(200) NOT NULL,
			assigned_by VARCHAR(200) NOT NULL,
			assigned_at DATETIME(6) NOT NULL,
			PRIMARY KEY (group_id, project_id)
		)`)
	if err != nil {
		return fmt.Errorf("create issue_assignments: %w", err)
	}
	return nil
}
