package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	doltRebaseConfirm    bool
	doltRebaseKeepRecent int
	doltRebaseDryRun     bool
)

var doltRebaseCmd = &cobra.Command{
	Use:   "rebase <database>",
	Short: "Surgical compaction: squash old commits, keep recent ones",
	Long: `Surgically compact a Dolt database using interactive rebase.

Unlike 'gt dolt flatten' (which destroys ALL history), surgical rebase
keeps recent commits individual while squashing old history into one.

Algorithm (based on Dolt's DOLT_REBASE):
  1. Creates anchor branch at root commit
  2. Creates work branch from main
  3. Starts interactive rebase — populates dolt_rebase table
  4. Marks old commits as 'squash', keeps recent N as 'pick'
  5. Executes the rebase plan
  6. Swaps branches: work becomes the new main
  7. Cleans up temporary branches
  8. Runs GC to reclaim space

WARNING: DOLT_REBASE is NOT safe with concurrent writes. If agents are
actively committing to this database, the rebase may fail with a graph-change
error. The Compactor Dog (daemon) has automatic retry logic for this case.
For manual use, re-run the command if it fails due to concurrent writes.
Flatten mode (gt dolt flatten) is safe with concurrent writes.

Use --keep-recent to control how many recent commits to preserve.
Use --dry-run to see the plan without executing it.

Requires --yes-i-am-sure flag as safety interlock.`,
	Args: cobra.ExactArgs(1),
	RunE: runDoltRebase,
}

func init() {
	doltRebaseCmd.Flags().BoolVar(&doltRebaseConfirm, "yes-i-am-sure", false,
		"Required safety flag to confirm compaction")
	doltRebaseCmd.Flags().IntVar(&doltRebaseKeepRecent, "keep-recent", 50,
		"Number of recent commits to keep as individual picks")
	doltRebaseCmd.Flags().BoolVar(&doltRebaseDryRun, "dry-run", false,
		"Show the rebase plan without executing it")
	doltCmd.AddCommand(doltRebaseCmd)
}

