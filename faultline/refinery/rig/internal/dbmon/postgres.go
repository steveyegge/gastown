package dbmon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PostgreSQL threshold defaults per the design spec.
const (
	defaultConnectionUsagePct = 80.0 // warn if active > 80% of max_connections
	defaultLongQuerySec       = 30.0 // warn if queries running > 30s
	defaultReplicationLagSec  = 10.0 // warn if replication lag > 10s
	defaultDeadTupleRatioPct  = 10.0 // warn if dead tuples > 10% of live
)

// pgThresholds holds configurable thresholds for PostgreSQL checks.
type pgThresholds struct {
	ConnectionUsagePct float64 `json:"connection_usage_pct"`
	LongQuerySec       float64 `json:"long_query_sec"`
	ReplicationLagSec  float64 `json:"replication_lag_sec"`
	DeadTupleRatioPct  float64 `json:"dead_tuple_ratio_pct"`
}

func parsePGThresholds(raw sql.NullString) pgThresholds {
	t := pgThresholds{
		ConnectionUsagePct: defaultConnectionUsagePct,
		LongQuerySec:       defaultLongQuerySec,
		ReplicationLagSec:  defaultReplicationLagSec,
		DeadTupleRatioPct:  defaultDeadTupleRatioPct,
	}
	if !raw.Valid || raw.String == "" {
		return t
	}
	_ = json.Unmarshal([]byte(raw.String), &t)
	return t
}

// rowScanner is satisfied by *sql.Row and test mocks.
type rowScanner interface {
	Scan(dest ...any) error
}

// pgConnector abstracts PostgreSQL database access for testing.
type pgConnector interface {
	PingContext(ctx context.Context) error
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
	QueryContext(ctx context.Context, query string, args ...any) (pgRows, error)
	Close() error
}

// pgRows abstracts *sql.Rows for testing.
type pgRows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
}

// pgConnectFunc opens a connection to a PostgreSQL database.
type pgConnectFunc func(connStr string) (pgConnector, error)

// sqlDBWrapper wraps *sql.DB to satisfy pgConnector.
type sqlDBWrapper struct {
	db *sql.DB
}

func (w *sqlDBWrapper) PingContext(ctx context.Context) error {
	return w.db.PingContext(ctx)
}

func (w *sqlDBWrapper) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return w.db.QueryRowContext(ctx, query, args...)
}

func (w *sqlDBWrapper) QueryContext(ctx context.Context, query string, args ...any) (pgRows, error) {
	return w.db.QueryContext(ctx, query, args...)
}

func (w *sqlDBWrapper) Close() error {
	return w.db.Close()
}

// defaultPGConnect opens a real PostgreSQL connection via database/sql.
// Requires the lib/pq driver to be imported in the binary's main package.
func defaultPGConnect(connStr string) (pgConnector, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Second)
	return &sqlDBWrapper{db: db}, nil
}

// NewPostgresChecker returns a CheckFunc for PostgreSQL targets.
func NewPostgresChecker() CheckFunc {
	return newPostgresCheckerWith(defaultPGConnect)
}

func newPostgresCheckerWith(connect pgConnectFunc) CheckFunc {
	return func(ctx context.Context, target DatabaseTarget) []CheckResult {
		thresholds := parsePGThresholds(target.Thresholds)
		now := time.Now().UTC()

		db, err := connect(target.ConnectionString)
		if err != nil {
			return []CheckResult{{
				DatabaseID: target.ID,
				CheckType:  "connection",
				Status:     CheckCritical,
				Message:    fmt.Sprintf("connection failed: %v", err),
				CheckedAt:  now,
			}}
		}
		defer db.Close()

		var results []CheckResult
		results = append(results, checkPGConnection(ctx, db, target.ID, now))
		results = append(results, checkPGConnectionUsage(ctx, db, target.ID, now, thresholds)...)
		results = append(results, checkPGDeadlocks(ctx, db, target.ID, now))
		results = append(results, checkPGDeadTuples(ctx, db, target.ID, now, thresholds))
		results = append(results, checkPGReplicationLag(ctx, db, target.ID, now, thresholds)...)
		results = append(results, checkPGLongQueries(ctx, db, target.ID, now, thresholds)...)
		return results
	}
}

