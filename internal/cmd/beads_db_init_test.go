//go:build integration

// Package cmd contains integration tests for beads db initialization after clone.
//
// Run with: go test -tags=integration ./internal/cmd -run TestBeadsDbInitAfterClone -v
//
// Bug: GitHub Issue #72
// When a repo with tracked .beads/ is added as a rig, beads.db doesn't exist
// (it's gitignored) and bd operations fail because no one runs `bd init`.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// createTrackedBeadsRepoWithIssues creates a git repo with .beads/ tracked that contains existing issues.
// This simulates a clone of a repo that has tracked beads with issues exported to issues.jsonl.
// The beads.db is NOT included (gitignored), so prefix must be detected from issues.jsonl.
func createTrackedBeadsRepoWithIssues(t *testing.T, path, prefix string, numIssues int) {
	t.Helper()

	// Create directory
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	// Initialize git repo with explicit main branch
	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create initial file and commit (so we have something before beads)
	readmePath := filepath.Join(path, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Initialize beads
	beadsDir := filepath.Join(path, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Run bd init
	cmd := exec.Command("bd", "--no-daemon", "init", "--prefix", prefix)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\nOutput: %s", err, output)
	}

	// Create issues
	for i := 1; i <= numIssues; i++ {
		cmd = exec.Command("bd", "--no-daemon", "-q", "create",
			"--type", "task", "--title", fmt.Sprintf("Test issue %d", i))
		cmd.Dir = path
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd create issue %d failed: %v\nOutput: %s", i, err, output)
		}
	}

	// Add .beads to git (simulating tracked beads)
	cmd = exec.Command("git", "add", ".beads")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add .beads: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "Add beads with issues")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit beads: %v\n%s", err, out)
	}

	// Remove beads.db to simulate what a clone would look like
	// (beads.db is gitignored, so cloned repos don't have it)
	dbPath := filepath.Join(beadsDir, "beads.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove beads.db: %v", err)
	}
}

