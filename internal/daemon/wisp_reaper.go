package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/steveyegge/gastown/internal/constants"
)

const (
	defaultWispReaperInterval = 30 * time.Minute
	wispReaperQueryTimeout    = 30 * time.Second
	// Wisps older than this are reaped (closed).
	defaultWispMaxAge = 24 * time.Hour
	// Closed wisps older than this are permanently deleted.
	defaultWispDeleteAge = 7 * 24 * time.Hour
	// Alert threshold: if open wisp count exceeds this, escalate.
	wispAlertThreshold = 500
	// Closed mail (gt:message) older than this is permanently deleted.
	defaultMailDeleteAge = 7 * 24 * time.Hour
	// Batch size for DELETE operations to avoid long-running transactions.
	deleteBatchSize = 100
)

// WispReaperConfig holds configuration for the wisp_reaper patrol.
// The reaper is restricted to the wisps table only — it never touches issues.
type WispReaperConfig struct {
	// Enabled controls whether the reaper runs.
	Enabled bool `json:"enabled"`

	// DryRun, when true, reports what would be reaped/purged without acting.
	DryRun bool `json:"dry_run,omitempty"`

	// IntervalStr is how often to run, as a string (e.g., "30m").
	IntervalStr string `json:"interval,omitempty"`

	// MaxAgeStr is how old a wisp must be before reaping (e.g., "24h").
	MaxAgeStr string `json:"max_age,omitempty"`

	// DeleteAgeStr is how long after closing before wisps are deleted (e.g., "168h" for 7 days).
	DeleteAgeStr string `json:"delete_age,omitempty"`

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
	dryRun := config.DryRun

	// Pour molecule to track this patrol cycle.
	mol := d.pourDogMolecule(constants.MolDogReaper, map[string]string{
		"max_age":   maxAge.String(),
		"purge_age": deleteAge.String(),
	})
	defer mol.close()

	if dryRun {
		d.logger.Printf("wisp_reaper: DRY RUN — reporting only, no changes will be made")
	}

	// --- SCAN STEP: discover databases and count candidates ---

	cutoff := time.Now().UTC().Add(-maxAge)
	deleteCutoff := time.Now().UTC().Add(-deleteAge)

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

	// --- REAP STEP: close stale wisps whose parent molecule is closed ---
	// Only wisps (ephemeral step tracking) are reaped — never issues.
	// A wisp is only eligible for reaping if its parent molecule is already
	// closed, proving the work completed.

	totalReaped := 0
	totalOpen := 0
	reapErrors := 0

	for _, dbName := range databases {
		if !validDBName.MatchString(dbName) {
			d.logger.Printf("wisp_reaper: skipping invalid database name: %q", dbName)
			continue
		}

		reaped, open, err := d.reapWispsInDB(dbName, cutoff, dryRun)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: close error: %v", dbName, err)
			reapErrors++
		} else {
			totalReaped += reaped
			totalOpen += open
			if reaped > 0 {
				prefix := ""
				if dryRun {
					prefix = "[DRY RUN] would have "
				}
				d.logger.Printf("wisp_reaper: %s: %sclosed %d stale wisps (older than %v), %d open remain",
					dbName, prefix, reaped, maxAge, open)
			}
		}
	}

	if totalReaped > 0 {
		prefix := ""
		if dryRun {
			prefix = "[DRY RUN] would have "
		}
		d.logger.Printf("wisp_reaper: total %sclosed %d stale wisps across %d databases, %d open remain",
			prefix, totalReaped, len(databases), totalOpen)
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

		purged, err := d.purgeClosedWispsInDB(dbName, deleteCutoff, dryRun)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: purge error: %v", dbName, err)
			purgeErrors++
		} else {
			totalPurged += purged
		}
	}

	if totalPurged > 0 {
		prefix := ""
		if dryRun {
			prefix = "[DRY RUN] would have "
		}
		d.logger.Printf("wisp_reaper: total %spurged %d closed wisp rows across %d databases",
			prefix, totalPurged, len(databases))
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

	d.logger.Printf("wisp_reaper: cycle complete — reaped=%d purged=%d mail_purged=%d open=%d databases=%d dryRun=%v",
		totalReaped, totalPurged, totalMailPurged, totalOpen, len(databases), dryRun)

	mol.closeStep("report")
}

// reapWispsInDB closes stale wisps in a single database.
// Only closes wisps whose parent molecule is already closed (proof the work completed).
// Wisps without a parent molecule (orphans) are also eligible after the age cutoff.
// Never touches the issues table.
// Returns (reaped count, remaining open count, error).
func (d *Daemon) reapWispsInDB(dbName string, cutoff time.Time, dryRun bool) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wispReaperQueryTimeout)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Close stale open wisps whose parent molecule is already closed,
	// OR that have no parent molecule (orphan wisps).
	// A wisp is a child if wisp_dependencies has a row with
	// issue_id=<wisp> and type='parent-child'. The depends_on_id is the parent.
	//
	// Eligible wisps: stale AND (parent is closed OR no parent exists).
	parentCheckSQL := fmt.Sprintf(`
		w.status IN ('open', 'hooked', 'in_progress')
		AND w.created_at < ?
		AND (
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
		)`, dbName, dbName, dbName)

	if dryRun {
		// In dry-run mode, count what would be reaped instead of updating.
		countEligible := fmt.Sprintf(
			"SELECT COUNT(*) FROM `%s`.wisps w WHERE %s",
			dbName, parentCheckSQL)
		var wouldReap int
		if err := db.QueryRowContext(ctx, countEligible, cutoff).Scan(&wouldReap); err != nil {
			return 0, 0, fmt.Errorf("dry-run count stale wisps: %w", err)
		}

		var openCount int
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName is an internal Dolt database name
		if err := db.QueryRowContext(ctx, countQuery).Scan(&openCount); err != nil {
			return wouldReap, 0, fmt.Errorf("count open wisps: %w", err)
		}
		return wouldReap, openCount, nil
	}

	// Disable auto-commit so the close operation produces a single Dolt commit.
	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	closeQuery := fmt.Sprintf(
		"UPDATE `%s`.wisps w SET w.status='closed', w.closed_at=NOW() WHERE %s",
		dbName, parentCheckSQL)

	result, err := db.ExecContext(ctx, closeQuery, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("close stale wisps: %w", err)
	}

	reaped, _ := result.RowsAffected()

	// Commit the close operation as a single Dolt commit.
	if reaped > 0 {
		commitMsg := fmt.Sprintf("reaper: close %d stale wisps in %s", reaped, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg is constructed from safe values
			d.logger.Printf("wisp_reaper: %s: dolt commit after reap failed: %v", dbName, err)
		}
	}

	// Count remaining open wisps.
	var openCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName is an internal Dolt database name
	if err := db.QueryRowContext(ctx, countQuery).Scan(&openCount); err != nil {
		return int(reaped), 0, fmt.Errorf("count open wisps: %w", err)
	}

	return int(reaped), openCount, nil
}

