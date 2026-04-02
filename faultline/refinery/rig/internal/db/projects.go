package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// DeploymentType describes where a project runs.
type DeploymentType string

const (
	DeployLocal  DeploymentType = "local"  // runs locally, can be health-checked
	DeployRemote DeploymentType = "remote" // mobile/desktop app, reports via relay
	DeployHosted DeploymentType = "hosted" // cloud-hosted service
)

// WebhookType describes the format used for webhook notifications.
type WebhookType string

const (
	WebhookSlack   WebhookType = "slack"   // Slack Block Kit format
	WebhookDiscord WebhookType = "discord" // Discord (Slack-compatible via /slack suffix)
	WebhookGeneric WebhookType = "generic" // Plain JSON POST
)

// WebhookTemplate holds a named webhook payload template for a specific event type.
type WebhookTemplate struct {
	ID        string `json:"id"`                  // unique identifier
	Name      string `json:"name"`                // display name (e.g. "Jira Bug Report")
	EventType string `json:"event_type"`          // "new_issue", "resolved", "regression", or "*" for all
	Body      string `json:"body"`                // template body with {{variable}} placeholders
	IsDefault bool   `json:"is_default,omitempty"` // true for built-in templates
}

// ProjectConfig holds optional metadata about a project's infrastructure.
type ProjectConfig struct {
	DeploymentType   DeploymentType    `json:"deployment_type,omitempty"`   // local, remote, hosted
	Environments     []string          `json:"environments,omitempty"`      // e.g. ["staging", "production"]
	Components       []string          `json:"components,omitempty"`        // e.g. ["web", "macos", "api", "database"]
	ParentProject    int64             `json:"parent_project,omitempty"`    // for monorepo sub-projects
	Description      string            `json:"description,omitempty"`       // short project description (<40 chars)
	URL              string            `json:"url,omitempty"`               // project web URL (e.g. http://localhost:3000)
	WebhookURL       string            `json:"webhook_url,omitempty"`       // notification webhook URL
	WebhookType      WebhookType       `json:"webhook_type,omitempty"`      // slack, discord, generic
	WebhookTemplates []WebhookTemplate `json:"webhook_templates,omitempty"` // custom payload templates
}

// Project represents a row from the projects table.
type Project struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	DSNPublicKey string         `json:"dsn_public_key,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Config       *ProjectConfig `json:"config,omitempty"`
}

// migrateProjectConfig adds the config and last_heartbeat columns to the projects table.
func (d *DB) migrateProjectConfig(ctx context.Context) error {
	for _, col := range []struct{ name, ddl string }{
		{"config", "ALTER TABLE projects ADD COLUMN config JSON"},
		{"last_heartbeat", "ALTER TABLE projects ADD COLUMN last_heartbeat DATETIME(6)"},
	} {
		var dummy string
		err := d.QueryRowContext(ctx, "SELECT "+col.name+" FROM projects LIMIT 1").Scan(&dummy)
		if err != nil && (strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "Unknown column")) {
			if _, err := d.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
			d.MarkDirty()
		}
	}
	return nil
}

// ListProjects returns all projects.
func (d *DB) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, name, slug, dsn_public_key, created_at, config FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var projects []Project
	for rows.Next() {
		var p Project
		var cfgJSON sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.DSNPublicKey, &p.CreatedAt, &cfgJSON); err != nil {
			return nil, err
		}
		if cfgJSON.Valid && cfgJSON.String != "" {
			var cfg ProjectConfig
			if json.Unmarshal([]byte(cfgJSON.String), &cfg) == nil {
				p.Config = &cfg
			}
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProjectConfig sets the config JSON for a project.
func (d *DB) UpdateProjectConfig(ctx context.Context, projectID int64, cfg *ProjectConfig) error {
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, `UPDATE projects SET config = ? WHERE id = ?`, string(cfgJSON), projectID)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// ProjectStats holds issue counts for a project.
type ProjectStats struct {
	TotalIssues      int        `json:"total_issues"`
	UnresolvedIssues int        `json:"unresolved_issues"`
	TotalEvents      int        `json:"total_events"`
	UnbeadedCount    int        `json:"unbeaded_count"`
	PrimaryPlatform  string     `json:"primary_platform"`
	Platforms        []string   `json:"platforms"` // all detected platforms
	LastSeen         *time.Time `json:"last_seen"` // most recent event or issue activity
}

// GetProjectStats returns issue/event counts for a project.
func (d *DB) GetProjectStats(ctx context.Context, projectID int64) (*ProjectStats, error) {
	var s ProjectStats
	_ = d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM issue_groups WHERE project_id = ?`, projectID,
	).Scan(&s.TotalIssues)
	_ = d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM issue_groups WHERE project_id = ? AND status IN ('unresolved', 'regressed')`, projectID,
	).Scan(&s.UnresolvedIssues)
	_ = d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ft_events WHERE project_id = ?`, projectID,
	).Scan(&s.TotalEvents)
	// Count unresolved issues with no bead filed.
	_ = d.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM issue_groups ig
		LEFT JOIN beads b ON ig.id = b.group_id AND ig.project_id = b.project_id
		WHERE ig.project_id = ? AND ig.status IN ('unresolved', 'regressed') AND b.group_id IS NULL`,
		projectID,
	).Scan(&s.UnbeadedCount)
	// Get the most common platform from recent events.
	var platform *string
	_ = d.QueryRowContext(ctx, `
		SELECT platform FROM ft_events WHERE project_id = ? AND platform != ''
		GROUP BY platform ORDER BY COUNT(*) DESC LIMIT 1`,
		projectID,
	).Scan(&platform)
	if platform != nil {
		s.PrimaryPlatform = *platform
	}
	// Get last activity timestamp (most recent of: event, issue update, heartbeat).
	var lastSeen *time.Time
	var ts1, ts2, ts3 sql.NullTime
	_ = d.QueryRowContext(ctx,
		`SELECT MAX(last_seen) FROM issue_groups WHERE project_id = ?`, projectID,
	).Scan(&ts1)
	_ = d.QueryRowContext(ctx,
		`SELECT MAX(timestamp) FROM ft_events WHERE project_id = ?`, projectID,
	).Scan(&ts2)
	_ = d.QueryRowContext(ctx,
		`SELECT last_heartbeat FROM projects WHERE id = ?`, projectID,
	).Scan(&ts3)
	for _, ts := range []sql.NullTime{ts1, ts2, ts3} {
		if ts.Valid && (lastSeen == nil || ts.Time.After(*lastSeen)) {
			t := ts.Time
			lastSeen = &t
		}
	}
	s.LastSeen = lastSeen

	// Get all detected platforms.
	platRows, err := d.QueryContext(ctx, `
		SELECT DISTINCT platform FROM ft_events WHERE project_id = ? AND platform != ''
		ORDER BY platform`,
		projectID,
	)
	if err == nil {
		defer func() { _ = platRows.Close() }()
		for platRows.Next() {
			var p string
			if platRows.Scan(&p) == nil {
				s.Platforms = append(s.Platforms, p)
			}
		}
	}
	return &s, nil
}

// EnsureProject inserts a project if it doesn't exist, or updates the name/key if it does.
// Used at startup to seed projects from FAULTLINE_PROJECTS config.
func (d *DB) EnsureProject(ctx context.Context, id int64, name, slug, publicKey string) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, dsn_public_key)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name = VALUES(name), slug = VALUES(slug), dsn_public_key = VALUES(dsn_public_key)`,
		id, name, slug, publicKey,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// RecordHeartbeat updates the last_heartbeat timestamp for a project.
