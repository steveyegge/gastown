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
	ID              string     `json:"id"`
	ProjectID       int64      `json:"project_id"`
	Fingerprint     string     `json:"fingerprint"`
	Title           string     `json:"title"`
	Culprit         string     `json:"culprit"`
	Level           string     `json:"level"`
	Platform        string     `json:"platform"`
	FirstSeen       time.Time  `json:"first_seen"`
	LastSeen        time.Time  `json:"last_seen"`
	EventCount      int        `json:"event_count"`
	Status          string     `json:"status"`
	BeadID          string     `json:"bead_id,omitempty"`
	RegressedAt     *time.Time `json:"regressed_at,omitempty"`
	RegressionCount int        `json:"regression_count"`
	RootCause       string     `json:"root_cause,omitempty"`
	FixExplanation  string     `json:"fix_explanation,omitempty"`
	FixCommit       string     `json:"fix_commit,omitempty"`
	MergedInto      string     `json:"merged_into,omitempty"`
	SnoozedUntil    *time.Time `json:"snoozed_until,omitempty"`
	SnoozeReason    string     `json:"snooze_reason,omitempty"`
	SnoozedBy       string     `json:"snoozed_by,omitempty"`
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
	ProjectID   int64
	Status      string // "unresolved", "resolved", "regressed", "active", "" for all
	Level       string // "error", "fatal", "warning", "" for all
	Environment string // "production", "staging", "" for all
	Platform    string // "go", "python", "cocoa", "" for all
	Sort        string // "last_seen", "first_seen", "event_count", "level"
	Order       string // "asc", "desc"
	Limit       int
	Offset      int
	Query       string // search in title
	Release     string // filter by release version
	Assignee    string // filter by assigned user name, "unassigned" for no assignment
}

// ListIssueGroups returns issue groups matching the given parameters.
func (d *DB) ListIssueGroups(ctx context.Context, p IssueListParams) ([]IssueGroup, int, error) {
	where := []string{"project_id = ?"}
	args := []interface{}{p.ProjectID}

	if p.Status == "active" {
		// "active" is a virtual status combining unresolved + regressed.
		where = append(where, "COALESCE(status, 'unresolved') IN ('unresolved', 'regressed')")
	} else if p.Status == "merged" {
		where = append(where, "COALESCE(status, 'unresolved') = 'merged'")
	} else if p.Status != "" {
		where = append(where, "COALESCE(status, 'unresolved') = ?")
		args = append(args, p.Status)
	}
	// Exclude merged issues from default listing unless explicitly filtering for them.
	if p.Status != "merged" {
		where = append(where, "COALESCE(status, 'unresolved') <> 'merged'")
	}
	if p.Level != "" {
		where = append(where, "level = ?")
		args = append(args, p.Level)
	}
	if p.Query != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+p.Query+"%")
	}
	if p.Environment != "" {
		where = append(where, "id IN (SELECT DISTINCT group_id FROM ft_events WHERE project_id = ? AND environment = ?)")
		args = append(args, p.ProjectID, p.Environment)
	}
	if p.Release != "" {
		where = append(where, "id IN (SELECT DISTINCT group_id FROM ft_events WHERE project_id = ? AND release_name = ?)")
		args = append(args, p.ProjectID, p.Release)
	}
	if p.Platform != "" {
		where = append(where, "COALESCE(platform, '') = ?")
		args = append(args, p.Platform)
	}
	if p.Assignee == "unassigned" {
		where = append(where, "id NOT IN (SELECT group_id FROM issue_assignments WHERE project_id = ?)")
		args = append(args, p.ProjectID)
	} else if p.Assignee != "" {
		where = append(where, "id IN (SELECT group_id FROM issue_assignments WHERE project_id = ? AND assigned_to = ?)")
		args = append(args, p.ProjectID, p.Assignee)
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
	case "severity":
		// Sort by severity: fatal > error > warning > info, then by event count.
		sortCol = "CASE level WHEN 'fatal' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END"
	}
	order := "DESC"
	if p.Order == "asc" {
		order = "ASC"
	}
	// Severity sort is always ascending (0=fatal first) with event_count tiebreaker.
	if p.Sort == "severity" {
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

	orderClause := fmt.Sprintf("%s %s", sortCol, order)
	if p.Sort == "severity" {
		orderClause = fmt.Sprintf("%s %s, event_count DESC", sortCol, order)
	}
	query := fmt.Sprintf(
		"SELECT id, project_id, fingerprint, title, culprit, level, COALESCE(platform, ''), first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, ''), regressed_at, regression_count, COALESCE(root_cause, ''), COALESCE(fix_explanation, ''), COALESCE(fix_commit, ''), COALESCE(merged_into, ''), snoozed_until, COALESCE(snooze_reason, ''), COALESCE(snoozed_by, '') FROM issue_groups WHERE %s ORDER BY %s LIMIT ? OFFSET ?",
		whereClause, orderClause,
	)
	args = append(args, limit, offset)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list issues: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.Platform, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID, &ig.RegressedAt, &ig.RegressionCount, &ig.RootCause, &ig.FixExplanation, &ig.FixCommit, &ig.MergedInto, &ig.SnoozedUntil, &ig.SnoozeReason, &ig.SnoozedBy); err != nil {
			return nil, 0, fmt.Errorf("scan issue: %w", err)
		}
		issues = append(issues, ig)
	}
	return issues, total, rows.Err()
}

