package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultWispReaperInterval = 30 * time.Minute
	wispReaperQueryTimeout    = 30 * time.Second
	// Wisps older than this are reaped (closed).
	defaultWispMaxAge = 24 * time.Hour
	// Closed wisps older than this are permanently deleted.
	defaultWispDeleteAge = 7 * 24 * time.Hour
	// Issues with no status change older than this are auto-closed.
	defaultStaleIssueAge = 30 * 24 * time.Hour
	// Alert threshold: if open wisp count exceeds this, escalate.
	wispAlertThreshold = 500
	// Closed mail (gt:message) older than this is permanently deleted.
	defaultMailDeleteAge = 30 * 24 * time.Hour
	// Batch size for DELETE operations to avoid long-running transactions.
	deleteBatchSize = 100
)

// WispReaperConfig holds configuration for the wisp_reaper patrol.
type WispReaperConfig struct {
	// Enabled controls whether the reaper runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to run, as a string (e.g., "30m").
	IntervalStr string `json:"interval,omitempty"`

	// MaxAgeStr is how old a wisp must be before reaping (e.g., "24h").
	MaxAgeStr string `json:"max_age,omitempty"`

	// DeleteAgeStr is how long after closing before wisps are deleted (e.g., "168h" for 7 days).
	DeleteAgeStr string `json:"delete_age,omitempty"`

	// StaleIssueAgeStr is how long an issue can be unchanged before auto-close (e.g., "720h" for 30 days).
	StaleIssueAgeStr string `json:"stale_issue_age,omitempty"`

	// Databases lists specific database names to reap.
	// If empty, auto-discovers from dolt server.
	Databases []string `json:"databases,omitempty"`
}

// wispReaperInterval returns the configured interval, or the default (30m).
func wispReaperInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.WispReaper != nil {
		if config.Patrols.WispReaper.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.WispReaper.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultWispReaperInterval
}

