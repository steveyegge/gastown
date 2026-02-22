//go:build integration

// Package cmd contains integration tests for the capacity scheduler subsystem.
// These tests exercise scheduler CLI operations (schedule, list, status, dispatch
// dry-run, circuit breaker) against a Dolt-server-backed beads DB. No Claude
// credentials, no agent sessions.
//
// Requires a Dolt server on port 3307 (managed by requireDoltServer/cleanupDoltServer).
//
// Run with:
//
//	go test -tags=integration -run 'TestScheduler' -timeout 5m -count=1 -v ./internal/cmd/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
)

// schedulerTestCounter generates unique prefixes for each test to isolate Dolt
// databases on the shared server. Without this, beads from earlier tests
// leak into later tests (all using the same database).
var schedulerTestCounter atomic.Int32

// initBeadsDBForServer initializes a beads DB that can operate against the
// shared Dolt test server on port 3307. Uses local init (bd init --prefix)
// which reliably creates the schema, then bd auto-detects the running server
// at runtime for SQL operations.
//
// Note: bd init --server (fresh, not migration) fails to create tables in CI.
// Local init works reliably and bd auto-detects the server for runtime ops.
func initBeadsDBForServer(t *testing.T, dir, prefix string) {
	t.Helper()

	cmd := exec.Command("bd", "init", "--prefix", prefix)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	t.Logf("bd init --prefix %s in %s: exit=%v\n%s", prefix, dir, err, out)
	if err != nil {
		t.Fatalf("bd init failed in %s: %v\n%s", dir, err, out)
	}

	// Create empty issues.jsonl to prevent bd auto-export from corrupting
	// routes.jsonl (same as initBeadsDBWithPrefix does).
	issuesPath := filepath.Join(dir, ".beads", "issues.jsonl")
	if err := os.WriteFile(issuesPath, []byte(""), 0644); err != nil {
		t.Fatalf("create issues.jsonl in %s: %v", dir, err)
	}
}

// setupSchedulerIntegrationTown creates a minimal town filesystem for scheduler tests.
// Uses the shared Dolt test server on port 3307 (managed by requireDoltServer)
// for beads databases. No gt install, no Claude credentials, no agent sessions.
//
// Returns (hqPath, rigPath, gtBinary, env).
func setupSchedulerIntegrationTown(t *testing.T) (hqPath, rigPath, gtBinary string, env []string) {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping scheduler integration test")
	}

	requireDoltServer(t)
	cleanStaleBeadsDatabases(t)
	gtBinary = buildGT(t)

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Configure git/dolt identity in isolated HOME (needed by bd init --server
	// which initializes a git repo inside .beads/).
	configureTestGitIdentity(t, tmpDir)

	// Generate unique prefixes per test to avoid cross-test data leakage on
	// the shared Dolt server. Each test gets its own databases (e.g., beads_h3, beads_r3).
	n := schedulerTestCounter.Add(1)
	hqPrefix := fmt.Sprintf("h%d", n)
	rigPrefix := fmt.Sprintf("r%d", n)

	hqPath = filepath.Join(tmpDir, "test-hq")
	rigPath = filepath.Join(hqPath, "testrig", "mayor", "rig")

	// --- mayor/ ---
	mayorDir := filepath.Join(hqPath, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	writeJSONFile(t, filepath.Join(mayorDir, "town.json"), &config.TownConfig{
		Type:    "town",
		Name:    "test",
		Version: config.CurrentTownVersion,
	})

	rigsConfig := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"testrig": {
				GitURL: "file:///dev/null",
				BeadsConfig: &config.BeadsConfig{
					Prefix: rigPrefix,
				},
			},
		},
	}
	if err := config.SaveRigsConfig(filepath.Join(mayorDir, "rigs.json"), rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	// --- settings/ (written later by configureScheduler, create dir now) ---
	if err := os.MkdirAll(filepath.Join(hqPath, "settings"), 0755); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}

	// --- town-level .beads/ ---
	townBeadsDir := filepath.Join(hqPath, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}
	routes := []beads.Route{
		{Prefix: hqPrefix + "-", Path: "."},
		{Prefix: rigPrefix + "-", Path: "testrig/mayor/rig"},
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}
	initBeadsDBForServer(t, hqPath, hqPrefix)

	// --- testrig directory (loadRig checks os.Stat on townRoot/<rigName>) ---
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("mkdir rigPath: %v", err)
	}
	initBeadsDBForServer(t, rigPath, rigPrefix)

	// Redirect: testrig/.beads/ → mayor/rig/.beads
	// beadsSearchDirs scans townRoot/<dir>/.beads — the redirect lets bd commands
	// from testrig/ resolve to the actual rig beads DB.
	rigBeadsRedirect := filepath.Join(hqPath, "testrig", ".beads")
	if err := os.MkdirAll(rigBeadsRedirect, 0755); err != nil {
		t.Fatalf("mkdir rig .beads redirect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsRedirect, "redirect"), []byte("mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write rig redirect: %v", err)
	}

	// --- Environment ---
	env = cleanSchedulerTestEnv(tmpDir)

	// Configure scheduler with defaults
	configureScheduler(t, hqPath, 10, 3)

	return hqPath, rigPath, gtBinary, env
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

// TestSchedulerCircuitBreakerExclusion verifies that a bead with dispatch_failures
// >= maxDispatchFailures is excluded from scheduler list and dry-run dispatch.
func TestSchedulerCircuitBreakerExclusion(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Create a bead and manually set it up as circuit-broken.
	beadID := createTestBead(t, rigPath, "Circuit breaker test")

	// Add gt:queued label
	addBeadLabel(t, beadID, capacity.LabelScheduled, rigPath)

	// Write scheduler metadata with dispatch_failures=3 (circuit-broken)
	meta := capacity.NewMetadata("testrig")
	meta.DispatchFailures = maxDispatchFailures // 3
	meta.LastFailure = "simulated failure"
	desc := capacity.FormatMetadata(meta)
	updateBeadDescription(t, beadID, desc, rigPath)

	// Verify: scheduler list should exclude this bead
	listed := getSchedulerList(t, gtBinary, hqPath, env)
	for _, item := range listed {
		if item["id"] == beadID {
			t.Errorf("circuit-broken bead %s should be excluded from scheduler list", beadID)
		}
	}

	// Verify: scheduler status should not count this bead
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	if total != 0 {
		t.Errorf("queued_total = %d, want 0 (circuit-broken bead excluded)", total)
	}

	// Verify: dry-run dispatch should not pick this bead
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "run", "--dry-run")
	if strings.Contains(out, beadID) {
		t.Errorf("dry-run dispatch should not mention circuit-broken bead %s", beadID)
	}
}

