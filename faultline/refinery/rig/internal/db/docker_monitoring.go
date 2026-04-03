package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MonitoredContainer represents a Docker container discovered for monitoring.
type MonitoredContainer struct {
	ID            string    `json:"id"`
	ProjectID     *int64    `json:"project_id,omitempty"`
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	ServiceName   string    `json:"service_name,omitempty"`
	Image         string    `json:"image,omitempty"`
	Enabled       bool      `json:"enabled"`
	Thresholds    []byte    `json:"thresholds,omitempty"` // JSON per-container overrides
	DiscoveredAt  time.Time `json:"discovered_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
}

// ContainerCheck records the result of a single container health check.
type ContainerCheck struct {
	ID          string    `json:"id"`
	ContainerID string    `json:"container_id"` // FK to monitored_containers.id
	ProjectID   *int64    `json:"project_id,omitempty"`
	CheckType   string    `json:"check_type"` // health, memory, cpu, restart, stopped, disk
	Status      string    `json:"status"`     // ok, warning, critical
	Value       *float64  `json:"value,omitempty"`
	Message     string    `json:"message,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// ContainerMonitorState tracks the current monitoring state for a container.
type ContainerMonitorState struct {
	ContainerID        string     `json:"container_id"` // FK to monitored_containers.id
	Status             string     `json:"status"`       // healthy, degraded, down
	LastTransitionAt   *time.Time `json:"last_transition_at,omitempty"`
	LastCheckAt        *time.Time `json:"last_check_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

// ListContainersByProject returns all monitored containers for a project.
func (d *DB) ListContainersByProject(ctx context.Context, projectID int64) ([]MonitoredContainer, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, container_id, container_name, service_name, image, enabled, thresholds, discovered_at, last_seen_at
		 FROM monitored_containers WHERE project_id = ? ORDER BY container_name ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var containers []MonitoredContainer
	for rows.Next() {
		var c MonitoredContainer
		var pid sql.NullInt64
		var svc, img sql.NullString
		var thresholds []byte
		if err := rows.Scan(&c.ID, &pid, &c.ContainerID, &c.ContainerName, &svc, &img,
			&c.Enabled, &thresholds, &c.DiscoveredAt, &c.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan container: %w", err)
		}
		if pid.Valid {
			c.ProjectID = &pid.Int64
		}
		if svc.Valid {
			c.ServiceName = svc.String
		}
		if img.Valid {
			c.Image = img.String
		}
		c.Thresholds = thresholds
		containers = append(containers, c)
	}
	return containers, rows.Err()
}

// ListAllContainers returns all monitored containers across all projects (admin).
func (d *DB) ListAllContainers(ctx context.Context) ([]MonitoredContainer, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, container_id, container_name, service_name, image, enabled, thresholds, discovered_at, last_seen_at
		 FROM monitored_containers ORDER BY container_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all containers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var containers []MonitoredContainer
	for rows.Next() {
		var c MonitoredContainer
		var pid sql.NullInt64
		var svc, img sql.NullString
		var thresholds []byte
		if err := rows.Scan(&c.ID, &pid, &c.ContainerID, &c.ContainerName, &svc, &img,
			&c.Enabled, &thresholds, &c.DiscoveredAt, &c.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan container: %w", err)
		}
		if pid.Valid {
			c.ProjectID = &pid.Int64
		}
		if svc.Valid {
			c.ServiceName = svc.String
		}
		if img.Valid {
			c.Image = img.String
		}
		c.Thresholds = thresholds
		containers = append(containers, c)
	}
	return containers, rows.Err()
}

// GetContainer returns a single monitored container by ID and project.
func (d *DB) GetContainer(ctx context.Context, projectID int64, id string) (*MonitoredContainer, error) {
	var c MonitoredContainer
	var pid sql.NullInt64
	var svc, img sql.NullString
	var thresholds []byte
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, container_id, container_name, service_name, image, enabled, thresholds, discovered_at, last_seen_at
		 FROM monitored_containers WHERE project_id = ? AND id = ?`, projectID, id,
	).Scan(&c.ID, &pid, &c.ContainerID, &c.ContainerName, &svc, &img,
		&c.Enabled, &thresholds, &c.DiscoveredAt, &c.LastSeenAt)
	if err != nil {
		return nil, err
	}
	if pid.Valid {
		c.ProjectID = &pid.Int64
	}
	if svc.Valid {
		c.ServiceName = svc.String
	}
	if img.Valid {
		c.Image = img.String
	}
	c.Thresholds = thresholds
	return &c, nil
}

