//go:build integration

package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/scheduler/capacity"
)

// TestSchedulerPromote verifies that promoting a bead moves it to the front of the queue.
func TestSchedulerPromote(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	bead1 := createTestBead(t, rigPath, "First bead")
	bead2 := createTestBead(t, rigPath, "Second bead")

	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead1,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-01T00:00:00Z",
	})
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead2,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-02T00:00:00Z",
	})

	// Promote bead2 — should move to front
	runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "promote", bead2)

	// Verify: bead2 should have epoch timestamp
	fields := findSlingContext(t, hqPath, bead2)
	if fields == nil {
		t.Fatal("bead2 sling context not found after promote")
	}
	if fields.EnqueuedAt != "0001-01-01T00:00:00Z" {
		t.Errorf("promoted bead EnqueuedAt = %q, want epoch", fields.EnqueuedAt)
	}

	// Verify: bead1 should be unchanged
	fields1 := findSlingContext(t, hqPath, bead1)
	if fields1 == nil {
		t.Fatal("bead1 sling context not found")
	}
	if fields1.EnqueuedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("bead1 EnqueuedAt changed unexpectedly: %q", fields1.EnqueuedAt)
	}
}

// TestSchedulerDemote verifies that demoting a bead moves it to the back of the queue.
func TestSchedulerDemote(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	bead1 := createTestBead(t, rigPath, "Early bead")

	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead1,
		TargetRig:  "testrig",
		EnqueuedAt: "2020-01-01T00:00:00Z",
	})

	// Demote bead1 — should move to back (timestamp = now)
	runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "demote", bead1)

	fields := findSlingContext(t, hqPath, bead1)
	if fields == nil {
		t.Fatal("bead1 sling context not found after demote")
	}
	if fields.EnqueuedAt < "2025" {
		t.Errorf("demoted bead EnqueuedAt = %q, want recent timestamp", fields.EnqueuedAt)
	}
}

// TestSchedulerReorderByPriority verifies that reordering by priority sorts P0 first.
func TestSchedulerReorderByPriority(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	beadP2 := createTestBead(t, rigPath, "P2 bead")
	beadP0 := createTestBead(t, rigPath, "P0 bead")
	beadP1 := createTestBead(t, rigPath, "P1 bead")

	// Set priorities via bd update
	bdUpdate(t, rigPath, beadP0, "0")
	bdUpdate(t, rigPath, beadP1, "1")
	bdUpdate(t, rigPath, beadP2, "2")

	// Schedule in reverse priority order (P2 first, P0 last)
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: beadP2,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-01T00:00:00Z",
	})
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: beadP0,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-02T00:00:00Z",
	})
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: beadP1,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-03T00:00:00Z",
	})

	// Reorder by priority
	runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "reorder", "--by", "priority")

	// Verify order: P0 should now be first (earliest timestamp)
	fieldsP0 := findSlingContext(t, hqPath, beadP0)
	fieldsP1 := findSlingContext(t, hqPath, beadP1)
	fieldsP2 := findSlingContext(t, hqPath, beadP2)

	if fieldsP0 == nil || fieldsP1 == nil || fieldsP2 == nil {
		t.Fatal("sling contexts lost after reorder")
	}

	if fieldsP0.EnqueuedAt >= fieldsP1.EnqueuedAt {
		t.Errorf("P0 (%s) should be before P1 (%s)", fieldsP0.EnqueuedAt, fieldsP1.EnqueuedAt)
	}
	if fieldsP1.EnqueuedAt >= fieldsP2.EnqueuedAt {
		t.Errorf("P1 (%s) should be before P2 (%s)", fieldsP1.EnqueuedAt, fieldsP2.EnqueuedAt)
	}
}

// TestSchedulerPromoteNotFound verifies error when promoting a bead not in the scheduler.
func TestSchedulerPromoteNotFound(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	out, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "scheduler", "promote", "nonexistent-bead")
	if err == nil {
		t.Error("expected error promoting nonexistent bead")
	}
	if !strings.Contains(out, "no sling context found") {
		t.Errorf("expected 'no sling context found' in output, got: %s", out)
	}
}

