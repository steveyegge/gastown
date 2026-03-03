// Package reaper provides wisp and issue cleanup operations for Dolt databases.
//
// These functions are the "callable helper functions" for the Dog-driven
// mol-dog-reaper formula. They execute SQL operations but do not make
// eligibility decisions — the Dog (or daemon orchestrator) decides what
// to reap, purge, and auto-close based on the formula.
package reaper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// validDBName matches safe database names (alphanumeric + underscore only).
var validDBName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// DefaultDatabases returns the list of known production databases.
var DefaultDatabases = []string{"hq", "beads", "gastown"}

// ScanResult holds the results of scanning a database for reaper candidates.
type ScanResult struct {
	Database        string    `json:"database"`
	ReapCandidates  int       `json:"reap_candidates"`
	PurgeCandidates int       `json:"purge_candidates"`
	MailCandidates  int       `json:"mail_candidates"`
	StaleCandidates int       `json:"stale_candidates"`
	OpenWisps       int       `json:"open_wisps"`
	Anomalies       []Anomaly `json:"anomalies,omitempty"`
}

// ReapResult holds the results of a reap operation.
type ReapResult struct {
	Database   string    `json:"database"`
	Reaped     int       `json:"reaped"`
	OpenRemain int       `json:"open_remain"`
	DryRun     bool      `json:"dry_run,omitempty"`
	Anomalies  []Anomaly `json:"anomalies,omitempty"`
}

// PurgeResult holds the results of a purge operation.
type PurgeResult struct {
	Database    string    `json:"database"`
	WispsPurged int       `json:"wisps_purged"`
	MailPurged  int       `json:"mail_purged"`
	DryRun      bool      `json:"dry_run,omitempty"`
	Anomalies   []Anomaly `json:"anomalies,omitempty"`
}

// AutoCloseResult holds the results of an auto-close operation.
type AutoCloseResult struct {
	Database string    `json:"database"`
	Closed   int       `json:"closed"`
	DryRun   bool      `json:"dry_run,omitempty"`
	Anomalies []Anomaly `json:"anomalies,omitempty"`
}

// Anomaly represents an unexpected condition found during reaper operations.
type Anomaly struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"`
}

const (
	// DefaultQueryTimeout is the timeout for individual reaper SQL queries.
	DefaultQueryTimeout = 30 * time.Second
	// DefaultBatchSize is the number of rows per batch DELETE operation.
	DefaultBatchSize = 100
)

// ValidateDBName returns an error if the database name is unsafe.
func ValidateDBName(dbName string) error {
	if !validDBName.MatchString(dbName) {
		return fmt.Errorf("invalid database name: %q", dbName)
	}
	return nil
}

// OpenDB opens a connection to the Dolt server for a given database.
func OpenDB(host string, port int, dbName string, readTimeout, writeTimeout time.Duration) (*sql.DB, error) {
	if err := ValidateDBName(dbName); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=%s&writeTimeout=%s",
		host, port, dbName,
		fmt.Sprintf("%ds", int(readTimeout.Seconds())),
		fmt.Sprintf("%ds", int(writeTimeout.Seconds())))
	return sql.Open("mysql", dsn)
}

// parentCheckWhere returns the SQL WHERE fragment that restricts operations to
// wisps whose parent molecule is closed, that have no parent (orphans), or
// whose parent was purged (dangling dependency reference).
func parentCheckWhere(dbName string) string {
	return fmt.Sprintf(`
		(
			NOT EXISTS (
				SELECT 1 FROM `+"`%s`"+`.wisp_dependencies wd
				WHERE wd.issue_id = w.id AND wd.type = 'parent-child'
			)
			OR
			EXISTS (
				SELECT 1 FROM `+"`%s`"+`.wisp_dependencies wd
				JOIN `+"`%s`"+`.wisps parent ON parent.id = wd.depends_on_id
				WHERE wd.issue_id = w.id AND wd.type = 'parent-child'
				AND parent.status = 'closed'
			)
			OR
			EXISTS (
				SELECT 1 FROM `+"`%s`"+`.wisp_dependencies wd
				LEFT JOIN `+"`%s`"+`.wisps parent ON parent.id = wd.depends_on_id
				WHERE wd.issue_id = w.id AND wd.type = 'parent-child'
				AND parent.id IS NULL
			)
		)`, dbName, dbName, dbName, dbName, dbName)
}

