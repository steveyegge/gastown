//go:build integration

// Package cmd contains integration tests for polecat lifecycle and work assignment.
//
// These tests cover the two gaps identified in the CI integration review
// (GH issue #20): polecat lifecycle (create, redirect, bd list/show) and
// work assignment (bead creation, hooking, convoy tracking).
//
// Run with: go test -tags=integration ./internal/cmd -run "TestPolecatLifecycle|TestWorkAssignment" -v
package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// setupPolecatWorkTestTown creates a Gas Town with rig beads plus a polecat
// directory that redirects to the rig. It also initializes town-level beads
// (for convoy tests using the hq- prefix).
//
// Layout:
//
//	townRoot/
//	  mayor/town.json
//	  .beads/          <- town-level (hq- prefix), initialized
//	  gastown/
//	    mayor/rig/
//	      .beads/      <- rig-level (gt- prefix), initialized
//	    polecats/alpha/
//	      .beads/redirect -> ../../mayor/rig/.beads
//	    crew/max/
//	      .beads/redirect -> ../../mayor/rig/.beads
func setupPolecatWorkTestTown(t *testing.T) (townRoot, rigDir, polecatDir string) {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Create mayor/town.json so FindTownRoot() works.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Routes: hq- -> town, gt- -> gastown rig
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},
		{Prefix: "gt-", Path: "gastown/mayor/rig"},
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Gastown rig structure
	rigDir = filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir gastown rig: %v", err)
	}
	gasBeadsDir := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(gasBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir gastown .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gasBeadsDir, "config.yaml"), []byte("prefix: gt\n"), 0644); err != nil {
		t.Fatalf("write gastown config: %v", err)
	}

	// Polecat directory with redirect -> rig .beads
	polecatDir = filepath.Join(townRoot, "gastown", "polecats", "alpha")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecats/alpha: %v", err)
	}
	polecatBeadsDir := filepath.Join(polecatDir, ".beads")
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecat .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(polecatBeadsDir, "redirect"), []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write polecat redirect: %v", err)
	}

	// Crew directory with redirect -> rig .beads
	crewDir := filepath.Join(townRoot, "gastown", "crew", "max")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatalf("mkdir crew/max: %v", err)
	}
	crewBeadsDir := filepath.Join(crewDir, ".beads")
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir crew .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(crewBeadsDir, "redirect"), []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write crew redirect: %v", err)
	}

	// Initialize rig beads (dolt backend)
	requireDoltServer(t)
	initBeadsDBWithPrefix(t, rigDir, "gt")

	// Initialize town-level beads (for convoys)
	initBeadsDBWithPrefix(t, townRoot, "hq")

	// Configure allowed_prefixes so convoy IDs (hq-cv-*) pass validation.
	prefixCmd := exec.Command("bd", "config", "set", "allowed_prefixes", "hq,hq-cv")
	prefixCmd.Dir = townRoot
	if out, err := prefixCmd.CombinedOutput(); err != nil {
		t.Fatalf("set allowed_prefixes: %v\n%s", err, out)
	}

	// Ensure custom types (convoy, etc.) are registered in town beads.
	if err := beads.EnsureCustomTypes(filepath.Join(townRoot, ".beads")); err != nil {
		t.Fatalf("EnsureCustomTypes: %v", err)
	}

	return townRoot, rigDir, polecatDir
}

// syncTownBeads runs bd sync to export the database to JSONL. In production
// the dolt daemon keeps these in sync. In tests we must sync manually after
// writes that use the bd CLI directly (e.g., convoy create, dep add).
func syncTownBeads(t *testing.T, townRoot string) {
	t.Helper()
	cmd := exec.Command("bd", "sync")
	cmd.Dir = townRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd sync in %s: %v\n%s", townRoot, err, out)
	}
}

// --- Polecat Lifecycle Tests ---