// wispReaperMaxAge returns the configured max age, or the default (24h).
func wispReaperMaxAge(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.WispReaper != nil {
		if config.Patrols.WispReaper.MaxAgeStr != "" {
			if d, err := time.ParseDuration(config.Patrols.WispReaper.MaxAgeStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultWispMaxAge
}

// wispDeleteAge returns the configured delete age, or the default (7 days).
func wispDeleteAge(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.WispReaper != nil {
		if config.Patrols.WispReaper.DeleteAgeStr != "" {
			if d, err := time.ParseDuration(config.Patrols.WispReaper.DeleteAgeStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultWispDeleteAge
}

// staleIssueAge returns the configured stale issue age, or the default (30 days).
func staleIssueAge(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.WispReaper != nil {
		if config.Patrols.WispReaper.StaleIssueAgeStr != "" {
			if d, err := time.ParseDuration(config.Patrols.WispReaper.StaleIssueAgeStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultStaleIssueAge
}

// reapWisps closes stale wisps and purges old closed wisps across all databases.
// Tracks progress via mol-dog-reaper molecule lifecycle.
// Non-fatal: errors are logged but don't stop the daemon.
func (d *Daemon) reapWisps() {
	if !IsPatrolEnabled(d.patrolConfig, "wisp_reaper") {
		return
	}

	config := d.patrolConfig.Patrols.WispReaper
	maxAge := wispReaperMaxAge(d.patrolConfig)
	deleteAge := wispDeleteAge(d.patrolConfig)

	// Pour molecule to track this patrol cycle.
	mol := d.pourDogMolecule("mol-dog-reaper", map[string]string{
		"max_age":   maxAge.String(),
		"purge_age": deleteAge.String(),
	})
	defer mol.close()

	// --- SCAN STEP: discover databases and count candidates ---

	cutoff := time.Now().UTC().Add(-maxAge)
	deleteCutoff := time.Now().UTC().Add(-deleteAge)
	issueAge := staleIssueAge(d.patrolConfig)
	issueCutoff := time.Now().UTC().Add(-issueAge)

	databases := config.Databases
	if len(databases) == 0 {
		databases = d.discoverDoltDatabases()
	}
	if len(databases) == 0 {
		d.logger.Printf("wisp_reaper: no databases to reap")
		mol.failStep("scan", "no databases found")
		return
	}

	d.logger.Printf("wisp_reaper: scanning %d databases", len(databases))
	mol.closeStep("scan")

	// --- REAP STEP: close stale wisps ---

	totalReaped := 0
	totalOpen := 0
	reapErrors := 0

	for _, dbName := range databases {
		if !validDBName.MatchString(dbName) {
			d.logger.Printf("wisp_reaper: skipping invalid database name: %q", dbName)
			continue
		}

		reaped, open, err := d.reapWispsInDB(dbName, cutoff)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: close error: %v", dbName, err)
			reapErrors++
		} else {
			totalReaped += reaped
			totalOpen += open
			if reaped > 0 {
				d.logger.Printf("wisp_reaper: %s: closed %d stale wisps (older than %v), %d open remain",
					dbName, reaped, maxAge, open)
			}
		}

		// Auto-close stale issues (part of the reap phase).
		closed, err := d.autoCloseStaleIssuesInDB(dbName, issueCutoff)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: auto-close issues error: %v", dbName, err)
		} else if closed > 0 {
			totalReaped += closed
		}
	}

	if totalReaped > 0 {
		d.logger.Printf("wisp_reaper: total closed %d stale wisps across %d databases, %d open remain",
			totalReaped, len(databases), totalOpen)
	}

	if reapErrors > 0 {
		mol.failStep("reap", fmt.Sprintf("%d databases had reap errors", reapErrors))
	} else {
		mol.closeStep("reap")
	}

	// --- PURGE STEP: delete closed wisps older than purge age ---

	totalPurged := 0
	purgeErrors := 0

	for _, dbName := range databases {
		if !validDBName.MatchString(dbName) {
			continue
		}

		purged, err := d.purgeClosedWispsInDB(dbName, deleteCutoff)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: purge error: %v", dbName, err)
			purgeErrors++
		} else {
			totalPurged += purged
		}
	}

	if totalPurged > 0 {
		d.logger.Printf("wisp_reaper: total purged %d closed wisp rows across %d databases",
			totalPurged, len(databases))
	}

	if purgeErrors > 0 {
		mol.failStep("purge", fmt.Sprintf("%d databases had purge errors", purgeErrors))
	} else {
		mol.closeStep("purge")
	}

	// --- MAIL PURGE STEP: delete closed mail older than 30 days ---

	mailCutoff := time.Now().UTC().Add(-defaultMailDeleteAge)
	totalMailPurged := 0

	for _, dbName := range databases {
		if !validDBName.MatchString(dbName) {
			continue
		}
		purged, err := d.purgeOldMailInDB(dbName, mailCutoff)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: mail purge error: %v", dbName, err)
		} else {
			totalMailPurged += purged
		}
	}

	if totalMailPurged > 0 {
		d.logger.Printf("wisp_reaper: total purged %d old mail rows across %d databases",
			totalMailPurged, len(databases))
	}

	// --- REPORT STEP: log summary and alert ---

	if totalOpen > wispAlertThreshold {
		d.logger.Printf("wisp_reaper: WARNING: %d open wisps exceed threshold %d — investigate wisp lifecycle",
			totalOpen, wispAlertThreshold)
	}

	d.logger.Printf("wisp_reaper: cycle complete — reaped=%d purged=%d open=%d databases=%d",
		totalReaped, totalPurged, totalOpen, len(databases))

	mol.closeStep("report")
}

// reapWispsInDB closes stale wisps in a single database.
// Returns (reaped count, remaining open count, error).
func (d *Daemon) reapWispsInDB(dbName string, cutoff time.Time) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wispReaperQueryTimeout)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Close stale open wisps (status=open, created before cutoff).
	// Also close stale hooked/in_progress wisps — these are abandoned molecule steps.
	closeQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name, not user input
		"UPDATE `%s`.wisps SET status='closed', closed_at=NOW() WHERE status IN ('open', 'hooked', 'in_progress') AND created_at < ?",
		dbName)
	result, err := db.ExecContext(ctx, closeQuery, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("close stale wisps: %w", err)
	}

	reaped, _ := result.RowsAffected()

	// Count remaining open wisps.
	var openCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName is an internal Dolt database name, not user input
	if err := db.QueryRowContext(ctx, countQuery).Scan(&openCount); err != nil {
		return int(reaped), 0, fmt.Errorf("count open wisps: %w", err)
	}

	return int(reaped), openCount, nil
}

// purgeClosedWispsInDB deletes closed wisp rows (and their auxiliary data) older than
// the delete cutoff. Deletes in batches to avoid long-running transactions.
// Returns the number of wisp rows deleted.
func (d *Daemon) purgeClosedWispsInDB(dbName string, deleteCutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Digest: count closed wisps eligible for deletion, grouped by wisp_type.
	digestQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name, not user input
		"SELECT COALESCE(wisp_type, 'unknown') AS wtype, COUNT(*) AS cnt FROM `%s`.wisps WHERE status = 'closed' AND closed_at < ? GROUP BY wtype",
		dbName)
	rows, err := db.QueryContext(ctx, digestQuery, deleteCutoff)
	if err != nil {
		return 0, fmt.Errorf("digest query: %w", err)
	}
	digestTotal := 0
	for rows.Next() {
		var wtype string
		var cnt int
		if err := rows.Scan(&wtype, &cnt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("digest scan: %w", err)
		}
		if cnt > 0 {
			d.logger.Printf("wisp_reaper: %s: delete digest: type=%s count=%d", dbName, wtype, cnt)
		}
		digestTotal += cnt
	}
	rows.Close()

	if digestTotal == 0 {
		return 0, nil
	}

	d.logger.Printf("wisp_reaper: %s: deleting %d closed wisp rows (closed before %v)",
		dbName, digestTotal, deleteCutoff.Format(time.RFC3339))

	// Batch delete: select IDs, delete aux tables first, then wisps.
	totalDeleted := 0
	for {
		// Get a batch of IDs to delete.
		idQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name, not user input
			"SELECT id FROM `%s`.wisps WHERE status = 'closed' AND closed_at < ? LIMIT %d",
			dbName, deleteBatchSize)
		idRows, err := db.QueryContext(ctx, idQuery, deleteCutoff)
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

		// Build IN clause with placeholders.
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		inClause := "(" + joinStrings(placeholders, ",") + ")"

		// Delete from auxiliary tables first (foreign key safety).
		auxTables := []string{"wisp_labels", "wisp_comments", "wisp_events", "wisp_dependencies"}
		for _, tbl := range auxTables {
			delAux := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE issue_id IN %s", dbName, tbl, inClause) //nolint:gosec // G201: dbName and tbl are internal constants, inClause is placeholders
			if _, err := db.ExecContext(ctx, delAux, args...); err != nil {
				// Log but continue — table might not exist in all databases.
				d.logger.Printf("wisp_reaper: %s: delete from %s: %v", dbName, tbl, err)
			}
		}

		// Delete the wisp rows themselves.
		delWisps := fmt.Sprintf("DELETE FROM `%s`.wisps WHERE id IN %s", dbName, inClause) //nolint:gosec // G201: dbName is an internal Dolt database name, inClause is placeholders
		result, err := db.ExecContext(ctx, delWisps, args...)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete wisps batch: %w", err)
		}
		affected, _ := result.RowsAffected()
		totalDeleted += int(affected)
	}

	if totalDeleted > 0 {
		d.logger.Printf("wisp_reaper: %s: deleted %d closed wisp rows and associated data",
			dbName, totalDeleted)
	}

	return totalDeleted, nil
}

