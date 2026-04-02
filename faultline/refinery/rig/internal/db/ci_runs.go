package db

import (
	"context"
	"time"
)

// CIRun represents a CI workflow run (success or failure).
type CIRun struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project_id"`
	Repo       string    `json:"repo"`
	Branch     string    `json:"branch"`
	CommitSHA  string    `json:"commit_sha"`
	Workflow   string    `json:"workflow"`
	RunID      int64     `json:"run_id"`
	Conclusion string    `json:"conclusion"` // success, failure
	RunURL     string    `json:"run_url"`
	Actor      string    `json:"actor"`
	Timestamp  time.Time `json:"timestamp"`
}

// migrateCIRuns creates the ci_runs table.
func (d *DB) migrateCIRuns(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ci_runs (
		id         BIGINT AUTO_INCREMENT PRIMARY KEY,
		project_id BIGINT NOT NULL,
		repo       VARCHAR(256) NOT NULL,
		branch     VARCHAR(256) NOT NULL,
		commit_sha VARCHAR(40) NOT NULL,
		workflow   VARCHAR(256),
		run_id     BIGINT NOT NULL,
		conclusion VARCHAR(32) NOT NULL,
		run_url    VARCHAR(512),
		actor      VARCHAR(128),
		timestamp  DATETIME(6) NOT NULL,
		INDEX idx_ci_project_time (project_id, timestamp),
		INDEX idx_ci_commit (commit_sha)
	)`)
	return err
}

// InsertCIRun records a CI workflow run.
func (d *DB) InsertCIRun(ctx context.Context, projectID int64, repo, branch, commitSHA, workflow string, runID int64, runURL, conclusion, actor string, ts time.Time) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO ci_runs (project_id, repo, branch, commit_sha, workflow, run_id, conclusion, run_url, actor, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, repo, branch, commitSHA, workflow, runID, conclusion, runURL, actor, ts,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// LatestGreenCIRun returns the most recent successful CI run for a branch
// after the given time. Returns nil if no green run is found.
func (d *DB) LatestGreenCIRun(ctx context.Context, projectID int64, branch string, since time.Time) (*CIRun, error) {
	var r CIRun
	err := d.QueryRowContext(ctx, `
		SELECT id, project_id, repo, branch, commit_sha, COALESCE(workflow,''), run_id,
		       conclusion, COALESCE(run_url,''), COALESCE(actor,''), timestamp
		FROM ci_runs
		WHERE project_id = ? AND branch = ? AND conclusion = 'success' AND timestamp > ?
		ORDER BY timestamp DESC
		LIMIT 1`,
		projectID, branch, since,
	).Scan(&r.ID, &r.ProjectID, &r.Repo, &r.Branch, &r.CommitSHA, &r.Workflow,
		&r.RunID, &r.Conclusion, &r.RunURL, &r.Actor, &r.Timestamp)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