// HourlyEventCounts returns event counts per hour for the last N hours for a project.
// Returns a slice of counts ordered oldest-to-newest (index 0 = N hours ago).
func (d *DB) HourlyEventCounts(ctx context.Context, projectID int64, hours int) ([]int, error) {
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	rows, err := d.QueryContext(ctx, `
		SELECT HOUR(timestamp) as h, DATE(timestamp) as d, COUNT(*) as cnt
		FROM ft_events
		WHERE project_id = ? AND timestamp > ?
		GROUP BY d, h
		ORDER BY d, h`,
		projectID, since,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	// Build a map of hour-offset → count.
	now := time.Now().UTC()
	buckets := make(map[int]int)
	for rows.Next() {
		var h, cnt int
		var d string
		if rows.Scan(&h, &d, &cnt) != nil {
			continue
		}
		// Parse date+hour to compute offset from now.
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		t = t.Add(time.Duration(h) * time.Hour)
		offset := int(now.Sub(t).Hours())
		if offset >= 0 && offset < hours {
			buckets[hours-1-offset] = cnt // oldest first
		}
	}

	result := make([]int, hours)
	for i := 0; i < hours; i++ {
		result[i] = buckets[i]
	}
	return result, rows.Err()
}

// ProjectPlatforms returns distinct platform values from issue groups for a project.
func (d *DB) ProjectPlatforms(ctx context.Context, projectID int64) ([]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT platform FROM issue_groups WHERE project_id = ? AND platform IS NOT NULL AND platform <> '' ORDER BY platform`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var plats []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			plats = append(plats, p)
		}
	}
	return plats, rows.Err()
}

// ProjectEnvironments returns distinct environment values from events for a project.
func (d *DB) ProjectEnvironments(ctx context.Context, projectID int64) ([]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT environment FROM ft_events WHERE project_id = ? AND environment <> '' ORDER BY environment`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var envs []string
	for rows.Next() {
		var e string
		if rows.Scan(&e) == nil {
			envs = append(envs, e)
		}
	}
	return envs, rows.Err()
}

// GetIssueGroup returns a single issue group by ID.
func (d *DB) GetIssueGroup(ctx context.Context, projectID int64, issueID string) (*IssueGroup, error) {
	var ig IssueGroup
	err := d.QueryRowContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, COALESCE(platform, ''), first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, ''), regressed_at, regression_count, COALESCE(root_cause, ''), COALESCE(fix_explanation, ''), COALESCE(fix_commit, ''), COALESCE(merged_into, ''), snoozed_until, COALESCE(snooze_reason, ''), COALESCE(snoozed_by, '') FROM issue_groups WHERE project_id = ? AND id = ?",
		projectID, issueID,
	).Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.Platform, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID, &ig.RegressedAt, &ig.RegressionCount, &ig.RootCause, &ig.FixExplanation, &ig.FixCommit, &ig.MergedInto, &ig.SnoozedUntil, &ig.SnoozeReason, &ig.SnoozedBy)
	if err != nil {
		return nil, err
	}
	return &ig, nil
}

