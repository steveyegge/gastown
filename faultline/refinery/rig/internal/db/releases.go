package db

import (
	"context"
	"fmt"
	"time"
)

// Release represents a row from the releases table.
type Release struct {
	ProjectID    int64     `json:"project_id"`
	Version      string    `json:"version"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	EventCount   int       `json:"event_count"`
	SessionCount int       `json:"session_count"`
	CrashFree    float64   `json:"crash_free_rate"`
}

// UpsertRelease creates or updates a release row when an event is ingested.
func (d *DB) UpsertRelease(ctx context.Context, projectID int64, version string, ts time.Time) error {
	if version == "" {
		return nil
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO releases (project_id, version, first_seen, last_seen, event_count)
		VALUES (?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE
			last_seen = GREATEST(last_seen, VALUES(last_seen)),
			event_count = event_count + 1`,
		projectID, version, ts, ts,
	)
	if err != nil {
		return fmt.Errorf("upsert release: %w", err)
	}
	d.MarkDirty()
	return nil
}

// ListReleases returns releases for a project ordered by last_seen desc.
func (d *DB) ListReleases(ctx context.Context, projectID int64, limit int) ([]Release, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := d.QueryContext(ctx,
		`SELECT project_id, version, first_seen, last_seen, event_count, session_count, crash_free_rate
		 FROM releases WHERE project_id = ? ORDER BY last_seen DESC LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var releases []Release
	for rows.Next() {
		var r Release
		if err := rows.Scan(&r.ProjectID, &r.Version, &r.FirstSeen, &r.LastSeen, &r.EventCount, &r.SessionCount, &r.CrashFree); err != nil {
			return nil, fmt.Errorf("scan release: %w", err)
		}
		releases = append(releases, r)
	}
	return releases, rows.Err()
}

// GetRelease returns a single release by project and version.
func (d *DB) GetRelease(ctx context.Context, projectID int64, version string) (*Release, error) {
	var r Release
	err := d.QueryRowContext(ctx,
		`SELECT project_id, version, first_seen, last_seen, event_count, session_count, crash_free_rate
		 FROM releases WHERE project_id = ? AND version = ?`,
		projectID, version,
	).Scan(&r.ProjectID, &r.Version, &r.FirstSeen, &r.LastSeen, &r.EventCount, &r.SessionCount, &r.CrashFree)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// LatestRelease returns the most recently seen release for a project.
func (d *DB) LatestRelease(ctx context.Context, projectID int64) (*Release, error) {
	var r Release
	err := d.QueryRowContext(ctx,
		`SELECT project_id, version, first_seen, last_seen, event_count, session_count, crash_free_rate
		 FROM releases WHERE project_id = ? ORDER BY last_seen DESC LIMIT 1`,
		projectID,
	).Scan(&r.ProjectID, &r.Version, &r.FirstSeen, &r.LastSeen, &r.EventCount, &r.SessionCount, &r.CrashFree)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// FirstReleaseForIssue returns the release version of the earliest event in an issue group.
func (d *DB) FirstReleaseForIssue(ctx context.Context, projectID int64, groupID string) (string, error) {
	var release string
	err := d.QueryRowContext(ctx,
		`SELECT COALESCE(release_name, '') FROM ft_events
		 WHERE project_id = ? AND group_id = ? AND release_name <> ''
		 ORDER BY timestamp ASC LIMIT 1`,
		projectID, groupID,
	).Scan(&release)
	if err != nil {
		return "", err
	}
	return release, nil
}

// LastReleaseForIssue returns the release version of the most recent event in an issue group.
func (d *DB) LastReleaseForIssue(ctx context.Context, projectID int64, groupID string) (string, error) {
	var release string
	err := d.QueryRowContext(ctx,
		`SELECT COALESCE(release_name, '') FROM ft_events
		 WHERE project_id = ? AND group_id = ? AND release_name <> ''
		 ORDER BY timestamp DESC LIMIT 1`,
		projectID, groupID,
	).Scan(&release)
	if err != nil {
		return "", err
	}
	return release, nil
}

// RegisterDeploy records a deploy event for a release.
func (d *DB) RegisterDeploy(ctx context.Context, projectID int64, version, environment, commitSHA string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	now := time.Now().UTC()
	// Ensure the release row exists.
	_, err := d.ExecContext(ctx, `
		INSERT INTO releases (project_id, version, first_seen, last_seen, event_count)
		VALUES (?, ?, ?, ?, 0)
		ON DUPLICATE KEY UPDATE last_seen = GREATEST(last_seen, VALUES(last_seen))`,
		projectID, version, now, now,
	)
	if err != nil {
		return fmt.Errorf("register deploy upsert release: %w", err)
	}
	d.MarkDirty()
	return nil
}
