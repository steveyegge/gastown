package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	// Issues stale longer than this are auto-closed.
	defaultStaleIssueAge = 30 * 24 * time.Hour
	// Batch size for DELETE operations to avoid long-running transactions.
	deleteBatchSize = 100
)

// WispReaperConfig holds configuration for the wisp_reaper patrol.
// The reaper is restricted to the wisps table only — it never touches issues
// (except for mail purge and stale-issue auto-close).
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

// reaperCycle holds the shared state for a single reaper cycle.
// Passed through the step functions to avoid long parameter lists.
type reaperCycle struct {
	databases  []string
	maxAge     time.Duration
	deleteAge  time.Duration
	dryRun     bool
	cutoff     time.Time
	deleteCutoff time.Time

	// Accumulated results for the report step.
	totalReaped     int
	totalOpen       int
	totalPurged     int
	totalMailPurged int
	totalAutoClosed int
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

// reapWisps is the thin orchestrator for the wisp_reaper patrol.
// It pours a mol-dog-reaper molecule and delegates to step functions
// that mirror the formula: scan → reap → purge → auto-close → report.
func (d *Daemon) reapWisps() {
	if !IsPatrolEnabled(d.patrolConfig, "wisp_reaper") {
		return
	}

	config := d.patrolConfig.Patrols.WispReaper
	rc := &reaperCycle{
		maxAge:    wispReaperMaxAge(d.patrolConfig),
		deleteAge: wispDeleteAge(d.patrolConfig),
		dryRun:    config.DryRun,
	}
	rc.cutoff = time.Now().UTC().Add(-rc.maxAge)
	rc.deleteCutoff = time.Now().UTC().Add(-rc.deleteAge)

	mol := d.pourDogMolecule(constants.MolDogReaper, map[string]string{
		"max_age":   rc.maxAge.String(),
		"purge_age": rc.deleteAge.String(),
	})
	defer mol.close()

	if rc.dryRun {
		d.logger.Printf("wisp_reaper: DRY RUN — reporting only, no changes will be made")
	}

	// Step 1: Scan — discover databases.
	rc.databases = config.Databases
	if len(rc.databases) == 0 {
		rc.databases = d.discoverDoltDatabases()
	}
	if len(rc.databases) == 0 {
		d.logger.Printf("wisp_reaper: no databases to reap")
		mol.failStep("scan", "no databases found")
		return
	}
	d.logger.Printf("wisp_reaper: scanning %d databases", len(rc.databases))
	mol.closeStep("scan")

	// Step 2: Reap — close stale wisps.
	d.reaperReap(rc, mol)

	// Step 3: Purge — delete old closed wisps and mail.
	d.reaperPurge(rc, mol)

	// Step 4: Auto-close — close stale issues.
	d.reaperAutoClose(rc, mol)

	// Step 5: Report — log summary.
	d.reaperReport(rc, mol)
}

// reaperReap closes stale wisps whose parent molecule is already closed.
// Only wisps (ephemeral step tracking) are reaped — never issues.
func (d *Daemon) reaperReap(rc *reaperCycle, mol *dogMol) {
	reapErrors := 0

	for _, dbName := range rc.databases {
		if !validDBName.MatchString(dbName) {
			d.logger.Printf("wisp_reaper: skipping invalid database name: %q", dbName)
			continue
		}

		reaped, open, err := d.reapWispsInDB(dbName, rc.cutoff, rc.dryRun)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: close error: %v", dbName, err)
			reapErrors++
			continue
		}

		rc.totalReaped += reaped
		rc.totalOpen += open
		if reaped > 0 {
			prefix := ""
			if rc.dryRun {
				prefix = "[DRY RUN] would have "
			}
			d.logger.Printf("wisp_reaper: %s: %sclosed %d stale wisps (older than %v), %d open remain",
				dbName, prefix, reaped, rc.maxAge, open)
		}
	}

	if rc.totalReaped > 0 {
		prefix := ""
		if rc.dryRun {
			prefix = "[DRY RUN] would have "
		}
		d.logger.Printf("wisp_reaper: total %sclosed %d stale wisps across %d databases, %d open remain",
			prefix, rc.totalReaped, len(rc.databases), rc.totalOpen)
	}

	if reapErrors > 0 {
		mol.failStep("reap", fmt.Sprintf("%d databases had reap errors", reapErrors))
	} else {
		mol.closeStep("reap")
	}
}

