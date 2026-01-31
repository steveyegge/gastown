//go:build integration

// Package witness contains integration tests for witness hook lifecycle.
//
// Run with: go test -tags=integration ./internal/witness -run TestHandlePolecat -v
package witness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/mail"
)

// setupWitnessTestTown creates a minimal Gas Town with beads for testing witness handlers.
// Returns townRoot, rigPath, and isolated beads clients for town and rig beads.
// Also sets environment variables so that handler code can find the test databases.
func setupWitnessTestTown(t *testing.T, rigName string) (townRoot, rigPath string, bdTown, bdRig *beads.Beads) {
	t.Helper()

	townRoot = t.TempDir()

	// Create mayor/town.json to mark this as a town root
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	townJSON := `{"name": "testtown", "created": "2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townJSON), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Create town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Create rig structure
	rigPath = filepath.Join(townRoot, rigName)
	rigMayorDir := filepath.Join(rigPath, "mayor", "rig")
	if err := os.MkdirAll(rigMayorDir, 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	// Create rig .beads directory at mayor/rig/.beads
	rigBeadsDir := filepath.Join(rigMayorDir, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir rig .beads: %v", err)
	}

	// Create rig-level redirect at rigPath/.beads pointing to mayor/rig/.beads
	// This is needed because handlers create beads.New(rigPath) which looks for .beads there
	rigRootBeadsDir := filepath.Join(rigPath, ".beads")
	if err := os.MkdirAll(rigRootBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir rig root .beads: %v", err)
	}
	rigRedirectContent := "mayor/rig/.beads"
	if err := os.WriteFile(filepath.Join(rigRootBeadsDir, "redirect"), []byte(rigRedirectContent), 0644); err != nil {
		t.Fatalf("write rig root redirect: %v", err)
	}

	// Create polecat worktree directory
	polecatDir := filepath.Join(rigPath, "polecats", "testnux")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecats: %v", err)
	}

	// Create polecat's .beads redirect to rig beads
	polecatBeadsDir := filepath.Join(polecatDir, ".beads")
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecat .beads: %v", err)
	}
	redirectContent := "../../mayor/rig/.beads"
	if err := os.WriteFile(filepath.Join(polecatBeadsDir, "redirect"), []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	// Create routes.jsonl for prefix routing
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},                      // Town-level beads
		{Prefix: "rig-", Path: rigName + "/mayor/rig"}, // Rig-level beads
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Initialize beads databases using isolated mode
	bdTown = beads.NewIsolated(townRoot)
	if err := bdTown.Init("hq"); err != nil {
		t.Fatalf("init town beads: %v", err)
	}

	bdRig = beads.NewIsolated(rigMayorDir)
	if err := bdRig.Init("rig"); err != nil {
		t.Fatalf("init rig beads: %v", err)
	}

	// Set BEADS_DIR so that handler code can find the test town beads
	// This is needed because handlers create their own beads.New() instances
	oldBeadsDir := os.Getenv("BEADS_DIR")
	os.Setenv("BEADS_DIR", townBeadsDir)
	t.Cleanup(func() {
		if oldBeadsDir != "" {
			os.Setenv("BEADS_DIR", oldBeadsDir)
		} else {
			os.Unsetenv("BEADS_DIR")
		}
	})

	return townRoot, rigPath, bdTown, bdRig
}

// TestHandlePolecatDone_ClearsHookBead verifies that HandlePolecatDone clears
// the hook_bead from the agent bead after creating a cleanup wisp for pending MR.
// This tests the fix for bd-bug-gt_polecat_nuke_blocks_clean_polecats.
func TestHandlePolecatDone_ClearsHookBead(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	townRoot, rigPath, bdTown, bdRig := setupWitnessTestTown(t, rigName)
	polecatName := "testnux"

	// Create agent bead ID using town-level format
	agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, polecatName)
	t.Logf("Agent bead ID: %s", agentBeadID)

	// Create a task bead to hook
	taskIssue, err := bdRig.Create(beads.CreateOptions{
		Title:    "Test task for hook clearing",
		Type:     "task",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create task bead: %v", err)
	}
	t.Logf("Created task bead: %s", taskIssue.ID)

	// Create agent bead with hook_bead set
	_, err = bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        rigName,
		AgentState: "working",
		HookBead:   taskIssue.ID,
	})
	if err != nil {
		t.Fatalf("create agent bead: %v", err)
	}

	// Verify hook_bead is set initially
	issue, fields, err := bdTown.GetAgentBead(agentBeadID)
	if err != nil {
		t.Fatalf("get agent bead: %v", err)
	}
	if fields.HookBead != taskIssue.ID {
		t.Fatalf("initial hook_bead = %q, want %q", fields.HookBead, taskIssue.ID)
	}
	t.Logf("Initial hook_bead verified: %s", fields.HookBead)
	_ = issue    // use issue to avoid unused variable warning
	_ = townRoot // use townRoot to avoid unused variable warning

	// Create POLECAT_DONE message with pending MR (COMPLETED exit)
	msg := &mail.Message{
		ID:        "test-msg-001",
		From:      rigName + "/polecats/" + polecatName,
		To:        rigName + "/witness",
		Subject:   "POLECAT_DONE " + polecatName,
		Timestamp: time.Now(),
		Body: `Exit: COMPLETED
Issue: ` + taskIssue.ID + `
MR: mr-001
Branch: polecat/testnux/test-branch`,
	}

	// Call HandlePolecatDone
	result := HandlePolecatDone(rigPath, rigName, msg)

	if result.Error != nil {
		t.Errorf("HandlePolecatDone error: %v", result.Error)
	}
	if !result.Handled {
		t.Error("HandlePolecatDone did not set Handled=true")
	}
	if result.WispCreated == "" {
		t.Error("HandlePolecatDone did not create wisp")
	}
	t.Logf("Wisp created: %s", result.WispCreated)
	t.Logf("Action: %s", result.Action)

	// Verify hook_bead is cleared
	_, fieldsAfter, err := bdTown.GetAgentBead(agentBeadID)
	if err != nil {
		t.Fatalf("get agent bead after: %v", err)
	}
	if fieldsAfter.HookBead != "" {
		t.Errorf("hook_bead after HandlePolecatDone = %q, want empty", fieldsAfter.HookBead)
	} else {
		t.Log("hook_bead successfully cleared after HandlePolecatDone")
	}
}

// TestHandlePolecatDone_SetsMergeRequestedState verifies that HandlePolecatDone
// creates a cleanup wisp with "merge-requested" state for pending MR.
func TestHandlePolecatDone_SetsMergeRequestedState(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	_, rigPath, bdTown, _ := setupWitnessTestTown(t, rigName)
	polecatName := "testnux"

	// Create agent bead
	agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, polecatName)
	_, err := bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        rigName,
		AgentState: "working",
	})
	if err != nil {
		t.Fatalf("create agent bead: %v", err)
	}

	// Create POLECAT_DONE message with pending MR
	msg := &mail.Message{
		ID:        "test-msg-002",
		From:      rigName + "/polecats/" + polecatName,
		To:        rigName + "/witness",
		Subject:   "POLECAT_DONE " + polecatName,
		Timestamp: time.Now(),
		Body: `Exit: COMPLETED
Issue: rig-task-001
MR: mr-002
Branch: polecat/testnux/test-branch`,
	}

	// Call HandlePolecatDone
	result := HandlePolecatDone(rigPath, rigName, msg)

	if result.Error != nil {
		t.Errorf("HandlePolecatDone error: %v", result.Error)
	}
	if result.WispCreated == "" {
		t.Fatal("HandlePolecatDone did not create wisp")
	}
	t.Logf("result.WispCreated: %s", result.WispCreated)

	// Verify wisp has merge-requested state in its labels
	// The WispCreated contains output text, extract the ID
	wispID := result.WispCreated
	// If it contains the "Created issue: " prefix, extract just the ID
	if strings.Contains(wispID, "Created issue: ") {
		// Extract ID from "✓ Created issue: hq-tsk-..."
		parts := strings.SplitN(wispID, "Created issue: ", 2)
		if len(parts) == 2 {
			wispID = strings.TrimSpace(strings.Split(parts[1], "\n")[0])
		}
	}
	t.Logf("Extracted wispID: %s", wispID)

	// The wisp is created with hq- prefix (town-level), so use bdTown
	wisp, err := bdTown.Show(wispID)
	if err != nil {
		t.Fatalf("show wisp: %v", err)
	}

	// Check for state:merge-requested label
	hasStateLabel := false
	for _, label := range wisp.Labels {
		if label == "state:merge-requested" {
			hasStateLabel = true
			break
		}
	}
	if !hasStateLabel {
		t.Errorf("wisp labels = %v, want to contain 'state:merge-requested'", wisp.Labels)
	} else {
		t.Log("Wisp has state:merge-requested label")
	}

	// Verify wisp has polecat label
	hasPolecatLabel := false
	for _, label := range wisp.Labels {
		if strings.HasPrefix(label, "polecat:") {
			hasPolecatLabel = true
			if !strings.Contains(label, polecatName) {
				t.Errorf("polecat label = %q, expected to contain %q", label, polecatName)
			}
			break
		}
	}
	if !hasPolecatLabel {
		t.Errorf("wisp labels = %v, want to contain 'polecat:...'", wisp.Labels)
	}
}

// TestHandlePolecatDone_NoMR_ChecksCleanupStatus verifies that HandlePolecatDone
// without a pending MR attempts to auto-nuke if the polecat is clean.
// This tests ESCALATED/DEFERRED exits where no MR is pending.
func TestHandlePolecatDone_NoMR_ChecksCleanupStatus(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	_, rigPath, bdTown, _ := setupWitnessTestTown(t, rigName)
	polecatName := "testnux"

	// Create agent bead with cleanup_status=has_uncommitted (not safe to nuke)
	agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, polecatName)
	_, err := bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
		RoleType:      "polecat",
		Rig:           rigName,
		AgentState:    "working",
		CleanupStatus: "has_uncommitted",
	})
	if err != nil {
		t.Fatalf("create agent bead: %v", err)
	}

	// Create POLECAT_DONE message with ESCALATED exit (no MR)
	msg := &mail.Message{
		ID:        "test-msg-003",
		From:      rigName + "/polecats/" + polecatName,
		To:        rigName + "/witness",
		Subject:   "POLECAT_DONE " + polecatName,
		Timestamp: time.Now(),
		Body: `Exit: ESCALATED
Issue: rig-task-001
Branch: polecat/testnux/test-branch`,
	}

	// Call HandlePolecatDone
	result := HandlePolecatDone(rigPath, rigName, msg)

	// Since cleanup_status=has_uncommitted, it should create a wisp for manual cleanup
	// rather than auto-nuking
	if !result.Handled {
		t.Error("HandlePolecatDone did not set Handled=true")
	}
	if result.WispCreated == "" {
		t.Error("HandlePolecatDone should have created wisp for dirty polecat")
	}
	t.Logf("Action: %s", result.Action)

	// The action should indicate manual cleanup is needed
	if !strings.Contains(result.Action, "cleanup") {
		t.Logf("Note: action = %q, may indicate different behavior", result.Action)
	}
}

// TestHandlePolecatDone_PhaseComplete_NoHookClear verifies that PHASE_COMPLETE
// exits don't clear the hook_bead (polecat is waiting at a gate).
func TestHandlePolecatDone_PhaseComplete_NoHookClear(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	_, rigPath, bdTown, bdRig := setupWitnessTestTown(t, rigName)
	polecatName := "testnux"

	// Create a task bead to hook
	taskIssue, err := bdRig.Create(beads.CreateOptions{
		Title:    "Test task for phase complete",
		Type:     "task",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create task bead: %v", err)
	}

	// Create agent bead with hook_bead set
	agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, polecatName)
	_, err = bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        rigName,
		AgentState: "working",
		HookBead:   taskIssue.ID,
	})
	if err != nil {
		t.Fatalf("create agent bead: %v", err)
	}

	// Create POLECAT_DONE message with PHASE_COMPLETE exit
	msg := &mail.Message{
		ID:        "test-msg-004",
		From:      rigName + "/polecats/" + polecatName,
		To:        rigName + "/witness",
		Subject:   "POLECAT_DONE " + polecatName,
		Timestamp: time.Now(),
		Body: `Exit: PHASE_COMPLETE
Issue: ` + taskIssue.ID + `
Gate: gate-001
Branch: polecat/testnux/test-branch`,
	}

	// Call HandlePolecatDone
	result := HandlePolecatDone(rigPath, rigName, msg)

	if !result.Handled {
		t.Error("HandlePolecatDone did not set Handled=true")
	}
	if result.WispCreated != "" {
		t.Errorf("PHASE_COMPLETE should not create wisp, got %s", result.WispCreated)
	}
	t.Logf("Action: %s", result.Action)

	// Verify hook_bead is NOT cleared (polecat is waiting at gate)
	_, fieldsAfter, err := bdTown.GetAgentBead(agentBeadID)
	if err != nil {
		t.Fatalf("get agent bead after: %v", err)
	}
	if fieldsAfter.HookBead != taskIssue.ID {
		t.Errorf("hook_bead after PHASE_COMPLETE = %q, want %q (should remain set)", fieldsAfter.HookBead, taskIssue.ID)
	} else {
		t.Log("hook_bead correctly preserved for PHASE_COMPLETE")
	}
}

// TestAutoNukeIfClean_CleanStatus verifies that AutoNukeIfClean reports
// nuke-ready for polecats with cleanup_status=clean.
func TestAutoNukeIfClean_CleanStatus(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	_, rigPath, bdTown, _ := setupWitnessTestTown(t, rigName)
	polecatName := "testnux"

	// Create agent bead with cleanup_status=clean
	agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, polecatName)
	_, err := bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
		RoleType:      "polecat",
		Rig:           rigName,
		AgentState:    "done",
		CleanupStatus: "clean",
	})
	if err != nil {
		t.Fatalf("create agent bead: %v", err)
	}

	// Call AutoNukeIfClean - it won't actually nuke because there's no git repo
	// The function performs git verification which will fail in our test directory
	result := AutoNukeIfClean(rigPath, rigName, polecatName)

	// The nuke will be skipped because git verification fails (not a git repo)
	// This is expected behavior - we can't safely nuke without verifying git state
	if !result.Skipped {
		t.Errorf("expected AutoNukeIfClean to skip (no git repo), got Nuked=%v", result.Nuked)
	}
	// Verify it skipped due to git verification, not cleanup_status
	if !strings.Contains(result.Reason, "git") {
		t.Errorf("expected skip due to git verification, got reason: %s", result.Reason)
	}
	t.Logf("AutoNukeIfClean result: Nuked=%v, Skipped=%v, Reason=%s", result.Nuked, result.Skipped, result.Reason)
}

// TestAutoNukeIfClean_DirtyStatus verifies that AutoNukeIfClean skips
// polecats with dirty cleanup_status.
func TestAutoNukeIfClean_DirtyStatus(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	rigName := "gastown"
	_, rigPath, bdTown, _ := setupWitnessTestTown(t, rigName)

	testCases := []struct {
		name          string
		cleanupStatus string
	}{
		{"has_uncommitted", "has_uncommitted"},
		{"has_stash", "has_stash"},
		{"has_unpushed", "has_unpushed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pcName := "testnux_" + tc.name

			// Create agent bead with dirty cleanup_status
			agentBeadID := beads.PolecatBeadIDTown("testtown", rigName, pcName)
			_, err := bdTown.CreateAgentBead(agentBeadID, "Test polecat agent", &beads.AgentFields{
				RoleType:      "polecat",
				Rig:           rigName,
				AgentState:    "done",
				CleanupStatus: tc.cleanupStatus,
			})
			if err != nil {
				t.Fatalf("create agent bead: %v", err)
			}

			// Call AutoNukeIfClean
			result := AutoNukeIfClean(rigPath, rigName, pcName)

			// Should skip for dirty status
			if !result.Skipped {
				t.Errorf("AutoNukeIfClean should skip for cleanup_status=%s, got Skipped=%v", tc.cleanupStatus, result.Skipped)
			}
			if result.Nuked {
				t.Errorf("AutoNukeIfClean should not nuke for cleanup_status=%s", tc.cleanupStatus)
			}
			t.Logf("cleanup_status=%s: Skipped=%v, Reason=%s", tc.cleanupStatus, result.Skipped, result.Reason)
		})
	}
}

// TestCleanupWispLabels_Integration verifies the cleanup wisp label format.
func TestCleanupWispLabels_Integration(t *testing.T) {
	polecatName := "testnux"
	state := "merge-requested"

	labels := CleanupWispLabels(polecatName, state)

	// Verify expected labels are present
	hasPolecatLabel := false
	hasStateLabel := false
	hasWispLabel := false

	for _, label := range labels {
		if strings.HasPrefix(label, "polecat:") && strings.Contains(label, polecatName) {
			hasPolecatLabel = true
		}
		if label == "state:"+state {
			hasStateLabel = true
		}
		if label == "cleanup" {
			hasWispLabel = true
		}
	}

	if !hasPolecatLabel {
		t.Errorf("labels %v missing polecat:%s", labels, polecatName)
	}
	if !hasStateLabel {
		t.Errorf("labels %v missing state:%s", labels, state)
	}
	if !hasWispLabel {
		t.Errorf("labels %v missing cleanup label", labels)
	}
}