// TestSchedulerMissingMetadataQuarantine verifies that a bead with gt:queued
// but no scheduler metadata gets gt:dispatch-failed on dispatch attempt.
func TestSchedulerMissingMetadataQuarantine(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Create a bead with gt:queued but NO scheduler metadata (simulating a
	// manually-labeled bead that bypassed gt sling deferred dispatch (max_polecats > 0)).
	beadID := createTestBead(t, rigPath, "Missing metadata test")
	addBeadLabel(t, beadID, capacity.LabelScheduled, rigPath)

	// Run dispatch (non-dry-run). The bead has no metadata so dispatch
	// should quarantine it with gt:dispatch-failed.
	runGTCmdMayFail(t, gtBinary, hqPath, env, "scheduler", "run", "--batch", "1")

	// Verify: bead should now have gt:dispatch-failed label
	if !beadHasLabel(t, beadID, "gt:dispatch-failed", rigPath) {
		t.Errorf("bead %s should have gt:dispatch-failed label after metadata-less dispatch", beadID)
	}
}

// TestSchedulerAutoConvoyCreation verifies that gt sling deferred dispatch (max_polecats > 0)
// creates an auto-convoy, stores the convoy ID in scheduler metadata, and the
// convoy is resolvable via bd show.
func TestSchedulerAutoConvoyCreation(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	beadID := createTestBead(t, rigPath, "Auto convoy test")

	// Schedule via gt sling deferred dispatch (max_polecats > 0)
	slingToScheduler(t, gtBinary, hqPath, env, beadID, "testrig")

	// Verify: bead should have gt:queued label
	if !beadHasLabel(t, beadID, capacity.LabelScheduled, rigPath) {
		t.Errorf("bead %s should have gt:queued label after sling deferred dispatch (max_polecats > 0)", beadID)
	}

	// Verify: description should contain scheduler metadata with convoy ID
	desc := getBeadDescription(t, beadID, rigPath)
	meta := capacity.ParseMetadata(desc)
	if meta == nil {
		t.Fatalf("bead %s has no scheduler metadata after sling deferred dispatch (max_polecats > 0)", beadID)
	}
	if meta.TargetRig != "testrig" {
		t.Errorf("target_rig = %q, want %q", meta.TargetRig, "testrig")
	}
	if meta.Convoy == "" {
		t.Fatalf("convoy ID not stored in scheduler metadata")
	}

	// Verify: convoy is resolvable via bd show from hq
	cmd := exec.Command("bd", "show", meta.Convoy, "--json", "--allow-stale")
	cmd.Dir = hqPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bd show convoy %s failed: %v", meta.Convoy, err)
	}
	var convoys []struct {
		ID        string `json:"id"`
		IssueType string `json:"issue_type"`
	}
	if err := json.Unmarshal(out, &convoys); err != nil {
		t.Fatalf("parse convoy show: %v", err)
	}
	if len(convoys) == 0 {
		t.Fatalf("convoy %s not found via bd show", meta.Convoy)
	}
	if convoys[0].IssueType != "convoy" {
		t.Errorf("convoy issue_type = %q, want %q", convoys[0].IssueType, "convoy")
	}

	// Verify: convoy has a "tracks" dependency pointing to the rig bead.
	// This is the core cross-rig link: convoy lives in HQ DB, bead in rig DB.
	depCmd := exec.Command("bd", "dep", "list", meta.Convoy, "--direction=down", "--type=tracks", "--json")
	depCmd.Dir = hqPath
	depOut, err := depCmd.Output()
	if err != nil {
		t.Fatalf("bd dep list %s --type=tracks failed: %v", meta.Convoy, err)
	}
	var deps []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(depOut, &deps); err != nil {
		t.Fatalf("parse dep list: %v\nraw: %s", err, depOut)
	}
	foundTracked := false
	for _, dep := range deps {
		if dep.ID == beadID {
			foundTracked = true
			break
		}
	}
	if !foundTracked {
		t.Errorf("convoy %s should track bead %s via tracks dep, got deps: %s", meta.Convoy, beadID, depOut)
	}
}

