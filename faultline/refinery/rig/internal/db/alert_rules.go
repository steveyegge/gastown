package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ConditionType describes what triggers an alert rule.
type ConditionType string

const (
	CondThreshold  ConditionType = "threshold"  // N events in M minutes
	CondNewIssue   ConditionType = "new_issue"  // first occurrence of new issue group
	CondRegression ConditionType = "regression" // resolved issue reappears
	CondSlowBurn   ConditionType = "slow_burn"  // unbeaded issue older than N minutes
)

// ActionType describes what happens when a rule fires.
type ActionType string

const (
	ActionBead    ActionType = "bead"    // file a Gas Town bead
	ActionSlack   ActionType = "slack"   // send to Slack webhook
	ActionWebhook ActionType = "webhook" // send to generic webhook
	ActionEmail   ActionType = "email"   // send email (future)
)

// AlertRule represents a configurable alert rule for a project.
type AlertRule struct {
	ID            string        `json:"id"`
	ProjectID     int64         `json:"project_id"`
	Name          string        `json:"name"`
	Enabled       bool          `json:"enabled"`
	ConditionType ConditionType `json:"condition_type"`
	Threshold     int           `json:"threshold"`      // event count for threshold conditions
	WindowMinutes int           `json:"window_minutes"`  // time window for threshold conditions
	LevelFilter   string        `json:"level_filter"`    // "error", "fatal", "any", or "" (any)
	ActionType    ActionType    `json:"action_type"`
	ActionTarget  string        `json:"action_target"`   // webhook URL, email address, or "" for bead
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// migrateAlertRules creates the alert_rules and alert_history tables.
func (d *DB) migrateAlertRules(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id              VARCHAR(36) PRIMARY KEY,
			project_id      BIGINT NOT NULL,
			name            VARCHAR(200) NOT NULL,
			enabled         BOOLEAN NOT NULL DEFAULT TRUE,
			condition_type  VARCHAR(32) NOT NULL,
			threshold       INT NOT NULL DEFAULT 1,
			window_minutes  INT NOT NULL DEFAULT 5,
			level_filter    VARCHAR(32) NOT NULL DEFAULT '',
			action_type     VARCHAR(32) NOT NULL DEFAULT 'bead',
			action_target   VARCHAR(512) NOT NULL DEFAULT '',
			created_at      DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at      DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_rules_project (project_id),
			INDEX idx_rules_enabled (project_id, enabled)
		)`,
		`CREATE TABLE IF NOT EXISTS alert_history (
			id              BIGINT AUTO_INCREMENT PRIMARY KEY,
			project_id      BIGINT NOT NULL,
			group_id        VARCHAR(36) NOT NULL,
			rule_id         VARCHAR(36) NOT NULL,
			rule_name       VARCHAR(200) NOT NULL,
			condition_type  VARCHAR(32) NOT NULL,
			action_type     VARCHAR(32) NOT NULL,
			action_target   VARCHAR(512) NOT NULL DEFAULT '',
			fired_at        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_history_project (project_id),
			INDEX idx_history_time (fired_at),
			INDEX idx_history_rule (rule_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("migrate alert rules: %w", err)
			}
		}
	}
	return nil
}

