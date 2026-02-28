package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	doltFlattenConfirm bool
)

var doltFlattenCmd = &cobra.Command{
	Use:   "flatten <database>",
	Short: "Flatten database history to a single commit (NUCLEAR OPTION)",
	Long: `Flatten a Dolt database's commit history to a single commit.

This is the NUCLEAR OPTION for compaction. It destroys all history.
Use only when automated compaction is insufficient.

Safety protocol:
  1. Pre-flight: verifies backup freshness and records row counts
  2. Creates a temporary branch, soft-resets to root, commits all data
  3. Verifies row counts match (integrity check)
  4. Moves main to the flattened commit
  5. Cleans up and runs gc

Requires --yes-i-am-sure flag as safety interlock.`,
	Args: cobra.ExactArgs(1),
	RunE: runDoltFlatten,
}

func init() {
	doltFlattenCmd.Flags().BoolVar(&doltFlattenConfirm, "yes-i-am-sure", false,
		"Required safety flag to confirm you want to destroy history")
	doltCmd.AddCommand(doltFlattenCmd)
}

func runDoltFlatten(cmd *cobra.Command, args []string) error {
	dbName := args[0]

	if !doltFlattenConfirm {
		return fmt.Errorf("this command destroys all commit history. Pass --yes-i-am-sure to proceed")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Verify server is running.
	running, _, err := doltserver.IsRunning(townRoot)
	if err != nil || !running {
		return fmt.Errorf("Dolt server is not running — start with 'gt dolt start'")
	}

	config := doltserver.DefaultConfig(townRoot)
	dsn := fmt.Sprintf("%s@tcp(%s)/%s?parseTime=true&timeout=5s&readTimeout=30s&writeTimeout=30s",
		config.User, config.HostPort(), dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("connecting to database %s: %w", dbName, err)
	}
	defer db.Close()

	// Verify database exists by querying it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var dummy int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&dummy); err != nil {
		return fmt.Errorf("database %q not reachable: %w", dbName, err)
	}

	// Pre-flight: check backup freshness.
	fmt.Printf("%s Pre-flight checks for %s\n", style.Bold.Render("●"), style.Bold.Render(dbName))

	backupDir := filepath.Join(townRoot, ".dolt-backup")
	if _, err := os.Stat(backupDir); err == nil {
		newest := findNewestFile(backupDir)
		if !newest.IsZero() {
			age := time.Since(newest)
			if age > 30*time.Minute {
				fmt.Printf("  %s Backup is %v old — consider running backup first\n",
					style.Bold.Render("!"), age.Round(time.Minute))
			} else {
				fmt.Printf("  %s Backup is %v old (OK)\n", style.Bold.Render("✓"), age.Round(time.Second))
			}
		}
	} else {
		fmt.Printf("  %s No backup directory found — proceed with caution\n", style.Bold.Render("!"))
	}

	// Count commits.
	var commitCount int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.dolt_log", dbName)).Scan(&commitCount); err != nil {
		return fmt.Errorf("counting commits: %w", err)
	}
	fmt.Printf("  Commits: %d\n", commitCount)

	if commitCount <= 2 {
		fmt.Printf("  %s Already minimal — nothing to flatten\n", style.Bold.Render("✓"))
		return nil
	}

	// Record pre-flight row counts.
	preCounts, err := flattenGetRowCounts(db, dbName)
	if err != nil {
		return fmt.Errorf("recording row counts: %w", err)
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

	fmt.Printf("\n%s Flattening %s...\n", style.Bold.Render("●"), dbName)

	// USE database.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("USE `%s`", dbName)); err != nil {
		return fmt.Errorf("use database: %w", err)
	}

	// Clean up any leftover compaction branch.
	const branchName = "gt-compaction"
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", branchName))

	// Create temp branch and checkout.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_CHECKOUT('-b', '%s')", branchName)); err != nil {
		return fmt.Errorf("create compaction branch: %w", err)
	}
	fmt.Printf("  Created branch %s\n", branchName)

	// Soft-reset to root.
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_RESET('--soft', '%s')", rootHash)); err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("soft reset: %w", err)
	}
	fmt.Printf("  Soft-reset to root %s\n", rootHash[:12])

	// Commit flattened data.
	commitMsg := fmt.Sprintf("flatten: compact %s history to single commit", dbName)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_COMMIT('-Am', '%s')", commitMsg)); err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("commit: %w", err)
	}
	fmt.Printf("  Committed flattened data\n")

	// Verify integrity.
	postCounts, err := flattenGetRowCounts(db, dbName)
	if err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("post-compact row counts: %w", err)
	}

	for table, preCount := range preCounts {
		postCount, ok := postCounts[table]
		if !ok {
			flattenCleanup(db, branchName)
			return fmt.Errorf("integrity FAIL: table %q missing after flatten", table)
		}
		if preCount != postCount {
			flattenCleanup(db, branchName)
			return fmt.Errorf("integrity FAIL: %q pre=%d post=%d", table, preCount, postCount)
		}
	}
	fmt.Printf("  %s Integrity verified (%d tables match)\n", style.Bold.Render("✓"), len(preCounts))

	// Concurrency check.
	currentHead, err := flattenGetHead(db, dbName)
	if err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("concurrency check: %w", err)
	}
	if currentHead != preHead {
		flattenCleanup(db, branchName)
		return fmt.Errorf("ABORT: main HEAD moved during flatten (%s → %s)", preHead[:8], currentHead[:8])
	}

	// Get compacted HEAD.
	var compactedHead string
	if err := db.QueryRowContext(ctx, "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1").Scan(&compactedHead); err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("get compacted HEAD: %w", err)
	}

	// Switch to main and reset.
	if _, err := db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')"); err != nil {
		flattenCleanup(db, branchName)
		return fmt.Errorf("checkout main: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_RESET('--hard', '%s')", compactedHead)); err != nil {
		return fmt.Errorf("reset main: %w", err)
	}
	fmt.Printf("  Main reset to compacted commit\n")

	// Delete temp branch.
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", branchName))

	// Verify final state.
	var finalCount int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.dolt_log", dbName)).Scan(&finalCount); err == nil {
		fmt.Printf("  Final commit count: %d\n", finalCount)
	}

	fmt.Printf("\n%s Flatten complete: %d → %d commits\n",
		style.Bold.Render("✓"), commitCount, finalCount)
	return nil
}

// flattenGetHead returns the HEAD commit hash via dolt_log.
func flattenGetHead(db *sql.DB, dbName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var hash string
	query := fmt.Sprintf("SELECT commit_hash FROM `%s`.dolt_log ORDER BY date DESC LIMIT 1", dbName)
	if err := db.QueryRowContext(ctx, query).Scan(&hash); err != nil {
		return "", err
	}
	return hash, nil
}

// flattenGetRowCounts returns table -> row count for all user tables.
func flattenGetRowCounts(db *sql.DB, dbName string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := fmt.Sprintf("SELECT table_name FROM information_schema.tables WHERE table_schema = '%s' AND table_name NOT LIKE 'dolt_%%'", dbName)
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

	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s`", dbName, table)).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return counts, nil
}

// flattenCleanup tries to switch back to main and delete the temp branch.
func flattenCleanup(db *sql.DB, branchName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = db.ExecContext(ctx, "CALL DOLT_CHECKOUT('main')")
	_, _ = db.ExecContext(ctx, fmt.Sprintf("CALL DOLT_BRANCH('-D', '%s')", branchName))
}

// findNewestFile walks a directory and returns the most recent file mtime.
func findNewestFile(dir string) time.Time {
	var newest time.Time
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}