// GetIssueGroupByFingerprint returns a single issue group by fingerprint hash.
func (d *DB) GetIssueGroupByFingerprint(ctx context.Context, projectID int64, fingerprint string) (*IssueGroup, error) {
	var ig IssueGroup
	err := d.QueryRowContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, COALESCE(platform, ''), first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, ''), regressed_at, regression_count, COALESCE(root_cause, ''), COALESCE(fix_explanation, ''), COALESCE(fix_commit, ''), COALESCE(merged_into, ''), snoozed_until, COALESCE(snooze_reason, ''), COALESCE(snoozed_by, '') FROM issue_groups WHERE project_id = ? AND fingerprint = ?",
		projectID, fingerprint,
	).Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.Platform, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID, &ig.RegressedAt, &ig.RegressionCount, &ig.RootCause, &ig.FixExplanation, &ig.FixCommit, &ig.MergedInto, &ig.SnoozedUntil, &ig.SnoozeReason, &ig.SnoozedBy)
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
		 FROM ft_events WHERE project_id = ? AND group_id = ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		projectID, groupID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
		 FROM ft_events WHERE project_id = ? AND event_id = ?`,
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

// RecentEvents returns events received after the given timestamp, across all projects.
func (d *DB) RecentEvents(ctx context.Context, since time.Time, limit int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform,
		        COALESCE(environment, ''), COALESCE(release_name, ''), COALESCE(exception_type, ''),
		        raw_json, timestamp, received_at
		 FROM ft_events WHERE timestamp > ? ORDER BY timestamp ASC LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent events: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

// LatestEventForGroup returns the most recent event for an issue group.
func (d *DB) LatestEventForGroup(ctx context.Context, projectID int64, groupID string) (*Event, error) {
	var e Event
	var rawStr string
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, event_id, fingerprint, group_id, level, culprit, message, platform,
		        COALESCE(environment, ''), COALESCE(release_name, ''), COALESCE(exception_type, ''),
		        raw_json, timestamp, received_at
		 FROM ft_events WHERE project_id = ? AND group_id = ? ORDER BY timestamp DESC LIMIT 1`,
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

// ResolveIssueGroup sets an issue group's status to resolved and records resolved_at.
func (d *DB) ResolveIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'resolved', resolved_at = NOW(6) WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UnresolveIssueGroup sets an issue group's status back to unresolved and clears resolved_at.
func (d *DB) UnresolveIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'unresolved', resolved_at = NULL WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// RegressIssueGroup sets an issue group's status to regressed,
// updates regressed_at, and increments regression_count.
func (d *DB) RegressIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'regressed', regressed_at = NOW(6), regression_count = regression_count + 1 WHERE project_id = ? AND id = ?",
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

// UpdateIssueResolution updates the root cause, fix explanation, and fix commit fields.
func (d *DB) UpdateIssueResolution(ctx context.Context, projectID int64, issueID, rootCause, fixExplanation, fixCommit string) error {
	sets := []string{}
	args := []interface{}{}
	if rootCause != "" {
		sets = append(sets, "root_cause = ?")
		args = append(args, rootCause)
	}
	if fixExplanation != "" {
		sets = append(sets, "fix_explanation = ?")
		args = append(args, fixExplanation)
	}
	if fixCommit != "" {
		sets = append(sets, "fix_commit = ?")
		args = append(args, fixCommit)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, projectID, issueID)
	_, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET %s WHERE project_id = ? AND id = ?", strings.Join(sets, ", ")),
		args...,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// BulkResolveIssues sets multiple issue groups to resolved.
func (d *DB) BulkResolveIssues(ctx context.Context, projectID int64, issueIDs []string) (int64, error) {
	if len(issueIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(issueIDs))
	args := make([]interface{}, 0, len(issueIDs)+1)
	args = append(args, projectID)
	for i, id := range issueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET status = 'resolved', resolved_at = NOW(6) WHERE project_id = ? AND id IN (%s)",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk resolve: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n, nil
}

// BulkIgnoreIssues sets multiple issue groups to ignored.
func (d *DB) BulkIgnoreIssues(ctx context.Context, projectID int64, issueIDs []string) (int64, error) {
	if len(issueIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(issueIDs))
	args := make([]interface{}, 0, len(issueIDs)+1)
	args = append(args, projectID)
	for i, id := range issueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET status = 'ignored' WHERE project_id = ? AND id IN (%s)",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk ignore: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n, nil
}

// BulkUnresolveIssues sets multiple issue groups back to unresolved.
func (d *DB) BulkUnresolveIssues(ctx context.Context, projectID int64, issueIDs []string) (int64, error) {
	if len(issueIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(issueIDs))
	args := make([]interface{}, 0, len(issueIDs)+1)
	args = append(args, projectID)
	for i, id := range issueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET status = 'unresolved', resolved_at = NULL WHERE project_id = ? AND id IN (%s)",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk unresolve: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n, nil
}

// migrateIssuePlatform adds the platform column to issue_groups.
func (d *DB) migrateIssuePlatform(ctx context.Context) error {
	var dummy interface{}
	err := d.QueryRowContext(ctx, "SELECT platform FROM issue_groups LIMIT 1").Scan(&dummy)
	if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
		if _, err := d.ExecContext(ctx, "ALTER TABLE issue_groups ADD COLUMN platform VARCHAR(64)"); err != nil {
			return err
		}
		// Backfill platform from the first event for each issue group.
		_, _ = d.ExecContext(ctx, `
			UPDATE issue_groups ig
			SET ig.platform = (
				SELECT e.platform FROM ft_events e
				WHERE e.group_id = ig.id AND e.platform IS NOT NULL AND e.platform <> ''
				ORDER BY e.timestamp ASC LIMIT 1
			)
			WHERE ig.platform IS NULL`)
		d.MarkDirty()
	}
	return nil
}