// reaperPurge deletes old closed wisps and old closed mail across all databases.
func (d *Daemon) reaperPurge(rc *reaperCycle, mol *dogMol) {
	purgeErrors := 0

	for _, dbName := range rc.databases {
		if !validDBName.MatchString(dbName) {
			continue
		}

		purged, err := d.purgeClosedWispsInDB(dbName, rc.deleteCutoff, rc.dryRun)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: purge error: %v", dbName, err)
			purgeErrors++
		} else {
			rc.totalPurged += purged
		}
	}

	if rc.totalPurged > 0 {
		prefix := ""
		if rc.dryRun {
			prefix = "[DRY RUN] would have "
		}
		d.logger.Printf("wisp_reaper: total %spurged %d closed wisp rows across %d databases",
			prefix, rc.totalPurged, len(rc.databases))
	}

	// Mail purge: delete closed mail older than retention.
	mailCutoff := time.Now().UTC().Add(-defaultMailDeleteAge)
	for _, dbName := range rc.databases {
		if !validDBName.MatchString(dbName) {
			continue
		}
		purged, err := d.purgeOldMailInDB(dbName, mailCutoff)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: mail purge error: %v", dbName, err)
		} else {
			rc.totalMailPurged += purged
		}
	}

	if rc.totalMailPurged > 0 {
		d.logger.Printf("wisp_reaper: total purged %d old mail rows across %d databases",
			rc.totalMailPurged, len(rc.databases))
	}

	if purgeErrors > 0 {
		mol.failStep("purge", fmt.Sprintf("%d databases had purge errors", purgeErrors))
	} else {
		mol.closeStep("purge")
	}
}

// reaperAutoClose closes issues that have been open with no updates for >30 days.
// Excludes P0/P1 priority, epics, and issues with active dependencies.
func (d *Daemon) reaperAutoClose(rc *reaperCycle, mol *dogMol) {
	staleCutoff := time.Now().UTC().Add(-defaultStaleIssueAge)
	autoCloseErrors := 0

	for _, dbName := range rc.databases {
		if !validDBName.MatchString(dbName) {
			continue
		}

		closed, err := d.autoCloseStaleIssuesInDB(dbName, staleCutoff, rc.dryRun)
		if err != nil {
			d.logger.Printf("wisp_reaper: %s: auto-close error: %v", dbName, err)
			autoCloseErrors++
		} else {
			rc.totalAutoClosed += closed
		}
	}

	if rc.totalAutoClosed > 0 {
		prefix := ""
		if rc.dryRun {
			prefix = "[DRY RUN] would have "
		}
		d.logger.Printf("wisp_reaper: total %sauto-closed %d stale issues across %d databases",
			prefix, rc.totalAutoClosed, len(rc.databases))
	}

	if autoCloseErrors > 0 {
		mol.failStep("auto-close", fmt.Sprintf("%d databases had auto-close errors", autoCloseErrors))
	} else {
		mol.closeStep("auto-close")
	}
}

// reaperReport logs the cycle summary and alerts on high wisp counts.
func (d *Daemon) reaperReport(rc *reaperCycle, mol *dogMol) {
	if rc.totalOpen > wispAlertThreshold {
		d.logger.Printf("wisp_reaper: WARNING: %d open wisps exceed threshold %d — investigate wisp lifecycle",
			rc.totalOpen, wispAlertThreshold)
	}

	d.logger.Printf("wisp_reaper: cycle complete — reaped=%d purged=%d mail_purged=%d auto_closed=%d open=%d databases=%d dryRun=%v",
		rc.totalReaped, rc.totalPurged, rc.totalMailPurged, rc.totalAutoClosed, rc.totalOpen, len(rc.databases), rc.dryRun)

	mol.closeStep("report")
}

// --- Per-database step implementations ---

// reaperOpenDB opens a connection to the Dolt server for a given database.
func (d *Daemon) reaperOpenDB(dbName string, readTimeout, writeTimeout time.Duration) (*sql.DB, error) {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=%s&writeTimeout=%s",
		"127.0.0.1", d.doltServerPort(), dbName,
		fmt.Sprintf("%ds", int(readTimeout.Seconds())),
		fmt.Sprintf("%ds", int(writeTimeout.Seconds())))
	return sql.Open("mysql", dsn)
}