// ListAlertRules returns all alert rules for a project.
func (d *DB) ListAlertRules(ctx context.Context, projectID int64) ([]AlertRule, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, name, enabled, condition_type, threshold, window_minutes,
		        level_filter, action_type, action_target, created_at, updated_at
		 FROM alert_rules WHERE project_id = ? ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []AlertRule
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &r.Enabled, &r.ConditionType,
			&r.Threshold, &r.WindowMinutes, &r.LevelFilter, &r.ActionType, &r.ActionTarget,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ListEnabledAlertRules returns only enabled rules for a project.
func (d *DB) ListEnabledAlertRules(ctx context.Context, projectID int64) ([]AlertRule, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, name, enabled, condition_type, threshold, window_minutes,
		        level_filter, action_type, action_target, created_at, updated_at
		 FROM alert_rules WHERE project_id = ? AND enabled = TRUE ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled alert rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []AlertRule
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &r.Enabled, &r.ConditionType,
			&r.Threshold, &r.WindowMinutes, &r.LevelFilter, &r.ActionType, &r.ActionTarget,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetAlertRule returns a single alert rule by ID.
func (d *DB) GetAlertRule(ctx context.Context, projectID int64, ruleID string) (*AlertRule, error) {
	var r AlertRule
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, name, enabled, condition_type, threshold, window_minutes,
		        level_filter, action_type, action_target, created_at, updated_at
		 FROM alert_rules WHERE project_id = ? AND id = ?`,
		projectID, ruleID,
	).Scan(&r.ID, &r.ProjectID, &r.Name, &r.Enabled, &r.ConditionType,
		&r.Threshold, &r.WindowMinutes, &r.LevelFilter, &r.ActionType, &r.ActionTarget,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// InsertAlertRule creates a new alert rule.
func (d *DB) InsertAlertRule(ctx context.Context, r *AlertRule) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	_, err := d.ExecContext(ctx, `
		INSERT INTO alert_rules (id, project_id, name, enabled, condition_type, threshold, window_minutes,
		                         level_filter, action_type, action_target, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.Name, r.Enabled, r.ConditionType, r.Threshold, r.WindowMinutes,
		r.LevelFilter, r.ActionType, r.ActionTarget, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert alert rule: %w", err)
	}
	d.MarkDirty()
	return nil
}

// UpdateAlertRule updates an existing alert rule.
func (d *DB) UpdateAlertRule(ctx context.Context, r *AlertRule) error {
	r.UpdatedAt = time.Now().UTC()
	_, err := d.ExecContext(ctx, `
		UPDATE alert_rules SET name=?, enabled=?, condition_type=?, threshold=?, window_minutes=?,
		       level_filter=?, action_type=?, action_target=?, updated_at=?
		WHERE project_id=? AND id=?`,
		r.Name, r.Enabled, r.ConditionType, r.Threshold, r.WindowMinutes,
		r.LevelFilter, r.ActionType, r.ActionTarget, r.UpdatedAt,
		r.ProjectID, r.ID,
	)
	if err != nil {
		return fmt.Errorf("update alert rule: %w", err)
	}
	d.MarkDirty()
	return nil
}

// DeleteAlertRule removes an alert rule.
func (d *DB) DeleteAlertRule(ctx context.Context, projectID int64, ruleID string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM alert_rules WHERE project_id=? AND id=?`,
		projectID, ruleID,
	)
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	d.MarkDirty()
	return nil
}

// InsertAlertHistory records that an alert rule fired.
func (d *DB) InsertAlertHistory(ctx context.Context, projectID int64, groupID, ruleID, ruleName string, condType ConditionType, actType ActionType, actTarget string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO alert_history (project_id, group_id, rule_id, rule_name, condition_type, action_type, action_target, fired_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, groupID, ruleID, ruleName, condType, actType, actTarget, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert alert history: %w", err)
	}
	d.MarkDirty()
	return nil
}

// AlertHistoryEntry represents a fired alert record.
type AlertHistoryEntry struct {
	ID            int64         `json:"id"`
	ProjectID     int64         `json:"project_id"`
	GroupID       string        `json:"group_id"`
	RuleID        string        `json:"rule_id"`
	RuleName      string        `json:"rule_name"`
	ConditionType ConditionType `json:"condition_type"`
	ActionType    ActionType    `json:"action_type"`
	ActionTarget  string        `json:"action_target"`
	FiredAt       time.Time     `json:"fired_at"`
}

// ListAlertHistory returns recent alert history for a project.
func (d *DB) ListAlertHistory(ctx context.Context, projectID int64, limit int) ([]AlertHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, group_id, rule_id, rule_name, condition_type, action_type, action_target, fired_at
		 FROM alert_history WHERE project_id = ? ORDER BY fired_at DESC LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list alert history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []AlertHistoryEntry
	for rows.Next() {
		var e AlertHistoryEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.GroupID, &e.RuleID, &e.RuleName,
			&e.ConditionType, &e.ActionType, &e.ActionTarget, &e.FiredAt); err != nil {
			return nil, fmt.Errorf("scan alert history: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// EnsureDefaultAlertRules creates default rules for a project if none exist.
// Returns the rules (existing or newly created).
func (d *DB) EnsureDefaultAlertRules(ctx context.Context, projectID int64) ([]AlertRule, error) {
	existing, err := d.ListAlertRules(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return existing, nil
	}

	defaults := []AlertRule{
		{
			ProjectID: projectID, Name: "Burst alert (3 errors in 5 min)",
			Enabled: true, ConditionType: CondThreshold,
			Threshold: 3, WindowMinutes: 5, LevelFilter: "error",
			ActionType: ActionBead,
		},
		{
			ProjectID: projectID, Name: "Fatal immediate",
			Enabled: true, ConditionType: CondThreshold,
			Threshold: 1, WindowMinutes: 1, LevelFilter: "fatal",
			ActionType: ActionBead,
		},
		{
			ProjectID: projectID, Name: "Slow burn (1 hour unattended)",
			Enabled: true, ConditionType: CondSlowBurn,
			Threshold: 1, WindowMinutes: 60,
			ActionType: ActionBead,
		},
		{
			ProjectID: projectID, Name: "Regression alert",
			Enabled: true, ConditionType: CondRegression,
			Threshold: 1, WindowMinutes: 0,
			ActionType: ActionBead,
		},
	}

	for i := range defaults {
		if err := d.InsertAlertRule(ctx, &defaults[i]); err != nil {
			return nil, fmt.Errorf("insert default rule: %w", err)
		}
	}
	return defaults, nil
}