// Scan counts reaper candidates in a database without modifying anything.
func Scan(db *sql.DB, dbName string, maxAge, purgeAge, mailDeleteAge, staleIssueAge time.Duration) (*ScanResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTimeout)
	defer cancel()

	result := &ScanResult{Database: dbName}
	now := time.Now().UTC()
	parentCheck := parentCheckWhere(dbName)

	// Count reap candidates: open wisps past max_age with eligible parent status.
	reapWhere := fmt.Sprintf(
		"w.status IN ('open', 'hooked', 'in_progress') AND w.created_at < ? AND %s", parentCheck)
	reapQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps w WHERE %s", dbName, reapWhere)
	if err := db.QueryRowContext(ctx, reapQuery, now.Add(-maxAge)).Scan(&result.ReapCandidates); err != nil {
		return nil, fmt.Errorf("count reap candidates: %w", err)
	}

	// Count purge candidates: closed wisps past purge_age.
	purgeQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? AND %s",
		dbName, parentCheck)
	if err := db.QueryRowContext(ctx, purgeQuery, now.Add(-purgeAge)).Scan(&result.PurgeCandidates); err != nil {
		return nil, fmt.Errorf("count purge candidates: %w", err)
	}

	// Count mail candidates.
	mailQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s`.issues WHERE status = 'closed' AND closed_at < ? AND id IN (SELECT issue_id FROM `%s`.labels WHERE label = 'gt:message')",
		dbName, dbName)
	if err := db.QueryRowContext(ctx, mailQuery, now.Add(-mailDeleteAge)).Scan(&result.MailCandidates); err != nil {
		return nil, fmt.Errorf("count mail candidates: %w", err)
	}

	// Count stale issue candidates.
	staleQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM `+"`%s`"+`.issues i
		WHERE i.status IN ('open', 'in_progress')
		AND i.updated_at < ?
		AND i.priority > 1
		AND i.issue_type != 'epic'
		AND i.id NOT IN (
			SELECT DISTINCT d.issue_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues dep ON d.depends_on_id = dep.id
			WHERE dep.status IN ('open', 'in_progress')
		)
		AND i.id NOT IN (
			SELECT DISTINCT d.depends_on_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues blocker ON d.issue_id = blocker.id
			WHERE blocker.status IN ('open', 'in_progress')
		)`, dbName, dbName, dbName, dbName, dbName)
	if err := db.QueryRowContext(ctx, staleQuery, now.Add(-staleIssueAge)).Scan(&result.StaleCandidates); err != nil {
		return nil, fmt.Errorf("count stale candidates: %w", err)
	}

	// Total open wisps.
	openQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName validated
	if err := db.QueryRowContext(ctx, openQuery).Scan(&result.OpenWisps); err != nil {
		return nil, fmt.Errorf("count open wisps: %w", err)
	}

	// Anomaly detection: dangling parent references.
	danglingQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM `+"`%s`"+`.wisp_dependencies wd
		LEFT JOIN `+"`%s`"+`.wisps parent ON parent.id = wd.depends_on_id
		WHERE wd.type = 'parent-child' AND parent.id IS NULL`, dbName, dbName)
	var danglingCount int
	if err := db.QueryRowContext(ctx, danglingQuery).Scan(&danglingCount); err == nil && danglingCount > 0 {
		result.Anomalies = append(result.Anomalies, Anomaly{
			Type:    "dangling_parent_ref",
			Message: fmt.Sprintf("%d wisp(s) have parent dependency records pointing to purged/missing parents", danglingCount),
			Count:   danglingCount,
		})
	}

	return result, nil
}

// Reap closes stale wisps in a database whose parent molecule is already closed.
func Reap(db *sql.DB, dbName string, maxAge time.Duration, dryRun bool) (*ReapResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTimeout)
	defer cancel()

	cutoff := time.Now().UTC().Add(-maxAge)
	parentCheck := parentCheckWhere(dbName)
	whereClause := fmt.Sprintf(
		"w.status IN ('open', 'hooked', 'in_progress') AND w.created_at < ? AND %s", parentCheck)

	result := &ReapResult{Database: dbName, DryRun: dryRun}

	if dryRun {
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps w WHERE %s", dbName, whereClause)
		if err := db.QueryRowContext(ctx, countQuery, cutoff).Scan(&result.Reaped); err != nil {
			return nil, fmt.Errorf("dry-run count: %w", err)
		}
		openQuery := fmt.Sprintf(
			"SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName validated
		if err := db.QueryRowContext(ctx, openQuery).Scan(&result.OpenRemain); err != nil {
			return nil, fmt.Errorf("count open: %w", err)
		}
		return result, nil
	}

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return nil, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	closeQuery := fmt.Sprintf(
		"UPDATE `%s`.wisps w SET w.status='closed', w.closed_at=NOW() WHERE %s", dbName, whereClause)
	sqlResult, err := db.ExecContext(ctx, closeQuery, cutoff)
	if err != nil {
		return nil, fmt.Errorf("close stale wisps: %w", err)
	}

	reaped, _ := sqlResult.RowsAffected()
	result.Reaped = int(reaped)

	if reaped > 0 {
		commitMsg := fmt.Sprintf("reaper: close %d stale wisps in %s", reaped, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg from safe values
			return result, fmt.Errorf("dolt commit: %w", err)
		}
	}

	openQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName validated
	if err := db.QueryRowContext(ctx, openQuery).Scan(&result.OpenRemain); err != nil {
		return result, fmt.Errorf("count open: %w", err)
	}

	return result, nil
}

// Purge deletes old closed wisps and mail from a database.
func Purge(db *sql.DB, dbName string, purgeAge, mailDeleteAge time.Duration, dryRun bool) (*PurgeResult, error) {
	result := &PurgeResult{Database: dbName, DryRun: dryRun}

	// Purge closed wisps.
	purged, anomalies, err := purgeClosedWisps(db, dbName, purgeAge, dryRun)
	if err != nil {
		return nil, fmt.Errorf("purge wisps: %w", err)
	}
	result.WispsPurged = purged
	result.Anomalies = append(result.Anomalies, anomalies...)

	// Purge old mail.
	mailPurged, err := purgeOldMail(db, dbName, mailDeleteAge, dryRun)
	if err != nil {
		return result, fmt.Errorf("purge mail: %w", err)
	}
	result.MailPurged = mailPurged

	return result, nil
}

func purgeClosedWisps(db *sql.DB, dbName string, purgeAge time.Duration, dryRun bool) (int, []Anomaly, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	deleteCutoff := time.Now().UTC().Add(-purgeAge)
	parentCheck := parentCheckWhere(dbName)
	var anomalies []Anomaly

	// Digest: count by wisp_type.
	digestQuery := fmt.Sprintf(
		"SELECT COALESCE(w.wisp_type, 'unknown') AS wtype, COUNT(*) AS cnt FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? AND %s GROUP BY wtype",
		dbName, parentCheck)
	rows, err := db.QueryContext(ctx, digestQuery, deleteCutoff)
	if err != nil {
		return 0, nil, fmt.Errorf("digest query: %w", err)
	}
	digestTotal := 0
	for rows.Next() {
		var wtype string
		var cnt int
		if err := rows.Scan(&wtype, &cnt); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("digest scan: %w", err)
		}
		digestTotal += cnt
	}
	rows.Close()

	if digestTotal == 0 {
		return 0, anomalies, nil
	}

	if dryRun {
		return digestTotal, anomalies, nil
	}

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, nil, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	// Batch delete.
	idQuery := fmt.Sprintf(
		"SELECT w.id FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? AND %s LIMIT %d",
		dbName, parentCheck, DefaultBatchSize)
	auxTables := []string{"wisp_labels", "wisp_comments", "wisp_events", "wisp_dependencies"}

	totalDeleted, err := batchDeleteRows(ctx, db, dbName, idQuery, deleteCutoff, "wisps", auxTables)
	if err != nil {
		return totalDeleted, anomalies, err
	}

	if totalDeleted > 0 {
		commitMsg := fmt.Sprintf("reaper: purge %d closed wisps from %s", totalDeleted, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg from safe values
			// Non-fatal — log but continue.
			anomalies = append(anomalies, Anomaly{
				Type:    "dolt_commit_failed",
				Message: fmt.Sprintf("dolt commit after purge failed: %v", err),
			})
		}
	}

	return totalDeleted, anomalies, nil
}

func purgeOldMail(db *sql.DB, dbName string, mailDeleteAge time.Duration, dryRun bool) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mailCutoff := time.Now().UTC().Add(-mailDeleteAge)

	countQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s`.issues WHERE status = 'closed' AND closed_at < ? AND id IN (SELECT issue_id FROM `%s`.labels WHERE label = 'gt:message')",
		dbName, dbName)
	var count int
	if err := db.QueryRowContext(ctx, countQuery, mailCutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("count mail: %w", err)
	}
	if count == 0 {
		return 0, nil
	}

	if dryRun {
		return count, nil
	}

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	idQuery := fmt.Sprintf(
		"SELECT i.id FROM `%s`.issues i INNER JOIN `%s`.labels l ON i.id = l.issue_id WHERE i.status = 'closed' AND i.closed_at < ? AND l.label = 'gt:message' LIMIT %d",
		dbName, dbName, DefaultBatchSize)
	auxTables := []string{"labels", "comments", "events", "dependencies"}

	totalDeleted, err := batchDeleteRows(ctx, db, dbName, idQuery, mailCutoff, "issues", auxTables)
	if err != nil {
		return totalDeleted, err
	}

	if totalDeleted > 0 {
		commitMsg := fmt.Sprintf("reaper: purge %d old mail from %s", totalDeleted, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg from safe values
			// Non-fatal.
		}
	}

	return totalDeleted, nil
}

