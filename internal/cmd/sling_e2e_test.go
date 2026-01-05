//go:build integration

// Package cmd contains end-to-end tests for gt sling functionality.
//
// Run with: go test -tags=integration ./internal/cmd -run TestSlingE2E -v
//
// These tests verify:
//   - Slinging tasks to all agent types (crew, mayor, deacon, witness, refinery, polecat)
//   - Slinging formulas --on beads to all agent types
//   - Both rig types: local-only .beads (rig root) and tracked .beads (mayor/rig)
//   - No Error: or Warning: messages in output (regression detection)
package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlingE2E_LocalBeads tests sling with local-only beads (rig root .beads).
func TestSlingE2E_LocalBeads(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	gtBinary := buildGT(t)
	townRoot := t.TempDir()

	// Install town
	installCmd := exec.Command(gtBinary, "install", townRoot, "--name", "test-town")
	installCmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Create and add rig
	gitURL := createTestGitRepo(t, "testrig")
	rigAddCmd := exec.Command(gtBinary, "rig", "add", "testrig", gitURL, "--prefix", "tr")
	rigAddCmd.Dir = townRoot
	if output, err := rigAddCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt rig add failed: %v\nOutput: %s", err, output)
	}

	// Create crew member
	crewAddCmd := exec.Command(gtBinary, "crew", "add", "max", "--rig", "testrig")
	crewAddCmd.Dir = townRoot
	if output, err := crewAddCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt crew add failed: %v\nOutput: %s", err, output)
	}

	// Create test formula
	createTestFormula(t, townRoot)

	rigBeadsPath := filepath.Join(townRoot, "testrig")
	t.Logf("Setup complete: local-only beads at %s", rigBeadsPath)

	runSlingTestsForRig(t, gtBinary, townRoot, "testrig", rigBeadsPath)
}

// TestSlingE2E_TrackedBeads tests sling with tracked beads (mayor/rig/.beads).
func TestSlingE2E_TrackedBeads(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	gtBinary := buildGT(t)
	townRoot := t.TempDir()

	// Install town
	installCmd := exec.Command(gtBinary, "install", townRoot, "--name", "test-town")
	installCmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Create git repo WITH tracked .beads
	gitURL := createTestGitRepoWithBeads(t, "testrig", "tr")

	// Add rig - gt rig add detects tracked .beads and auto-initializes the database
	// from config.yaml prefix (see manager.go line 355)
	rigAddCmd := exec.Command(gtBinary, "rig", "add", "testrig", gitURL, "--prefix", "tr")
	rigAddCmd.Dir = townRoot
	rigAddOutput, err := rigAddCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt rig add failed: %v\nOutput: %s", err, rigAddOutput)
	}
	// Check for warnings - redirect setup should succeed
	if strings.Contains(string(rigAddOutput), "Warning:") {
		t.Errorf("Unexpected warning in gt rig add:\n%s", rigAddOutput)
	}

	trackedBeadsPath := filepath.Join(townRoot, "testrig", "mayor", "rig")

	// Create crew member
	crewAddCmd := exec.Command(gtBinary, "crew", "add", "max", "--rig", "testrig")
	crewAddCmd.Dir = townRoot
	if output, err := crewAddCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt crew add failed: %v\nOutput: %s", err, output)
	}

	// Create test formula
	createTestFormula(t, townRoot)

	t.Logf("Setup complete: tracked beads at %s", trackedBeadsPath)

	runSlingTestsForRig(t, gtBinary, townRoot, "testrig", trackedBeadsPath)
}

