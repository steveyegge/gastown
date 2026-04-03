package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// MonitoredDatabase represents a database being monitored.
type MonitoredDatabase struct {
	ID               string    `json:"id"`
	ProjectID        int64     `json:"project_id"`
	Name             string    `json:"name"`
	DBType           string    `json:"db_type"`
	ConnectionString []byte    `json:"connection_string"` // encrypted
	Enabled          bool      `json:"enabled"`
	CheckIntervalSec int       `json:"check_interval_secs"`
	Thresholds       []byte    `json:"thresholds,omitempty"` // JSON
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DBCheck records the result of a single database health check.
type DBCheck struct {
	ID         string    `json:"id"`
	DatabaseID string    `json:"database_id"`
	ProjectID  int64     `json:"project_id"`
	CheckType  string    `json:"check_type"`
	Status     string    `json:"status"`
	Value      *float64  `json:"value,omitempty"`
	Message    string    `json:"message,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// DBMonitorState tracks the current monitoring state for each database.
type DBMonitorState struct {
	DatabaseID          string    `json:"database_id"`
	Status              string    `json:"status"`
	LastTransitionAt    time.Time `json:"last_transition_at,omitempty"`
	LastCheckAt         time.Time `json:"last_check_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

// Migration functions are in db_monitors.go (canonical schema).

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// ListMonitoredDatabasesByProject returns all monitored databases for a project.
func (d *DB) ListMonitoredDatabasesByProject(ctx context.Context, projectID int64) ([]MonitoredDatabase, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, project_id, name, db_type, connection_string, enabled, check_interval_secs, created_at, updated_at
		 FROM monitored_databases WHERE project_id = ? ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list monitored databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dbs []MonitoredDatabase
	for rows.Next() {
		var m MonitoredDatabase
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.DBType, &m.ConnectionString,
			&m.Enabled, &m.CheckIntervalSec, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan monitored database: %w", err)
		}
		dbs = append(dbs, m)
	}
	return dbs, rows.Err()
}

// GetMonitoredDatabase returns a single monitored database by ID and project.
func (d *DB) GetMonitoredDatabase(ctx context.Context, projectID int64, id string) (*MonitoredDatabase, error) {
	var m MonitoredDatabase
	err := d.QueryRowContext(ctx,
		`SELECT id, project_id, name, db_type, connection_string, enabled, check_interval_secs, created_at, updated_at
		 FROM monitored_databases WHERE project_id = ? AND id = ?`, projectID, id,
	).Scan(&m.ID, &m.ProjectID, &m.Name, &m.DBType, &m.ConnectionString,
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
	if m.ID == "" {
		m.ID = newUUID()
	}
	if m.CheckIntervalSec <= 0 {
		m.CheckIntervalSec = 60
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO monitored_databases (id, project_id, name, db_type, connection_string, enabled, check_interval_secs, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ProjectID, m.Name, m.DBType, m.ConnectionString, m.Enabled, m.CheckIntervalSec, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert monitored database: %w", err)
	}
	d.MarkDirty()
	return nil
}

// UpdateMonitoredDatabase updates an existing monitored database entry.
func (d *DB) UpdateMonitoredDatabase(ctx context.Context, m *MonitoredDatabase) error {
	m.UpdatedAt = time.Now().UTC()
	_, err := d.ExecContext(ctx, `
		UPDATE monitored_databases SET name=?, db_type=?, connection_string=?, enabled=?, check_interval_secs=?, updated_at=?
		WHERE project_id=? AND id=?`,
		m.Name, m.DBType, m.ConnectionString, m.Enabled, m.CheckIntervalSec, m.UpdatedAt,
		m.ProjectID, m.ID,
	)
	if err != nil {
		return fmt.Errorf("update monitored database: %w", err)
	}
	d.MarkDirty()
	return nil
}

// DeleteMonitoredDatabase removes a monitored database and its related state.
func (d *DB) DeleteMonitoredDatabase(ctx context.Context, projectID int64, id string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM monitored_databases WHERE project_id=? AND id=?`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete monitored database: %w", err)
	}
	_, _ = d.ExecContext(ctx, `DELETE FROM db_monitor_state WHERE database_id=?`, id)
	d.MarkDirty()
	return nil
}

// ListDBChecksByDatabase returns check history for a monitored database, most recent first.
func (d *DB) ListDBChecksByDatabase(ctx context.Context, databaseID string, limit int) ([]DBCheck, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, database_id, project_id, check_type, status, value, message, checked_at
		 FROM db_checks WHERE database_id = ? ORDER BY checked_at DESC LIMIT ?`,
		databaseID, limit)
	if err != nil {
		return nil, fmt.Errorf("list db checks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var checks []DBCheck
	for rows.Next() {
		var c DBCheck
		var val sql.NullFloat64
		var msg sql.NullString
		var pid sql.NullInt64
		if err := rows.Scan(&c.ID, &c.DatabaseID, &pid, &c.CheckType, &c.Status, &val, &msg, &c.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan db check: %w", err)
		}
		if val.Valid {
			c.Value = &val.Float64
		}
		if msg.Valid {
			c.Message = msg.String
		}
		if pid.Valid {
			c.ProjectID = pid.Int64
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
