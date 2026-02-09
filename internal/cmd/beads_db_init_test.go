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

	t.Run("TrackedRepoWithExplicitPrefix", func(t *testing.T) {
		// When adopting a directory with tracked .beads/ and providing --prefix,
		// the rig should be registered with the specified prefix.

		townRoot := filepath.Join(tmpDir, "town-prefix-test")
		reposDir := filepath.Join(tmpDir, "repos")
		os.MkdirAll(reposDir, 0755)

		// Create a repo with existing beads
		existingRepo := filepath.Join(reposDir, "existing-repo")
		createTrackedBeadsRepoWithIssues(t, existingRepo, "existing-prefix", 3)

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "prefix-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Move repo into town root so --adopt can find it
		adoptPath := filepath.Join(townRoot, "myrig")
		if err := os.Rename(existingRepo, adoptPath); err != nil {
			t.Fatalf("move repo into town: %v", err)
		}

		// Add rig with explicit prefix and --force (no git remote in test repo)
		cmd = exec.Command(gtBinary, "rig", "add", "myrig", adoptPath, "--adopt", "--force", "--prefix", "existing-prefix")
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

		if !strings.Contains(string(routesContent), `"prefix":"existing-prefix`) {
			t.Errorf("routes.jsonl should contain existing-prefix, got:\n%s", routesContent)
		}
	})

	t.Run("TrackedRepoWithDerivedPrefix", func(t *testing.T) {
		// When adopting a directory without --prefix, the rig should derive
		// a prefix from the rig name.

		townRoot := filepath.Join(tmpDir, "town-derived")
		reposDir := filepath.Join(tmpDir, "repos-derived")
		os.MkdirAll(reposDir, 0755)

		derivedRepo := filepath.Join(reposDir, "derived-repo")
		createTrackedBeadsRepoWithNoIssues(t, derivedRepo, "original-prefix")

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "derived-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Move repo into town root so --adopt can find it
		adoptPath := filepath.Join(townRoot, "testrig")
		if err := os.Rename(derivedRepo, adoptPath); err != nil {
			t.Fatalf("move repo into town: %v", err)
		}

		// Add rig WITHOUT --prefix - should derive from rig name "testrig"
		cmd = exec.Command(gtBinary, "rig", "add", "testrig", adoptPath, "--adopt", "--force")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt rig add (no --prefix) failed: %v\nOutput: %s", err, output)
		}

		// Verify the rig was registered (routes.jsonl should exist and have an entry)
		routesContent, err := os.ReadFile(filepath.Join(townRoot, ".beads", "routes.jsonl"))
		if err != nil {
			t.Fatalf("read routes.jsonl: %v", err)
		}

		// Should have some prefix entry for testrig
		if !strings.Contains(string(routesContent), "testrig") {
			t.Errorf("routes.jsonl should reference testrig, got:\n%s", routesContent)
		}
	})

	t.Run("AdoptNonExistentDirectoryFails", func(t *testing.T) {
		// When --adopt is used but the directory doesn't exist inside the town,
		// it should fail with a clear error.

		townRoot := filepath.Join(tmpDir, "town-nodir")

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "nodir-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Try to adopt a non-existent directory
		cmd = exec.Command(gtBinary, "rig", "add", "ghostrig", "fake-url", "--adopt")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()

		if err == nil {
			t.Fatalf("gt rig add --adopt should have failed for non-existent dir, but succeeded.\nOutput: %s", output)
		}

		if !strings.Contains(string(output), "does not exist") {
			t.Errorf("expected 'does not exist' in error, got:\n%s", output)
		}
	})

	t.Run("AdoptExistingRigFails", func(t *testing.T) {
		// When --adopt is used for a rig name that's already registered,
		// it should fail.

		townRoot := filepath.Join(tmpDir, "town-dup")
		reposDir := filepath.Join(tmpDir, "repos-dup")
		os.MkdirAll(reposDir, 0755)

		repo := filepath.Join(reposDir, "dup-repo")
		createTrackedBeadsRepoWithNoIssues(t, repo, "dup-prefix")

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "dup-test")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Move repo and adopt it
		adoptPath := filepath.Join(townRoot, "duprig")
		if err := os.Rename(repo, adoptPath); err != nil {
			t.Fatalf("move repo into town: %v", err)
		}

		cmd = exec.Command(gtBinary, "rig", "add", "duprig", adoptPath, "--adopt", "--force", "--prefix", "dup")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("first adopt failed: %v\nOutput: %s", err, output)
		}

		// Try to adopt again - should fail
		cmd = exec.Command(gtBinary, "rig", "add", "duprig", adoptPath, "--adopt", "--force", "--prefix", "dup")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()

		if err == nil {
			t.Fatalf("second adopt should have failed, but succeeded.\nOutput: %s", output)
		}

		if !strings.Contains(string(output), "already exists") {
			t.Errorf("expected 'already exists' in error, got:\n%s", output)
		}
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
