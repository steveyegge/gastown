//go:build integration

package doltserver

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// isolatedServer holds the connection details of a test Dolt server.
type isolatedServer struct {
	TownRoot string
	Port     int
}

// startIsolatedDoltServer starts a Dolt SQL server on a dynamic port with an
// isolated data directory. It sets GT_DOLT_PORT so that DefaultConfig and all
// downstream functions (IsRunning, serverExecSQL, buildDoltSQLCmd, etc.)
// connect to this server instead of the production server on port 3307.
// The server is killed when the test completes.
func startIsolatedDoltServer(t *testing.T) *isolatedServer {
	t.Helper()

	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not found in PATH — skipping integration test")
	}

	townRoot := t.TempDir()
	// Read config BEFORE setting GT_DOLT_PORT so DataDir path is correct.
	dataDir := DefaultConfig(townRoot).DataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating data dir: %v", err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Override GT_DOLT_PORT so all DefaultConfig calls use our port.
	// This is critical: without it, IsRunning/serverExecSQL would fall back
	// to port 3307 and hit the production server.
	t.Setenv("GT_DOLT_PORT", strconv.Itoa(port))

	// Configure dolt identity in an isolated root.
	doltEnv := append(os.Environ(), "DOLT_ROOT_PATH="+townRoot)
	for _, args := range [][]string{
		{"dolt", "config", "--global", "--add", "user.name", "integration-test"},
		{"dolt", "config", "--global", "--add", "user.email", "test@integration.test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = doltEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", args[1], err, out)
		}
	}

	// Start dolt sql-server on the dynamic port.
	serverCmd := exec.Command("dolt", "sql-server",
		"--port", fmt.Sprintf("%d", port),
		"--data-dir", dataDir,
	)
	serverCmd.Env = doltEnv
	serverCmd.Stdout = nil
	serverCmd.Stderr = nil
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("starting dolt sql-server: %v", err)
	}
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
		_ = serverCmd.Wait()
	})

	// Wait for server readiness via MySQL ping.
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/?timeout=1s", port)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			if err := db.Ping(); err == nil {
				db.Close()
				return &isolatedServer{TownRoot: townRoot, Port: port}
			}
			db.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("dolt sql-server did not become ready on port %d within 15s", port)
	return nil // unreachable
}

// TestRealWLCommonsStore_Conformance runs the conformance suite against a real Dolt server.
func TestRealWLCommonsStore_Conformance(t *testing.T) {
	srv := startIsolatedDoltServer(t)

	// Pre-create the database before parallel subtests to avoid
	// concurrent CREATE DATABASE races.
	store := NewWLCommons(srv.TownRoot)
	if err := store.EnsureDB(); err != nil {
		t.Fatalf("EnsureDB() error: %v", err)
	}

	wlCommonsConformance(t, func(t *testing.T) WLCommonsStore {
		return NewWLCommons(srv.TownRoot)
	})
}

// TestIsNothingToCommit_RealDolt verifies that isNothingToCommit correctly detects
// the error produced by DOLT_COMMIT when no changes exist. This pins the detection
// logic against the actual Dolt error text so that Dolt upgrades that change the
// message wording are caught immediately.
func TestIsNothingToCommit_RealDolt(t *testing.T) {
	srv := startIsolatedDoltServer(t)

	// Create a database and table so we have a valid context for DOLT_COMMIT.
	initScript := fmt.Sprintf(`CREATE DATABASE IF NOT EXISTS %s;
USE %s;
CREATE TABLE IF NOT EXISTS _ping (id INT PRIMARY KEY);
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'init ping table');
`, WLCommonsDB, WLCommonsDB)
	if err := doltSQLScript(srv.TownRoot, initScript); err != nil {
		t.Fatalf("init script error: %v", err)
	}

	// Now try to commit with no changes — this should produce the "nothing to commit" error.
	noopScript := fmt.Sprintf(`USE %s;
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'noop');
`, WLCommonsDB)
	err := doltSQLScript(srv.TownRoot, noopScript)
	if err == nil {
		t.Fatal("expected error from DOLT_COMMIT with no changes, got nil")
	}

	if !isNothingToCommit(err) {
		t.Errorf("isNothingToCommit(%q) = false, want true — Dolt error text may have changed", err)
	}
}