// parentCheckWhere returns the SQL WHERE fragment that restricts operations to
// wisps whose parent molecule is closed or that have no parent (orphans).
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
		)`, dbName, dbName, dbName)
}

// reapWispsInDB closes stale wisps in a single database.
// Only closes wisps whose parent molecule is already closed (proof the work completed).
// Wisps without a parent molecule (orphans) are also eligible after the age cutoff.
// Returns (reaped count, remaining open count, error).
func (d *Daemon) reapWispsInDB(dbName string, cutoff time.Time, dryRun bool) (int, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wispReaperQueryTimeout)
	defer cancel()

	db, err := d.reaperOpenDB(dbName, 10*time.Second, 10*time.Second)
	if err != nil {
		return 0, 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	parentCheck := parentCheckWhere(dbName)
	whereClause := fmt.Sprintf(
		"w.status IN ('open', 'hooked', 'in_progress') AND w.created_at < ? AND %s", parentCheck)

	if dryRun {
		var wouldReap int
		countEligible := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps w WHERE %s", dbName, whereClause)
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

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	closeQuery := fmt.Sprintf("UPDATE `%s`.wisps w SET w.status='closed', w.closed_at=NOW() WHERE %s", dbName, whereClause)
	result, err := db.ExecContext(ctx, closeQuery, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("close stale wisps: %w", err)
	}

	reaped, _ := result.RowsAffected()

	if reaped > 0 {
		commitMsg := fmt.Sprintf("reaper: close %d stale wisps in %s", reaped, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg is constructed from safe values
			d.logger.Printf("wisp_reaper: %s: dolt commit after reap failed: %v", dbName, err)
		}
	}

	var openCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.wisps WHERE status IN ('open', 'hooked', 'in_progress')", dbName) //nolint:gosec // G201: dbName is an internal Dolt database name
	if err := db.QueryRowContext(ctx, countQuery).Scan(&openCount); err != nil {
		return int(reaped), 0, fmt.Errorf("count open wisps: %w", err)
	}

	return int(reaped), openCount, nil
}

// purgeClosedWispsInDB deletes closed wisp rows (and their auxiliary data) older than
// the delete cutoff. Only purges wisps whose parent molecule is closed or that have
// no parent (orphans). Deletes in batches wrapped in a single Dolt commit.
func (d *Daemon) purgeClosedWispsInDB(dbName string, deleteCutoff time.Time, dryRun bool) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := d.reaperOpenDB(dbName, 30*time.Second, 30*time.Second)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	parentCheck := parentCheckWhere(dbName)

	// Digest: count closed wisps eligible for deletion, grouped by wisp_type.
	digestQuery := fmt.Sprintf(
		"SELECT COALESCE(w.wisp_type, 'unknown') AS wtype, COUNT(*) AS cnt FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? AND %s GROUP BY wtype",
		dbName, parentCheck)
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

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	// Batch delete: select IDs, delete aux tables first, then wisps.
	idQuery := fmt.Sprintf(
		"SELECT w.id FROM `%s`.wisps w WHERE w.status = 'closed' AND w.closed_at < ? AND %s LIMIT %d",
		dbName, parentCheck, deleteBatchSize)
	auxTables := []string{"wisp_labels", "wisp_comments", "wisp_events", "wisp_dependencies"}

	totalDeleted, err := d.batchDeleteRows(ctx, db, dbName, idQuery, deleteCutoff, "wisps", auxTables)
	if err != nil {
		return totalDeleted, err
	}

	if totalDeleted > 0 {
		commitMsg := fmt.Sprintf("reaper: purge %d closed wisps from %s", totalDeleted, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil {
			d.logger.Printf("wisp_reaper: %s: dolt commit after purge failed: %v", dbName, err)
		}
		d.logger.Printf("wisp_reaper: %s: deleted %d closed wisp rows and associated data",
			dbName, totalDeleted)
	}

	return totalDeleted, nil
}

// purgeOldMailInDB deletes closed mail (gt:message labeled) issues older than the
// mail cutoff. Skips open/unread mail so messages to parked rigs don't vanish.
// All deletes are wrapped in a single Dolt commit.
func (d *Daemon) purgeOldMailInDB(dbName string, mailCutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := d.reaperOpenDB(dbName, 30*time.Second, 30*time.Second)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

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

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	idQuery := fmt.Sprintf(
		"SELECT i.id FROM `%s`.issues i INNER JOIN `%s`.labels l ON i.id = l.issue_id WHERE i.status = 'closed' AND i.closed_at < ? AND l.label = 'gt:message' LIMIT %d",
		dbName, dbName, deleteBatchSize)
	auxTables := []string{"labels", "comments", "events", "dependencies"}

	totalDeleted, err := d.batchDeleteRows(ctx, db, dbName, idQuery, mailCutoff, "issues", auxTables)
	if err != nil {
		return totalDeleted, err
	}

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

// autoCloseStaleIssuesInDB closes issues that have been open with no updates for >30 days.
// Excludes P0/P1 priority, epics, and issues with active dependencies (blocking or blocked-by
// open issues). Returns the number of issues auto-closed.
func (d *Daemon) autoCloseStaleIssuesInDB(dbName string, staleCutoff time.Time, dryRun bool) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wispReaperQueryTimeout)
	defer cancel()

	db, err := d.reaperOpenDB(dbName, 10*time.Second, 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	// Find stale issues: open >30 days, not updated, not P0/P1, not epic,
	// no active dependencies (neither blocking nor blocked-by open issues).
	query := fmt.Sprintf(`
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

	var count int
	if err := db.QueryRowContext(ctx, query, staleCutoff).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stale issues: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	if dryRun {
		d.logger.Printf("wisp_reaper: %s: [DRY RUN] would auto-close %d stale issues", dbName, count)
		return count, nil
	}

	d.logger.Printf("wisp_reaper: %s: auto-closing %d stale issues (no updates since %v)",
		dbName, count, staleCutoff.Format(time.RFC3339))

	if _, err := db.ExecContext(ctx, "SET @@autocommit = 0"); err != nil {
		return 0, fmt.Errorf("disable autocommit: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "SET @@autocommit = 1")
	}()

	updateQuery := fmt.Sprintf(`
		UPDATE `+"`%s`"+`.issues i
		SET i.status = 'closed', i.closed_at = NOW()
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

	result, err := db.ExecContext(ctx, updateQuery, staleCutoff)
	if err != nil {
		return 0, fmt.Errorf("auto-close stale issues: %w", err)
	}

	closed, _ := result.RowsAffected()

	if closed > 0 {
		commitMsg := fmt.Sprintf("reaper: auto-close %d stale issues in %s", closed, dbName)
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil { //nolint:gosec // G201: commitMsg is constructed from safe values
			d.logger.Printf("wisp_reaper: %s: dolt commit after auto-close failed: %v", dbName, err)
		}
		d.logger.Printf("wisp_reaper: %s: auto-closed %d stale issues", dbName, int(closed))
	}

	return int(closed), nil
}

// batchDeleteRows deletes rows from a primary table and its auxiliary tables in batches.
// The idQuery must SELECT a single id column and accept one time.Time parameter.
// auxTables are deleted first (foreign key safety), then the primary table.
// Caller is responsible for autocommit and Dolt commit.
func (d *Daemon) batchDeleteRows(ctx context.Context, db *sql.DB, dbName string, idQuery string, cutoffArg time.Time, primaryTable string, auxTables []string) (int, error) {
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
		inClause := "(" + joinStrings(placeholders, ",") + ")"

		for _, tbl := range auxTables {
			delAux := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE issue_id IN %s", dbName, tbl, inClause) //nolint:gosec // G201: dbName and tbl are internal constants, inClause is placeholders
			if _, err := db.ExecContext(ctx, delAux, args...); err != nil {
				d.logger.Printf("wisp_reaper: %s: delete from %s: %v", dbName, tbl, err)
			}
		}

		delPrimary := fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE id IN %s", dbName, primaryTable, inClause) //nolint:gosec // G201: dbName is an internal Dolt database name, inClause is placeholders
		result, err := db.ExecContext(ctx, delPrimary, args...)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete %s batch: %w", primaryTable, err)
		}
		affected, _ := result.RowsAffected()
		totalDeleted += int(affected)
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

// discoverDoltDatabases queries SHOW DATABASES on the Dolt server and returns
// all production databases, filtering out test pollution and system databases.
func (d *Daemon) discoverDoltDatabases() []string {
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/?parseTime=true&timeout=5s", d.doltServerPort())
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		d.logger.Printf("wisp_reaper: discoverDoltDatabases: open failed: %v, using fallback", err)
		return []string{"hq"}
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		d.logger.Printf("wisp_reaper: discoverDoltDatabases: query failed: %v, using fallback", err)
		return []string{"hq"}
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		// Skip system databases and test pollution.
		if name == "information_schema" || name == "mysql" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "testdb_") || strings.HasPrefix(lower, "beads_t") ||
			strings.HasPrefix(lower, "beads_pt") || strings.HasPrefix(lower, "doctest_") {
			continue
		}
		databases = append(databases, name)
	}

	if len(databases) == 0 {
		return []string{"hq"}
	}
	d.logger.Printf("wisp_reaper: discovered %d databases: %v", len(databases), databases)
	return databases
}

// doltServerPort returns the configured Dolt server port.
func (d *Daemon) doltServerPort() int {
	if d.doltServer != nil {
		return d.doltServer.config.Port
	}
	return 3307 // Default
}