// createTestFormula creates the test-work formula in town beads.
func createTestFormula(t *testing.T, townRoot string) {
	t.Helper()
	testFormula := `description = "Simple test formula for e2e tests"
formula = "test-work"
type = "workflow"
version = 1

[[steps]]
id = "do-work"
title = "Complete the task"
description = "Do the assigned work"
`
	townFormulasDir := filepath.Join(townRoot, ".beads", "formulas")
	if err := os.MkdirAll(townFormulasDir, 0755); err != nil {
		t.Fatalf("mkdir town formulas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townFormulasDir, "test-work.formula.toml"), []byte(testFormula), 0644); err != nil {
		t.Fatalf("write town formula: %v", err)
	}
}

// runSlingTestsForRig runs the sling test suite for a specific rig.
func runSlingTestsForRig(t *testing.T, gtBinary, townRoot, rigName, rigBeadsPath string) {
	// Define agent test cases
	type agentTestCase struct {
		name    string
		target  string
		beadDir string
	}

	agents := []agentTestCase{
		{"Crew", rigName + "/crew/max", rigBeadsPath},
		{"Mayor", "mayor", townRoot},
		{"Deacon", "deacon", townRoot},
		{"Witness", rigName + "/witness", rigBeadsPath},
		{"Refinery", rigName + "/refinery", rigBeadsPath},
	}

	// Helper to create a task bead
	createTask := func(t *testing.T, dir, title string) string {
		t.Helper()
		cmd := exec.Command("bd", "--no-daemon", "create", "--type", "task", "--title", title, "--json")
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd create task failed: %v\nOutput: %s", err, output)
		}
		return extractBeadID(t, string(output))
	}

	// Helper to check for errors/warnings in output
	checkNoErrorsOrWarnings := func(t *testing.T, output, context string) {
		t.Helper()
		if strings.Contains(output, "Warning:") {
			t.Errorf("Unexpected warning in %s:\n%s", context, output)
		}
		if strings.Contains(output, "Error:") {
			t.Errorf("Unexpected error in %s:\n%s", context, output)
		}
	}

	// Helper to verify hook has work
	verifyHookHasWork := func(t *testing.T, target, expectedBeadID string) {
		t.Helper()
		cmd := exec.Command(gtBinary, "hook", "status", target, "--json")
		cmd.Dir = townRoot
		output, err := cmd.CombinedOutput()
		t.Logf("Hook status for %s:\n%s", target, string(output))

		if err != nil {
			t.Errorf("gt hook status %s failed: %v", target, err)
			return
		}

		var hookStatus struct {
			HasWork    bool `json:"has_work"`
			PinnedBead struct {
				ID         string `json:"id"`
				Dependents []struct {
					ID string `json:"id"`
				} `json:"dependents"`
			} `json:"pinned_bead"`
		}
		if err := json.Unmarshal(output, &hookStatus); err != nil {
			t.Errorf("Failed to parse hook status JSON: %v", err)
			return
		}

		if !hookStatus.HasWork {
			t.Errorf("Expected has_work: true for %s", target)
		}

		// Check if expectedBeadID is pinned or in dependents
		found := hookStatus.PinnedBead.ID == expectedBeadID
		for _, dep := range hookStatus.PinnedBead.Dependents {
			if dep.ID == expectedBeadID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Hook for %s should contain bead %s", target, expectedBeadID)
		}
	}

	// Helper to extract polecat name from output
	extractPolecatName := func(t *testing.T, output string) string {
		t.Helper()
		marker := "Allocated polecat: "
		start := strings.Index(output, marker)
		if start == -1 {
			t.Fatalf("could not find polecat name in output: %s", output)
		}
		start += len(marker)
		end := strings.Index(output[start:], "\n")
		if end == -1 {
			end = len(output) - start
		}
		return strings.TrimSpace(output[start : start+end])
	}

	// === TEST SLINGING TASKS ===
	t.Run("SlingTask", func(t *testing.T) {
		for _, agent := range agents {
			t.Run(agent.name, func(t *testing.T) {
				taskID := createTask(t, agent.beadDir, "Task for "+agent.name)
				t.Logf("Created task: %s", taskID)

				cmd := exec.Command(gtBinary, "sling", taskID, agent.target, "--naked")
				cmd.Dir = townRoot
				output, err := cmd.CombinedOutput()
				outputStr := string(output)
				t.Logf("Sling output:\n%s", outputStr)

				if err != nil {
					t.Fatalf("gt sling failed: %v\nOutput: %s", err, outputStr)
				}

				checkNoErrorsOrWarnings(t, outputStr, "sling task")
				verifyHookHasWork(t, agent.target, taskID)
			})
		}
	})

	// === TEST SLINGING FORMULAS ===
	t.Run("SlingFormula", func(t *testing.T) {
		for _, agent := range agents {
			t.Run(agent.name, func(t *testing.T) {
				taskID := createTask(t, agent.beadDir, "Formula target for "+agent.name)
				t.Logf("Created task: %s", taskID)

				cmd := exec.Command(gtBinary, "sling", "test-work", "--on", taskID, agent.target, "--naked")
				cmd.Dir = townRoot
				output, err := cmd.CombinedOutput()
				outputStr := string(output)
				t.Logf("Sling formula output:\n%s", outputStr)

				if err != nil {
					t.Fatalf("gt sling formula failed: %v\nOutput: %s", err, outputStr)
				}

				checkNoErrorsOrWarnings(t, outputStr, "sling formula")
				verifyHookHasWork(t, agent.target, taskID)
			})
		}
	})

	// === TEST SLINGING TO POLECAT (auto-spawn) ===
	t.Run("SlingToPolecat", func(t *testing.T) {
		taskID := createTask(t, rigBeadsPath, "Task for auto-spawned polecat")
		t.Logf("Created task: %s", taskID)

		cmd := exec.Command(gtBinary, "sling", taskID, rigName, "--naked")
		cmd.Dir = townRoot
		output, err := cmd.CombinedOutput()
		outputStr := string(output)
		t.Logf("Sling to rig output:\n%s", outputStr)

		if err != nil {
			t.Fatalf("gt sling to rig failed: %v\nOutput: %s", err, outputStr)
		}

		checkNoErrorsOrWarnings(t, outputStr, "sling to polecat")
		polecatName := extractPolecatName(t, outputStr)
		verifyHookHasWork(t, rigName+"/polecats/"+polecatName, taskID)
	})

	// === TEST FORMULA ON POLECAT ===
	t.Run("SlingFormulaToPolecat", func(t *testing.T) {
		taskID := createTask(t, rigBeadsPath, "Formula target for polecat")
		t.Logf("Created task: %s", taskID)

		cmd := exec.Command(gtBinary, "sling", "test-work", "--on", taskID, rigName, "--naked")
		cmd.Dir = townRoot
		output, err := cmd.CombinedOutput()
		outputStr := string(output)
		t.Logf("Sling formula to rig output:\n%s", outputStr)

		if err != nil {
			t.Fatalf("gt sling formula to rig failed: %v\nOutput: %s", err, outputStr)
		}

		checkNoErrorsOrWarnings(t, outputStr, "sling formula to polecat")
		polecatName := extractPolecatName(t, outputStr)
		verifyHookHasWork(t, rigName+"/polecats/"+polecatName, taskID)
	})
}

