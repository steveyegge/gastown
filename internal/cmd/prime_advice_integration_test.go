//go:build integration

// Package cmd contains integration tests for advice display in gt prime.
//
// Run with: go test -tags=integration ./internal/cmd -run TestAdvice -v
package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
)

// setupAdviceTestTown creates a minimal Gas Town with two rigs for testing advice scoping.
// Returns townRoot.
func setupAdviceTestTown(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()

	// Create town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Create routes.jsonl with two rigs: gastown and beads
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},                 // Town-level beads
		{Prefix: "gt-", Path: "gastown/mayor/rig"}, // Gastown rig
		{Prefix: "bd-", Path: "beads/mayor/rig"},   // Beads rig
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Create gastown rig structure
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(gasRigPath, 0755); err != nil {
		t.Fatalf("mkdir gastown: %v", err)
	}

	// Create gastown .beads directory with its own config
	gasBeadsDir := filepath.Join(gasRigPath, ".beads")
	if err := os.MkdirAll(gasBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir gastown .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gasBeadsDir, "config.yaml"), []byte("prefix: gt\n"), 0644); err != nil {
		t.Fatalf("write gastown config: %v", err)
	}

	// Create beads rig structure
	beadsRigPath := filepath.Join(townRoot, "beads", "mayor", "rig")
	if err := os.MkdirAll(beadsRigPath, 0755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}

	// Create beads .beads directory
	beadsBeadsDir := filepath.Join(beadsRigPath, ".beads")
	if err := os.MkdirAll(beadsBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir beads .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsBeadsDir, "config.yaml"), []byte("prefix: bd\n"), 0644); err != nil {
		t.Fatalf("write beads config: %v", err)
	}

	// Create polecat directories with redirects
	// gastown polecat
	gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
	if err := os.MkdirAll(gasPolecatDir, 0755); err != nil {
		t.Fatalf("mkdir gastown polecat: %v", err)
	}
	gasPolecatBeadsDir := filepath.Join(gasPolecatDir, ".beads")
	if err := os.MkdirAll(gasPolecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir gastown polecat .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gasPolecatBeadsDir, "redirect"), []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write gastown polecat redirect: %v", err)
	}

	// beads polecat
	beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
	if err := os.MkdirAll(beadsPolecatDir, 0755); err != nil {
		t.Fatalf("mkdir beads polecat: %v", err)
	}
	beadsPolecatBeadsDir := filepath.Join(beadsPolecatDir, ".beads")
	if err := os.MkdirAll(beadsPolecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir beads polecat .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsPolecatBeadsDir, "redirect"), []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write beads polecat redirect: %v", err)
	}

	return townRoot
}

// createAdvice creates an advice bead using bd advice add.
func createAdvice(t *testing.T, dir, title, description string, labels []string) string {
	t.Helper()

	args := []string{"--sandbox", "advice", "add", title}
	if description != "" {
		args = append(args, "-d", description)
	}
	for _, l := range labels {
		args = append(args, "-l", l)
	}
	args = append(args, "--json")

	cmd := bdcmd.CommandInDir(dir, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd advice add failed in %s: %v\n%s", dir, err, output)
	}

	// Parse ID from output
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		// Try to extract ID from non-JSON output
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Created") {
				// Parse "Created advice bead: gt-abc123"
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					return strings.TrimSpace(parts[len(parts)-1])
				}
			}
		}
		t.Fatalf("could not parse advice ID from output: %s", output)
	}
	return result.ID
}

// listAdviceForAgent lists advice applicable to an agent.
func listAdviceForAgent(t *testing.T, dir, agentID string) []AdviceBead {
	t.Helper()

	cmd := bdcmd.CommandInDir(dir, "--sandbox", "advice", "list", "--for="+agentID, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Silently return empty if command fails (e.g., no advice)
		return nil
	}

	if len(output) == 0 || strings.TrimSpace(string(output)) == "[]" {
		return nil
	}

	var advice []AdviceBead
	if err := json.Unmarshal(output, &advice); err != nil {
		t.Logf("Warning: could not parse advice list output: %v", err)
		return nil
	}
	return advice
}