func runDoltRebase(cmd *cobra.Command, args []string) error {
	dbName := args[0]

	if !doltRebaseConfirm && !doltRebaseDryRun {
		return fmt.Errorf("this command rewrites commit history. Pass --yes-i-am-sure to proceed (or --dry-run to preview)")
	}

	if doltRebaseKeepRecent < 0 {
		return fmt.Errorf("--keep-recent must be non-negative (got %d)", doltRebaseKeepRecent)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	running, _, err := doltserver.IsRunning(townRoot)
	if err != nil || !running {
		return fmt.Errorf("Dolt server is not running — start with 'gt dolt start'")
	}

	config := doltserver.DefaultConfig(townRoot)
	dsn := fmt.Sprintf("%s@tcp(%s)/%s?parseTime=true&timeout=5s&readTimeout=60s&writeTimeout=300s",
		config.User, config.HostPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("connecting to database %s: %w", dbName, err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Verify database exists.
	var dummy int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&dummy); err != nil {
		return fmt.Errorf("database %q not reachable: %w", dbName, err)
	}

	fmt.Printf("%s Pre-flight checks for %s (surgical rebase)\n", style.Bold.Render("●"), style.Bold.Render(dbName))

	// Count commits.
	var commitCount int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.dolt_log", dbName)).Scan(&commitCount); err != nil {
		return fmt.Errorf("counting commits: %w", err)
	}
	fmt.Printf("  Commits: %d\n", commitCount)
	fmt.Printf("  Keep recent: %d\n", doltRebaseKeepRecent)

	// Need at least 3 commits: root (pick) + at least 1 to squash + 1 to keep.
	minCommits := doltRebaseKeepRecent + 2
	if commitCount < minCommits {
		fmt.Printf("  %s Too few commits (%d) for surgical rebase with --keep-recent=%d (need at least %d)\n",
			style.Bold.Render("✓"), commitCount, doltRebaseKeepRecent, minCommits)
		return nil
	}

	// Record pre-flight row counts and table schemas.
	preCounts, err := flattenGetRowCounts(db, dbName)
	if err != nil {
		return fmt.Errorf("recording row counts: %w", err)
	}
	preSchemas, err := rebaseGetTableSchemas(db, dbName)
	if err != nil {
		return fmt.Errorf("recording table schemas: %w", err)
	}
	fmt.Printf("  Tables: %d\n", len(preCounts))
	for table, count := range preCounts {
		fmt.Printf("    %s: %d rows\n", table, count)
	}

	// Get HEAD hash for concurrency check.
	preHead, err := flattenGetHead(db, dbName)
	if err != nil {
		return fmt.Errorf("getting HEAD: %w", err)
	}
	fmt.Printf("  HEAD: %s\n", preHead[:12])

	// Get root commit.
	var rootHash string
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT commit_hash FROM `%s`.dolt_log ORDER BY date ASC LIMIT 1", dbName)).Scan(&rootHash); err != nil {
		return fmt.Errorf("finding root commit: %w", err)
	}
	fmt.Printf("  Root: %s\n", rootHash[:12])

	// USE database for all subsequent operations.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("USE `%s`", dbName)); err != nil {
		return fmt.Errorf("use database: %w", err)
	}

	const baseBranch = "compact-base"
	const workBranch = "compact-work"

	// Clean up any leftover branches from a previous failed run.
	rebaseCleanup(db, baseBranch, workBranch)

	fmt.Printf("\n%s Starting surgical rebase...\n", style.Bold.Render("●"))

	// Step 1: Create anchor branch at root commit.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('%s', '%s')", baseBranch, rootHash)); err != nil {
		return fmt.Errorf("create base branch at root: %w", err)
	}
	fmt.Printf("  Created %s at root %s\n", baseBranch, rootHash[:12])

	// Step 2: Create work branch from main.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('%s', 'main')", workBranch)); err != nil {
		rebaseCleanupBase(db, baseBranch)
		return fmt.Errorf("create work branch from main: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_CHECKOUT('%s')", workBranch)); err != nil {
		rebaseCleanupAll(db, baseBranch, workBranch)
		return fmt.Errorf("checkout work branch: %w", err)
	}
	fmt.Printf("  Created %s from main, checked out\n", workBranch)

	// Step 3: Start interactive rebase — populates dolt_rebase system table.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_REBASE('--interactive', '%s')", baseBranch)); err != nil {
		rebaseCleanupAll(db, baseBranch, workBranch)
		return fmt.Errorf("start interactive rebase: %w", err)
	}
	fmt.Printf("  Interactive rebase started (dolt_rebase table populated)\n")

	// Step 4: Inspect the rebase plan.
	var totalPlan int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_rebase").Scan(&totalPlan); err != nil {
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return fmt.Errorf("counting rebase entries: %w", err)
	}
	fmt.Printf("  Rebase plan: %d commits\n", totalPlan)

	// Calculate how many to squash: everything except first (must stay pick) and last N.
	// Dolt returns MIN/MAX as decimal strings (e.g. "1.00") via []uint8 byte slices,
	// which cannot be scanned directly into int or float64. Scan as string, parse, cast.
	var minOrderStr, maxOrderStr string
	if err := db.QueryRowContext(ctx, "SELECT MIN(rebase_order), MAX(rebase_order) FROM dolt_rebase").Scan(&minOrderStr, &maxOrderStr); err != nil {
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return fmt.Errorf("getting rebase order range: %w", err)
	}
	minOrderF, err := strconv.ParseFloat(minOrderStr, 64)
	if err != nil {
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return fmt.Errorf("parsing min rebase_order %q: %w", minOrderStr, err)
	}
	maxOrderF, err := strconv.ParseFloat(maxOrderStr, 64)
	if err != nil {
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return fmt.Errorf("parsing max rebase_order %q: %w", maxOrderStr, err)
	}
	minOrder, maxOrder := int(minOrderF), int(maxOrderF)

	squashThreshold := maxOrder - doltRebaseKeepRecent
	toSquash := 0
	if squashThreshold > minOrder {
		toSquash = squashThreshold - minOrder
	}

	fmt.Printf("  Squashing: %d old commits (keeping first as pick + last %d)\n",
		toSquash, doltRebaseKeepRecent)

	if toSquash == 0 {
		fmt.Printf("  %s Nothing to squash — all commits are recent\n", style.Bold.Render("✓"))
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return nil
	}

	if doltRebaseDryRun {
		// Show the plan.
		fmt.Printf("\n%s Dry-run rebase plan:\n", style.Bold.Render("●"))
		rows, err := db.QueryContext(ctx, "SELECT rebase_order, action, commit_hash, commit_message FROM dolt_rebase ORDER BY rebase_order")
		if err != nil {
			rebaseAbortAndCleanup(db, baseBranch, workBranch)
			return fmt.Errorf("reading rebase plan: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var order int
			var action, hash, msg string
			if err := rows.Scan(&order, &action, &hash, &msg); err != nil {
				continue
			}
			marker := "pick"
			if order > minOrder && order <= squashThreshold {
				marker = "squash"
			}
			if len(msg) > 60 {
				msg = msg[:60] + "..."
			}
			if len(hash) > 8 {
				hash = hash[:8]
			}
			fmt.Printf("  %3d  %-7s  %s  %s\n", order, marker, hash, msg)
		}

		fmt.Printf("\n  Would squash %d commits, keep %d recent + 1 root pick\n",
			toSquash, doltRebaseKeepRecent)
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return nil
	}

	// Step 5: Modify the plan — mark old commits as squash.
	// First commit (minOrder) MUST stay 'pick' — squash needs a parent to fold into.
	// Keep last N commits as 'pick'.
	result, err := db.ExecContext(ctx, fmt.Sprintf(
		"UPDATE dolt_rebase SET action = 'squash' WHERE rebase_order > %d AND rebase_order <= %d",
		minOrder, squashThreshold))
	if err != nil {
		rebaseAbortAndCleanup(db, baseBranch, workBranch)
		return fmt.Errorf("updating rebase plan: %w", err)
	}
	affected, _ := result.RowsAffected()
	fmt.Printf("  Marked %d commits as squash\n", affected)

	// Step 6: Execute the rebase plan.
	fmt.Printf("  Executing rebase (this may take a while)...\n")
	if _, err := db.ExecContext(ctx, "CALL DOLT_REBASE('--continue')"); err != nil {
		// Rebase failed — conflicts cause automatic abort.
		rebaseCleanupAll(db, baseBranch, workBranch)
		return fmt.Errorf("rebase execution failed (possible conflicts — automatic abort): %w", err)
	}
	fmt.Printf("  %s Rebase executed successfully\n", style.Bold.Render("✓"))

	// Step 7: Verify integrity and repair tables dropped by squash.
	// Dolt's squash is lossy — it can drop tables whose DDL was in squashed
	// commits. We detect missing tables and restore them from 'main' (which
	// still exists at this point) before the branch swap.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("USE `%s`", dbName)); err != nil {
		fmt.Printf("  %s WARNING: could not re-select database after rebase: %v\n",
			style.Bold.Render("!"), err)
	}
	postCounts, err := flattenGetRowCounts(db, dbName)
	if err != nil {
		fmt.Printf("  %s WARNING: could not verify row counts after rebase: %v\n",
			style.Bold.Render("!"), err)
		fmt.Printf("  Proceeding with branch swap — data should be intact\n")
	} else {
		var restoredTables []string
		var droppedEmpty []string
		for table, preCount := range preCounts {
			postCount, ok := postCounts[table]
			if !ok {
				// Table missing after squash. Try direct query first (stale info_schema).
				var directCount int
				directErr := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s`", dbName, table)).Scan(&directCount)
				if directErr == nil {
					// Table exists, just not in information_schema.
					postCount = directCount
				} else if preCount == 0 {
					// Empty table dropped by squash — recreate from saved schema.
					ddl, hasDDL := preSchemas[table]
					if hasDDL {
						if _, createErr := db.ExecContext(ctx, ddl); createErr != nil {
							fmt.Printf("  %s Could not recreate empty table %q: %v\n",
								style.Bold.Render("!"), table, createErr)
							droppedEmpty = append(droppedEmpty, table)
						} else {
							restoredTables = append(restoredTables, table)
						}
					} else {
						droppedEmpty = append(droppedEmpty, table)
					}
					continue
				} else {
					// Non-empty table genuinely lost — restore from main branch.
					ddl, hasDDL := preSchemas[table]
					if !hasDDL {
						rebaseCleanupAll(db, baseBranch, workBranch)
						return fmt.Errorf("integrity FAIL: table %q (%d rows) dropped by squash, no schema to restore", table, preCount)
					}
					if restoreErr := rebaseRestoreTable(db, ctx, dbName, table, ddl); restoreErr != nil {
						rebaseCleanupAll(db, baseBranch, workBranch)
						return fmt.Errorf("integrity FAIL: table %q (%d rows) dropped by squash, restore failed: %w", table, preCount, restoreErr)
					}
					// Verify restored count.
					if verifyErr := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s`", dbName, table)).Scan(&postCount); verifyErr != nil {
						rebaseCleanupAll(db, baseBranch, workBranch)
						return fmt.Errorf("integrity FAIL: restored table %q but cannot verify: %w", table, verifyErr)
					}
					restoredTables = append(restoredTables, table)
				}
			}
			if preCount != postCount {
				rebaseCleanupAll(db, baseBranch, workBranch)
				return fmt.Errorf("integrity FAIL: %q pre=%d post=%d", table, preCount, postCount)
			}
		}
		if len(restoredTables) > 0 {
			fmt.Printf("  %s Restored %d tables dropped by squash: %v\n",
				style.Bold.Render("!"), len(restoredTables), restoredTables)
			// Commit the restored tables so they're part of the rebased history.
			if _, commitErr := db.ExecContext(ctx, `CALL DOLT_COMMIT('-Am', 'chore: restore tables dropped by rebase squash')`); commitErr != nil {
				fmt.Printf("  %s WARNING: could not commit restored tables: %v\n",
					style.Bold.Render("!"), commitErr)
			}
		}
		if len(droppedEmpty) > 0 {
			fmt.Printf("  %s %d empty tables could not be restored (no schema): %v\n",
				style.Bold.Render("!"), len(droppedEmpty), droppedEmpty)
		}
		fmt.Printf("  %s Integrity verified (%d tables, %d restored)\n",
			style.Bold.Render("✓"), len(preCounts), len(restoredTables))
	}

	// Step 8: Concurrency check — verify main hasn't moved.
	currentHead, err := flattenGetHead(db, dbName)
	if err != nil {
		rebaseCleanupAll(db, baseBranch, workBranch)
		return fmt.Errorf("concurrency check: %w", err)
	}
	if currentHead != preHead {
		rebaseCleanupAll(db, baseBranch, workBranch)
		return fmt.Errorf("ABORT: main HEAD moved during rebase (%s → %s)", preHead[:8], currentHead[:8])
	}

	// Step 9: Swap branches — make compact-work the new main.
	// We're already on compact-work from the rebase.
	if _, err := db.ExecContext(ctx, "CALL DOLT_BRANCH('-D', 'main')"); err != nil {
		// Can't delete main — leave compact-work in place for manual recovery.
		return fmt.Errorf("delete old main: %w (compact-work branch preserved for manual recovery)", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-m', '%s', 'main')", workBranch)); err != nil {
		return fmt.Errorf("rename work branch to main: %w", err)
	}
	// Delete the base branch.
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", baseBranch))
	// Checkout main.
	if _, err := db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')"); err != nil {
		return fmt.Errorf("checkout new main: %w", err)
	}
	fmt.Printf("  Branch swap complete — compact-work is now main\n")

	// Verify final state.
	var finalCount int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.dolt_log", dbName)).Scan(&finalCount); err == nil {
		fmt.Printf("  Final commit count: %d\n", finalCount)
	}

	fmt.Printf("\n%s Surgical rebase complete: %d → %d commits (kept %d recent)\n",
		style.Bold.Render("✓"), commitCount, finalCount, doltRebaseKeepRecent)
	return nil
}

