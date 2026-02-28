package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultCompactorDogInterval = 24 * time.Hour
	// defaultCompactorCommitThreshold is the minimum commit count before compaction triggers.
	// 500 commits is a reasonable daily threshold — prevents unbounded growth
	// without compacting too aggressively. Configurable via daemon.json.
	defaultCompactorCommitThreshold = 500
	// compactorQueryTimeout is the timeout for individual SQL queries during compaction.
	compactorQueryTimeout = 30 * time.Second
	// compactorGCTimeout is the timeout for dolt gc after compaction.
	compactorGCTimeout = 5 * time.Minute
	// compactorBranchName is the temporary branch used during compaction.
	compactorBranchName = "gt-compaction"
)

// CompactorDogConfig holds configuration for the compactor_dog patrol.
type CompactorDogConfig struct {
	Enabled     bool     `json:"enabled"`
	IntervalStr string   `json:"interval,omitempty"`
	// Threshold is the minimum commit count before compaction triggers.
	// Defaults to 500 if not set.
	Threshold int `json:"threshold,omitempty"`
	// Databases lists specific database names to compact.
	// If empty, falls back to wisp_reaper config, then auto-discovery.
	Databases []string `json:"databases,omitempty"`
}

// compactorDogInterval returns the configured interval, or the default (24h).
func compactorDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.CompactorDog != nil {
		if config.Patrols.CompactorDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.CompactorDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultCompactorDogInterval
}

// compactorDogThreshold returns the configured commit threshold, or the default (500).
func compactorDogThreshold(config *DaemonPatrolConfig) int {
	if config != nil && config.Patrols != nil && config.Patrols.CompactorDog != nil {
		if config.Patrols.CompactorDog.Threshold > 0 {
			return config.Patrols.CompactorDog.Threshold
		}
	}
	return defaultCompactorCommitThreshold
}

// runCompactorDog checks each production database's commit count and
// flattens any that exceed the threshold. The flatten algorithm:
//  1. Record main HEAD hash and row counts (pre-flight)
//  2. Create temp branch gt-compaction from main
//  3. Soft-reset to root commit (keeps all data staged)
//  4. Commit all data as single commit
//  5. Verify row counts match (integrity check)
//  6. Move main to the new single commit
//  7. Delete temp branch
//
// After successful compaction, runs dolt gc to reclaim unreferenced chunks.
// Order matters: rebase first (compaction), gc second.
//
// Concurrency safety: if main HEAD moves during compaction, abort.
func (d *Daemon) runCompactorDog() {
	if !IsPatrolEnabled(d.patrolConfig, "compactor_dog") {
		return
	}

	threshold := compactorDogThreshold(d.patrolConfig)
	d.logger.Printf("compactor_dog: starting compaction cycle (threshold=%d)", threshold)

	mol := d.pourDogMolecule("mol-dog-compactor", nil)
	defer mol.close()

	databases := d.compactorDatabases()
	if len(databases) == 0 {
		d.logger.Printf("compactor_dog: no databases to compact")
		mol.failStep("scan", "no databases found")
		return
	}

	compacted := 0
	skipped := 0
	errors := 0

	for _, dbName := range databases {
		commitCount, err := d.compactorCountCommits(dbName)
		if err != nil {
			d.logger.Printf("compactor_dog: %s: error counting commits: %v", dbName, err)
			errors++
			continue
		}

		if commitCount < threshold {
			d.logger.Printf("compactor_dog: %s: %d commits (below threshold %d), skipping",
				dbName, commitCount, threshold)
			skipped++
			continue
		}

		d.logger.Printf("compactor_dog: %s: %d commits (threshold %d) — compacting",
			dbName, commitCount, threshold)

		if err := d.compactDatabase(dbName); err != nil {
			d.logger.Printf("compactor_dog: %s: compaction FAILED: %v", dbName, err)
			d.escalate("compactor_dog", fmt.Sprintf("Compaction failed for %s: %v", dbName, err))
			errors++
		} else {
			compacted++
			// Run gc after successful compaction to reclaim unreferenced chunks.
			// Order matters: rebase first (compactDatabase), gc second.
			if err := d.compactorRunGC(dbName); err != nil {
				d.logger.Printf("compactor_dog: %s: gc after compaction failed: %v", dbName, err)
			}
		}
	}

	if errors > 0 {
		mol.failStep("compact", fmt.Sprintf("%d databases had errors", errors))
	} else {
		mol.closeStep("compact")
	}

	d.logger.Printf("compactor_dog: cycle complete — compacted=%d skipped=%d errors=%d",
		compacted, skipped, errors)
	mol.closeStep("report")
}

