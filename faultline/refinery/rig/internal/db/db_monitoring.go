package db

import (
	"context"
	"time"
)

// MonitoredDatabase represents a database being monitored.
type MonitoredDatabase struct {
	ID               int64     `json:"id"`
	ProjectID        int64     `json:"project_id"`
	Name             string    `json:"name"`
	Engine           string    `json:"engine"`
	ConnectionString []byte    `json:"connection_string"` // encrypted
	Enabled          bool      `json:"enabled"`
	CheckIntervalSec int       `json:"check_interval_sec"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DBCheck records the result of a single database health check.
type DBCheck struct {
	ID                  int64     `json:"id"`
	MonitoredDatabaseID int64     `json:"monitored_database_id"`
	Status              string    `json:"status"` // ok, degraded, down
	LatencyMS           int       `json:"latency_ms"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	CheckedAt           time.Time `json:"checked_at"`
}

// DBMonitorState tracks the current monitoring state for each database.
type DBMonitorState struct {
	MonitoredDatabaseID int64     `json:"monitored_database_id"`
	CurrentStatus       string    `json:"current_status"`
	LastCheckAt         time.Time `json:"last_check_at"`
	LastOKAt            time.Time `json:"last_ok_at,omitempty"`
	ConsecutiveFails    int       `json:"consecutive_fails"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// migrateMonitoredDatabases creates the monitored_databases table.
func (d *DB) migrateMonitoredDatabases(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitored_databases (
		id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
		project_id         BIGINT NOT NULL,
		name               VARCHAR(200) NOT NULL,
		engine             VARCHAR(50) NOT NULL,
		connection_string  VARBINARY(2048) NOT NULL,
		enabled            BOOLEAN NOT NULL DEFAULT TRUE,
		check_interval_sec INT NOT NULL DEFAULT 60,
		created_at         DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at         DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_md_project (project_id),
		INDEX idx_md_enabled (enabled)
	)`)
	return err
}

// migrateDBChecks creates the db_checks table.
func (d *DB) migrateDBChecks(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS db_checks (
		id                    BIGINT AUTO_INCREMENT PRIMARY KEY,
		monitored_database_id BIGINT NOT NULL,
		status                VARCHAR(16) NOT NULL,
		latency_ms            INT NOT NULL DEFAULT 0,
		error_message         TEXT,
		checked_at            DATETIME(6) NOT NULL,
		INDEX idx_dbc_db_time (monitored_database_id, checked_at),
		INDEX idx_dbc_status (status)
	)`)
	return err
}

// migrateDBMonitorState creates the db_monitor_state table.
func (d *DB) migrateDBMonitorState(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS db_monitor_state (
		monitored_database_id BIGINT PRIMARY KEY,
		current_status        VARCHAR(16) NOT NULL DEFAULT 'unknown',
		last_check_at         DATETIME(6),
		last_ok_at            DATETIME(6),
		consecutive_fails     INT NOT NULL DEFAULT 0,
		updated_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
	)`)
	return err
}