// hasAdviceWithTitle checks if advice list contains an advice with given title.
func hasAdviceWithTitle(advice []AdviceBead, title string) bool {
	for _, a := range advice {
		if a.Title == title {
			return true
		}
	}
	return false
}

// TestAdviceCrossRigIsolation verifies that rig-scoped advice only appears
// to agents in that rig.
//
// This test would have caught the scoping leak bug where gastown advice
// was showing up in beads polecats.
func TestAdviceCrossRigIsolation(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping advice test")
	}

	townRoot := setupAdviceTestTown(t)

	// Initialize beads databases
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	beadsRigPath := filepath.Join(townRoot, "beads", "mayor", "rig")
	initBeadsDBWithPrefix(t, townRoot, "hq")
	initBeadsDBWithPrefix(t, gasRigPath, "gt")
	initBeadsDBWithPrefix(t, beadsRigPath, "bd")

	// Create gastown-scoped advice
	gastownAdviceTitle := "Gastown-only test advice"
	createAdvice(t, gasRigPath, gastownAdviceTitle,
		"This advice should only appear for gastown agents",
		[]string{"rig:gastown"})

	// Create beads-scoped advice
	beadsAdviceTitle := "Beads-only test advice"
	createAdvice(t, beadsRigPath, beadsAdviceTitle,
		"This advice should only appear for beads agents",
		[]string{"rig:beads"})

	// Test 1: Gastown polecat should see gastown advice, NOT beads advice
	t.Run("gastown_polecat_sees_only_gastown_advice", func(t *testing.T) {
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice := listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")

		if !hasAdviceWithTitle(advice, gastownAdviceTitle) {
			t.Errorf("gastown polecat should see gastown advice %q", gastownAdviceTitle)
		}
		if hasAdviceWithTitle(advice, beadsAdviceTitle) {
			t.Errorf("gastown polecat should NOT see beads advice %q", beadsAdviceTitle)
		}
	})

	// Test 2: Beads polecat should see beads advice, NOT gastown advice
	t.Run("beads_polecat_sees_only_beads_advice", func(t *testing.T) {
		beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
		advice := listAdviceForAgent(t, beadsPolecatDir, "beads/polecats/beta")

		if !hasAdviceWithTitle(advice, beadsAdviceTitle) {
			t.Errorf("beads polecat should see beads advice %q", beadsAdviceTitle)
		}
		if hasAdviceWithTitle(advice, gastownAdviceTitle) {
			t.Errorf("beads polecat should NOT see gastown advice %q", gastownAdviceTitle)
		}
	})
}