// TestPolecatLifecycle_CreateAndVerifyStructure verifies that a polecat's
// .beads/redirect file exists and that ResolveBeadsDir follows it to the rig.
func TestPolecatLifecycle_CreateAndVerifyStructure(t *testing.T) {
	_, rigDir, polecatDir := setupPolecatWorkTestTown(t)

	// Verify redirect file exists
	redirectPath := filepath.Join(polecatDir, ".beads", "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("read redirect file: %v", err)
	}
	if string(content) != "../../mayor/rig/.beads" {
		t.Errorf("redirect content = %q, want %q", string(content), "../../mayor/rig/.beads")
	}

	// Verify ResolveBeadsDir follows the redirect
	resolved := beads.ResolveBeadsDir(polecatDir)
	expectedBeadsDir := filepath.Join(rigDir, ".beads")
	if resolved != expectedBeadsDir {
		t.Errorf("ResolveBeadsDir(polecatDir) = %s, want %s", resolved, expectedBeadsDir)
	}
}

// TestPolecatLifecycle_BeadsShowFromPolecatContext verifies that bd show
// resolves issues through the polecat redirect.
func TestPolecatLifecycle_BeadsShowFromPolecatContext(t *testing.T) {
	_, rigDir, polecatDir := setupPolecatWorkTestTown(t)

	// Create an issue in the rig
	rigIssue := createTestIssue(t, rigDir, "Show from polecat context")

	// Show the issue from polecat context using the Go API
	polecatBeads := beads.New(polecatDir)
	shown, err := polecatBeads.Show(rigIssue.ID)
	if err != nil {
		t.Fatalf("Show(%s) from polecat: %v", rigIssue.ID, err)
	}

	if shown.ID != rigIssue.ID {
		t.Errorf("shown.ID = %s, want %s", shown.ID, rigIssue.ID)
	}
	if shown.Title != "Show from polecat context" {
		t.Errorf("shown.Title = %q, want %q", shown.Title, "Show from polecat context")
	}
	if shown.Status != "open" {
		t.Errorf("shown.Status = %q, want %q", shown.Status, "open")
	}
}