// TestSchedulerRunBeadDryRun verifies that --bead filters dispatch to a single bead.
func TestSchedulerRunBeadDryRun(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	bead1 := createTestBead(t, rigPath, "Run bead test")

	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead1,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-01T00:00:00Z",
	})

	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "run", "--bead", bead1, "--dry-run")
	if !strings.Contains(out, bead1) {
		t.Errorf("dry-run --bead output should mention %s, got: %s", bead1, out)
	}
}

// TestSchedulerRunBeadBatchMutualExclusion verifies --bead and --batch are mutually exclusive.
func TestSchedulerRunBeadBatchMutualExclusion(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	_, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "scheduler", "run", "--bead", "foo", "--batch", "5")
	if err == nil {
		t.Error("expected error with both --bead and --batch")
	}
}

// TestSchedulerReorderInvalidField verifies error for unsupported reorder field.
func TestSchedulerReorderInvalidField(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	_, err := runGTCmdMayFail(t, gtBinary, hqPath, env, "scheduler", "reorder", "--by", "created")
	if err == nil {
		t.Error("expected error with unsupported reorder field")
	}
}

// TestSchedulerClearPositionalArg verifies that `gt scheduler clear <bead-id>` (positional)
// removes only the specified bead from the scheduler, not the entire queue.
// This is a regression test for the desire-path bug where users typed the bead ID as
// a positional arg (matching `gt scheduler promote/demote` UX), cobra silently ignored
// it, and the entire queue was cleared.
func TestSchedulerClearPositionalArg(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	bead1 := createTestBead(t, rigPath, "First bead")
	bead2 := createTestBead(t, rigPath, "Second bead")

	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead1,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-01T00:00:00Z",
	})
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead2,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-02T00:00:00Z",
	})

	// Clear only bead1 using positional arg (not --bead flag)
	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "clear", bead1)
	if !strings.Contains(out, bead1) {
		t.Errorf("clear output should mention %s, got: %s", bead1, out)
	}

	// bead2 sling context should still be open
	fields2 := findSlingContext(t, hqPath, bead2)
	if fields2 == nil {
		t.Error("bead2 sling context was closed — positional-arg clear nuked entire queue")
	}

	// bead1 sling context should be gone
	fields1 := findSlingContext(t, hqPath, bead1)
	if fields1 != nil {
		t.Error("bead1 sling context still open after clear")
	}
}

// TestSchedulerClearFlagStillWorks verifies the --bead flag form still works.
func TestSchedulerClearFlagStillWorks(t *testing.T) {
	hqPath, rigPath, gtBinary, env := setupSchedulerIntegrationTown(t)

	bead1 := createTestBead(t, rigPath, "Flag bead")
	bead2 := createTestBead(t, rigPath, "Survivor bead")

	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead1,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-01T00:00:00Z",
	})
	createSlingContext(t, hqPath, &capacity.SlingContextFields{
		Version:    1,
		WorkBeadID: bead2,
		TargetRig:  "testrig",
		EnqueuedAt: "2025-01-02T00:00:00Z",
	})

	// Clear only bead1 using --bead flag
	runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "clear", "--bead", bead1)

	// bead2 should still be scheduled
	fields2 := findSlingContext(t, hqPath, bead2)
	if fields2 == nil {
		t.Error("bead2 sling context was closed — --bead clear nuked entire queue")
	}

	// bead1 should be cleared
	fields1 := findSlingContext(t, hqPath, bead1)
	if fields1 != nil {
		t.Error("bead1 sling context still open after --bead clear")
	}
}

// TestSchedulerClearNotFound verifies the error message when clearing a bead not in scheduler.
func TestSchedulerClearNotFound(t *testing.T) {
	hqPath, _, gtBinary, env := setupSchedulerIntegrationTown(t)

	out := runGTCmdOutput(t, gtBinary, hqPath, env, "scheduler", "clear", "nonexistent-bead")
	if !strings.Contains(out, "No sling context found") {
		t.Errorf("expected 'No sling context found' in output, got: %s", out)
	}
}

// bdUpdate updates a bead's priority using bd update.
func bdUpdate(t *testing.T, dir, beadID, priority string) {
	t.Helper()
	cmd := exec.Command("bd", "update", beadID, "--priority="+priority)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd update %s --priority=%s failed: %v\n%s", beadID, priority, err, out)
	}
}
