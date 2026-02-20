//go:build integration

package doltserver

import (
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

// setupTestTown creates a temporary town root with a running Dolt server for testing.
func setupTestTown(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dataDir := tmpDir + "/dolt-data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// TestRealWLCommonsStore_Conformance runs the conformance suite against a real Dolt server.
func TestRealWLCommonsStore_Conformance(t *testing.T) {
	requireDoltServer(t)
	wlCommonsConformance(t, func(t *testing.T) WLCommonsStore {
		return NewWLCommons(setupTestTown(t))
	})
}