// TestBeadsDbInitAfterClone tests that when a tracked beads repo is added as a rig,
// the beads database is properly initialized even though beads.db doesn't exist.
func TestBeadsDbInitAfterClone(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	tmpDir := t.TempDir()
	gtBinary := buildGT(t)

	t.Run("TrackedRepoWithExistingPrefix", func(t *testing.T) {
		// GitHub Issue #72: gt rig add should detect existing prefix from tracked beads
		// https://github.com/steveyegge/gastown/issues/72
		//
		// This tests that when a tracked beads repo has existing issues in issues.jsonl,
		// gt rig add can detect the prefix from those issues WITHOUT --prefix flag.

		townRoot := filepath.Join(tmpDir, "town-prefix-test")
		reposDir := filepath.Join(tmpDir, "repos")
		os.MkdirAll(reposDir, 0755)

		// Create a repo with existing beads prefix "existing-prefix" AND issues
		// This creates issues.jsonl with issues like "existing-prefix-1", etc.
		existingRepo := filepath.Join(reposDir, "existing-repo")
		createTrackedBeadsRepoWithIssues(t, existingRepo, "existing-prefix", 3)

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "prefix-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Add rig WITHOUT specifying --prefix - should detect "existing-prefix" from issues.jsonl
		cmd = exec.Command(gtBinary, "rig", "add", "myrig", existingRepo)
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt rig add failed: %v\nOutput: %s", err, output)
		}

		// Verify routes.jsonl has the prefix
		routesContent, err := os.ReadFile(filepath.Join(townRoot, ".beads", "routes.jsonl"))
		if err != nil {
			t.Fatalf("read routes.jsonl: %v", err)
		}

		if !strings.Contains(string(routesContent), `"prefix":"existing-prefix-"`) {
			t.Errorf("routes.jsonl should contain existing-prefix-, got:\n%s", routesContent)
		}

		// NOW TRY TO USE bd - this is the key test for the bug
		// Without the fix, beads.db doesn't exist and bd operations fail
		rigPath := filepath.Join(townRoot, "myrig", "mayor", "rig")
		cmd = exec.Command("bd", "--no-daemon", "--json", "-q", "create",
			"--type", "task", "--title", "test-from-rig")
		cmd.Dir = rigPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create failed (bug!): %v\nOutput: %s\n\nThis is the bug: beads.db doesn't exist after clone because bd init was never run", err, output)
		}

		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("parse output: %v", err)
		}

		if !strings.HasPrefix(result.ID, "existing-prefix-") {
			t.Errorf("expected existing-prefix- prefix, got %s", result.ID)
		}
	})

	t.Run("TrackedRepoWithNoIssuesRequiresPrefix", func(t *testing.T) {
		// Regression test: When a tracked beads repo has NO issues (fresh init),
		// gt rig add must use the --prefix flag since there's nothing to detect from.

		townRoot := filepath.Join(tmpDir, "town-no-issues")
		reposDir := filepath.Join(tmpDir, "repos-no-issues")
		os.MkdirAll(reposDir, 0755)

		// Create a tracked beads repo with NO issues (just bd init)
		emptyRepo := filepath.Join(reposDir, "empty-repo")
		createTrackedBeadsRepoWithNoIssues(t, emptyRepo, "empty-prefix")

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "no-issues-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Add rig WITH --prefix since we can't detect from empty issues.jsonl
		cmd = exec.Command(gtBinary, "rig", "add", "emptyrig", emptyRepo, "--prefix", "empty-prefix")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt rig add with --prefix failed: %v\nOutput: %s", err, output)
		}

		// Verify routes.jsonl has the prefix
		routesContent, err := os.ReadFile(filepath.Join(townRoot, ".beads", "routes.jsonl"))
		if err != nil {
			t.Fatalf("read routes.jsonl: %v", err)
		}

		if !strings.Contains(string(routesContent), `"prefix":"empty-prefix-"`) {
			t.Errorf("routes.jsonl should contain empty-prefix-, got:\n%s", routesContent)
		}

		// Verify bd operations work with the configured prefix
		rigPath := filepath.Join(townRoot, "emptyrig", "mayor", "rig")
		cmd = exec.Command("bd", "--no-daemon", "--json", "-q", "create",
			"--type", "task", "--title", "test-from-empty-repo")
		cmd.Dir = rigPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create failed: %v\nOutput: %s", err, output)
		}

		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("parse output: %v", err)
		}

		if !strings.HasPrefix(result.ID, "empty-prefix-") {
			t.Errorf("expected empty-prefix- prefix, got %s", result.ID)
		}
	})

	t.Run("TrackedRepoWithPrefixMismatchErrors", func(t *testing.T) {
		// Test that when --prefix is explicitly provided but doesn't match
		// the prefix detected from existing issues, gt rig add fails with an error.

		townRoot := filepath.Join(tmpDir, "town-mismatch")
		reposDir := filepath.Join(tmpDir, "repos-mismatch")
		os.MkdirAll(reposDir, 0755)

		// Create a repo with existing beads prefix "real-prefix" with issues
		mismatchRepo := filepath.Join(reposDir, "mismatch-repo")
		createTrackedBeadsRepoWithIssues(t, mismatchRepo, "real-prefix", 2)

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "mismatch-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Add rig with WRONG --prefix - should fail
		cmd = exec.Command(gtBinary, "rig", "add", "mismatchrig", mismatchRepo, "--prefix", "wrong-prefix")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()

		// Should fail
		if err == nil {
			t.Fatalf("gt rig add should have failed with prefix mismatch, but succeeded.\nOutput: %s", output)
		}

		// Verify error message mentions the mismatch
		outputStr := string(output)
		if !strings.Contains(outputStr, "prefix mismatch") {
			t.Errorf("expected 'prefix mismatch' in error, got:\n%s", outputStr)
		}
		if !strings.Contains(outputStr, "real-prefix") {
			t.Errorf("expected 'real-prefix' (detected) in error, got:\n%s", outputStr)
		}
		if !strings.Contains(outputStr, "wrong-prefix") {
			t.Errorf("expected 'wrong-prefix' (provided) in error, got:\n%s", outputStr)
		}
	})

	t.Run("TrackedRepoWithNoIssuesFallsBackToDerivedPrefix", func(t *testing.T) {
		// Test the fallback behavior: when a tracked beads repo has NO issues
		// and NO --prefix is provided, gt rig add should derive prefix from rig name.

		townRoot := filepath.Join(tmpDir, "town-derived")
		reposDir := filepath.Join(tmpDir, "repos-derived")
		os.MkdirAll(reposDir, 0755)

		// Create a tracked beads repo with NO issues
		derivedRepo := filepath.Join(reposDir, "derived-repo")
		createTrackedBeadsRepoWithNoIssues(t, derivedRepo, "original-prefix")

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "derived-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Add rig WITHOUT --prefix - should derive from rig name "testrig"
		// deriveBeadsPrefix("testrig") should produce some abbreviation
		cmd = exec.Command(gtBinary, "rig", "add", "testrig", derivedRepo)
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt rig add (no --prefix) failed: %v\nOutput: %s", err, output)
		}

		// The output should mention "Using prefix" since detection failed
		if !strings.Contains(string(output), "Using prefix") {
			t.Logf("Output: %s", output)
		}

		// Verify bd operations work - the key test is that beads.db was initialized
		rigPath := filepath.Join(townRoot, "testrig", "mayor", "rig")
		cmd = exec.Command("bd", "--no-daemon", "--json", "-q", "create",
			"--type", "task", "--title", "test-derived-prefix")
		cmd.Dir = rigPath
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create failed (beads.db not initialized?): %v\nOutput: %s", err, output)
		}

		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("parse output: %v", err)
		}

		// The ID should have SOME prefix (derived from "testrig")
		// We don't care exactly what it is, just that bd works
		if result.ID == "" {
			t.Error("expected non-empty issue ID")
		}
		t.Logf("Created issue with derived prefix: %s", result.ID)
	})
}

// createTrackedBeadsRepoWithNoIssues creates a git repo with .beads/ tracked but NO issues.
// This simulates a fresh bd init that was committed before any issues were created.
func createTrackedBeadsRepoWithNoIssues(t *testing.T, path, prefix string) {
	t.Helper()

	// Create directory
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	// Initialize git repo with explicit main branch
	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create initial file and commit
	readmePath := filepath.Join(path, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Initialize beads
	beadsDir := filepath.Join(path, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Run bd init (creates beads.db but no issues)
	cmd := exec.Command("bd", "--no-daemon", "init", "--prefix", prefix)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\nOutput: %s", err, output)
	}

	// Add .beads to git (simulating tracked beads)
	cmd = exec.Command("git", "add", ".beads")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add .beads: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "Add beads (no issues)")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit beads: %v\n%s", err, out)
	}

	// Remove beads.db to simulate what a clone would look like
	dbPath := filepath.Join(beadsDir, "beads.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove beads.db: %v", err)
	}
}
