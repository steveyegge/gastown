package dbmon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Default thresholds for Dolt checks.
const (
	defaultCommitLagWarning = 5 * time.Minute
)

// doltThresholds holds configurable thresholds parsed from the target's JSON.
type doltThresholds struct {
	CommitLagWarningS *float64 `json:"commit_lag_warning_s"`
}

func parseDoltThresholds(raw sql.NullString) doltThresholds {
	var t doltThresholds
	if raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &t)
	}
	return t
}

func (t doltThresholds) commitLagWarning() time.Duration {
	if t.CommitLagWarningS != nil {
		return time.Duration(*t.CommitLagWarningS * float64(time.Second))
	}
	return defaultCommitLagWarning
}

// CheckDolt performs health checks against a Dolt database.
// Checks: connection ping, query latency, commit lag, orphan database count.
func CheckDolt(ctx context.Context, target DatabaseTarget) []CheckResult {
	var results []CheckResult

	db, err := sql.Open("mysql", target.ConnectionString)
	if err != nil {
		results = append(results, NewCheckResult(
			target.ID, "connection", CheckCritical, nil,
			fmt.Sprintf("failed to open connection: %v", err),
		))
		return results
	}
	defer func() { _ = db.Close() }()

	thresholds := parseDoltThresholds(target.Thresholds)

	results = append(results, checkDoltPing(ctx, db, target.ID))
	results = append(results, checkDoltQueryLatency(ctx, db, target.ID))
	results = append(results, checkDoltCommitLag(ctx, db, target.ID, thresholds))
	results = append(results, checkDoltOrphanCount(ctx, db, target.ID))

	return results
}

// checkDoltPing tests basic connectivity and measures ping latency.
func checkDoltPing(ctx context.Context, db *sql.DB, dbID string) CheckResult {
	start := time.Now()
	err := db.PingContext(ctx)
	latencyMs := float64(time.Since(start).Milliseconds())

	if err != nil {
		return NewCheckResult(dbID, "connection", CheckCritical, &latencyMs,
			fmt.Sprintf("ping failed: %v", err))
	}
	return NewCheckResult(dbID, "connection", CheckOK, &latencyMs,
		fmt.Sprintf("ping ok (%dms)", int64(latencyMs)))
}

// checkDoltQueryLatency runs a simple SELECT and measures response time.
func checkDoltQueryLatency(ctx context.Context, db *sql.DB, dbID string) CheckResult {
	start := time.Now()
	var result int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	latencyMs := float64(time.Since(start).Milliseconds())

	if err != nil {
		return NewCheckResult(dbID, "latency", CheckCritical, &latencyMs,
			fmt.Sprintf("query failed: %v", err))
	}
	return NewCheckResult(dbID, "latency", CheckOK, &latencyMs,
		fmt.Sprintf("query latency %dms", int64(latencyMs)))
}

// checkDoltCommitLag measures the time since the last dolt_log entry.
func checkDoltCommitLag(ctx context.Context, db *sql.DB, dbID string, thresholds doltThresholds) CheckResult {
	var dateStr string
	err := db.QueryRowContext(ctx,
		"SELECT date FROM dolt_log ORDER BY date DESC LIMIT 1",
	).Scan(&dateStr)
	if err != nil {
		return NewCheckResult(dbID, "commit_lag", CheckCritical, nil,
			fmt.Sprintf("failed to query dolt_log: %v", err))
	}

	lastCommit, err := time.Parse("2006-01-02 15:04:05.999", dateStr)
	if err != nil {
		// Try alternate format without fractional seconds.
		lastCommit, err = time.Parse("2006-01-02 15:04:05", dateStr)
		if err != nil {
			return NewCheckResult(dbID, "commit_lag", CheckCritical, nil,
				fmt.Sprintf("failed to parse commit date %q: %v", dateStr, err))
		}
	}

	lag := time.Since(lastCommit)
	lagSeconds := lag.Seconds()

	status := CheckOK
	msg := fmt.Sprintf("last commit %s ago", lag.Truncate(time.Second))

	if lag > thresholds.commitLagWarning() {
		status = CheckWarning
		msg = fmt.Sprintf("commit lag %s exceeds threshold %s",
			lag.Truncate(time.Second), thresholds.commitLagWarning())
	}

	return NewCheckResult(dbID, "commit_lag", status, &lagSeconds, msg)
}

// checkDoltOrphanCount counts databases matching test/orphan patterns.
func checkDoltOrphanCount(ctx context.Context, db *sql.DB, dbID string) CheckResult {
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return NewCheckResult(dbID, "orphan_count", CheckCritical, nil,
			fmt.Sprintf("failed to list databases: %v", err))
	}
	defer func() { _ = rows.Close() }()

	var orphanCount int
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		if isOrphanDatabase(name) {
			orphanCount++
		}
	}
	if err := rows.Err(); err != nil {
		return NewCheckResult(dbID, "orphan_count", CheckCritical, nil,
			fmt.Sprintf("error reading databases: %v", err))
	}

	count := float64(orphanCount)
	status := CheckOK
	msg := fmt.Sprintf("%d orphan databases", orphanCount)

	if orphanCount > 0 {
		status = CheckWarning
		msg = fmt.Sprintf("%d orphan databases found (testdb_*, beads_t*, beads_pt*, doctest_*)", orphanCount)
	}

	return NewCheckResult(dbID, "orphan_count", status, &count, msg)
}

// isOrphanDatabase returns true if the database name matches known test/orphan patterns.
func isOrphanDatabase(name string) bool {
	prefixes := []string{"testdb_", "beads_t", "beads_pt", "doctest_"}
	for _, p := range prefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}