// TestAdviceScopeHierarchy verifies the scope hierarchy:
// - Global advice appears everywhere
// - Rig advice only in that rig
// - Role advice only for that role in that rig
func TestAdviceScopeHierarchy(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping advice test")
	}

	townRoot := setupAdviceTestTown(t)

	// Initialize beads databases
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	beadsRigPath := filepath.Join(townRoot, "beads", "mayor", "rig")
	initBeadsDBWithPrefix(t, townRoot, "hq")
	initBeadsDBWithPrefix(t, gasRigPath, "gt")
	initBeadsDBWithPrefix(t, beadsRigPath, "bd")

	// Create advice at different scopes
	// Note: Advice must be created in the database where agents query it.
	// "Global" advice in a rig applies to all agents querying that rig's database.

	// Global advice - create in BOTH rigs (since it should be visible to all)
	globalAdviceTitle := "Global advice for all agents"
	createAdvice(t, gasRigPath, globalAdviceTitle,
		"This advice applies to everyone in gastown",
		[]string{"global"})
	createAdvice(t, beadsRigPath, globalAdviceTitle,
		"This advice applies to everyone in beads",
		[]string{"global"})

	// Rig-scoped advice (gastown only)
	rigAdviceTitle := "Gastown rig advice"
	createAdvice(t, gasRigPath, rigAdviceTitle,
		"This advice applies to all gastown agents",
		[]string{"rig:gastown"})

	// Role-scoped advice (polecat only in gastown)
	roleAdviceTitle := "Polecat role advice"
	createAdvice(t, gasRigPath, roleAdviceTitle,
		"This advice applies to polecats in gastown",
		[]string{"role:polecat"})

	// Test: Gastown polecat sees all three levels
	t.Run("gastown_polecat_sees_all_matching_scopes", func(t *testing.T) {
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice := listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")

		if !hasAdviceWithTitle(advice, globalAdviceTitle) {
			t.Errorf("gastown polecat should see global advice %q", globalAdviceTitle)
		}
		if !hasAdviceWithTitle(advice, rigAdviceTitle) {
			t.Errorf("gastown polecat should see rig advice %q", rigAdviceTitle)
		}
		if !hasAdviceWithTitle(advice, roleAdviceTitle) {
			t.Errorf("gastown polecat should see role advice %q", roleAdviceTitle)
		}
	})

	// Test: Beads polecat sees only global (not gastown-specific)
	t.Run("beads_polecat_sees_only_global", func(t *testing.T) {
		beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
		advice := listAdviceForAgent(t, beadsPolecatDir, "beads/polecats/beta")

		if !hasAdviceWithTitle(advice, globalAdviceTitle) {
			t.Errorf("beads polecat should see global advice %q", globalAdviceTitle)
		}
		if hasAdviceWithTitle(advice, rigAdviceTitle) {
			t.Errorf("beads polecat should NOT see gastown rig advice %q", rigAdviceTitle)
		}
		if hasAdviceWithTitle(advice, roleAdviceTitle) {
			t.Errorf("beads polecat should NOT see gastown polecat role advice %q", roleAdviceTitle)
		}
	})

	// Test: Gastown witness sees rig but not polecat-role advice
	t.Run("gastown_witness_sees_rig_not_polecat_role", func(t *testing.T) {
		// Witness would be at gastown/witness
		advice := listAdviceForAgent(t, gasRigPath, "gastown/witness")

		if !hasAdviceWithTitle(advice, globalAdviceTitle) {
			t.Errorf("gastown witness should see global advice %q", globalAdviceTitle)
		}
		if !hasAdviceWithTitle(advice, rigAdviceTitle) {
			t.Errorf("gastown witness should see rig advice %q", rigAdviceTitle)
		}
		// Witness is not a polecat, so shouldn't see polecat-specific advice
		if hasAdviceWithTitle(advice, roleAdviceTitle) {
			t.Errorf("gastown witness should NOT see polecat role advice %q", roleAdviceTitle)
		}
	})
}