// TestSchedulerBlockedStatusReporting verifies that scheduler list correctly reports
// blocked:true/false and scheduler status reports correct queued_ready count.
func TestSchedulerBlockedStatusReporting(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Create three beads: one to be ready, one to be blocked, one blocker
	readyID := createTestBead(t, rigPath, "Ready bead")
	blockedID := createTestBead(t, rigPath, "Blocked bead")
	blockerID := createTestBead(t, rigPath, "Blocker bead")

	// Schedule ready and blocked beads via gt sling deferred dispatch (max_polecats > 0)
	slingToScheduler(t, gtBinary, hqPath, env, readyID, "testrig")
	slingToScheduler(t, gtBinary, hqPath, env, blockedID, "testrig")

	// Add blocking dependency: blockerID blocks blockedID
	addBeadDependency(t, blockedID, blockerID, rigPath)

	// Verify: scheduler list should show both, with correct blocked status
	listed := getSchedulerList(t, gtBinary, hqPath, env)
	if len(listed) < 2 {
		t.Fatalf("scheduler list returned %d items, want >= 2", len(listed))
	}

	foundReady := false
	foundBlocked := false
	for _, item := range listed {
		id, _ := item["id"].(string)
		blocked, _ := item["blocked"].(bool)
		switch id {
		case readyID:
			foundReady = true
			if blocked {
				t.Errorf("bead %s should NOT be blocked", readyID)
			}
		case blockedID:
			foundBlocked = true
			if !blocked {
				t.Errorf("bead %s SHOULD be blocked", blockedID)
			}
		}
	}
	if !foundReady {
		t.Errorf("ready bead %s not found in scheduler list", readyID)
	}
	if !foundBlocked {
		t.Errorf("blocked bead %s not found in scheduler list", blockedID)
	}

	// Verify: scheduler status should report correct counts
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	ready := int(status["queued_ready"].(float64))
	if total != 2 {
		t.Errorf("queued_total = %d, want 2", total)
	}
	if ready != 1 {
		t.Errorf("queued_ready = %d, want 1", ready)
	}
}

// TestSchedulerSlingDryRun verifies that gt sling deferred dispatch (max_polecats > 0) --dry-run
// has no side effects: no label added, no metadata written, no convoy created.
func TestSchedulerSlingDryRun(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	beadID := createTestBead(t, rigPath, "Dry run test")

	// Capture description before dry-run
	descBefore := getBeadDescription(t, beadID, rigPath)

	// Run sling deferred dispatch (max_polecats > 0) --dry-run
	slingToScheduler(t, gtBinary, hqPath, env, beadID, "testrig", "--dry-run")

	// Verify: no gt:queued label
	if beadHasLabel(t, beadID, capacity.LabelScheduled, rigPath) {
		t.Errorf("dry-run should NOT add gt:queued label")
	}

	// Verify: description unchanged
	descAfter := getBeadDescription(t, beadID, rigPath)
	if descAfter != descBefore {
		t.Errorf("dry-run should NOT modify description\nbefore: %q\nafter:  %q", descBefore, descAfter)
	}

	// Verify: no convoy created (HQ beads DB should have no convoy issues)
	cmd := exec.Command("bd", "list", "--type=convoy", "--json")
	cmd.Dir = hqPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bd list convoys failed: %v", err)
	}
	var convoys []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &convoys); err != nil {
		t.Fatalf("parse convoy list: %v", err)
	}
	if len(convoys) != 0 {
		t.Errorf("dry-run should NOT create convoys, found %d", len(convoys))
	}
}

// --------------------------------------------------------------------------
// Multi-rig tests
// --------------------------------------------------------------------------