// TestPolecatLifecycle_BeadsListFromPolecatContext verifies that bd list
// returns all issues when run from a polecat directory.
func TestPolecatLifecycle_BeadsListFromPolecatContext(t *testing.T) {
	_, rigDir, polecatDir := setupPolecatWorkTestTown(t)

	// Create multiple issues in the rig
	titles := []string{"Polecat list issue 1", "Polecat list issue 2", "Polecat list issue 3"}
	var createdIDs []string
	for _, title := range titles {
		issue := createTestIssue(t, rigDir, title)
		createdIDs = append(createdIDs, issue.ID)
		t.Logf("Created issue: %s (%s)", issue.ID, title)
	}

	// List from polecat context
	polecatBeads := beads.New(polecatDir)
	issues, err := polecatBeads.List(beads.ListOptions{
		Status:   "open",
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("List from polecat: %v", err)
	}

	// Verify all created issues are visible
	for _, id := range createdIDs {
		if !hasIssueID(issues, id) {
			t.Errorf("issue %s not found in polecat list (got %d issues)", id, len(issues))
		}
	}
}

// TestPolecatLifecycle_AssigneeFormatConsistency verifies that the assignee
// format {rig}/{role}/{name} is consistent across hook and list operations.
func TestPolecatLifecycle_AssigneeFormatConsistency(t *testing.T) {
	_, rigDir, _ := setupPolecatWorkTestTown(t)

	b := beads.New(rigDir)

	agents := []struct {
		id    string
		title string
	}{
		{"gastown/polecats/alpha", "Alpha's task"},
		{"gastown/crew/max", "Max's task"},
	}

	status := beads.StatusHooked
	for _, agent := range agents {
		issue, err := b.Create(beads.CreateOptions{
			Title:    agent.title,
			Type:     "task",
			Priority: 2,
		})
		if err != nil {
			t.Fatalf("create bead for %s: %v", agent.id, err)
		}

		agentID := agent.id
		if err := b.Update(issue.ID, beads.UpdateOptions{
			Status:   &status,
			Assignee: &agentID,
		}); err != nil {
			t.Fatalf("hook bead to %s: %v", agent.id, err)
		}
	}

	// Verify each agent's beads are filterable by assignee
	for _, agent := range agents {
		hooked, err := b.List(beads.ListOptions{
			Status:   beads.StatusHooked,
			Assignee: agent.id,
			Priority: -1,
		})
		if err != nil {
			t.Fatalf("list for %s: %v", agent.id, err)
		}
		if len(hooked) != 1 {
			t.Errorf("agent %s: expected 1 hooked bead, got %d", agent.id, len(hooked))
		}
		if len(hooked) > 0 && hooked[0].Title != agent.title {
			t.Errorf("agent %s: title = %q, want %q", agent.id, hooked[0].Title, agent.title)
		}
	}
}

// --- Work Assignment Tests ---

// TestWorkAssignment_HookBeadToPolecatAndVerify verifies the full hook/unhook
// lifecycle: create bead, hook to polecat, verify from polecat context, unhook.
func TestWorkAssignment_HookBeadToPolecatAndVerify(t *testing.T) {
	_, rigDir, polecatDir := setupPolecatWorkTestTown(t)

	rigBeads := beads.New(rigDir)
	agentID := "gastown/polecats/alpha"

	// Create bead in rig
	issue, err := rigBeads.Create(beads.CreateOptions{
		Title:    "Work for alpha polecat",
		Type:     "task",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}
	t.Logf("Created bead: %s", issue.ID)

	// Hook to polecat
	status := beads.StatusHooked
	if err := rigBeads.Update(issue.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook bead: %v", err)
	}

	// Verify from polecat context: hooked bead is visible
	polecatBeads := beads.New(polecatDir)
	hooked, err := polecatBeads.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked from polecat: %v", err)
	}
	if len(hooked) != 1 {
		t.Fatalf("expected 1 hooked bead from polecat context, got %d", len(hooked))
	}
	if hooked[0].ID != issue.ID {
		t.Errorf("hooked bead ID = %s, want %s", hooked[0].ID, issue.ID)
	}

	// Verify status and assignee via Show
	shown, err := polecatBeads.Show(issue.ID)
	if err != nil {
		t.Fatalf("show from polecat: %v", err)
	}
	if shown.Status != beads.StatusHooked {
		t.Errorf("status = %q, want %q", shown.Status, beads.StatusHooked)
	}
	if shown.Assignee != agentID {
		t.Errorf("assignee = %q, want %q", shown.Assignee, agentID)
	}

	// Unhook: set status back to open
	openStatus := "open"
	if err := rigBeads.Update(issue.ID, beads.UpdateOptions{
		Status: &openStatus,
	}); err != nil {
		t.Fatalf("unhook bead: %v", err)
	}

	// Verify no hooked beads remain
	unhookedList, err := polecatBeads.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list after unhook: %v", err)
	}
	if len(unhookedList) != 0 {
		t.Errorf("expected 0 hooked beads after unhook, got %d", len(unhookedList))
	}

	// Verify bead is back to open
	afterUnhook, err := rigBeads.Show(issue.ID)
	if err != nil {
		t.Fatalf("show after unhook: %v", err)
	}
	if afterUnhook.Status != "open" {
		t.Errorf("status after unhook = %q, want %q", afterUnhook.Status, "open")
	}
}