// compactorDatabases returns the list of databases to consider for compaction.
// Checks its own config first, falls back to wisp_reaper config, then auto-discovery.
func (d *Daemon) compactorDatabases() []string {
	if d.patrolConfig != nil && d.patrolConfig.Patrols != nil {
		if cd := d.patrolConfig.Patrols.CompactorDog; cd != nil {
			if len(cd.Databases) > 0 {
				return cd.Databases
			}
		}
		if d.patrolConfig.Patrols.WispReaper != nil {
			if dbs := d.patrolConfig.Patrols.WispReaper.Databases; len(dbs) > 0 {
				return dbs
			}
		}
	}
	return d.discoverDoltDatabases()
}

// compactorCountCommits counts the number of commits in the database's dolt_log.
func (d *Daemon) compactorCountCommits(dbName string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	db, err := d.compactorOpenDB(dbName)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.dolt_log", dbName)
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count dolt_log: %w", err)
	}
	return count, nil
}

// compactDatabase performs the full flatten operation on a single database.
// This is the core compaction algorithm, also used by `gt dolt flatten`.
func (d *Daemon) compactDatabase(dbName string) error {
	db, err := d.compactorOpenDB(dbName)
	if err != nil {
		return err
	}
	defer db.Close()

	// Step 1: Record pre-flight state — main HEAD hash and row counts.
	preHead, err := d.compactorGetHead(db, dbName)
	if err != nil {
		return fmt.Errorf("pre-flight HEAD: %w", err)
	}
	preCounts, err := d.compactorGetRowCounts(db, dbName)
	if err != nil {
		return fmt.Errorf("pre-flight row counts: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: pre-flight HEAD=%s, tables=%d", dbName, preHead[:8], len(preCounts))

	// Step 2: Find the root commit (earliest in history).
	rootHash, err := d.compactorGetRootCommit(db, dbName)
	if err != nil {
		return fmt.Errorf("find root commit: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: root commit=%s", dbName, rootHash[:8])

	// Step 3: Create temporary compaction branch from main.
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, fmt.Sprintf("USE `%s`", dbName)); err != nil {
		return fmt.Errorf("use database: %w", err)
	}

	// Clean up any leftover compaction branch from a previous failed run.
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", compactorBranchName))

	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_CHECKOUT('-b', '%s')", compactorBranchName)); err != nil {
		return fmt.Errorf("create compaction branch: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: created branch %s", dbName, compactorBranchName)

	// Step 4: Soft-reset to root commit — all data remains staged.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_RESET('--soft', '%s')", rootHash)); err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("soft reset to root: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: soft-reset to root %s", dbName, rootHash[:8])

	// Step 5: Commit all data as a single commit.
	commitMsg := fmt.Sprintf("compaction: flatten %s history to single commit", dbName)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("commit flattened data: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: committed flattened data", dbName)

	// Step 6: Verify integrity — row counts must match pre-flight.
	postCounts, err := d.compactorGetRowCounts(db, dbName)
	if err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("post-compact row counts: %w", err)
	}

	for table, preCount := range preCounts {
		postCount, ok := postCounts[table]
		if !ok {
			d.compactorCleanup(db, dbName)
			return fmt.Errorf("integrity check: table %q missing after compaction", table)
		}
		if preCount != postCount {
			d.compactorCleanup(db, dbName)
			return fmt.Errorf("integrity check: table %q count mismatch: pre=%d post=%d", table, preCount, postCount)
		}
	}
	d.logger.Printf("compactor_dog: %s: integrity verified (%d tables match)", dbName, len(preCounts))

	// Step 7: Concurrency check — verify main hasn't moved.
	currentHead, err := d.compactorGetHead(db, dbName)
	if err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("concurrency check HEAD: %w", err)
	}
	if currentHead != preHead {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("concurrency abort: main HEAD moved from %s to %s during compaction", preHead[:8], currentHead[:8])
	}

	// Step 8: Switch back to main and hard-reset to the compacted commit.
	compactedHead, err := d.compactorGetCurrentHead(db)
	if err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("get compacted HEAD: %w", err)
	}

	if _, err := db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')"); err != nil {
		d.compactorCleanup(db, dbName)
		return fmt.Errorf("checkout main: %w", err)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_RESET('--hard', '%s')", compactedHead)); err != nil {
		return fmt.Errorf("reset main to compacted: %w", err)
	}
	d.logger.Printf("compactor_dog: %s: main reset to compacted commit %s", dbName, compactedHead[:8])

	// Step 9: Delete temp branch.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", compactorBranchName)); err != nil {
		d.logger.Printf("compactor_dog: %s: warning: failed to delete temp branch: %v", dbName, err)
	}

	// Step 10: Verify final commit count.
	finalCount, err := d.compactorCountCommits(dbName)
	if err != nil {
		d.logger.Printf("compactor_dog: %s: warning: could not verify final commit count: %v", dbName, err)
	} else {
		d.logger.Printf("compactor_dog: %s: compaction complete — %d commits remain", dbName, finalCount)
	}

	return nil
}

