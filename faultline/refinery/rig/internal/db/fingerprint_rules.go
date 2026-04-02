package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// FingerprintRule defines a custom fingerprint matching rule for a project.
type FingerprintRule struct {
	ID          string    `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	MatchType   string    `json:"match_type"`  // exception_type, message, module, tag
	Pattern     string    `json:"pattern"`     // regex pattern
	Fingerprint string    `json:"fingerprint"` // fingerprint hash to assign on match
	Priority    int       `json:"priority"`    // higher = checked first
	CreatedAt   time.Time `json:"created_at"`
}

// migrateFingerprintRules creates the fingerprint_rules table.
func (d *DB) migrateFingerprintRules(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS fingerprint_rules (
		id          VARCHAR(36) PRIMARY KEY,
		project_id  BIGINT NOT NULL,
		title       VARCHAR(200) NOT NULL,
		match_type  VARCHAR(20) NOT NULL,
		pattern     VARCHAR(500) NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		priority    INT NOT NULL DEFAULT 0,
		created_at  DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_project (project_id)
	)`)
	return err
}

// ListFingerprintRules returns all fingerprint rules for a project, ordered by priority descending.
func (d *DB) ListFingerprintRules(ctx context.Context, projectID int64) ([]FingerprintRule, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, title, match_type, pattern, fingerprint, priority, created_at
		 FROM fingerprint_rules WHERE project_id = ? ORDER BY priority DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var rules []FingerprintRule
	for rows.Next() {
		var r FingerprintRule
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Title, &r.MatchType, &r.Pattern,
			&r.Fingerprint, &r.Priority, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// CreateFingerprintRule inserts a new fingerprint rule and returns its ID.
func (d *DB) CreateFingerprintRule(ctx context.Context, rule FingerprintRule) (string, error) {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	_, err := d.ExecContext(ctx,
		`INSERT INTO fingerprint_rules (id, project_id, title, match_type, pattern, fingerprint, priority)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.ProjectID, rule.Title, rule.MatchType, rule.Pattern, rule.Fingerprint, rule.Priority)
	if err != nil {
		return "", err
	}
	d.MarkDirty()
	return rule.ID, nil
}

// DeleteFingerprintRule removes a fingerprint rule by project and rule ID.
func (d *DB) DeleteFingerprintRule(ctx context.Context, projectID int64, ruleID string) error {
	res, err := d.ExecContext(ctx,
		`DELETE FROM fingerprint_rules WHERE project_id = ? AND id = ?`, projectID, ruleID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		d.MarkDirty()
	}
	return nil
}