// setupMultiRigSchedulerTown creates a town with TWO rigs for cross-rig tests.
// Returns (hqPath, rig1Path, rig2Path, gtBinary, env).
func setupMultiRigSchedulerTown(t *testing.T) (hqPath, rig1Path, rig2Path, gtBinary string, env []string) {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping scheduler integration test")
	}

	requireDoltServer(t)
	cleanStaleBeadsDatabases(t)
	gtBinary = buildGT(t)

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	configureTestGitIdentity(t, tmpDir)

	// Unique prefixes: hN for HQ, rN for rig1, sN for rig2.
	n := schedulerTestCounter.Add(1)
	hqPrefix := fmt.Sprintf("h%d", n)
	rig1Prefix := fmt.Sprintf("r%d", n)
	rig2Prefix := fmt.Sprintf("s%d", n)

	hqPath = filepath.Join(tmpDir, "test-hq")
	rig1Path = filepath.Join(hqPath, "rig1", "mayor", "rig")
	rig2Path = filepath.Join(hqPath, "rig2", "mayor", "rig")

	// --- mayor/ ---
	mayorDir := filepath.Join(hqPath, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	writeJSONFile(t, filepath.Join(mayorDir, "town.json"), &config.TownConfig{
		Type:    "town",
		Name:    "test",
		Version: config.CurrentTownVersion,
	})

	rigsConfig := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"rig1": {
				GitURL: "file:///dev/null",
				BeadsConfig: &config.BeadsConfig{
					Prefix: rig1Prefix,
				},
			},
			"rig2": {
				GitURL: "file:///dev/null",
				BeadsConfig: &config.BeadsConfig{
					Prefix: rig2Prefix,
				},
			},
		},
	}
	if err := config.SaveRigsConfig(filepath.Join(mayorDir, "rigs.json"), rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	// --- settings/ ---
	if err := os.MkdirAll(filepath.Join(hqPath, "settings"), 0755); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}

	// --- town-level .beads/ with routes for all three DBs ---
	townBeadsDir := filepath.Join(hqPath, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}
	routes := []beads.Route{
		{Prefix: hqPrefix + "-", Path: "."},
		{Prefix: rig1Prefix + "-", Path: "rig1/mayor/rig"},
		{Prefix: rig2Prefix + "-", Path: "rig2/mayor/rig"},
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}
	initBeadsDBForServer(t, hqPath, hqPrefix)

	// --- rig1 ---
	if err := os.MkdirAll(rig1Path, 0755); err != nil {
		t.Fatalf("mkdir rig1Path: %v", err)
	}
	initBeadsDBForServer(t, rig1Path, rig1Prefix)
	// Write routes to rig1's .beads/ so bd can resolve cross-rig IDs (needed for
	// cross-rig dep creation via external refs).
	if err := beads.WriteRoutes(filepath.Join(rig1Path, ".beads"), routes); err != nil {
		t.Fatalf("write rig1 routes: %v", err)
	}
	rig1Redirect := filepath.Join(hqPath, "rig1", ".beads")
	if err := os.MkdirAll(rig1Redirect, 0755); err != nil {
		t.Fatalf("mkdir rig1 redirect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rig1Redirect, "redirect"), []byte("mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write rig1 redirect: %v", err)
	}

	// --- rig2 ---
	if err := os.MkdirAll(rig2Path, 0755); err != nil {
		t.Fatalf("mkdir rig2Path: %v", err)
	}
	initBeadsDBForServer(t, rig2Path, rig2Prefix)
	if err := beads.WriteRoutes(filepath.Join(rig2Path, ".beads"), routes); err != nil {
		t.Fatalf("write rig2 routes: %v", err)
	}
	rig2Redirect := filepath.Join(hqPath, "rig2", ".beads")
	if err := os.MkdirAll(rig2Redirect, 0755); err != nil {
		t.Fatalf("mkdir rig2 redirect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rig2Redirect, "redirect"), []byte("mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write rig2 redirect: %v", err)
	}

	// --- Environment ---
	env = cleanSchedulerTestEnv(tmpDir)

	// Configure scheduler with defaults
	configureScheduler(t, hqPath, 10, 3)

	return hqPath, rig1Path, rig2Path, gtBinary, env
}

// TestSchedulerMultiRigDispatch verifies that scheduler list and status correctly
// discover scheduled beads across multiple rigs. beadsSearchDirs scans all
// rig directories under the town root.
func TestSchedulerMultiRigDispatch(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create one bead in each rig.
	bead1 := createTestBead(t, rig1Path, "Rig1 bead")
	bead2 := createTestBead(t, rig2Path, "Rig2 bead")

	// Schedule both to their respective rigs.
	slingToScheduler(t, gtBinary, hqPath, env, bead1, "rig1")
	slingToScheduler(t, gtBinary, hqPath, env, bead2, "rig2")

	// Verify: scheduler list should find both beads.
	listed := getSchedulerList(t, gtBinary, hqPath, env)
	found := map[string]bool{}
	for _, item := range listed {
		if id, ok := item["id"].(string); ok {
			found[id] = true
		}
	}
	if !found[bead1] {
		t.Errorf("bead %s (rig1) not found in scheduler list", bead1)
	}
	if !found[bead2] {
		t.Errorf("bead %s (rig2) not found in scheduler list", bead2)
	}

	// Verify: scheduler status should report total=2, ready=2.
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	ready := int(status["queued_ready"].(float64))
	if total != 2 {
		t.Errorf("queued_total = %d, want 2", total)
	}
	if ready != 2 {
		t.Errorf("queued_ready = %d, want 2", ready)
	}

	// Verify: dry-run dispatch should mention both beads.
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "run", "--dry-run")
	if !strings.Contains(out, bead1) {
		t.Errorf("dry-run should mention rig1 bead %s", bead1)
	}
	if !strings.Contains(out, bead2) {
		t.Errorf("dry-run should mention rig2 bead %s", bead2)
	}
}