// compactorCleanup attempts to switch back to main and delete the temp branch.
// Called on error during compaction to leave the database in a clean state.
func (d *Daemon) compactorCleanup(db *sql.DB, dbName string) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	d.logger.Printf("compactor_dog: %s: cleaning up compaction branch", dbName)

	// Try to get back to main.
	_, _ = db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')")
	// Delete the compaction branch.
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", compactorBranchName))
}

// compactorOpenDB opens a connection to the Dolt server for the given database.
func (d *Daemon) compactorOpenDB(dbName string) (*sql.DB, error) {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s?parseTime=true&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"127.0.0.1", d.doltServerPort(), dbName)
	return sql.Open("mysql", dsn)
}

// compactorGetHead returns the current HEAD commit hash of the main branch.
func (d *Daemon) compactorGetHead(db *sql.DB, dbName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	var hash string
	query := fmt.Sprintf("SELECT DOLT_HASHOF('main') FROM `%s`.dual", dbName)
	if err := db.QueryRowContext(ctx, query).Scan(&hash); err != nil {
		// Fallback: try without dual table.
		query = fmt.Sprintf("SELECT commit_hash FROM `%s`.dolt_log ORDER BY date DESC LIMIT 1", dbName)
		if err := db.QueryRowContext(ctx, query).Scan(&hash); err != nil {
			return "", err
		}
	}
	return hash, nil
}

// compactorGetCurrentHead returns the HEAD commit hash of whatever branch is currently checked out.
func (d *Daemon) compactorGetCurrentHead(db *sql.DB) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	var hash string
	if err := db.QueryRowContext(ctx, "SELECT @@gastown_head_hash").Scan(&hash); err != nil {
		// Fallback: use dolt_log to get current HEAD.
		if err := db.QueryRowContext(ctx, "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1").Scan(&hash); err != nil {
			return "", err
		}
	}
	return hash, nil
}

// compactorGetRootCommit returns the hash of the earliest commit in the database.
func (d *Daemon) compactorGetRootCommit(db *sql.DB, dbName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	var hash string
	query := fmt.Sprintf("SELECT commit_hash FROM `%s`.dolt_log ORDER BY date ASC LIMIT 1", dbName)
	if err := db.QueryRowContext(ctx, query).Scan(&hash); err != nil {
		return "", err
	}
	return hash, nil
}

// compactorGetRowCounts returns a map of table -> row count for all user tables.
func (d *Daemon) compactorGetRowCounts(db *sql.DB, dbName string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), compactorQueryTimeout)
	defer cancel()

	// Get list of user tables (excluding dolt system tables).
	query := fmt.Sprintf("SELECT table_name FROM information_schema.tables WHERE table_schema = '%s' AND table_name NOT LIKE 'dolt_%%'", dbName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s`", dbName, table)
		if err := db.QueryRowContext(ctx, countQuery).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}

	return counts, nil
}

// compactorRunGC runs dolt gc on a single database directory after compaction.
// GC reclaims unreferenced chunks left behind by the rebase operation.
func (d *Daemon) compactorRunGC(dbName string) error {
	var dataDir string
	if d.doltServer != nil && d.doltServer.IsEnabled() && d.doltServer.config.DataDir != "" {
		dataDir = d.doltServer.config.DataDir
	} else {
		dataDir = filepath.Join(d.config.TownRoot, ".dolt-data")
	}

	dbDir := filepath.Join(dataDir, dbName)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		return fmt.Errorf("database directory %s does not exist", dbDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), compactorGCTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "dolt", "gc")
	cmd.Dir = dbDir

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			d.logger.Printf("compactor_dog: gc: %s: TIMEOUT after %v", dbName, elapsed)
			return fmt.Errorf("gc timeout after %v", elapsed)
		}
		errMsg := strings.TrimSpace(string(output))
		d.logger.Printf("compactor_dog: gc: %s: failed after %v: %v (%s)", dbName, elapsed, err, errMsg)
		return fmt.Errorf("%v: %s", err, errMsg)
	}

	d.logger.Printf("compactor_dog: gc: %s: completed in %v", dbName, elapsed)
	return nil
}
