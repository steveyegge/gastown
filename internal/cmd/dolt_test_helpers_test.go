//go:build integration

package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/steveyegge/gastown/internal/testutil"
)

// requireDoltServer delegates to testutil.RequireDoltServer.
func requireDoltServer(t *testing.T) {
	t.Helper()
	testutil.RequireDoltServer(t)
}

// cleanupDoltServer delegates to testutil.CleanupDoltServer.
func cleanupDoltServer() {
	testutil.CleanupDoltServer()
}

// configureTestGitIdentity sets git global config in an isolated HOME directory
// so that EnsureDoltIdentity (called during gt install preflight) can copy
// identity from git to dolt.
func configureTestGitIdentity(t *testing.T, homeDir string) {
	t.Helper()
	env := append(os.Environ(), "HOME="+homeDir)
	for _, args := range [][]string{
		{"config", "--global", "user.name", "Test User"},
		{"config", "--global", "user.email", "test@test.com"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

// doltCleanupOnce ensures database cleanup happens at most once per binary.
var (
	doltCleanupOnce sync.Once
	doltCleanupErr  error
)

// cleanStaleBeadsDatabases drops stale beads_* databases left by earlier tests
// (e.g., beads_db_init_test.go) from the running Dolt server. This prevents
// phantom catalog entries from causing "database not found" errors during
// bd init --server migration sweeps in queue tests.
//
// Uses SQL-level cleanup (DROP DATABASE) rather than server restart, because
// restarting the Dolt server causes bd init --server to fail at creating
// database schema (tables).
func cleanStaleBeadsDatabases(t *testing.T) {
	t.Helper()
	doltCleanupOnce.Do(func() {
		doltCleanupErr = dropStaleBeadsDatabases()
	})
	if doltCleanupErr != nil {
		t.Fatalf("stale database cleanup failed: %v", doltCleanupErr)
	}
}

// dropStaleBeadsDatabases connects to the Dolt server and drops all beads_*
// databases that were created by earlier tests. Uses three strategies:
//  1. SHOW DATABASES → DROP any visible beads_* databases
//  2. DROP known phantom database names from beads_db_init_test.go
//  3. Physical cleanup of beads_* directories from the server's data-dir
func dropStaleBeadsDatabases() error {
	dsn := "root:@tcp(127.0.0.1:" + testutil.DoltTestPort() + ")/"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("connecting to dolt server: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("pinging dolt server: %w", err)
	}

	var dropped []string

	// Strategy 1: Drop beads_* and known test databases (not ALL non-system databases,
	// to avoid destroying unrelated integration state on shared servers).
	systemDBs := map[string]bool{
		"information_schema": true,
		"mysql":              true,
	}
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] SHOW DATABASES failed: %v\n", err)
	} else {
		var allDBs []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				continue
			}
			allDBs = append(allDBs, name)
			// Only drop databases matching known test patterns
			shouldDrop := false
			if strings.HasPrefix(name, "beads_") {
				shouldDrop = true
			} else if name == "hq" {
				shouldDrop = true // Created by beads_db_init_test.go
			}
			if shouldDrop && !systemDBs[name] {
				if _, err := db.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
					fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] DROP %s failed: %v\n", name, err)
				} else {
					dropped = append(dropped, name)
				}
			}
		}
		rows.Close()
		fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] visible databases: %v\n", allDBs)
	}

	// Strategy 2: Try to DROP known phantom database names from beads_db_init_test.go.
	// These may be invisible to SHOW DATABASES but still in Dolt's in-memory catalog.
	knownPrefixes := []string{
		"existing-prefix", "empty-prefix", "real-prefix",
		"original-prefix", "reinit-prefix",
		"myrig", "emptyrig", "mismatchrig", "testrig", "reinitrig",
		"prefix-test", "no-issues-test", "mismatch-test", "derived-test", "reinit-test",
	}
	for _, pfx := range knownPrefixes {
		name := "beads_" + pfx
		if _, err := db.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] DROP phantom %s: %v\n", name, err)
		} else {
			dropped = append(dropped, name+"(phantom)")
		}
	}

	// Strategy 3: Purge dropped databases from Dolt's catalog.
	if _, err := db.Exec("CALL dolt_purge_dropped_databases()"); err != nil {
		fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] purge failed: %v\n", err)
	}

	// Strategy 4: Remove beads_* and known test database directories from the
	// server's data-dir. Scoped to avoid removing unrelated databases.
	pidPath := testutil.PidFilePathForPort(testutil.DoltTestPort())
	pidData, _ := os.ReadFile(pidPath)
	if pidData != nil {
		lines := strings.SplitN(string(pidData), "\n", 3)
		if len(lines) >= 2 {
			dataDir := strings.TrimSpace(lines[1])
			if dataDir != "" {
				entries, _ := os.ReadDir(dataDir)
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					shouldRemove := strings.HasPrefix(e.Name(), "beads_") || e.Name() == "hq"
					if shouldRemove {
						os.RemoveAll(dataDir + "/" + e.Name())
						dropped = append(dropped, e.Name()+"(disk)")
					}
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "[dropStaleBeadsDatabases] cleaned: %v\n", dropped)
	return nil
}