// --------------------------------------------------------------------------
// Cross-rig container tests
//
// These tests verify that gt sling deferred dispatch (max_polecats > 0) correctly auto-resolves
// each child's target rig from its bead ID prefix, enabling multi-rig epics
// and convoys.
// --------------------------------------------------------------------------

// TestSchedulerMultiRigEpicAutoResolve verifies that gt sling <epic> deferred dispatch (max_polecats > 0)
// auto-resolves each child's target rig from its prefix. An epic in rig1 with
// children in rig1 and rig2 should schedule each child to its respective rig.
func TestSchedulerMultiRigEpicAutoResolve(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create an epic in rig1.
	epicID := createTestBeadOfType(t, rig1Path, "Multi-rig epic", "epic")

	// Create children in different rigs.
	child1 := createTestBead(t, rig1Path, "Rig1 child")
	child2 := createTestBead(t, rig2Path, "Rig2 child")

	// Link children to epic via depends_on (epic → child).
	// child1 is local to rig1 — resolves directly.
	addBeadDependencyOfType(t, epicID, child1, "depends_on", rig1Path)
	// child2 is in rig2 — resolved via routes.jsonl as an external ref.
	addBeadDependencyOfType(t, epicID, child2, "depends_on", rig1Path)

	// Dry-run: verify auto-rig-resolution routes each child correctly.
	// Uses --dry-run to avoid needing formula infrastructure (mol-polecat-work).
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", epicID, "--dry-run")

	// Verify: child1 should be routed to rig1
	expected1 := fmt.Sprintf("%s -> rig1", child1)
	if !strings.Contains(out, expected1) {
		t.Errorf("epic dry-run should route %s -> rig1\noutput: %s", child1, out)
	}

	// Verify: child2 should be routed to rig2
	expected2 := fmt.Sprintf("%s -> rig2", child2)
	if !strings.Contains(out, expected2) {
		t.Errorf("epic dry-run should route %s -> rig2\noutput: %s", child2, out)
	}

	// Non-dry-run: actually schedule each child to its auto-resolved rig.
	// Use gt sling per-child (with --hook-raw-bead to skip formula) to verify
	// end-to-end scheduling works for beads from different rigs.
	slingToScheduler(t, gtBinary, hqPath, env, child1, "rig1")
	slingToScheduler(t, gtBinary, hqPath, env, child2, "rig2")

	// Verify: both children should have gt:queued label
	if !beadHasLabel(t, child1, capacity.LabelScheduled, rig1Path) {
		t.Errorf("child1 %s should have gt:queued label", child1)
	}
	if !beadHasLabel(t, child2, capacity.LabelScheduled, rig2Path) {
		t.Errorf("child2 %s should have gt:queued label", child2)
	}

	// Verify: scheduler metadata should show correct target rigs
	desc1 := getBeadDescription(t, child1, rig1Path)
	meta1 := capacity.ParseMetadata(desc1)
	if meta1 == nil || meta1.TargetRig != "rig1" {
		t.Errorf("child1 target_rig = %v, want rig1", meta1)
	}
	desc2 := getBeadDescription(t, child2, rig2Path)
	meta2 := capacity.ParseMetadata(desc2)
	if meta2 == nil || meta2.TargetRig != "rig2" {
		t.Errorf("child2 target_rig = %v, want rig2", meta2)
	}

	// Verify: scheduler status should find both children
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	if total != 2 {
		t.Errorf("queued_total = %d, want 2", total)
	}
}

// TestSchedulerConvoyFlagRejection verifies that task-only flags are rejected
// when gt sling deferred dispatch (max_polecats > 0) auto-detects a convoy ID.
func TestSchedulerConvoyFlagRejection(t *testing.T) {
	hqPath, _, _, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create a convoy in HQ.
	convoyID := createTestBeadOfType(t, hqPath, "Flag rejection convoy", "convoy")

	// Attempt to schedule convoy with task-only flag --ralph.
	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", convoyID, "--ralph")
	if err == nil {
		t.Fatalf("gt sling %s deferred dispatch (max_polecats > 0) --ralph should fail, but succeeded:\n%s", convoyID, out)
	}
	if !strings.Contains(out, "convoy mode does not support") {
		t.Errorf("expected 'convoy mode does not support' error, got:\n%s", out)
	}
	if !strings.Contains(out, "--ralph") {
		t.Errorf("error should mention --ralph, got:\n%s", out)
	}
}