// purgeOldMailInDB deletes closed mail (gt:message labeled) issues older than the
// mail cutoff. Skips open/unread mail so messages to parked rigs don't vanish.
// Deletes in batches following the same pattern as purgeClosedWispsInDB.
func (d *Daemon) purgeOldMailInDB(dbName string, mailCutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Count eligible mail for logging.
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

	d.logger.Printf("wisp_reaper: %s: deleting %d closed mail rows older than %v",
		dbName, count, mailCutoff.Format(time.RFC3339))

	// Batch delete: same pattern as wisp purge.
	totalDeleted := 0
	for {
		idQuery := fmt.Sprintf(
			"SELECT i.id FROM `%s`.issues i INNER JOIN `%s`.labels l ON i.id = l.issue_id WHERE i.status = 'closed' AND i.closed_at < ? AND l.label = 'gt:message' LIMIT %d",
			dbName, dbName, deleteBatchSize)
		idRows, err := db.QueryContext(ctx, idQuery, mailCutoff)
		if err != nil {
			return totalDeleted, fmt.Errorf("select mail batch: %w", err)
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
		inClause := "(" + joinStrings(placeholders, ",") + ")"

		// Delete from auxiliary tables first.
		auxTables := []string{"labels", "comments", "events", "dependencies"}
		for _, tbl := range auxTables {
			delAux := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE issue_id IN %s", dbName, tbl, inClause)
			if _, err := db.ExecContext(ctx, delAux, args...); err != nil {
				d.logger.Printf("wisp_reaper: %s: mail delete from %s: %v", dbName, tbl, err)
			}
		}

		// Delete the issue rows.
		delIssues := fmt.Sprintf("DELETE FROM `%s`.issues WHERE id IN %s", dbName, inClause)
		result, err := db.ExecContext(ctx, delIssues, args...)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete mail batch: %w", err)
		}
		affected, _ := result.RowsAffected()
		totalDeleted += int(affected)
	}

	if totalDeleted > 0 {
		d.logger.Printf("wisp_reaper: %s: deleted %d old mail rows and associated data",
			dbName, totalDeleted)
	}

	return totalDeleted, nil
}