// AutoClose closes issues that have been open with no updates past staleAge.
// Excludes P0/P1 priority, epics, and issues with active dependencies.
func AutoClose(db *sql.DB, dbName string, staleAge time.Duration, dryRun bool) (*AutoCloseResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTimeout)
	defer cancel()

	staleCutoff := time.Now().UTC().Add(-staleAge)
	result := &AutoCloseResult{Database: dbName, DryRun: dryRun}

	whereClause := fmt.Sprintf(`
		i.status IN ('open', 'in_progress')
		AND i.updated_at < ?
		AND i.priority > 1
		AND i.issue_type != 'epic'
		AND i.id NOT IN (
			SELECT DISTINCT d.issue_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues dep ON d.depends_on_id = dep.id
			WHERE dep.status IN ('open', 'in_progress')
		)
		AND i.id NOT IN (
			SELECT DISTINCT d.depends_on_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues blocker ON d.issue_id = blocker.id
			WHERE blocker.status IN ('open', 'in_progress')
		)`, dbName, dbName, dbName, dbName)

	// Two-step SELECT-then-UPDATE to avoid self-referencing subquery in UPDATE,
	// which is not valid MySQL (Error 1093) and fragile in Dolt (dolthub/dolt#10600).
	selectQuery := fmt.Sprintf("SELECT i.id FROM `%s`.issues i WHERE %s", dbName, whereClause)
	rows, err := db.QueryContext(ctx, selectQuery, staleCutoff)
	if err != nil {
		return nil, fmt.Errorf("select stale: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan stale id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	if dryRun {
		result.Closed = len(ids)
		return result, nil
	}

	if len(ids) == 0 {
		return result, nil
	}

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return nil, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	updateQuery := fmt.Sprintf(
		"UPDATE `%s`.issues SET status = 'closed', closed_at = NOW() WHERE id IN (%s)",
		dbName, strings.Join(placeholders, ","))
	if _, err := db.ExecContext(ctx, updateQuery, args...); err != nil {
		return nil, fmt.Errorf("auto-close: %w", err)
	}

	result.Closed = len(ids)

	if len(ids) > 0 {
		commitMsg := fmt.Sprintf("reaper: auto-close %d stale issues in %s", len(ids), dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg from safe values
			result.Anomalies = append(result.Anomalies, Anomaly{
				Type:    "dolt_commit_failed",
				Message: fmt.Sprintf("dolt commit after auto-close failed: %v", err),
			})
		}
	}

	return result, nil
}