// TestSchedulerEpicFlagRejection verifies that task-only flags are rejected
// when gt sling deferred dispatch (max_polecats > 0) auto-detects an epic ID.
func TestSchedulerEpicFlagRejection(t *testing.T) {
	hqPath, rig1Path, _, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create an epic in rig1.
	epicID := createTestBeadOfType(t, rig1Path, "Flag rejection epic", "epic")
	// Create a child so the epic has something to schedule.
	child := createTestBead(t, rig1Path, "Epic child")
	addBeadDependencyOfType(t, epicID, child, "depends_on", rig1Path)

	// Attempt to schedule epic with task-only flag --account.
	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", epicID, "--account", "foo")
	if err == nil {
		t.Fatalf("gt sling %s deferred dispatch (max_polecats > 0) --account foo should fail, but succeeded:\n%s", epicID, out)
	}
	if !strings.Contains(out, "epic mode does not support") {
		t.Errorf("expected 'epic mode does not support' error, got:\n%s", out)
	}
	if !strings.Contains(out, "--account") {
		t.Errorf("error should mention --account, got:\n%s", out)
	}
}

// TestSchedulerEpicDetection verifies that gt sling <epic-id> deferred dispatch (max_polecats > 0)
// auto-detects the epic and routes to the epic handler (dry-run).
func TestSchedulerEpicDetection(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create an epic with cross-rig children.
	epicID := createTestBeadOfType(t, rig1Path, "Detection epic", "epic")
	child1 := createTestBead(t, rig1Path, "Rig1 child")
	child2 := createTestBead(t, rig2Path, "Rig2 child")
	addBeadDependencyOfType(t, epicID, child1, "depends_on", rig1Path)
	addBeadDependencyOfType(t, epicID, child2, "depends_on", rig1Path)

	// gt sling <epic-id> deferred dispatch (max_polecats > 0) --dry-run should auto-detect epic and list children.
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", epicID, "--dry-run")

	// Should show both children with rig resolution.
	if !strings.Contains(out, child1) {
		t.Errorf("epic dry-run should mention child1 %s\noutput: %s", child1, out)
	}
	if !strings.Contains(out, child2) {
		t.Errorf("epic dry-run should mention child2 %s\noutput: %s", child2, out)
	}
	if !strings.Contains(out, "Would schedule") {
		t.Errorf("epic dry-run should show 'Would schedule'\noutput: %s", out)
	}
}

// TestSchedulerMixedBatchRejection verifies that gt sling with a task + epic
// (without a rig target) fails. The epic ID is not a valid target, so the
// command rejects it. With deferred dispatch, the 2-arg case expects a rig
// as the second argument.
func TestSchedulerMixedBatchRejection(t *testing.T) {
	hqPath, rig1Path, _, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create a task bead and an epic in rig1.
	taskID := createTestBead(t, rig1Path, "Task bead")
	epicID := createTestBeadOfType(t, rig1Path, "Epic bead", "epic")

	// Attempt to sling a task + epic together (no rig target).
	// Should fail because the epic ID is not a valid rig target.
	_, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", taskID, epicID, "--dry-run")
	if err == nil {
		t.Fatalf("gt sling %s %s should fail (epic is not a rig target), but succeeded", taskID, epicID)
	}
}

// TestSchedulerMultiRigConvoyAutoResolve verifies that gt sling <convoy> deferred dispatch (max_polecats > 0)
// auto-resolves each tracked issue's target rig from its prefix. A convoy in HQ
// tracking beads in rig1 and rig2 should schedule each bead to its respective rig.
func TestSchedulerMultiRigConvoyAutoResolve(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create a convoy in HQ (the typical location for convoys).
	convoyID := createTestBeadOfType(t, hqPath, "Multi-rig convoy", "convoy")

	// Create beads in different rigs.
	bead1 := createTestBead(t, rig1Path, "Rig1 tracked bead")
	bead2 := createTestBead(t, rig2Path, "Rig2 tracked bead")

	// Add tracks deps from convoy (HQ) to beads in each rig.
	// bead1 and bead2 are in different DBs — stored as external refs in HQ.
	addBeadDependencyOfType(t, convoyID, bead1, "tracks", hqPath)
	addBeadDependencyOfType(t, convoyID, bead2, "tracks", hqPath)

	// Dry-run: verify auto-rig-resolution routes each bead correctly.
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", convoyID, "--dry-run")

	// Verify: bead1 should be routed to rig1
	expected1 := fmt.Sprintf("%s -> rig1", bead1)
	if !strings.Contains(out, expected1) {
		t.Errorf("convoy dry-run should route %s -> rig1\noutput: %s", bead1, out)
	}

	// Verify: bead2 should be routed to rig2
	expected2 := fmt.Sprintf("%s -> rig2", bead2)
	if !strings.Contains(out, expected2) {
		t.Errorf("convoy dry-run should route %s -> rig2\noutput: %s", bead2, out)
	}

	// Non-dry-run: actually schedule each bead to its auto-resolved rig.
	slingToScheduler(t, gtBinary, hqPath, env, bead1, "rig1")
	slingToScheduler(t, gtBinary, hqPath, env, bead2, "rig2")

	// Verify: both beads should have gt:queued label
	if !beadHasLabel(t, bead1, capacity.LabelScheduled, rig1Path) {
		t.Errorf("bead1 %s should have gt:queued label", bead1)
	}
	if !beadHasLabel(t, bead2, capacity.LabelScheduled, rig2Path) {
		t.Errorf("bead2 %s should have gt:queued label", bead2)
	}

	// Verify: scheduler metadata should show correct target rigs
	desc1 := getBeadDescription(t, bead1, rig1Path)
	meta1 := capacity.ParseMetadata(desc1)
	if meta1 == nil || meta1.TargetRig != "rig1" {
		t.Errorf("bead1 target_rig = %v, want rig1", meta1)
	}
	desc2 := getBeadDescription(t, bead2, rig2Path)
	meta2 := capacity.ParseMetadata(desc2)
	if meta2 == nil || meta2.TargetRig != "rig2" {
		t.Errorf("bead2 target_rig = %v, want rig2", meta2)
	}

	// Verify: scheduler status should find both beads
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	if total != 2 {
		t.Errorf("queued_total = %d, want 2", total)
	}
}