// TestWorkAssignment_ConvoyCreateAndTrackIssues verifies convoy creation with
// tracked issues, progress tracking, and partial completion.
//
// All issues and the convoy are created in the same database (town beads)
// to enable reliable dep list verification. Cross-prefix routing between
// different beads databases is tested separately in TestSlingCrossRigRoutingResolution.
func TestWorkAssignment_ConvoyCreateAndTrackIssues(t *testing.T) {
	townRoot, _, _ := setupPolecatWorkTestTown(t)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	townBeads := beads.New(townRoot)

	// Create 3 issues in town beads (same database as convoy for dep list)
	var issueIDs []string
	for i := 1; i <= 3; i++ {
		issue := createTestIssue(t, townRoot, "Convoy tracked issue "+string(rune('0'+i)))
		issueIDs = append(issueIDs, issue.ID)
		t.Logf("Created issue: %s", issue.ID)
	}

	// Create a convoy in town beads using bd CLI (mirrors gt convoy create).
	convoyID := "hq-cv-test1"
	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=Test convoy for tracking",
		"--description=Convoy tracking 3 issues",
		"--json",
	}
	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeadsDir
	createOut, err := createCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create convoy: %v\n%s", err, createOut)
	}
	t.Logf("Created convoy: %s", convoyID)

	// Sync to keep JSONL in sync (no daemon in test env)
	syncTownBeads(t, townRoot)

	// Add tracking dependencies: convoy tracks each issue
	for _, issueID := range issueIDs {
		depArgs := []string{"dep", "add", convoyID, issueID, "--type=tracks"}
		depCmd := exec.Command("bd", depArgs...)
		depCmd.Dir = townBeadsDir
		if out, err := depCmd.CombinedOutput(); err != nil {
			t.Fatalf("dep add %s %s: %v\n%s", convoyID, issueID, err, out)
		}
	}

	// Sync after dep add
	syncTownBeads(t, townRoot)

	// Verify convoy lists tracked issues via bd dep list
	depListArgs := []string{"dep", "list", convoyID, "--direction=down", "--type=tracks", "--json"}
	depListCmd := exec.Command("bd", depListArgs...)
	depListCmd.Dir = townRoot
	depListOut, err := depListCmd.Output()
	if err != nil {
		combinedCmd := exec.Command("bd", depListArgs...)
		combinedCmd.Dir = townRoot
		combinedOut, _ := combinedCmd.CombinedOutput()
		t.Fatalf("dep list: %v\n%s", err, combinedOut)
	}

	var trackedDeps []beads.IssueDep
	if err := json.Unmarshal(depListOut, &trackedDeps); err != nil {
		t.Fatalf("parse dep list output: %v\n%s", err, depListOut)
	}

	if len(trackedDeps) != 3 {
		t.Fatalf("expected 3 tracked deps, got %d", len(trackedDeps))
	}

	// Close 2 of 3 issues
	for i := 0; i < 2; i++ {
		if err := townBeads.Close(issueIDs[i]); err != nil {
			t.Fatalf("close issue %s: %v", issueIDs[i], err)
		}
		t.Logf("Closed issue: %s", issueIDs[i])
	}

	// Sync after closing
	syncTownBeads(t, townRoot)

	// Re-query tracked deps and verify partial completion
	depListCmd2 := exec.Command("bd", depListArgs...)
	depListCmd2.Dir = townRoot
	depListOut2, err := depListCmd2.Output()
	if err != nil {
		t.Fatalf("dep list after partial close: %v", err)
	}

	var trackedDeps2 []beads.IssueDep
	if err := json.Unmarshal(depListOut2, &trackedDeps2); err != nil {
		t.Fatalf("parse dep list after close: %v\n%s", err, depListOut2)
	}

	closedCount := 0
	openCount := 0
	for _, dep := range trackedDeps2 {
		switch dep.Status {
		case "closed":
			closedCount++
		case "open":
			openCount++
		}
	}
	if closedCount != 2 {
		t.Errorf("expected 2 closed tracked issues, got %d", closedCount)
	}
	if openCount != 1 {
		t.Errorf("expected 1 open tracked issue, got %d", openCount)
	}

	// Convoy itself should still be open (not all items done)
	convoy, err := townBeads.Show(convoyID)
	if err != nil {
		t.Fatalf("show convoy: %v", err)
	}
	if convoy.Status != "open" {
		t.Errorf("convoy status = %q, want %q (not all items closed)", convoy.Status, "open")
	}
}