// checkPGConnection pings the database.
func checkPGConnection(ctx context.Context, db pgConnector, dbID string, now time.Time) CheckResult {
	start := time.Now()
	err := db.PingContext(ctx)
	latency := time.Since(start).Seconds() * 1000 // ms

	if err != nil {
		return CheckResult{
			DatabaseID: dbID,
			CheckType:  "connection",
			Status:     CheckCritical,
			Message:    fmt.Sprintf("ping failed: %v", err),
			CheckedAt:  now,
		}
	}
	return CheckResult{
		DatabaseID: dbID,
		CheckType:  "connection",
		Status:     CheckOK,
		Value:      &latency,
		Message:    fmt.Sprintf("ping ok (%.0fms)", latency),
		CheckedAt:  now,
	}
}

// checkPGConnectionUsage checks active connections vs max_connections.
func checkPGConnectionUsage(ctx context.Context, db pgConnector, dbID string, now time.Time, t pgThresholds) []CheckResult {
	var maxConns int
	if err := db.QueryRowContext(ctx, "SHOW max_connections").Scan(&maxConns); err != nil {
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "connection_usage",
			Status:     CheckWarning,
			Message:    fmt.Sprintf("could not read max_connections: %v", err),
			CheckedAt:  now,
		}}
	}

	var activeConns int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE state IS NOT NULL").Scan(&activeConns); err != nil {
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "connection_usage",
			Status:     CheckWarning,
			Message:    fmt.Sprintf("could not read active connections: %v", err),
			CheckedAt:  now,
		}}
	}

	usagePct := 0.0
	if maxConns > 0 {
		usagePct = float64(activeConns) / float64(maxConns) * 100
	}

	status := CheckOK
	msg := fmt.Sprintf("%d/%d connections (%.1f%%)", activeConns, maxConns, usagePct)
	if usagePct > t.ConnectionUsagePct {
		status = CheckWarning
		msg = fmt.Sprintf("high connection usage: %s", msg)
	}

	return []CheckResult{{
		DatabaseID: dbID,
		CheckType:  "connection_usage",
		Status:     status,
		Value:      &usagePct,
		Message:    msg,
		CheckedAt:  now,
	}}
}

// checkPGDeadlocks checks the deadlock counter from pg_stat_database.
func checkPGDeadlocks(ctx context.Context, db pgConnector, dbID string, now time.Time) CheckResult {
	var deadlocks int64
	err := db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(deadlocks), 0) FROM pg_stat_database").Scan(&deadlocks)
	if err != nil {
		return CheckResult{
			DatabaseID: dbID,
			CheckType:  "deadlocks",
			Status:     CheckWarning,
			Message:    fmt.Sprintf("could not read deadlocks: %v", err),
			CheckedAt:  now,
		}
	}

	val := float64(deadlocks)
	return CheckResult{
		DatabaseID: dbID,
		CheckType:  "deadlocks",
		Status:     CheckOK,
		Value:      &val,
		Message:    fmt.Sprintf("total deadlocks: %d", deadlocks),
		CheckedAt:  now,
	}
}

// checkPGDeadTuples checks the dead tuple ratio across user tables.
func checkPGDeadTuples(ctx context.Context, db pgConnector, dbID string, now time.Time, t pgThresholds) CheckResult {
	var liveTuples, deadTuples int64
	err := db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(n_live_tup), 0), COALESCE(SUM(n_dead_tup), 0) FROM pg_stat_user_tables",
	).Scan(&liveTuples, &deadTuples)
	if err != nil {
		return CheckResult{
			DatabaseID: dbID,
			CheckType:  "dead_tuples",
			Status:     CheckWarning,
			Message:    fmt.Sprintf("could not read tuple stats: %v", err),
			CheckedAt:  now,
		}
	}

	ratio := 0.0
	if liveTuples+deadTuples > 0 {
		ratio = float64(deadTuples) / float64(liveTuples+deadTuples) * 100
	}

	status := CheckOK
	msg := fmt.Sprintf("dead tuple ratio: %.1f%% (%d dead / %d total)", ratio, deadTuples, liveTuples+deadTuples)
	if ratio > t.DeadTupleRatioPct {
		status = CheckWarning
		msg = fmt.Sprintf("high %s", msg)
	}

	return CheckResult{
		DatabaseID: dbID,
		CheckType:  "dead_tuples",
		Status:     status,
		Value:      &ratio,
		Message:    msg,
		CheckedAt:  now,
	}
}

