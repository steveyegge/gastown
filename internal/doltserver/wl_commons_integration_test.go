//go:build integration

package doltserver

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// requireDoltServer skips the test if a Dolt server is not available.
func requireDoltServer(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not found in PATH — skipping integration test")
	}
}

// setupTestTown creates a temporary town root with Dolt data directory matching
// DefaultConfig().DataDir so the runtime paths are consistent.
func setupTestTown(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dataDir := DefaultConfig(tmpDir).DataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// TestRealWLCommonsStore_Conformance runs the conformance suite against a real Dolt server.
func TestRealWLCommonsStore_Conformance(t *testing.T) {
	requireDoltServer(t)
	wlCommonsConformance(t, func(t *testing.T) WLCommonsStore {
		store := NewWLCommons(setupTestTown(t))
		if err := store.EnsureDB(); err != nil {
			t.Fatalf("EnsureDB() error: %v", err)
		}
		return store
	})
}

// TestIsNothingToCommit_RealDolt verifies that isNothingToCommit correctly detects
// the error produced by DOLT_COMMIT when no changes exist. This pins the detection
// logic against the actual Dolt error text so that Dolt upgrades that change the
// message wording are caught immediately.
func TestIsNothingToCommit_RealDolt(t *testing.T) {
	requireDoltServer(t)
	townRoot := setupTestTown(t)

	// Create a database and table so we have a valid context for DOLT_COMMIT.
	initScript := fmt.Sprintf(`CREATE DATABASE IF NOT EXISTS %s;
USE %s;
CREATE TABLE IF NOT EXISTS _ping (id INT PRIMARY KEY);
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'init ping table');
`, WLCommonsDB, WLCommonsDB)
	if err := doltSQLScript(townRoot, initScript); err != nil {
		t.Fatalf("init script error: %v", err)
	}

	// Now try to commit with no changes — this should produce the "nothing to commit" error.
	noopScript := fmt.Sprintf(`USE %s;
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'noop');
`, WLCommonsDB)
	err := doltSQLScript(townRoot, noopScript)
	if err == nil {
		t.Fatal("expected error from DOLT_COMMIT with no changes, got nil")
	}

	if !isNothingToCommit(err) {
		t.Errorf("isNothingToCommit(%q) = false, want true — Dolt error text may have changed", err)
	}
}