// batchDeleteRows deletes rows from a primary table and its auxiliary tables in batches.
func batchDeleteRows(ctx context.Context, db *sql.DB, dbName string, idQuery string, cutoffArg time.Time, primaryTable string, auxTables []string) (int, error) {
	totalDeleted := 0
	for {
		idRows, err := db.QueryContext(ctx, idQuery, cutoffArg)
		if err != nil {
			return totalDeleted, fmt.Errorf("select batch: %w", err)
		}

		var ids []string
		for idRows.Next() {
			var id string
			if err := idRows.Scan(&id); err != nil {
				idRows.Close()
				return totalDeleted, fmt.Errorf("scan id: %w", err)
			}
			ids = append(ids, id)
		}
		idRows.Close()

		if len(ids) == 0 {
			break
		}

		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		inClause := "(" + strings.Join(placeholders, ",") + ")"

		for _, tbl := range auxTables {
			delAux := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE issue_id IN %s", dbName, tbl, inClause) //nolint:gosec // G201: dbName and tbl are internal
			if _, err := db.ExecContext(ctx, delAux, args...); err != nil {
				// Non-fatal: log and continue.
			}
		}

		// Clean up reverse dependency references to prevent dangling parent refs.
		delReverse := fmt.Sprintf("DELETE FROM `%s`.`wisp_dependencies` WHERE depends_on_id IN %s", dbName, inClause) //nolint:gosec // G201: internal
		if _, err := db.ExecContext(ctx, delReverse, args...); err != nil {
			// Non-fatal.
		}

		delPrimary := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE id IN %s", dbName, primaryTable, inClause) //nolint:gosec // G201: internal
		sqlResult, err := db.ExecContext(ctx, delPrimary, args...)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete %s batch: %w", primaryTable, err)
		}
		affected, _ := sqlResult.RowsAffected()
		totalDeleted += int(affected)
	}

	return totalDeleted, nil
}

// FormatJSON marshals any value to indented JSON.
func FormatJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