// migrateMergedInto adds the merged_into column to issue_groups.
func (d *DB) migrateMergedInto(ctx context.Context) error {
	var dummy interface{}
	err := d.QueryRowContext(ctx, "SELECT merged_into FROM issue_groups LIMIT 1").Scan(&dummy)
	if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
		if _, err := d.ExecContext(ctx, "ALTER TABLE issue_groups ADD COLUMN merged_into VARCHAR(36)"); err != nil {
			return err
		}
		d.MarkDirty()
	}
	return nil
}

// MergeIssue merges the source issue into the target issue. It:
// 1. Sets the source issue status to "merged" and merged_into to the target ID
// 2. Creates a fingerprint rule so future events with the source fingerprint route to the target
// 3. Adds the source's event count to the target
func (d *DB) MergeIssue(ctx context.Context, projectID int64, sourceID, targetID string) error {
	// Get source and target to validate and read fingerprints.
	source, err := d.GetIssueGroup(ctx, projectID, sourceID)
	if err != nil {
		return fmt.Errorf("get source issue: %w", err)
	}
	target, err := d.GetIssueGroup(ctx, projectID, targetID)
	if err != nil {
		return fmt.Errorf("get target issue: %w", err)
	}
	if source.Status == "merged" {
		return fmt.Errorf("source issue is already merged")
	}
	if target.Status == "merged" {
		return fmt.Errorf("cannot merge into an issue that is itself merged")
	}

	// Create a fingerprint rule: events matching source's fingerprint → target's fingerprint.
	rule := FingerprintRule{
		ProjectID:   projectID,
		Title:       fmt.Sprintf("Merge: %s → %s", source.Title, target.Title),
		MatchType:   "fingerprint_merge",
		Pattern:     source.Fingerprint,
		Fingerprint: target.Fingerprint,
		Priority:    1000, // high priority to take precedence
	}
	if _, err := d.CreateFingerprintRule(ctx, rule); err != nil {
		return fmt.Errorf("create merge rule: %w", err)
	}

	// Set source status to merged and record the target.
	_, err = d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'merged', merged_into = ? WHERE project_id = ? AND id = ?",
		targetID, projectID, sourceID,
	)
	if err != nil {
		return fmt.Errorf("update source status: %w", err)
	}

	// Add source event count to target and update last_seen if needed.
	_, err = d.ExecContext(ctx,
		`UPDATE issue_groups SET
			event_count = event_count + ?,
			last_seen = CASE WHEN ? > last_seen THEN ? ELSE last_seen END,
			first_seen = CASE WHEN ? < first_seen THEN ? ELSE first_seen END
		 WHERE project_id = ? AND id = ?`,
		source.EventCount,
		source.LastSeen, source.LastSeen,
		source.FirstSeen, source.FirstSeen,
		projectID, targetID,
	)
	if err != nil {
		return fmt.Errorf("update target counts: %w", err)
	}

	// Reassign events from source to target group.
	_, err = d.ExecContext(ctx,
		"UPDATE ft_events SET group_id = ? WHERE project_id = ? AND group_id = ?",
		targetID, projectID, sourceID,
	)
	if err != nil {
		return fmt.Errorf("reassign events: %w", err)
	}

	d.MarkDirty()
	return nil
}

