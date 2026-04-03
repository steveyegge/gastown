package dockermon

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// SQLDBProvider is a concrete DBProvider backed by *sql.DB.
type SQLDBProvider struct {
	DB *sql.DB
}

func (p *SQLDBProvider) UpsertContainer(ctx context.Context, c *Container) error {
	var pid sql.NullInt64
	if c.ProjectID != nil {
		pid = sql.NullInt64{Int64: *c.ProjectID, Valid: true}
	}
	_, err := p.DB.ExecContext(ctx, `
		REPLACE INTO monitored_containers
			(id, project_id, container_id, container_name, service_name, image, enabled, thresholds, discovered_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, pid, c.ContainerID, c.ContainerName, c.ServiceName, c.Image,
		c.Enabled, c.Thresholds, c.DiscoveredAt, c.LastSeenAt,
	)
	return err
}

func (p *SQLDBProvider) ListContainers(ctx context.Context) ([]Container, error) {
	rows, err := p.DB.QueryContext(ctx, `
		SELECT id, project_id, container_id, container_name, service_name, image, enabled, thresholds, discovered_at, last_seen_at
		FROM monitored_containers WHERE enabled = true`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Container
	for rows.Next() {
		var c Container
		var pid sql.NullInt64
		if err := rows.Scan(&c.ID, &pid, &c.ContainerID, &c.ContainerName,
			&c.ServiceName, &c.Image, &c.Enabled, &c.Thresholds,
			&c.DiscoveredAt, &c.LastSeenAt); err != nil {
			continue
		}
		if pid.Valid {
			c.ProjectID = &pid.Int64
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *SQLDBProvider) WriteCheckResults(ctx context.Context, results []CheckResult) error {
	for _, r := range results {
		var val sql.NullFloat64
		if r.Value != nil {
			val = sql.NullFloat64{Float64: *r.Value, Valid: true}
		}
		var pid sql.NullInt64
		if r.ProjectID != nil {
			pid = sql.NullInt64{Int64: *r.ProjectID, Valid: true}
		}
		_, err := p.DB.ExecContext(ctx, `
			INSERT INTO container_checks (id, container_id, project_id, check_type, status, value, message, checked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), r.ContainerID, pid, r.CheckType, string(r.Status), val, r.Message, r.CheckedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *SQLDBProvider) LoadMonitorState(ctx context.Context, containerID string) (*MonitorState, error) {
	var s MonitorState
	var status string
	var transAt, checkAt sql.NullTime
	err := p.DB.QueryRowContext(ctx, `
		SELECT container_id, status, last_transition_at, last_check_at, consecutive_failures
		FROM container_monitor_state WHERE container_id = ?`, containerID,
	).Scan(&s.ContainerID, &status, &transAt, &checkAt, &s.ConsecutiveFailures)
	if err == sql.ErrNoRows {
		return &MonitorState{ContainerID: containerID, Status: StatusHealthy}, nil
	}
	if err != nil {
		return nil, err
	}
	s.Status = Status(status)
	if transAt.Valid {
		s.LastTransitionAt = &transAt.Time
	}
	if checkAt.Valid {
		s.LastCheckAt = &checkAt.Time
	}
	return &s, nil
}

func (p *SQLDBProvider) SaveMonitorState(ctx context.Context, state *MonitorState) error {
	var transAt, checkAt sql.NullTime
	if state.LastTransitionAt != nil {
		transAt = sql.NullTime{Time: *state.LastTransitionAt, Valid: true}
	}
	if state.LastCheckAt != nil {
		checkAt = sql.NullTime{Time: *state.LastCheckAt, Valid: true}
	}
	_, err := p.DB.ExecContext(ctx, `
		REPLACE INTO container_monitor_state (container_id, status, last_transition_at, last_check_at, consecutive_failures)
		VALUES (?, ?, ?, ?, ?)`,
		state.ContainerID, string(state.Status), transAt, checkAt, state.ConsecutiveFailures,
	)
	return err
}

func (p *SQLDBProvider) MarkContainerLastSeen(ctx context.Context, id string, at time.Time) error {
	_, err := p.DB.ExecContext(ctx, `UPDATE monitored_containers SET last_seen_at = ? WHERE id = ?`, at, id)
	return err
}