// ListContainerChecks returns check history for a container, most recent first.
func (d *DB) ListContainerChecks(ctx context.Context, containerID string, limit int) ([]ContainerCheck, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, container_id, project_id, check_type, status, value, message, checked_at
		 FROM container_checks WHERE container_id = ? ORDER BY checked_at DESC LIMIT ?`,
		containerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list container checks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var checks []ContainerCheck
	for rows.Next() {
		var c ContainerCheck
		var val sql.NullFloat64
		var msg sql.NullString
		var pid sql.NullInt64
		if err := rows.Scan(&c.ID, &c.ContainerID, &pid, &c.CheckType, &c.Status, &val, &msg, &c.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan container check: %w", err)
		}
		if val.Valid {
			c.Value = &val.Float64
		}
		if msg.Valid {
			c.Message = msg.String
		}
		if pid.Valid {
			c.ProjectID = &pid.Int64
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// GetContainerMonitorState returns the current monitoring state for a container.
func (d *DB) GetContainerMonitorState(ctx context.Context, containerID string) (*ContainerMonitorState, error) {
	var s ContainerMonitorState
	err := d.QueryRowContext(ctx,
		`SELECT container_id, status, last_transition_at, last_check_at, consecutive_failures
		 FROM container_monitor_state WHERE container_id = ?`, containerID,
	).Scan(&s.ContainerID, &s.Status, &s.LastTransitionAt, &s.LastCheckAt, &s.ConsecutiveFailures)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetProjectDockerThresholds returns the docker_thresholds JSON for a project.
func (d *DB) GetProjectDockerThresholds(ctx context.Context, projectID int64) ([]byte, error) {
	var raw sql.NullString
	err := d.QueryRowContext(ctx,
		`SELECT docker_thresholds FROM projects WHERE id = ?`, projectID,
	).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	return []byte(raw.String), nil
}

// UpdateProjectDockerThresholds sets the docker_thresholds JSON for a project.
func (d *DB) UpdateProjectDockerThresholds(ctx context.Context, projectID int64, thresholds []byte) error {
	_, err := d.ExecContext(ctx,
		`UPDATE projects SET docker_thresholds = ? WHERE id = ?`,
		string(thresholds), projectID)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

func (d *DB) migrateDockerMonitoring(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS monitored_containers (
			id              VARCHAR(36) PRIMARY KEY,
			project_id      BIGINT,
			container_id    VARCHAR(64) NOT NULL,
			container_name  VARCHAR(200) NOT NULL,
			service_name    VARCHAR(200),
			image           VARCHAR(512),
			enabled         BOOLEAN DEFAULT true,
			thresholds      JSON,
			discovered_at   DATETIME(6) NOT NULL,
			last_seen_at    DATETIME(6) NOT NULL,
			INDEX idx_mc_project (project_id),
			INDEX idx_mc_container (container_id)
		)`,

		`CREATE TABLE IF NOT EXISTS container_checks (
			id            VARCHAR(36) PRIMARY KEY,
			container_id  VARCHAR(36) NOT NULL,
			project_id    BIGINT,
			check_type    VARCHAR(64) NOT NULL,
			status        VARCHAR(16) NOT NULL,
			value         DOUBLE,
			message       TEXT,
			checked_at    DATETIME(6) NOT NULL,
			INDEX idx_cc_container (container_id),
			INDEX idx_cc_checked_at (checked_at)
		)`,

		`CREATE TABLE IF NOT EXISTS container_monitor_state (
			container_id         VARCHAR(36) PRIMARY KEY,
			status               VARCHAR(16) DEFAULT 'healthy',
			last_transition_at   DATETIME(6),
			last_check_at        DATETIME(6),
			consecutive_failures INT DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate docker_monitoring: %w", err)
		}
	}

	// Add docker_thresholds JSON column to projects table if it doesn't exist.
	_, _ = d.ExecContext(ctx, `ALTER TABLE projects ADD COLUMN docker_thresholds JSON`)

	return nil
}