// autoCloseStaleIssuesInDB closes issues that have been open with no status change
// for longer than the stale cutoff. Excludes:
//   - P0 and P1 issues (priority <= 1)
//   - Epics (issue_type = 'epic')
//   - Issues with active dependencies (blocking or blocked-by open issues)
//
// Logs each closure individually with issue ID, title, age, and database.
// Returns the number of issues auto-closed.
func (d *Daemon) autoCloseStaleIssuesInDB(dbName string, staleCutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wispReaperQueryTimeout)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Check if issues table exists (not all databases have it).
	var dummy int
	checkQuery := fmt.Sprintf("SELECT 1 FROM `%s`.issues LIMIT 1", dbName) //nolint:gosec // G201: dbName is an internal Dolt database name
	if err := db.QueryRowContext(ctx, checkQuery).Scan(&dummy); err != nil {
		// Table doesn't exist or is empty — skip silently.
		return 0, nil
	}

	// Find stale issue candidates: open >30 days, priority > 1 (exempt P0/P1),
	// not epics, and not linked to any open issues via dependencies.
	// We SELECT first (instead of blind UPDATE) so we can log each closure individually.
	candidateQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name
		`SELECT id, title, updated_at FROM `+"`%s`"+`.issues
		WHERE status IN ('open', 'in_progress')
		AND updated_at < ?
		AND priority > 1
		AND issue_type != 'epic'
		AND id NOT IN (
			SELECT DISTINCT d.issue_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues i ON d.depends_on_id = i.id
			WHERE i.status IN ('open', 'in_progress')
		)
		AND id NOT IN (
			SELECT DISTINCT d.depends_on_id FROM `+"`%s`"+`.dependencies d
			INNER JOIN `+"`%s`"+`.issues i ON d.issue_id = i.id
			WHERE i.status IN ('open', 'in_progress')
		)`,
		dbName, dbName, dbName, dbName, dbName)

	rows, err := db.QueryContext(ctx, candidateQuery, staleCutoff)
	if err != nil {
		// Dependencies table might not exist — fall back to simpler query without dep check.
		return d.autoCloseStaleIssuesSimple(ctx, db, dbName, staleCutoff)
	}
	defer rows.Close()

	var candidates []struct {
		id        string
		title     string
		updatedAt time.Time
	}
	for rows.Next() {
		var c struct {
			id        string
			title     string
			updatedAt time.Time
		}
		if err := rows.Scan(&c.id, &c.title, &c.updatedAt); err != nil {
			return 0, fmt.Errorf("scan candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate candidates: %w", err)
	}

	if len(candidates) == 0 {
		return 0, nil
	}

	// Close each candidate and log individually.
	closed := 0
	now := time.Now().UTC()
	for _, c := range candidates {
		closeQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name
			"UPDATE `%s`.issues SET status='closed', closed_at=NOW(), close_reason='stale:auto-closed by reaper' WHERE id = ? AND status IN ('open', 'in_progress')",
			dbName)
		result, err := db.ExecContext(ctx, closeQuery, c.id)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: failed to auto-close %s: %v", dbName, c.id, err)
			continue
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			age := now.Sub(c.updatedAt).Truncate(time.Hour)
			d.logger.Printf("wisp_reaper: %s: auto-closed %s %q (age: %v)", dbName, c.id, c.title, age)
			closed++
		}
	}

	return closed, nil
}