// --------------------------------------------------------------------------
// Dispatch mode tests (direct, disabled)
// --------------------------------------------------------------------------

// TestSchedulerDisabledMode verifies that max_polecats=0 behaves as direct dispatch
// (same as -1). Beads should NOT be queued — they fall through to normal dispatch.
func TestSchedulerDisabledMode(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Reconfigure scheduler to disabled mode (max_polecats=0)
	configureScheduler(t, hqPath, 0, 1)

	beadID := createTestBead(t, rigPath, "Disabled mode test")

	// gt sling --dry-run should succeed (direct dispatch, not deferred)
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", beadID, "testrig", "--hook-raw-bead", "--dry-run")
	if strings.Contains(out, "scheduler is disabled") {
		t.Errorf("max_polecats=0 should act as direct dispatch, not error:\n%s", out)
	}
	if strings.Contains(out, "Would schedule") {
		t.Errorf("max_polecats=0 should NOT schedule (deferred), got:\n%s", out)
	}

	// Bead should NOT have gt:queued label
	if beadHasLabel(t, beadID, capacity.LabelScheduled, rigPath) {
		t.Errorf("disabled mode should NOT add gt:queued label")
	}
}

// TestSchedulerDirectModeNoQueue verifies that max_polecats=-1 (direct dispatch mode)
// does not queue beads. Scheduler run and status should show zero queued.
func TestSchedulerDirectModeNoQueue(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Reconfigure scheduler to direct dispatch mode
	configureScheduler(t, hqPath, -1, 1)

	// Scheduler status should report zero queued and not be paused
	status := getSchedulerStatus(t, gtBinary, hqPath, env)
	total := int(status["queued_total"].(float64))
	if total != 0 {
		t.Errorf("queued_total = %d, want 0 in direct mode", total)
	}

	// Scheduler run --dry-run should be a no-op (nothing to dispatch)
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "run", "--dry-run")
	if strings.Contains(out, "Would dispatch") {
		t.Errorf("direct mode should have nothing to dispatch, got:\n%s", out)
	}
}

// TestSchedulerDeferredTaskWithoutRig verifies that in deferred mode (max_polecats > 0),
// gt sling <task-bead> (without a rig) returns an error requiring a rig target.
func TestSchedulerDeferredTaskWithoutRig(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	beadID := createTestBead(t, rigPath, "No rig test")

	// gt sling <bead> (no rig) in deferred mode should error
	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", beadID, "--hook-raw-bead")
	if err == nil {
		t.Fatalf("gt sling %s without rig in deferred mode should fail, but succeeded:\n%s", beadID, out)
	}
	if !strings.Contains(out, "deferred dispatch requires a rig target") {
		t.Errorf("expected 'deferred dispatch requires a rig target' error, got:\n%s", out)
	}
}

// TestSchedulerConfigSetZero verifies that gt config set scheduler.max_polecats 0
// is accepted (disabled mode is a valid config).
func TestSchedulerConfigSetZero(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	// Set max_polecats=0 should succeed
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "config", "set", "scheduler.max_polecats", "0")
	if strings.Contains(out, "invalid") {
		t.Errorf("max_polecats=0 should be accepted, got:\n%s", out)
	}

	// Read it back — should return 0
	out = runGTCmdOutput(t, gtBinary, hqPath, env, "config", "get", "scheduler.max_polecats")
	if strings.TrimSpace(out) != "0" {
		t.Errorf("max_polecats = %q, want %q", strings.TrimSpace(out), "0")
	}
}