// UnmergeIssue reverses a merge: restores source status and removes the fingerprint rule.
func (d *DB) UnmergeIssue(ctx context.Context, projectID int64, sourceID string) error {
	source, err := d.GetIssueGroup(ctx, projectID, sourceID)
	if err != nil {
		return fmt.Errorf("get source issue: %w", err)
	}
	if source.Status != "merged" || source.MergedInto == "" {
		return fmt.Errorf("issue is not merged")
	}

	// Delete the fingerprint rule for this merge (match by pattern = source fingerprint).
	_, err = d.ExecContext(ctx,
		"DELETE FROM fingerprint_rules WHERE project_id = ? AND match_type = 'fingerprint_merge' AND pattern = ?",
		projectID, source.Fingerprint,
	)
	if err != nil {
		return fmt.Errorf("delete merge rule: %w", err)
	}

	// Reassign events back to source group.
	_, err = d.ExecContext(ctx,
		"UPDATE ft_events SET group_id = ? WHERE project_id = ? AND group_id = ? AND fingerprint = ?",
		sourceID, projectID, source.MergedInto, source.Fingerprint,
	)
	if err != nil {
		return fmt.Errorf("reassign events back: %w", err)
	}

	// Recount events for both groups.
	for _, id := range []string{sourceID, source.MergedInto} {
		_, _ = d.ExecContext(ctx,
			`UPDATE issue_groups SET
				event_count = (SELECT COUNT(*) FROM ft_events WHERE group_id = ?),
				first_seen = COALESCE((SELECT MIN(timestamp) FROM ft_events WHERE group_id = ?), first_seen),
				last_seen = COALESCE((SELECT MAX(timestamp) FROM ft_events WHERE group_id = ?), last_seen)
			 WHERE project_id = ? AND id = ?`,
			id, id, id, projectID, id,
		)
	}

	// Restore source status.
	_, err = d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'unresolved', merged_into = NULL WHERE project_id = ? AND id = ?",
		projectID, sourceID,
	)
	if err != nil {
		return fmt.Errorf("restore source status: %w", err)
	}

	d.MarkDirty()
	return nil
}

// ListMergedInto returns all issues merged into the given target issue.
func (d *DB) ListMergedInto(ctx context.Context, projectID int64, targetID string) ([]IssueGroup, error) {
	rows, err := d.QueryContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, COALESCE(platform, ''), first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, ''), regressed_at, regression_count, COALESCE(root_cause, ''), COALESCE(fix_explanation, ''), COALESCE(fix_commit, ''), COALESCE(merged_into, ''), snoozed_until, COALESCE(snooze_reason, ''), COALESCE(snoozed_by, '') FROM issue_groups WHERE project_id = ? AND merged_into = ?",
		projectID, targetID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.Platform, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID, &ig.RegressedAt, &ig.RegressionCount, &ig.RootCause, &ig.FixExplanation, &ig.FixCommit, &ig.MergedInto, &ig.SnoozedUntil, &ig.SnoozeReason, &ig.SnoozedBy); err != nil {
			return nil, err
		}
		issues = append(issues, ig)
	}
	return issues, rows.Err()
}

// migrateIssueResolution adds root_cause, fix_explanation, and fix_commit columns to issue_groups.
func (d *DB) migrateIssueResolution(ctx context.Context) error {
	for _, col := range []struct{ name, ddl string }{
		{"root_cause", "ALTER TABLE issue_groups ADD COLUMN root_cause TEXT"},
		{"fix_explanation", "ALTER TABLE issue_groups ADD COLUMN fix_explanation TEXT"},
		{"fix_commit", "ALTER TABLE issue_groups ADD COLUMN fix_commit VARCHAR(256)"},
	} {
		var dummy interface{}
		err := d.QueryRowContext(ctx, "SELECT "+col.name+" FROM issue_groups LIMIT 1").Scan(&dummy)
		if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
			if _, err := d.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
			d.MarkDirty()
		}
	}
	return nil
}