// rebaseCleanup removes leftover branches from a previous failed rebase.
func rebaseCleanup(db *sql.DB, baseBranch, workBranch string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Try to get back to main first.
	_, _ = db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')")
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", workBranch))
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", baseBranch))
}

// rebaseAbortAndCleanup aborts an in-progress rebase then cleans up branches.
//nolint:unparam // baseBranch always "compact-base" — API kept flexible for future callers
func rebaseAbortAndCleanup(db *sql.DB, baseBranch, workBranch string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = db.ExecContext(ctx, "CALL DOLT_REBASE('--abort')")
	_, _ = db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')")
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", workBranch))
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", baseBranch))
}

// rebaseCleanupAll cleans up both branches after a failed rebase.
//nolint:unparam // baseBranch always "compact-base" — API kept flexible for future callers
func rebaseCleanupAll(db *sql.DB, baseBranch, workBranch string) {
	rebaseCleanup(db, baseBranch, workBranch)
}

// rebaseCleanupBase cleans up only the base branch (work branch not yet created).
func rebaseCleanupBase(db *sql.DB, baseBranch string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", baseBranch))
}

// rebaseGetTableSchemas captures SHOW CREATE TABLE for all user tables.
func rebaseGetTableSchemas(db *sql.DB, dbName string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Filter to BASE TABLE only — views return 4 columns from SHOW CREATE TABLE
	// and don't need to be restored (they're recreated from their definition).
	query := fmt.Sprintf("SELECT table_name FROM information_schema.tables WHERE table_schema = '%s' AND table_name NOT LIKE 'dolt_%%' AND table_type = 'BASE TABLE'", dbName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
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

	schemas := make(map[string]string, len(tables))
	for _, table := range tables {
		// SHOW CREATE TABLE returns 2 columns for base tables (Table, Create Table)
		// but 4 for views (View, Create View, charset, collation). Dolt's
		// information_schema sometimes misreports views as BASE TABLE, so we
		// handle both cases by scanning all columns dynamically.
		showRows, err := db.QueryContext(ctx, fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", dbName, table))
		if err != nil {
			return nil, fmt.Errorf("schema for %s: %w", table, err)
		}
		cols, err := showRows.Columns()
		if err != nil {
			showRows.Close()
			return nil, fmt.Errorf("schema columns for %s: %w", table, err)
		}
		if !showRows.Next() {
			showRows.Close()
			return nil, fmt.Errorf("schema for %s: no rows returned", table)
		}
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := showRows.Scan(ptrs...); err != nil {
			showRows.Close()
			return nil, fmt.Errorf("schema for %s: %w", table, err)
		}
		showRows.Close()
		// DDL is always in the second column regardless of column count.
		if len(vals) >= 2 && vals[1].Valid {
			schemas[table] = vals[1].String
		}
	}
	return schemas, nil
}

// rebaseRestoreTable recreates a table dropped by squash and copies data from main.
// Uses Dolt's revision database syntax (dbName/main) to read from main without
// checking out the branch, avoiding concurrency issues with other connections.
func rebaseRestoreTable(db *sql.DB, ctx context.Context, dbName, table, ddl string) error {
	// Recreate the table on compact-work.
	createStmt := strings.Replace(ddl, "CREATE TABLE `"+table+"`", "CREATE TABLE IF NOT EXISTS `"+table+"`", 1)
	if _, err := db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("recreate table: %w", err)
	}

	// Copy data from main using Dolt's revision database syntax: `db/branch`.`table`
	// This reads from main without switching branches.
	insertSQL := fmt.Sprintf("INSERT INTO `%s`.`%s` SELECT * FROM `%s/main`.`%s`", dbName, table, dbName, table)
	if _, err := db.ExecContext(ctx, insertSQL); err != nil {
		return fmt.Errorf("copy data from main: %w", err)
	}

	return nil
}