// TestSchedulerDeferredNonRigRejection verifies that in deferred mode (max_polecats > 0),
// gt sling <bead> <non-rig> is rejected rather than falling through to direct dispatch.
func TestSchedulerDeferredNonRigRejection(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	beadID := createTestBead(t, rigPath, "Non-rig rejection test")
	otherBead := createTestBead(t, rigPath, "Not a rig target")

	// gt sling <bead> <non-rig-bead> in deferred mode should error
	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", beadID, otherBead, "--hook-raw-bead")
	if err == nil {
		t.Fatalf("gt sling %s %s (non-rig) in deferred mode should fail, but succeeded:\n%s", beadID, otherBead, out)
	}
	if !strings.Contains(out, "deferred dispatch requires a rig target") {
		t.Errorf("expected 'deferred dispatch requires a rig target' error, got:\n%s", out)
	}

	// gt sling <bead> . in deferred mode should also be rejected
	out, err = runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", beadID, ".", "--hook-raw-bead")
	if err == nil {
		t.Fatalf("gt sling %s . in deferred mode should fail, but succeeded:\n%s", beadID, out)
	}
	if !strings.Contains(out, "deferred dispatch requires a rig target") {
		t.Errorf("expected 'deferred dispatch requires a rig target' error for '.', got:\n%s", out)
	}
}

// TestSchedulerDirectEpicDispatch verifies that gt sling <epic-id> --dry-run
// with max_polecats=-1 (direct mode) routes to the direct dispatch path.
func TestSchedulerDirectEpicDispatch(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Reconfigure to direct dispatch mode
	configureScheduler(t, hqPath, -1, 1)

	// Create an epic with cross-rig children.
	epicID := createTestBeadOfType(t, rig1Path, "Direct dispatch epic", "epic")
	child1 := createTestBead(t, rig1Path, "Rig1 direct child")
	child2 := createTestBead(t, rig2Path, "Rig2 direct child")
	addBeadDependencyOfType(t, epicID, child1, "depends_on", rig1Path)
	addBeadDependencyOfType(t, epicID, child2, "depends_on", rig1Path)

	// gt sling <epic-id> --dry-run in direct mode should show direct dispatch, not scheduling
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", epicID, "--dry-run")

	// Should mention children
	if !strings.Contains(out, child1) {
		t.Errorf("direct epic dry-run should mention child1 %s\noutput: %s", child1, out)
	}
	if !strings.Contains(out, child2) {
		t.Errorf("direct epic dry-run should mention child2 %s\noutput: %s", child2, out)
	}
	// Direct dispatch uses "Would sling" not "Would schedule"
	if strings.Contains(out, "Would schedule") {
		t.Errorf("direct mode should NOT show 'Would schedule'\noutput: %s", out)
	}
}

// TestSchedulerBatchEpicRejection verifies that in deferred mode (max_polecats > 0),
// gt sling <epic-id> <task-id> <rig> rejects the epic ID rather than scheduling it as a task.
func TestSchedulerBatchEpicRejection(t *testing.T) {
	hqPath, rig1Path, _, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Create an epic and a task bead in rig1.
	epicID := createTestBeadOfType(t, rig1Path, "Batch epic", "epic")
	taskID := createTestBead(t, rig1Path, "Batch task")

	// gt sling <epic> <task> <rig> in deferred mode should reject the epic
	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "sling", epicID, taskID, "rig1", "--hook-raw-bead")
	if err == nil {
		t.Fatalf("gt sling %s %s rig1 should reject epic in batch, but succeeded:\n%s", epicID, taskID, out)
	}
	if !strings.Contains(out, "cannot be batch-scheduled") {
		t.Errorf("expected 'cannot be batch-scheduled' error, got:\n%s", out)
	}
}

// TestSchedulerDirectConvoyDispatch verifies that gt sling <convoy-id> --dry-run
// with max_polecats=-1 (direct mode) routes to the direct dispatch path.
func TestSchedulerDirectConvoyDispatch(t *testing.T) {
	hqPath, rig1Path, rig2Path, gtBinary, env := setupMultiRigSchedulerTown(t)

	// Reconfigure to direct dispatch mode
	configureScheduler(t, hqPath, -1, 1)

	// Create a convoy in HQ tracking beads in different rigs.
	convoyID := createTestBeadOfType(t, hqPath, "Direct dispatch convoy", "convoy")
	bead1 := createTestBead(t, rig1Path, "Rig1 direct tracked")
	bead2 := createTestBead(t, rig2Path, "Rig2 direct tracked")
	addBeadDependencyOfType(t, convoyID, bead1, "tracks", hqPath)
	addBeadDependencyOfType(t, convoyID, bead2, "tracks", hqPath)

	// gt sling <convoy-id> --dry-run in direct mode
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "sling", convoyID, "--dry-run")

	// Should mention tracked beads
	if !strings.Contains(out, bead1) {
		t.Errorf("direct convoy dry-run should mention bead1 %s\noutput: %s", bead1, out)
	}
	if !strings.Contains(out, bead2) {
		t.Errorf("direct convoy dry-run should mention bead2 %s\noutput: %s", bead2, out)
	}
	// Direct dispatch uses "Would sling" not "Would schedule"
	if strings.Contains(out, "Would schedule") {
		t.Errorf("direct mode should NOT show 'Would schedule'\noutput: %s", out)
	}
}
