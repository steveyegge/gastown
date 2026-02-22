//go:build integration

package cmd

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const doltTestPort = "3307"

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

// doltServer tracks the singleton dolt sql-server process for integration tests.
// Started once per test binary invocation via sync.Once; cleaned up at process exit.
var (
	doltServerOnce sync.Once
	doltServerErr  error
	// doltLockFile is held with LOCK_SH for the lifetime of the test binary.
	// This prevents other test processes from killing the server while we're
	// still using it. See cleanupDoltServer for the shutdown protocol.
	doltLockFile *os.File
	// doltWeStarted tracks whether this process started the server (vs reusing).
	doltWeStarted bool
)

// requireDoltServer ensures a dolt sql-server is running on port 3307 for
// integration tests that need it. The server is shared across all tests in
// the same test binary invocation.
//
// Port contention strategy:
//
//  1. In-process: sync.Once ensures only one goroutine attempts startup.
//
//  2. Cross-process: a file lock (/tmp/dolt-test-server.lock) serializes
//     startup across concurrent test binaries. The first process to acquire
//     LOCK_EX starts the server and writes its PID + data dir to
//     /tmp/dolt-test-server.pid. After startup, the lock is downgraded to
//     LOCK_SH (shared) and held for the lifetime of the test binary.
//
//  3. Safe shutdown: cleanupDoltServer tries to upgrade from LOCK_SH to
//     LOCK_EX (non-blocking). If it succeeds, no other test processes hold
//     the shared lock, so it's safe to kill the server. The server PID is
//     read from the PID file, so ANY last-exiting process can clean up —
//     not just the one that started the server.
//
//  4. External server: if port 3307 is already listening before any test
//     process acquires the lock, we reuse it. No PID file is written, and
//     cleanup never kills an external server.
//
// Why port 3307 is fixed: the entire gt/bd stack (doltserver.DefaultPort,
// gt install, gt dolt start, bd init) assumes port 3307.
// A random port would require threading an override through all layers.
func requireDoltServer(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed, skipping test")
	}

	doltServerOnce.Do(func() {
		doltServerErr = startDoltServer()
	})

	if doltServerErr != nil {
		t.Fatalf("dolt server setup failed: %v", doltServerErr)
	}
}

func doltTestAddr() string {
	return "127.0.0.1:" + doltTestPort
}

const (
	// lockFilePath serializes server startup/shutdown across test processes.
	lockFilePath = "/tmp/dolt-test-server.lock"
	// pidFilePath stores the server PID and data dir for cross-process cleanup.
	pidFilePath = "/tmp/dolt-test-server.pid"
)

func startDoltServer() error {
	// Open the lock file (kept open for the lifetime of the test binary).
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("opening lock file %s: %w", lockFilePath, err)
	}

	// Acquire exclusive lock for the startup phase.
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		lockFile.Close()
		return fmt.Errorf("acquiring startup lock: %w", err)
	}

	// Under the exclusive lock: check if a server is already running
	// (started by another process that held the lock before us, or external).
	if portReady(2 * time.Second) {
		// Downgrade to shared lock — signals "I'm using the server".
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
			lockFile.Close()
			return fmt.Errorf("downgrading to shared lock: %w", err)
		}
		doltLockFile = lockFile
		return nil
	}

	// No server running — start one.
	dataDir, err := os.MkdirTemp("", "dolt-test-server-*")
	if err != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("creating dolt data dir: %w", err)
	}

	cmd := exec.Command("dolt", "sql-server",
		"--port", doltTestPort,
		"--data-dir", dataDir,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		os.RemoveAll(dataDir)
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("starting dolt sql-server: %w", err)
	}

	// Write PID file so any last-exiting process can clean up.
	// Format: "PID\nDATA_DIR\n"
	pidContent := fmt.Sprintf("%d\n%s\n", cmd.Process.Pid, dataDir)
	if err := os.WriteFile(pidFilePath, []byte(pidContent), 0666); err != nil {
		cmd.Process.Kill()
		os.RemoveAll(dataDir)
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Reap the process in the background so ProcessState is populated on exit.
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()

	// Wait for server to accept connections (up to 30 seconds).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if portReady(time.Second) {
			// Server is ready. Downgrade to shared lock.
			if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
				lockFile.Close()
				return fmt.Errorf("downgrading to shared lock: %w", err)
			}
			doltLockFile = lockFile
			doltWeStarted = true
			return nil
		}
		// Check if process exited (port bind failure, etc).
		select {
		case <-exited:
			os.RemoveAll(dataDir)
			os.Remove(pidFilePath)
			syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
			lockFile.Close()
			return fmt.Errorf("dolt sql-server exited prematurely")
		default:
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Timed out — kill and clean up.
	cmd.Process.Kill()
	<-exited
	os.RemoveAll(dataDir)
	os.Remove(pidFilePath)
	syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	lockFile.Close()
	return fmt.Errorf("dolt sql-server did not become ready within 30s")
}

// portReady returns true if the dolt test port is accepting TCP connections.
func portReady(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", doltTestAddr(), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// cleanupDoltServer conditionally kills the test dolt server. Called from TestMain.
//
// Shutdown protocol: try to upgrade from LOCK_SH to LOCK_EX (non-blocking).
//   - If we get LOCK_EX: no other test processes hold the shared lock, so we're
//     the last user. Read the PID file to find and kill the server.
//   - If LOCK_EX fails (EWOULDBLOCK): another process still holds LOCK_SH,
//     meaning it's actively using the server. Skip cleanup — the last process
//     to exit will handle it.
//
// The PID file enables any last-exiting process to clean up, not just the
// process that originally started the server. This prevents leaked servers
// when the starter exits before other consumers.
func cleanupDoltServer() {
	// Release our shared lock regardless.
	defer func() {
		if doltLockFile != nil {
			syscall.Flock(int(doltLockFile.Fd()), syscall.LOCK_UN)
			doltLockFile.Close()
			doltLockFile = nil
		}
	}()

	if doltLockFile == nil {
		return
	}

	// Try to acquire exclusive lock (non-blocking). If another process
	// holds LOCK_SH, this fails with EWOULDBLOCK — the server is still in use.
	err := syscall.Flock(int(doltLockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Another process is using the server. Don't kill it.
		return
	}
	// We got LOCK_EX — we're the last process. Kill from PID file.

	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		// No PID file — either external server or already cleaned up.
		return
	}

	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) < 2 {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return
	}
	dataDir := strings.TrimSpace(lines[1])

	// Kill the server process.
	proc, err := os.FindProcess(pid)
	if err == nil {
		proc.Kill()
		proc.Wait()
	}

	// Clean up data dir and PID file.
	if dataDir != "" {
		os.RemoveAll(dataDir)
	}
	os.Remove(pidFilePath)
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
	dsn := "root:@tcp(127.0.0.1:" + doltTestPort + ")/"
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
	pidData, _ := os.ReadFile(pidFilePath)
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
