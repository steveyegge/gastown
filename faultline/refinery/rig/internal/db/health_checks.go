package db

import (
	"context"
	"time"
)

// HealthCheckResult holds the latest health check for a project.
type HealthCheckResult struct {
	ProjectID  int64     `json:"project_id"`
	Up         bool      `json:"up"`
	ResponseMS int       `json:"response_ms"`
	StatusCode int       `json:"status_code"`
	CheckedAt  time.Time `json:"checked_at"`
}

// migrateHealthChecks creates the health_checks table.
func (d *DB) migrateHealthChecks(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS health_checks (
		id          BIGINT AUTO_INCREMENT PRIMARY KEY,
		project_id  BIGINT NOT NULL,
		up          BOOLEAN NOT NULL,
		response_ms INT NOT NULL DEFAULT 0,
		status_code INT NOT NULL DEFAULT 0,
		checked_at  DATETIME(6) NOT NULL,
		INDEX idx_hc_project_time (project_id, checked_at)
	)`)
	return err
}

// LatestHealthCheck returns the most recent health check for a project.
func (d *DB) LatestHealthCheck(ctx context.Context, projectID int64) (*HealthCheckResult, error) {
	var r HealthCheckResult
	err := d.QueryRowContext(ctx, `
		SELECT project_id, up, response_ms, status_code, checked_at
		FROM health_checks
		WHERE project_id = ?
		ORDER BY checked_at DESC
		LIMIT 1`,
		projectID,
	).Scan(&r.ProjectID, &r.Up, &r.ResponseMS, &r.StatusCode, &r.CheckedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// LatestHealthChecks returns the most recent health check for each given project ID.
func (d *DB) LatestHealthChecks(ctx context.Context, projectIDs []int64) (map[int64]*HealthCheckResult, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	result := make(map[int64]*HealthCheckResult, len(projectIDs))
	for _, pid := range projectIDs {
		r, err := d.LatestHealthCheck(ctx, pid)
		if err == nil {
			result[pid] = r
		}
	}
	return result, nil
}

// UptimeSince returns the percentage of checks that were "up" since the given time.
func (d *DB) UptimeSince(ctx context.Context, projectID int64, since time.Time) (float64, int, error) {
	var total, upCount int
	err := d.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN up THEN 1 ELSE 0 END), 0)
		FROM health_checks
		WHERE project_id = ? AND checked_at > ?`,
		projectID, since,
	).Scan(&total, &upCount)
	if err != nil || total == 0 {
		return 0, 0, err
	}
	return float64(upCount) / float64(total) * 100, total, nil
}
