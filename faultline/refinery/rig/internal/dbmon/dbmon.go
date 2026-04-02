// Package dbmon provides periodic database health monitoring.
// It polls configured databases on per-target intervals, tracks health state
// (healthy/degraded/down), and fires callbacks on state transitions.
package dbmon

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status represents the health state of a monitored database.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

// CheckStatus represents the result of an individual check.
type CheckStatus string

const (
	CheckOK       CheckStatus = "ok"
	CheckWarning  CheckStatus = "warning"
	CheckCritical CheckStatus = "critical"
)

// DatabaseTarget is a database to monitor, loaded from monitored_databases.
type DatabaseTarget struct {
	ID               string
	ProjectID        *int64
	Name             string
	DBType           string
	ConnectionString string
	Enabled          bool
	CheckIntervalS   int
	Thresholds       sql.NullString // JSON
}

// CheckResult is the outcome of a single check against a database.
type CheckResult struct {
	DatabaseID string
	ProjectID  *int64
	CheckType  string
	Status     CheckStatus
	Value      *float64
	Message    string
	CheckedAt  time.Time
}

// MonitorState is the persisted state of a monitored database.
type MonitorState struct {
	DatabaseID          string
	Status              Status
	LastTransitionAt    *time.Time
	LastCheckAt         *time.Time
	ConsecutiveFailures int
}

// OnStateChangeFunc is called when a database transitions between health states.
type OnStateChangeFunc func(target DatabaseTarget, oldStatus, newStatus Status)

// DBProvider abstracts data access for the monitor.
type DBProvider interface {
	ListMonitoredDatabases(ctx context.Context) ([]DatabaseTarget, error)
	WriteCheckResults(ctx context.Context, results []CheckResult) error
	LoadMonitorState(ctx context.Context, databaseID string) (*MonitorState, error)
	SaveMonitorState(ctx context.Context, state *MonitorState) error
}

// CheckFunc performs health checks against a database target and returns results.
// Implementations are provided per database type (postgres, dolt, redis).
type CheckFunc func(ctx context.Context, target DatabaseTarget) []CheckResult

// Monitor runs periodic health checks against monitored databases.
type Monitor struct {
	provider      DBProvider
	log           *slog.Logger
	timeout       time.Duration
	maxWorkers    int
	lastCheck     map[string]time.Time // databaseID → last check time
	lastStatus    map[string]Status    // databaseID → last known status
	checkFuncs    map[string]CheckFunc // dbType → check function
	OnStateChange OnStateChangeFunc
}

// New creates a database monitor.
func New(provider DBProvider, log *slog.Logger, timeout time.Duration) *Monitor {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Monitor{
		provider:   provider,
		log:        log,
		timeout:    timeout,
		maxWorkers: 10,
		lastCheck:  make(map[string]time.Time),
		lastStatus: make(map[string]Status),
		checkFuncs: make(map[string]CheckFunc),
	}
}

// RegisterChecker registers a check function for a database type.
func (m *Monitor) RegisterChecker(dbType string, fn CheckFunc) {
	m.checkFuncs[dbType] = fn
}

// Run starts the monitor loop, waking every second to dispatch due checks.
// Blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.log.Info("database monitor started", "timeout", m.timeout, "max_workers", m.maxWorkers)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick(ctx)
		}
	}
}