// TestWorkAssignment_FullWorkflowEndToEnd exercises the complete lifecycle:
// create issue -> hook to polecat -> verify from polecat context ->
// create convoy tracking the same issue -> close bead -> verify convoy
// sees the closure.
//
// All beads are created in town beads (hq- prefix) so that convoy dep list
// works within a single database. Cross-prefix routing is tested separately
// in TestSlingCrossRigRoutingResolution. The polecat hook/show lifecycle
// using rig beads is covered by TestWorkAssignment_HookBeadToPolecatAndVerify.
func TestWorkAssignment_FullWorkflowEndToEnd(t *testing.T) {
	townRoot, _, _ := setupPolecatWorkTestTown(t)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	townBeads := beads.New(townRoot)

	// Step 1: Create issue in town beads
	issue := createTestIssue(t, townRoot, "End-to-end workflow test")
	t.Logf("Created issue: %s", issue.ID)

	// Step 2: Verify issue is queryable
	shown, err := townBeads.Show(issue.ID)
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if shown.Status != "open" {
		t.Errorf("initial status = %q, want %q", shown.Status, "open")
	}

	// Step 3: Hook to an agent
	agentID := "gastown/polecats/alpha"
	status := beads.StatusHooked
	if err := townBeads.Update(issue.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook issue: %v", err)
	}

	// Verify hook
	hooked, err := townBeads.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked: %v", err)
	}
	if len(hooked) != 1 || hooked[0].ID != issue.ID {
		t.Fatalf("expected 1 hooked bead %s, got %d beads", issue.ID, len(hooked))
	}

	// Step 4: Create convoy tracking this issue
	convoyID := "hq-cv-e2e01"
	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=E2E test convoy",
		"--description=End-to-end test convoy",
		"--json",
	}
	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeadsDir
	if out, err := createCmd.CombinedOutput(); err != nil {
		t.Fatalf("create convoy: %v\n%s", err, out)
	}
	syncTownBeads(t, townRoot)

	// Add tracking dependency
	depCmd := exec.Command("bd", "dep", "add", convoyID, issue.ID, "--type=tracks")
	depCmd.Dir = townBeadsDir
	if out, err := depCmd.CombinedOutput(); err != nil {
		t.Fatalf("dep add: %v\n%s", err, out)
	}
	syncTownBeads(t, townRoot)

	// Verify convoy shows tracked issue
	depListArgs := []string{"dep", "list", convoyID, "--direction=down", "--type=tracks", "--json"}
	depListCmd := exec.Command("bd", depListArgs...)
	depListCmd.Dir = townRoot
	depListOut, err := depListCmd.Output()
	if err != nil {
		combinedCmd := exec.Command("bd", depListArgs...)
		combinedCmd.Dir = townRoot
		combinedOut, _ := combinedCmd.CombinedOutput()
		t.Fatalf("dep list before close: %v\n%s", err, combinedOut)
	}

	var deps []beads.IssueDep
	if err := json.Unmarshal(depListOut, &deps); err != nil {
		t.Fatalf("parse dep list: %v\n%s", err, depListOut)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 tracked dep, got %d", len(deps))
	}

	// Step 5: Close the bead
	if err := townBeads.Close(issue.ID); err != nil {
		t.Fatalf("close issue: %v", err)
	}
	syncTownBeads(t, townRoot)

	// Verify bead is closed
	closed, err := townBeads.Show(issue.ID)
	if err != nil {
		t.Fatalf("show closed issue: %v", err)
	}
	if closed.Status != "closed" {
		t.Errorf("issue status after close = %q, want %q", closed.Status, "closed")
	}

	// Step 6: Verify convoy dep list reflects closure
	depListCmd2 := exec.Command("bd", depListArgs...)
	depListCmd2.Dir = townRoot
	depListOut2, err := depListCmd2.Output()
	if err != nil {
		t.Fatalf("dep list after close: %v", err)
	}

	var depsAfter []beads.IssueDep
	if err := json.Unmarshal(depListOut2, &depsAfter); err != nil {
		t.Fatalf("parse dep list after close: %v\n%s", err, depListOut2)
	}
	if len(depsAfter) != 1 {
		t.Fatalf("expected 1 tracked dep after close, got %d", len(depsAfter))
	}
	if depsAfter[0].Status != "closed" {
		t.Errorf("tracked dep status after close = %q, want %q", depsAfter[0].Status, "closed")
	}

	// Convoy itself should still be open (auto-close is handled by gt convoy
	// check / the observer, not by bd itself).
	convoyAfter, err := townBeads.Show(convoyID)
	if err != nil {
		t.Fatalf("show convoy after close: %v", err)
	}
	if convoyAfter.Status != "open" {
		t.Errorf("convoy status = %q, want %q (auto-close is observer's job)", convoyAfter.Status, "open")
	}
}