// checkPGReplicationLag checks replication lag from pg_stat_replication.
// Returns ok with no-replicas message if no replicas are connected.
// Uses replay_lag (PG 10+) which reports the time difference between
// the primary's current WAL position and the replica's replay position.
func checkPGReplicationLag(ctx context.Context, db pgConnector, dbID string, now time.Time, t pgThresholds) []CheckResult {
	rows, err := db.QueryContext(ctx,
		`SELECT client_addr,
		        COALESCE(EXTRACT(EPOCH FROM replay_lag), 0) AS lag_sec
		 FROM pg_stat_replication`)
	if err != nil {
		// Not all instances have replication; a permission error is not critical.
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "replication_lag",
			Status:     CheckOK,
			Message:    "replication check skipped (not available or no permissions)",
			CheckedAt:  now,
		}}
	}
	defer rows.Close()

	var results []CheckResult
	hasReplicas := false
	for rows.Next() {
		hasReplicas = true
		var addr sql.NullString
		var lagSec float64
		if err := rows.Scan(&addr, &lagSec); err != nil {
			continue
		}
		replica := "unknown"
		if addr.Valid {
			replica = addr.String
		}

		status := CheckOK
		msg := fmt.Sprintf("replica %s: lag %.1fs", replica, lagSec)
		if lagSec > t.ReplicationLagSec {
			status = CheckWarning
			msg = fmt.Sprintf("high replication lag — %s", msg)
		}
		results = append(results, CheckResult{
			DatabaseID: dbID,
			CheckType:  "replication_lag",
			Status:     status,
			Value:      &lagSec,
			Message:    msg,
			CheckedAt:  now,
		})
	}

	if !hasReplicas {
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "replication_lag",
			Status:     CheckOK,
			Message:    "no replicas connected",
			CheckedAt:  now,
		}}
	}

	return results
}

// checkPGLongQueries checks for long-running queries from pg_stat_activity.
func checkPGLongQueries(ctx context.Context, db pgConnector, dbID string, now time.Time, t pgThresholds) []CheckResult {
	rows, err := db.QueryContext(ctx,
		`SELECT pid, EXTRACT(EPOCH FROM (now() - query_start)) AS duration_sec,
		        LEFT(query, 100) AS query_preview
		 FROM pg_stat_activity
		 WHERE state = 'active'
		   AND query_start IS NOT NULL
		   AND EXTRACT(EPOCH FROM (now() - query_start)) > $1
		 ORDER BY duration_sec DESC
		 LIMIT 10`, t.LongQuerySec)
	if err != nil {
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "long_queries",
			Status:     CheckWarning,
			Message:    fmt.Sprintf("could not read long queries: %v", err),
			CheckedAt:  now,
		}}
	}
	defer rows.Close()

	var longQueries int
	var maxDuration float64
	for rows.Next() {
		var pid int
		var dur float64
		var query string
		if err := rows.Scan(&pid, &dur, &query); err != nil {
			continue
		}
		longQueries++
		if dur > maxDuration {
			maxDuration = dur
		}
	}

	if longQueries == 0 {
		return []CheckResult{{
			DatabaseID: dbID,
			CheckType:  "long_queries",
			Status:     CheckOK,
			Message:    fmt.Sprintf("no queries running longer than %.0fs", t.LongQuerySec),
			CheckedAt:  now,
		}}
	}

	val := maxDuration
	return []CheckResult{{
		DatabaseID: dbID,
		CheckType:  "long_queries",
		Status:     CheckWarning,
		Value:      &val,
		Message:    fmt.Sprintf("%d queries running > %.0fs (max %.0fs)", longQueries, t.LongQuerySec, maxDuration),
		CheckedAt:  now,
	}}
}