// createTestGitRepoWithBeads creates a test git repo that has .beads tracked.
func createTestGitRepoWithBeads(t *testing.T, name, prefix string) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\nOutput: %s", args[0], err, output)
		}
	}

	// Initialize beads database
	bdInitCmd := exec.Command("bd", "--no-daemon", "init", "--prefix", prefix, "--branch", "beads-sync")
	bdInitCmd.Dir = repoDir
	if output, err := bdInitCmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\nOutput: %s", err, output)
	}

	// WORKAROUND: bd init --prefix doesn't persist prefix to config.yaml (only to database).
	// Since database is gitignored, we need to manually set prefix in config.yaml.
	// TODO: File bd bug - bd init --prefix should write to config.yaml
	configPath := filepath.Join(repoDir, ".beads", "config.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	// Replace commented issue-prefix with actual value
	updated := strings.Replace(string(configData),
		`# issue-prefix: ""`,
		`issue-prefix: "`+prefix+`"`,
		1)
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Create README
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	// Commit everything including .beads
	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit with .beads"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\nOutput: %s", args[0], err, output)
		}
	}

	return repoDir
}

// extractBeadID extracts a bead ID from bd create --json output.
func extractBeadID(t *testing.T, output string) string {
	t.Helper()
	start := strings.Index(output, `"id":`)
	if start == -1 {
		t.Fatalf("could not find id in output: %s", output)
	}
	start += len(`"id":`)
	rest := strings.TrimLeft(output[start:], " \t\n")
	if len(rest) == 0 || rest[0] != '"' {
		t.Fatalf("could not parse id value from output: %s", output)
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end == -1 {
		t.Fatalf("could not find id end quote in output: %s", output)
	}
	return rest[:end]
}
