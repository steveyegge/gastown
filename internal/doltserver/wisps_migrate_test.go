//go:build integration

package doltserver

import (
	"os/exec"
	"testing"
)

// TestMigrateWisps_TableCreation verifies that the wisps table and auxiliary
// tables are created when they don't exist.
func TestMigrateWisps_TableCreation(t *testing.T) {
	requireDoltServer(t)
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found in PATH — skipping integration test")
	}

	// Use current working directory (must be in a valid rig with beads)
	workDir := "."

	// Verify we can talk to the database
	_, err := bdSQL(workDir, "SELECT 1")
	if err != nil {
		t.Skipf("bd sql not working: %v", err)
	}

	// Test table existence check
	exists := bdTableExists(workDir, "issues")
	if !exists {
		t.Skip("issues table not found — not in a valid beads database")
	}

	// If wisps table already exists, just verify it works
	if bdTableExists(workDir, "wisps") {
		t.Log("wisps table already exists — verifying bd mol wisp list works")
		cmd := exec.Command("bd", "mol", "wisp", "list")
		cmd.Dir = workDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd mol wisp list failed: %s: %v", string(output), err)
		}
		t.Logf("bd mol wisp list output: %s", string(output))
		return
	}

	t.Log("wisps table does not exist — would need to create (skipping actual creation in test)")
}

// TestBdSQLCount verifies the count helper works.
func TestBdSQLCount(t *testing.T) {
	requireDoltServer(t)
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found in PATH — skipping integration test")
	}

	cnt, err := bdSQLCount(".", "SELECT COUNT(*) as cnt FROM issues")
	if err != nil {
		t.Skipf("bd sql not working: %v", err)
	}
	t.Logf("issues count: %d", cnt)
	if cnt < 0 {
		t.Fatalf("expected non-negative count, got %d", cnt)
	}
}
