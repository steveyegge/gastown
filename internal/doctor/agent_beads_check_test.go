package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAgentBeadsExistCheck_NoRoutes verifies the check handles missing routes.
func TestAgentBeadsExistCheck_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()

	// No .beads dir at all
	check := NewAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// With no routes, only global agents (deacon, mayor) are checked
	// They won't exist without Dolt, so we expect error
	t.Logf("Result: status=%v, message=%s", result.Status, result.Message)
	if result.Status == StatusOK {
		t.Error("expected error for missing global agent beads")
	}
}

// TestAgentBeadsExistCheck_NoRigs verifies the check handles empty routes.
func TestAgentBeadsExistCheck_NoRigs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .beads dir with empty routes.jsonl
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// With empty routes, only global agents (deacon, mayor) are checked
	// They won't exist without Dolt, so we expect error or warning
	t.Logf("Result: status=%v, message=%s", result.Status, result.Message)
}

// TestAgentBeadsExistCheck_ExpectedIDs verifies the check looks for correct agent bead IDs.
func TestAgentBeadsExistCheck_ExpectedIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up routes pointing to a rig with known prefix
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Use "sw" prefix to match sallaWork pattern
	routesContent := `{"prefix":"sw-","path":"sallaWork/mayor/rig"}` + "\n"
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig beads directory
	rigBeadsDir := filepath.Join(tmpDir, "sallaWork", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should report missing beads
	if result.Status == StatusOK {
		t.Errorf("expected error for missing agent beads, got: %s", result.Message)
	}

	// Should mention the expected bead IDs in details
	if len(result.Details) == 0 {
		t.Error("expected details to contain missing bead IDs")
	}

	// Verify the expected IDs are in the details
	expectedIDs := []string{"sw-sallaWork-witness", "sw-sallaWork-refinery"}
	for _, expectedID := range expectedIDs {
		found := false
		for _, detail := range result.Details {
			if detail == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected missing bead ID %s in details, got: %v", expectedID, result.Details)
		}
	}

	t.Logf("Result: status=%v, message=%s, details=%v", result.Status, result.Message, result.Details)
}

// TestListCrewWorkers_FiltersWorktrees verifies that listCrewWorkers skips
// git worktrees (directories where .git is a file) and only returns canonical
// crew workers (where .git is a directory). This is the fix for GH#2767.
func TestListCrewWorkers_FiltersWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "myrig"
	crewDir := filepath.Join(tmpDir, rigName, "crew")

	// Create a canonical crew worker: .git is a directory
	canonicalDir := filepath.Join(crewDir, "alice")
	if err := os.MkdirAll(filepath.Join(canonicalDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a worktree: .git is a file (contains gitdir pointer)
	worktreeDir := filepath.Join(crewDir, "alice-worktree")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"),
		[]byte("gitdir: /path/to/main/.git/worktrees/alice-worktree\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a second canonical worker
	bobDir := filepath.Join(crewDir, "bob")
	if err := os.MkdirAll(filepath.Join(bobDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a directory without .git at all (should be included — not a worktree)
	plainDir := filepath.Join(crewDir, "charlie")
	if err := os.MkdirAll(plainDir, 0755); err != nil {
		t.Fatal(err)
	}

	workers := listCrewWorkers(tmpDir, rigName)

	// Should include alice, bob, charlie but NOT alice-worktree
	expected := map[string]bool{"alice": false, "bob": false, "charlie": false}
	for _, w := range workers {
		if w == "alice-worktree" {
			t.Errorf("listCrewWorkers should skip worktree 'alice-worktree', got: %v", workers)
		}
		if _, ok := expected[w]; ok {
			expected[w] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("listCrewWorkers should include canonical worker %q, got: %v", name, workers)
		}
	}
}

// TestListPolecats_FiltersWorktrees verifies that listPolecats skips
// git worktrees, same as listCrewWorkers. See GH#2767.
func TestListPolecats_FiltersWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "myrig"
	polecatDir := filepath.Join(tmpDir, rigName, "polecats")

	// Canonical polecat
	if err := os.MkdirAll(filepath.Join(polecatDir, "scout", ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Worktree polecat (.git is a file)
	wtDir := filepath.Join(polecatDir, "scout-wt")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, ".git"),
		[]byte("gitdir: /path/to/main/.git/worktrees/scout-wt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	polecats := listPolecats(tmpDir, rigName)

	if len(polecats) != 1 || polecats[0] != "scout" {
		t.Errorf("listPolecats should return only [scout], got: %v", polecats)
	}
}