// TestAdviceEndToEndFlow verifies the complete flow of creating and displaying advice.
func TestAdviceEndToEndFlow(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping advice test")
	}

	townRoot := setupAdviceTestTown(t)

	// Initialize beads databases
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	beadsRigPath := filepath.Join(townRoot, "beads", "mayor", "rig")
	initBeadsDBWithPrefix(t, townRoot, "hq")
	initBeadsDBWithPrefix(t, gasRigPath, "gt")
	initBeadsDBWithPrefix(t, beadsRigPath, "bd")

	// Step 1: Create advice with --rig flag (using shorthand)
	t.Run("create_with_rig_shorthand", func(t *testing.T) {
		// Use bd advice add --rig=gastown
		cmd := bdcmd.CommandInDir(gasRigPath, "--sandbox", "advice", "add",
			"Rig shorthand test",
			"--rig=gastown",
			"-d", "Created with --rig shorthand")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd advice add --rig failed: %v\n%s", err, output)
		}

		// Verify gastown polecat sees it
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice := listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")
		if !hasAdviceWithTitle(advice, "Rig shorthand test") {
			t.Error("advice created with --rig shorthand should be visible to gastown polecat")
		}

		// Verify beads polecat does NOT see it
		beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
		advice = listAdviceForAgent(t, beadsPolecatDir, "beads/polecats/beta")
		if hasAdviceWithTitle(advice, "Rig shorthand test") {
			t.Error("advice created with --rig=gastown should NOT be visible to beads polecat")
		}
	})

	// Step 2: Create global advice (with -l global)
	t.Run("create_global_advice", func(t *testing.T) {
		// Create global advice in gastown rig
		cmd := bdcmd.CommandInDir(gasRigPath, "--sandbox", "advice", "add",
			"Global flow test",
			"-l", "global",
			"-d", "Created as global advice")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd advice add global failed in gastown: %v\n%s", err, output)
		}

		// Also create in beads rig (global advice needs to be in each database
		// where agents query - this reflects the decentralized beads architecture)
		cmd2 := bdcmd.CommandInDir(beadsRigPath, "--sandbox", "advice", "add",
			"Global flow test",
			"-l", "global",
			"-d", "Created as global advice")
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			t.Fatalf("bd advice add global failed in beads: %v\n%s", err2, output2)
		}

		// Verify both polecats see it
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice := listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")
		if !hasAdviceWithTitle(advice, "Global flow test") {
			t.Error("global advice should be visible to gastown polecat")
		}

		beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
		advice = listAdviceForAgent(t, beadsPolecatDir, "beads/polecats/beta")
		if !hasAdviceWithTitle(advice, "Global flow test") {
			t.Error("global advice should be visible to beads polecat")
		}
	})

	// Step 3: Create advice with --role flag (role-only, no rig)
	// Note: The subscription model uses OR logic - advice matches if ANY label matches.
	// So advice with BOTH rig:X AND role:Y is visible to ALL agents in rig X.
	// To test role-only isolation, we create advice with ONLY the role label.
	t.Run("create_with_role_shorthand", func(t *testing.T) {
		cmd := bdcmd.CommandInDir(gasRigPath, "--sandbox", "advice", "add",
			"Witness role test",
			"--role=witness", // Only role label, no rig
			"-d", "Created with --role shorthand for witness")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd advice add --role failed: %v\n%s", err, output)
		}

		// Verify gastown witness sees it
		advice := listAdviceForAgent(t, gasRigPath, "gastown/witness")
		if !hasAdviceWithTitle(advice, "Witness role test") {
			t.Error("advice created with --role=witness should be visible to gastown witness")
		}

		// Verify gastown polecat does NOT see it (different role)
		// This works because we only labeled with role:witness, not rig:gastown
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice = listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")
		if hasAdviceWithTitle(advice, "Witness role test") {
			t.Error("advice created with --role=witness should NOT be visible to gastown polecat")
		}
	})
}

// TestAdviceAgentSpecific verifies that agent-specific advice works correctly.
func TestAdviceAgentSpecific(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping advice test")
	}

	townRoot := setupAdviceTestTown(t)

	// Initialize beads databases
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	beadsRigPath := filepath.Join(townRoot, "beads", "mayor", "rig")
	initBeadsDBWithPrefix(t, townRoot, "hq")
	initBeadsDBWithPrefix(t, gasRigPath, "gt")
	initBeadsDBWithPrefix(t, beadsRigPath, "bd")

	// Create agent-specific advice for gastown/polecats/alpha
	cmd := bdcmd.CommandInDir(gasRigPath, "--sandbox", "advice", "add",
		"Alpha-specific advice",
		"--agent=gastown/polecats/alpha",
		"-d", "Only alpha should see this")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd advice add --agent failed: %v\n%s", err, output)
	}

	// Test: Alpha sees it
	t.Run("target_agent_sees_advice", func(t *testing.T) {
		gasPolecatDir := filepath.Join(townRoot, "gastown", "polecats", "alpha")
		advice := listAdviceForAgent(t, gasPolecatDir, "gastown/polecats/alpha")
		if !hasAdviceWithTitle(advice, "Alpha-specific advice") {
			t.Error("gastown/polecats/alpha should see their specific advice")
		}
	})

	// Test: Other gastown polecat doesn't see it
	t.Run("other_gastown_polecat_does_not_see", func(t *testing.T) {
		// Query as a different polecat
		advice := listAdviceForAgent(t, gasRigPath, "gastown/polecats/gamma")
		if hasAdviceWithTitle(advice, "Alpha-specific advice") {
			t.Error("gastown/polecats/gamma should NOT see alpha-specific advice")
		}
	})

	// Test: Beads polecat doesn't see it
	t.Run("beads_polecat_does_not_see", func(t *testing.T) {
		beadsPolecatDir := filepath.Join(townRoot, "beads", "polecats", "beta")
		advice := listAdviceForAgent(t, beadsPolecatDir, "beads/polecats/beta")
		if hasAdviceWithTitle(advice, "Alpha-specific advice") {
			t.Error("beads/polecats/beta should NOT see alpha-specific advice")
		}
	})
}
