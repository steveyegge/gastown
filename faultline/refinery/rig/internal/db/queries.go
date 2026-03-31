package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IssueGroup represents a row from the issue_groups table.
type IssueGroup struct {
	ID          string    `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Fingerprint string    `json:"fingerprint"`
	Title       string    `json:"title"`
	Culprit     string    `json:"culprit"`
	Level       string    `json:"level"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	EventCount  int       `json:"event_count"`
	Status      string    `json:"status"`
	BeadID      string    `json:"bead_id,omitempty"`
}

// Event represents a row from the events table.
type Event struct {
	ID            string          `json:"id"`
	ProjectID     int64           `json:"project_id"`
	EventID       string          `json:"event_id"`
	Fingerprint   string          `json:"fingerprint"`
	GroupID       string          `json:"group_id"`
	Level         string          `json:"level"`
	Culprit       string          `json:"culprit"`
	Message       string          `json:"message"`
	Platform      string          `json:"platform"`
	Environment   string          `json:"environment"`
	Release       string          `json:"release"`
	ExceptionType string          `json:"exception_type"`
	RawJSON       json.RawMessage `json:"raw_json"`
	Timestamp     time.Time       `json:"timestamp"`
	ReceivedAt    time.Time       `json:"received_at"`
}

// IssueListParams controls filtering, sorting, and pagination for issue queries.
type IssueListParams struct {
	ProjectID int64
	Status    string // "unresolved", "resolved", "regressed", "" for all
	Level     string // "error", "fatal", "warning", "" for all
	Sort      string // "last_seen", "first_seen", "event_count", "level"
	Order     string // "asc", "desc"
	Limit     int
	Offset    int
	Query     string // search in title
}

// ListIssueGroups returns issue groups matching the given parameters.
func (d *DB) ListIssueGroups(ctx context.Context, p IssueListParams) ([]IssueGroup, int, error) {
	where := []string{"project_id = ?"}
	args := []interface{}{p.ProjectID}

	if p.Status != "" {
		where = append(where, "COALESCE(status, 'unresolved') = ?")
		args = append(args, p.Status)
	}
	if p.Level != "" {
		where = append(where, "level = ?")
		args = append(args, p.Level)
	}
	if p.Query != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+p.Query+"%")
	}

	whereClause := strings.Join(where, " AND ")

	// Count total matching.
	var total int
	err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM issue_groups WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count issues: %w", err)
	}

	// Sort.
	sortCol := "last_seen"
	switch p.Sort {
	case "first_seen":
		sortCol = "first_seen"
	case "event_count":
		sortCol = "event_count"
	case "level":
		sortCol = "level"
	}
	order := "DESC"
	if p.Order == "asc" {
		order = "ASC"
	}

	limit := p.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(
		"SELECT id, project_id, fingerprint, title, culprit, level, first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, '') FROM issue_groups WHERE %s ORDER BY %s %s LIMIT ? OFFSET ?",
		whereClause, sortCol, order,
	)
	args = append(args, limit, offset)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID); err != nil {
			return nil, 0, fmt.Errorf("scan issue: %w", err)
		}
		issues = append(issues, ig)
	}
	return issues, total, rows.Err()
}

// GetIssueGroup returns a single issue group by ID.
func (d *DB) GetIssueGroup(ctx context.Context, projectID int64, issueID string) (*IssueGroup, error) {
	var ig IssueGroup
	err := d.QueryRowContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, '') FROM issue_groups WHERE project_id = ? AND id = ?",
		projectID, issueID,
	).Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID)
	if err != nil {
		return nil, err
	}
	return &ig, nil
}

// GetIssueGroupByFingerprint returns a single issue group by fingerprint hash.
func (d *DB) GetIssueGroupByFingerprint(ctx context.Context, projectID int64, fingerprint string) (*IssueGroup, error) {
	var ig IssueGroup
	err := d.QueryRowContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, '') FROM issue_groups WHERE project_id = ? AND fingerprint = ?",
		projectID, fingerprint,
	).Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID)
	if err != nil {
		return nil, err
	}
	return &ig, nil
}

// ListEventsByGroup returns events for a given issue group, newest first.
func (d *DB) ListEventsByGroup(ctx context.Context, projectID int64, groupID string, limit, offset int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform,
		        COALESCE(environment, ''), COALESCE(release_name, ''), COALESCE(exception_type, ''),
		        raw_json, timestamp, received_at
		 FROM events WHERE project_id = ? AND group_id = ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		projectID, groupID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var rawStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EventID, &e.Fingerprint, &e.GroupID,
			&e.Level, &e.Culprit, &e.Message, &e.Platform,
			&e.Environment, &e.Release, &e.ExceptionType,
			&rawStr, &e.Timestamp, &e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.RawJSON = json.RawMessage(rawStr)
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetEvent returns a single event by ID.
func (d *DB) GetEvent(ctx context.Context, projectID int64, eventID string) (*Event, error) {
	var e Event
	var rawStr string
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform,
		        COALESCE(environment, ''), COALESCE(release_name, ''), COALESCE(exception_type, ''),
		        raw_json, timestamp, received_at
		 FROM events WHERE project_id = ? AND event_id = ?`,
		projectID, eventID,
	).Scan(&e.ID, &e.ProjectID, &e.EventID, &e.Fingerprint, &e.GroupID,
		&e.Level, &e.Culprit, &e.Message, &e.Platform,
		&e.Environment, &e.Release, &e.ExceptionType,
		&rawStr, &e.Timestamp, &e.ReceivedAt)
	if err != nil {
		return nil, err
	}
	e.RawJSON = json.RawMessage(rawStr)
	return &e, nil
}

// LatestEventForGroup returns the most recent event for an issue group.
func (d *DB) LatestEventForGroup(ctx context.Context, projectID int64, groupID string) (*Event, error) {
	var e Event
	var rawStr string
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform,
		        COALESCE(environment, ''), COALESCE(release_name, ''), COALESCE(exception_type, ''),
		        raw_json, timestamp, received_at
		 FROM events WHERE project_id = ? AND group_id = ? ORDER BY timestamp DESC LIMIT 1`,
		projectID, groupID,
	).Scan(&e.ID, &e.ProjectID, &e.EventID, &e.Fingerprint, &e.GroupID,
		&e.Level, &e.Culprit, &e.Message, &e.Platform,
		&e.Environment, &e.Release, &e.ExceptionType,
		&rawStr, &e.Timestamp, &e.ReceivedAt)
	if err != nil {
		return nil, err
	}
	e.RawJSON = json.RawMessage(rawStr)
	return &e, nil
}

// ResolveIssueGroup sets an issue group's status to resolved.
func (d *DB) ResolveIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'resolved' WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UnresolveIssueGroup sets an issue group's status back to unresolved.
func (d *DB) UnresolveIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'unresolved' WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// IgnoreIssueGroup sets an issue group's status to ignored.
func (d *DB) IgnoreIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'ignored' WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}