// tick loads targets, finds which are due, and dispatches checks to workers.
func (m *Monitor) tick(ctx context.Context) {
	targets, err := m.provider.ListMonitoredDatabases(ctx)
	if err != nil {
		m.log.Error("dbmon: list databases", "err", err)
		return
	}

	now := time.Now()
	var due []DatabaseTarget
	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		interval := time.Duration(t.CheckIntervalS) * time.Second
		if interval <= 0 {
			interval = 60 * time.Second
		}
		last, ok := m.lastCheck[t.ID]
		if !ok || now.Sub(last) >= interval {
			due = append(due, t)
		}
	}

	if len(due) == 0 {
		return
	}

	// Dispatch to bounded worker pool.
	sem := make(chan struct{}, m.maxWorkers)
	var mu sync.Mutex
	var allResults []checkBatch

	var wg sync.WaitGroup
	for _, t := range due {
		wg.Add(1)
		sem <- struct{}{} // acquire slot
		go func(target DatabaseTarget) {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			results := m.checkOne(ctx, target)

			mu.Lock()
			allResults = append(allResults, checkBatch{target: target, results: results})
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	// Process results: persist and evaluate state.
	for _, batch := range allResults {
		m.lastCheck[batch.target.ID] = now
		m.processResults(ctx, batch.target, batch.results)
	}
}

type checkBatch struct {
	target  DatabaseTarget
	results []CheckResult
}

// checkOne runs the registered checker for a single target with a timeout.
func (m *Monitor) checkOne(ctx context.Context, target DatabaseTarget) []CheckResult {
	fn, ok := m.checkFuncs[target.DBType]
	if !ok {
		return []CheckResult{{
			DatabaseID: target.ID,
			ProjectID:  target.ProjectID,
			CheckType:  "connection",
			Status:     CheckCritical,
			Message:    "no checker registered for db_type: " + target.DBType,
			CheckedAt:  time.Now().UTC(),
		}}
	}

	checkCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	return fn(checkCtx, target)
}

// processResults persists check results and evaluates state transitions.
func (m *Monitor) processResults(ctx context.Context, target DatabaseTarget, results []CheckResult) {
	if len(results) == 0 {
		return
	}

	// Assign IDs and timestamps to results that need them.
	for i := range results {
		if results[i].DatabaseID == "" {
			results[i].DatabaseID = target.ID
		}
		if results[i].ProjectID == nil {
			results[i].ProjectID = target.ProjectID
		}
		if results[i].CheckedAt.IsZero() {
			results[i].CheckedAt = time.Now().UTC()
		}
	}

	if err := m.provider.WriteCheckResults(ctx, results); err != nil {
		m.log.Error("dbmon: write check results", "err", err, "database", target.ID)
	}

	// Evaluate aggregate state: any critical→down, any warning→degraded, all ok→healthy.
	newStatus := evaluateStatus(results)

	// Load or initialize persisted state.
	state, err := m.provider.LoadMonitorState(ctx, target.ID)
	if err != nil {
		m.log.Error("dbmon: load state", "err", err, "database", target.ID)
		state = &MonitorState{DatabaseID: target.ID, Status: StatusHealthy}
	}

	oldStatus, known := m.lastStatus[target.ID]
	if !known {
		oldStatus = state.Status
	}

	now := time.Now().UTC()
	state.LastCheckAt = &now

	if newStatus == StatusDown {
		state.ConsecutiveFailures++
	} else {
		state.ConsecutiveFailures = 0
	}

	if newStatus != oldStatus {
		state.Status = newStatus
		state.LastTransitionAt = &now
		m.lastStatus[target.ID] = newStatus

		m.log.Info("dbmon: state change",
			"database", target.Name,
			"db_type", target.DBType,
			"old_status", string(oldStatus),
			"new_status", string(newStatus),
		)

		if m.OnStateChange != nil {
			m.OnStateChange(target, oldStatus, newStatus)
		}
	} else {
		m.lastStatus[target.ID] = newStatus
		state.Status = newStatus
	}

	if err := m.provider.SaveMonitorState(ctx, state); err != nil {
		m.log.Error("dbmon: save state", "err", err, "database", target.ID)
	}
}

// evaluateStatus derives aggregate status from individual check results.
// Any critical → down, any warning → degraded, all ok → healthy.
func evaluateStatus(results []CheckResult) Status {
	for _, r := range results {
		if r.Status == CheckCritical {
			return StatusDown
		}
	}
	for _, r := range results {
		if r.Status == CheckWarning {
			return StatusDegraded
		}
	}
	return StatusHealthy
}

// NewCheckResult is a convenience constructor for check results.
func NewCheckResult(databaseID string, checkType string, status CheckStatus, value *float64, message string) CheckResult {
	return CheckResult{
		DatabaseID: databaseID,
		CheckType:  checkType,
		Status:     status,
		Value:      value,
		Message:    message,
		CheckedAt:  time.Now().UTC(),
	}
}

// SQLDBProvider is a concrete DBProvider backed by *sql.DB.
type SQLDBProvider struct {
	DB *sql.DB
}

func (p *SQLDBProvider) ListMonitoredDatabases(ctx context.Context) ([]DatabaseTarget, error) {
	rows, err := p.DB.QueryContext(ctx, `
		SELECT id, project_id, name, db_type, connection_string, enabled, check_interval_secs, thresholds
		FROM monitored_databases WHERE enabled = true`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var targets []DatabaseTarget
	for rows.Next() {
		var t DatabaseTarget
		var pid sql.NullInt64
		if err := rows.Scan(&t.ID, &pid, &t.Name, &t.DBType, &t.ConnectionString, &t.Enabled, &t.CheckIntervalS, &t.Thresholds); err != nil {
			continue
		}
		if pid.Valid {
			t.ProjectID = &pid.Int64
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
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
			INSERT INTO db_checks (id, database_id, project_id, check_type, status, value, message, checked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), r.DatabaseID, pid, r.CheckType, string(r.Status), val, r.Message, r.CheckedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *SQLDBProvider) LoadMonitorState(ctx context.Context, databaseID string) (*MonitorState, error) {
	var s MonitorState
	var status string
	var transAt, checkAt sql.NullTime
	err := p.DB.QueryRowContext(ctx, `
		SELECT database_id, status, last_transition_at, last_check_at, consecutive_failures
		FROM db_monitor_state WHERE database_id = ?`, databaseID,
	).Scan(&s.DatabaseID, &status, &transAt, &checkAt, &s.ConsecutiveFailures)
	if err == sql.ErrNoRows {
		return &MonitorState{DatabaseID: databaseID, Status: StatusHealthy}, nil
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
		REPLACE INTO db_monitor_state (database_id, status, last_transition_at, last_check_at, consecutive_failures)
		VALUES (?, ?, ?, ?, ?)`,
		state.DatabaseID, string(state.Status), transAt, checkAt, state.ConsecutiveFailures,
	)
	return err
}