// autoCloseStaleIssuesSimple is a fallback for databases without a dependencies table.
// Closes stale issues excluding epics and P0/P1, but cannot check dependencies.
func (d *Daemon) autoCloseStaleIssuesSimple(ctx context.Context, db *sql.DB, dbName string, staleCutoff time.Time) (int, error) {
	// Find candidates without dependency check.
	candidateQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name
		"SELECT id, title, updated_at FROM `%s`.issues WHERE status IN ('open', 'in_progress') AND updated_at < ? AND priority > 1 AND issue_type != 'epic'",
		dbName)

	rows, err := db.QueryContext(ctx, candidateQuery, staleCutoff)
	if err != nil {
		return 0, fmt.Errorf("auto-close stale issues (simple): %w", err)
	}
	defer rows.Close()

	closed := 0
	now := time.Now().UTC()
	for rows.Next() {
		var id, title string
		var updatedAt time.Time
		if err := rows.Scan(&id, &title, &updatedAt); err != nil {
			return closed, fmt.Errorf("scan candidate: %w", err)
		}

		closeQuery := fmt.Sprintf( //nolint:gosec // G201: dbName is an internal Dolt database name
			"UPDATE `%s`.issues SET status='closed', closed_at=NOW(), close_reason='stale:auto-closed by reaper' WHERE id = ? AND status IN ('open', 'in_progress')",
			dbName)
		result, err := db.ExecContext(ctx, closeQuery, id)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: failed to auto-close %s: %v", dbName, id, err)
			continue
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			age := now.Sub(updatedAt).Truncate(time.Hour)
			d.logger.Printf("wisp_reaper: %s: auto-closed %s %q (age: %v, no dep check)", dbName, id, title, age)
			closed++
		}
	}

	return closed, nil
}

// joinStrings joins strings with a separator. Simple helper to avoid importing strings.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}

// discoverDoltDatabases returns the list of known production databases.
// Hardcoded for now — matches the databases in daemon.json and dolt-data.
func (d *Daemon) discoverDoltDatabases() []string {
	return []string{"hq", "beads", "gastown"}
}

// doltServerPort returns the configured Dolt server port.
func (d *Daemon) doltServerPort() int {
	if d.doltServer != nil {
		return d.doltServer.config.Port
	}
	return 3307 // Default
}