// migrateSnooze adds snoozed_until, snooze_reason, and snoozed_by columns to issue_groups.
func (d *DB) migrateSnooze(ctx context.Context) error {
	for _, col := range []struct{ name, ddl string }{
		{"snoozed_until", "ALTER TABLE issue_groups ADD COLUMN snoozed_until DATETIME(6)"},
		{"snooze_reason", "ALTER TABLE issue_groups ADD COLUMN snooze_reason VARCHAR(500)"},
		{"snoozed_by", "ALTER TABLE issue_groups ADD COLUMN snoozed_by VARCHAR(100)"},
	} {
		var dummy interface{}
		err := d.QueryRowContext(ctx, "SELECT "+col.name+" FROM issue_groups LIMIT 1").Scan(&dummy)
		if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
			if _, err := d.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
			d.MarkDirty()
		}
	}
	return nil
}

// SnoozeIssueGroup sets an issue to snoozed status with a duration.
func (d *DB) SnoozeIssueGroup(ctx context.Context, projectID int64, issueID, reason, snoozedBy string, duration time.Duration) error {
	until := time.Now().UTC().Add(duration)
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'snoozed', snoozed_until = ?, snooze_reason = ?, snoozed_by = ? WHERE project_id = ? AND id = ?",
		until, reason, snoozedBy, projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UnsnoozeIssueGroup clears snooze fields and restores unresolved status.
func (d *DB) UnsnoozeIssueGroup(ctx context.Context, projectID int64, issueID string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE issue_groups SET status = 'unresolved', snoozed_until = NULL, snooze_reason = NULL, snoozed_by = NULL WHERE project_id = ? AND id = ?",
		projectID, issueID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// GetExpiredSnoozedIssues returns issues where snoozed_until has passed.
func (d *DB) GetExpiredSnoozedIssues(ctx context.Context) ([]IssueGroup, error) {
	rows, err := d.QueryContext(ctx,
		"SELECT id, project_id, fingerprint, title, culprit, level, COALESCE(platform, ''), first_seen, last_seen, event_count, COALESCE(status, 'unresolved'), COALESCE(bead_id, ''), regressed_at, regression_count, COALESCE(root_cause, ''), COALESCE(fix_explanation, ''), COALESCE(fix_commit, ''), COALESCE(merged_into, ''), snoozed_until, COALESCE(snooze_reason, ''), COALESCE(snoozed_by, '') FROM issue_groups WHERE status = 'snoozed' AND snoozed_until IS NOT NULL AND snoozed_until <= ?",
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var issues []IssueGroup
	for rows.Next() {
		var ig IssueGroup
		if err := rows.Scan(&ig.ID, &ig.ProjectID, &ig.Fingerprint, &ig.Title, &ig.Culprit, &ig.Level, &ig.Platform, &ig.FirstSeen, &ig.LastSeen, &ig.EventCount, &ig.Status, &ig.BeadID, &ig.RegressedAt, &ig.RegressionCount, &ig.RootCause, &ig.FixExplanation, &ig.FixCommit, &ig.MergedInto, &ig.SnoozedUntil, &ig.SnoozeReason, &ig.SnoozedBy); err != nil {
			return nil, err
		}
		issues = append(issues, ig)
	}
	return issues, rows.Err()
}

// BulkSnoozeIssues sets multiple issue groups to snoozed status.
func (d *DB) BulkSnoozeIssues(ctx context.Context, projectID int64, issueIDs []string, reason, snoozedBy string, duration time.Duration) (int64, error) {
	if len(issueIDs) == 0 {
		return 0, nil
	}
	until := time.Now().UTC().Add(duration)
	placeholders := make([]string, len(issueIDs))
	args := make([]interface{}, 0, len(issueIDs)+5)
	args = append(args, until, reason, snoozedBy, projectID)
	for i, id := range issueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET status = 'snoozed', snoozed_until = ?, snooze_reason = ?, snoozed_by = ? WHERE project_id = ? AND id IN (%s)",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk snooze: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n, nil
}

// BulkUnsnoozeIssues clears snooze fields on multiple issue groups.
func (d *DB) BulkUnsnoozeIssues(ctx context.Context, projectID int64, issueIDs []string) (int64, error) {
	if len(issueIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(issueIDs))
	args := make([]interface{}, 0, len(issueIDs)+1)
	args = append(args, projectID)
	for i, id := range issueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE issue_groups SET status = 'unresolved', snoozed_until = NULL, snooze_reason = NULL, snoozed_by = NULL WHERE project_id = ? AND id IN (%s)",
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk unsnooze: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return n, nil
}
