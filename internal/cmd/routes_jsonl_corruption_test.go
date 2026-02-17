//go:build integration

// Package cmd contains integration tests for routes.jsonl corruption prevention.
//
// Run with: go test -tags=integration ./internal/cmd -run TestRoutesJSONLCorruption -v
//
// Bug: bd's auto-export writes issue data to routes.jsonl when issues.jsonl doesn't exist,
// corrupting the routing configuration.
package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRoutesJSONLCorruption tests that routes.jsonl is not corrupted by bd auto-export.
func TestRoutesJSONLCorruption(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}
	// Skip if dolt is not available (needed for embedded backend in CorruptionReproduction
	// and for gt install which starts its own dolt server).
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed, skipping test")
	}
	// NOTE: Do NOT call requireDoltServer(t) here. The gt install subprocess
	// manages its own dolt server lifecycle (InitRig → Start → initTownBeads).
	// Pre-starting a server on port 3307 causes gt install to detect it as
	// "orphaned" (mismatched data directory), kill it, and start a new one —
	// creating a race condition where bd init connects before the replacement
	// server is ready, causing issues.jsonl creation to fail.

	t.Run("TownLevelRoutesNotCorrupted", func(t *testing.T) {
		// Test that gt install creates issues.jsonl before routes.jsonl
		// so that bd auto-export doesn't corrupt routes.jsonl
		tmpDir := t.TempDir()
		configureTestGitIdentity(t, tmpDir)
		townRoot := filepath.Join(tmpDir, "test-town")

		gtBinary := buildGT(t)

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "test-town")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		output, err := cmd.CombinedOutput()
		t.Logf("gt install output:\n%s", output)
		if err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Verify issues.jsonl exists
		issuesPath := filepath.Join(townRoot, ".beads", "issues.jsonl")
		if _, err := os.Stat(issuesPath); os.IsNotExist(err) {
			// Log .beads directory contents for debugging
			beadsDir := filepath.Join(townRoot, ".beads")
			if entries, dirErr := os.ReadDir(beadsDir); dirErr == nil {
				var names []string
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Errorf("issues.jsonl should exist after gt install; .beads contents: %v", names)
			} else {
				t.Errorf("issues.jsonl should exist after gt install; .beads dir error: %v", dirErr)
			}
		}

		// Verify routes.jsonl exists and has valid content
		routesPath := filepath.Join(townRoot, ".beads", "routes.jsonl")
		routesContent, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("failed to read routes.jsonl: %v", err)
		}

		// routes.jsonl should contain routing config, not issue data
		if !strings.Contains(string(routesContent), `"prefix"`) {
			t.Errorf("routes.jsonl should contain prefix routing, got: %s", routesContent)
		}
		if strings.Contains(string(routesContent), `"title"`) {
			t.Errorf("routes.jsonl should NOT contain issue data (title field), got: %s", routesContent)
		}

		// Create an issue and verify routes.jsonl is still valid
		cmd = exec.Command("bd", "-q", "create", "--type", "task", "--title", "test issue")
		cmd.Dir = townRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd create failed: %v\nOutput: %s", err, output)
		}

		// Re-read routes.jsonl - it should NOT be corrupted
		routesContent, err = os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("failed to read routes.jsonl after create: %v", err)
		}

		if strings.Contains(string(routesContent), `"title"`) {
			t.Errorf("routes.jsonl was corrupted with issue data after bd create: %s", routesContent)
		}
		if !strings.Contains(string(routesContent), `"prefix"`) {
			t.Errorf("routes.jsonl lost its routing config: %s", routesContent)
		}
	})

	t.Run("RigLevelNoRoutesJSONL", func(t *testing.T) {
		// Test that gt rig add does NOT create routes.jsonl in rig beads
		// (rig-level routes.jsonl breaks bd's walk-up routing to town routes)
		tmpDir := t.TempDir()
		configureTestGitIdentity(t, tmpDir)
		townRoot := filepath.Join(tmpDir, "test-town")

		gtBinary := buildGT(t)

		// Install town
		cmd := exec.Command(gtBinary, "install", townRoot, "--name", "test-town")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}

		// Create a test repo directly at the expected rig location
		rigDir := filepath.Join(townRoot, "testrig")
		createTestGitRepoAt(t, rigDir)

		// Add a rig using --adopt --force (local repo has no remote)
		cmd = exec.Command(gtBinary, "rig", "add", "testrig", "--adopt", "--force")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt rig add failed: %v\nOutput: %s", err, output)
		}

		// Verify rig beads directory exists (if created by adopt)
		rigBeadsDir := filepath.Join(townRoot, "testrig", ".beads")
		if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
			// Adopt mode doesn't create .beads - skip beads assertions
			t.Skip("adopt mode does not create .beads directory")
		}

		// Verify routes.jsonl does NOT exist in rig beads
		rigRoutesPath := filepath.Join(rigBeadsDir, "routes.jsonl")
		if _, err := os.Stat(rigRoutesPath); err == nil {
			t.Error("routes.jsonl should NOT exist in rig beads (breaks walk-up routing)")
		}
	})

	t.Run("CorruptionReproduction", func(t *testing.T) {
		// This test reproduces the bug: if issues.jsonl doesn't exist,
		// bd auto-export writes to routes.jsonl
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		os.MkdirAll(beadsDir, 0755)

		// Initialize beads
		cmd := exec.Command("bd", "init", "--prefix", "test", "--quiet")
		cmd.Dir = tmpDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd init failed: %v\nOutput: %s", err, output)
		}

		// Remove issues.jsonl if it exists (to simulate the bug condition)
		issuesPath := filepath.Join(beadsDir, "issues.jsonl")
		os.Remove(issuesPath)

		// Create routes.jsonl with valid routing config
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		routesContent := `{"prefix":"test-","path":"."}`
		if err := os.WriteFile(routesPath, []byte(routesContent+"\n"), 0644); err != nil {
			t.Fatalf("failed to write routes.jsonl: %v", err)
		}

		// Create an issue - this triggers auto-export
		cmd = exec.Command("bd", "-q", "create", "--type", "task", "--title", "bug reproduction")
		cmd.Dir = tmpDir
		cmd.CombinedOutput() // Ignore error - we're testing the corruption

		// Check if routes.jsonl was corrupted
		newRoutesContent, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("failed to read routes.jsonl: %v", err)
		}

		// If routes.jsonl contains "title", it was corrupted with issue data
		if strings.Contains(string(newRoutesContent), `"title"`) {
			t.Errorf("routes.jsonl was corrupted with issue data: %s", string(newRoutesContent))
		}
	})
}

// Note: createTestGitRepo is defined in rig_integration_test.go
