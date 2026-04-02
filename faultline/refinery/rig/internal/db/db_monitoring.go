package db

import (
	"context"
	"database/sql"
	"fmt"
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

// ListMonitoredDatabasesByProject returns all monitored databases for a project.
func (d *DB) ListMonitoredDatabasesByProject(ctx context.Context, projectID int64) ([]MonitoredDatabase, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, name, engine, connection_string, enabled, check_interval_sec, created_at, updated_at
		 FROM monitored_databases WHERE project_id = ? ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list monitored databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dbs []MonitoredDatabase
	for rows.Next() {
		var m MonitoredDatabase
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Engine, &m.ConnectionString,
			&m.Enabled, &m.CheckIntervalSec, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan monitored database: %w", err)
		}
		dbs = append(dbs, m)
	}
	return dbs, rows.Err()
}

// GetMonitoredDatabase returns a single monitored database by ID and project.
func (d *DB) GetMonitoredDatabase(ctx context.Context, projectID, id int64) (*MonitoredDatabase, error) {
	var m MonitoredDatabase
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, name, engine, connection_string, enabled, check_interval_sec, created_at, updated_at
		 FROM monitored_databases WHERE project_id = ? AND id = ?`, projectID, id,
	).Scan(&m.ID, &m.ProjectID, &m.Name, &m.Engine, &m.ConnectionString,
		&m.Enabled, &m.CheckIntervalSec, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// InsertMonitoredDatabase creates a new monitored database entry.
func (d *DB) InsertMonitoredDatabase(ctx context.Context, m *MonitoredDatabase) error {
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	if m.CheckIntervalSec <= 0 {
		m.CheckIntervalSec = 60
	}
	result, err := d.ExecContext(ctx, `
		INSERT INTO monitored_databases (project_id, name, engine, connection_string, enabled, check_interval_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ProjectID, m.Name, m.Engine, m.ConnectionString, m.Enabled, m.CheckIntervalSec, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert monitored database: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	m.ID = id
	d.MarkDirty()
	return nil
}

// UpdateMonitoredDatabase updates an existing monitored database entry.
func (d *DB) UpdateMonitoredDatabase(ctx context.Context, m *MonitoredDatabase) error {
	m.UpdatedAt = time.Now().UTC()
	_, err := d.ExecContext(ctx, `
		UPDATE monitored_databases SET name=?, engine=?, connection_string=?, enabled=?, check_interval_sec=?, updated_at=?
		WHERE project_id=? AND id=?`,
		m.Name, m.Engine, m.ConnectionString, m.Enabled, m.CheckIntervalSec, m.UpdatedAt,
		m.ProjectID, m.ID,
	)
	if err != nil {
		return fmt.Errorf("update monitored database: %w", err)
	}
	d.MarkDirty()
	return nil
}

// DeleteMonitoredDatabase removes a monitored database and its related state.
func (d *DB) DeleteMonitoredDatabase(ctx context.Context, projectID, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM monitored_databases WHERE project_id=? AND id=?`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete monitored database: %w", err)
	}
	// Clean up related monitor state.
	_, _ = d.ExecContext(ctx, `DELETE FROM db_monitor_state WHERE monitored_database_id=?`, id)
	d.MarkDirty()
	return nil
}

// ListDBChecksByDatabase returns check history for a monitored database, most recent first.
func (d *DB) ListDBChecksByDatabase(ctx context.Context, databaseID int64, limit int) ([]DBCheck, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, monitored_database_id, status, latency_ms, error_message, checked_at
		 FROM db_checks WHERE monitored_database_id = ? ORDER BY checked_at DESC LIMIT ?`,
		databaseID, limit)
	if err != nil {
		return nil, fmt.Errorf("list db checks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var checks []DBCheck
	for rows.Next() {
		var c DBCheck
		var errMsg sql.NullString
		if err := rows.Scan(&c.ID, &c.MonitoredDatabaseID, &c.Status, &c.LatencyMS, &errMsg, &c.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan db check: %w", err)
		}
		if errMsg.Valid {
			c.ErrorMessage = errMsg.String
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