func (d *DB) RecordHeartbeat(ctx context.Context, projectID int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE projects SET last_heartbeat = ? WHERE id = ?`,
		time.Now().UTC(), projectID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// DeleteProject removes a project and all its associated data.
func (d *DB) DeleteProject(ctx context.Context, projectID int64) error {
	// Delete dependent data first, then the project itself.
	for _, q := range []string{
		`DELETE FROM ft_events WHERE project_id = ?`,
		`DELETE FROM ft_error_lifecycle WHERE project_id = ?`,
		`DELETE FROM health_checks WHERE project_id = ?`,
		`DELETE FROM ci_runs WHERE project_id = ?`,
		`DELETE FROM fingerprint_rules WHERE project_id = ?`,
		`DELETE FROM sessions WHERE project_id = ?`,
		`DELETE FROM beads WHERE project_id = ?`,
		`DELETE FROM issue_groups WHERE project_id = ?`,
		`DELETE FROM projects WHERE id = ?`,
	} {
		if _, err := d.ExecContext(ctx, q, projectID); err != nil {
			// Table may not exist yet — skip gracefully.
			if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "doesn't exist") {
				return err
			}
		}
	}
	d.MarkDirty()
	return nil
}

// DeleteAllProjects removes all projects and their associated data.
func (d *DB) DeleteAllProjects(ctx context.Context) (int, error) {
	projects, err := d.ListProjects(ctx)
	if err != nil {
		return 0, err
	}
	for _, p := range projects {
		if err := d.DeleteProject(ctx, p.ID); err != nil {
			return 0, err
		}
	}
	return len(projects), nil
}

// GetProject returns a single project by ID.
func (d *DB) GetProject(ctx context.Context, projectID int64) (*Project, error) {
	var p Project
	var cfgJSON sql.NullString
	err := d.QueryRowContext(ctx,
		`SELECT id, name, slug, dsn_public_key, created_at, config FROM projects WHERE id = ?`,
		projectID,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.DSNPublicKey, &p.CreatedAt, &cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfgJSON.Valid && cfgJSON.String != "" {
		var cfg ProjectConfig
		if json.Unmarshal([]byte(cfgJSON.String), &cfg) == nil {
			p.Config = &cfg
		}
	}
	return &p, nil
}
