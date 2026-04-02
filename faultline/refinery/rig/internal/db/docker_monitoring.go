package db

import (
	"context"
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