// purgeClosedWispsInDB deletes closed wisp rows (and their auxiliary data) older than
// the delete cutoff. Only purges wisps whose parent molecule is closed or that have
// no parent (orphans). Deletes in batches to avoid long-running transactions.
// All deletes are wrapped in a single Dolt commit to minimize commit graph growth.
// Returns the number of wisp rows deleted.
func (d *Daemon) purgeClosedWispsInDB(dbName string, deleteCutoff time.Time, dryRun bool) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"127.0.0.1", d.doltServerPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Parent molecule check SQL fragment: only target wisps whose parent
	// molecule is closed or that have no parent (orphans).
	parentCheckSQL := fmt.Sprintf(`
		AND (
			NOT EXISTS (
				SELECT 1 FROM `+"`%s`"+`.wisp_dependencies wd
				WHERE wd.issue_id = w.id AND wd.type = 'parent-child'
			)
			OR EXISTS (
				SELECT 1 FROM `+"`%s`"+`.wisp_dependencies wd
				JOIN `+"`%s`"+`.wisps parent ON parent.id = wd.depends_on_id
				WHERE wd.issue_id = w.id AND wd.type = 'parent-child'
				AND parent.status = 'closed'
			)
		)`, dbName, dbName, dbName)

	// Digest: count closed wisps eligible for deletion, grouped by wisp_type.
	digestQuery := fmt.Sprintf(
		"SELECT COALESCE(w.wisp_type, 'unknown') AS wtype, COUNT(*) AS cnt FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? %s GROUP BY wtype",
		dbName, parentCheckSQL)
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
			prefix := ""
			if dryRun {
				prefix = "[DRY RUN] "
			}
			d.logger.Printf("wisp_reaper: %s: %sdelete digest: type=%s count=%d", dbName, prefix, wtype, cnt)
		}
		digestTotal += cnt
	}
	rows.Close()

	if digestTotal == 0 {
		return 0, nil
	}

	if dryRun {
		d.logger.Printf("wisp_reaper: %s: [DRY RUN] would delete %d closed wisp rows (closed before %v)",
			dbName, digestTotal, deleteCutoff.Format(time.RFC3339))
		return digestTotal, nil
	}

	d.logger.Printf("wisp_reaper: %s: deleting %d closed wisp rows (closed before %v)",
		dbName, digestTotal, deleteCutoff.Format(time.RFC3339))

	// Disable auto-commit so all batch deletes produce a single Dolt commit.
	// This prevents N*5 auto-commits (4 aux tables + 1 wisps per batch) from
	// bloating the commit graph that the Compactor must later flatten.
	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		// Re-enable auto-commit on exit regardless of outcome.
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	// Batch delete: select IDs, delete aux tables first, then wisps.
	// Only select wisps whose parent molecule is closed or that have no parent.
	totalDeleted := 0
	for {
		idQuery := fmt.Sprintf(
			"SELECT w.id FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? %s LIMIT %d",
			dbName, parentCheckSQL, deleteBatchSize)
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

	// Commit all deletes as a single Dolt commit to keep the commit graph clean.
	if totalDeleted > 0 {
		commitMsg := fmt.Sprintf("reaper: purge %d closed wisps from %s", totalDeleted, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil {
			d.logger.Printf("wisp_reaper: %s: dolt commit after purge failed: %v", dbName, err)
			// Deletes still happened in the working set; auto-commit re-enable will capture them.
		}
		d.logger.Printf("wisp_reaper: %s: deleted %d closed wisp rows and associated data",
			dbName, totalDeleted)
	}

	return totalDeleted, nil
}

// purgeOldMailInDB deletes closed mail (gt:message labeled) issues older than the
// mail cutoff. Skips open/unread mail so messages to parked rigs don't vanish.
// Deletes in batches following the same pattern as purgeClosedWispsInDB.
// All deletes are wrapped in a single Dolt commit to minimize commit graph growth.
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

	// Disable auto-commit so all batch deletes produce a single Dolt commit.
	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

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

	// Commit all deletes as a single Dolt commit.
	if totalDeleted > 0 {
		commitMsg := fmt.Sprintf("reaper: purge %d old mail from %s", totalDeleted, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil {
			d.logger.Printf("wisp_reaper: %s: dolt commit after mail purge failed: %v", dbName, err)
		}
		d.logger.Printf("wisp_reaper: %s: deleted %d old mail rows and associated data",
			dbName, totalDeleted)
	}

	return totalDeleted, nil
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
