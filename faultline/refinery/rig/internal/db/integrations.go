package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IntegrationConfig represents a per-project integration configuration.
type IntegrationConfig struct {
	ID              string          `json:"id"`
	ProjectID       int64           `json:"project_id"`
	IntegrationType string          `json:"integration_type"` // github_issues, pagerduty, jira, linear
	Name            string          `json:"name"`
	Enabled         bool            `json:"enabled"`
	Config          json.RawMessage `json:"config"`  // type-specific JSON config
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// migrateIntegrations creates the integrations_config table.
func (d *DB) migrateIntegrations(ctx context.Context) error {
	stmt := `CREATE TABLE IF NOT EXISTS integrations_config (
		id                VARCHAR(36) PRIMARY KEY,
		project_id        BIGINT NOT NULL,
		integration_type  VARCHAR(64) NOT NULL,
		name              VARCHAR(200) NOT NULL,
		enabled           BOOLEAN NOT NULL DEFAULT TRUE,
		config            JSON NOT NULL,
		created_at        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_integ_project (project_id),
		INDEX idx_integ_type (project_id, integration_type),
		UNIQUE KEY uq_integ_project_name (project_id, name)
	)`
	if _, err := d.ExecContext(ctx, stmt); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("migrate integrations: %w", err)
		}
	}
	return nil
}

// ListIntegrations returns all integration configs for a project.
func (d *DB) ListIntegrations(ctx context.Context, projectID int64) ([]IntegrationConfig, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, integration_type, name, enabled, config, created_at, updated_at
		 FROM integrations_config WHERE project_id = ? ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []IntegrationConfig
	for rows.Next() {
		var c IntegrationConfig
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.IntegrationType, &c.Name,
			&c.Enabled, &c.Config, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan integration: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// ListEnabledIntegrations returns only enabled integrations for a project.
func (d *DB) ListEnabledIntegrations(ctx context.Context, projectID int64) ([]IntegrationConfig, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, integration_type, name, enabled, config, created_at, updated_at
		 FROM integrations_config WHERE project_id = ? AND enabled = TRUE ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled integrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []IntegrationConfig
	for rows.Next() {
		var c IntegrationConfig
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.IntegrationType, &c.Name,
			&c.Enabled, &c.Config, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan integration: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// GetIntegration returns a single integration config by ID.
func (d *DB) GetIntegration(ctx context.Context, projectID int64, integrationID string) (*IntegrationConfig, error) {
	var c IntegrationConfig
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, integration_type, name, enabled, config, created_at, updated_at
		 FROM integrations_config WHERE project_id = ? AND id = ?`,
		projectID, integrationID,
	).Scan(&c.ID, &c.ProjectID, &c.IntegrationType, &c.Name,
		&c.Enabled, &c.Config, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("integration not found")
		}
		return nil, err
	}
	return &c, nil
}

// InsertIntegration creates a new integration config.
func (d *DB) InsertIntegration(ctx context.Context, c *IntegrationConfig) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Config == nil {
		c.Config = json.RawMessage(`{}`)
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO integrations_config (id, project_id, integration_type, name, enabled, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.IntegrationType, c.Name, c.Enabled, c.Config, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert integration: %w", err)
	}
	d.MarkDirty()
	return nil
}

// UpdateIntegration updates an existing integration config.
func (d *DB) UpdateIntegration(ctx context.Context, c *IntegrationConfig) error {
	c.UpdatedAt = time.Now().UTC()
	_, err := d.ExecContext(ctx, `
		UPDATE integrations_config SET integration_type=?, name=?, enabled=?, config=?, updated_at=?
		WHERE project_id=? AND id=?`,
		c.IntegrationType, c.Name, c.Enabled, c.Config, c.UpdatedAt,
		c.ProjectID, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update integration: %w", err)
	}
	d.MarkDirty()
	return nil
}

// DeleteIntegration removes an integration config.
func (d *DB) DeleteIntegration(ctx context.Context, projectID int64, integrationID string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM integrations_config WHERE project_id=? AND id=?`,
		projectID, integrationID,
	)
	if err != nil {
		return fmt.Errorf("delete integration: %w", err)
	}
	d.MarkDirty()
	return nil
}
