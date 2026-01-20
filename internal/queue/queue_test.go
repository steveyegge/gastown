package queue

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestQueue_Add_Basic(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)
	// Bead must exist before we can add a label to it
	ops.AddBead("gt-abc", "open", []string{})
	q := New(ops)

	if err := q.Add("gt-abc"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify label was added
	labels := ops.GetLabels("gt-abc")
	if len(labels) != 1 || labels[0] != QueueLabel {
		t.Errorf("Expected [queued] label, got %v", labels)
	}
}

func TestQueue_Add_RejectsTownLevelBead(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)
	q := New(ops)

	err := q.Add("hq-abc")
	if err == nil {
		t.Error("Expected error for town-level bead")
	}
}

func TestQueue_Add_RejectsEmptyPrefix(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	q := New(ops)

	err := q.Add("noprefix")
	if err == nil {
		t.Error("Expected error for bead without prefix")
	}
}

func TestQueue_Add_RejectsUnroutedPrefix(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	// No routes configured
	q := New(ops)

	err := q.Add("xx-abc")
	if err == nil {
		t.Error("Expected error for unrouted prefix")
	}
}

func TestQueue_Load_Basic(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	// Add a queued bead
	ops.AddBead("gt-abc", "open", []string{QueueLabel})

	q := New(ops)
	items, err := q.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}

	if items[0].BeadID != "gt-abc" {
		t.Errorf("Expected gt-abc, got %s", items[0].BeadID)
	}

	if items[0].RigName != "gastown" {
		t.Errorf("Expected gastown, got %s", items[0].RigName)
	}
}

func TestQueue_Load_MultipleRigs(t *testing.T) {
	gasOps := beads.NewFakeBeadsOpsForRig("gastown")
	greenOps := beads.NewFakeBeadsOpsForRig("greenplace")

	// Set up routes
	gasOps.AddRouteWithRig("gt-", "gastown", gasOps)
	gasOps.AddRouteWithRig("gp-", "greenplace", greenOps)

	// Add beads to each rig
	gasOps.AddBead("gt-abc", "open", []string{QueueLabel})
	greenOps.AddBead("gp-xyz", "open", []string{QueueLabel})

	q := New(gasOps)
	items, err := q.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}
}

func TestQueue_Load_FiltersNonQueued(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	// Add beads - one queued, one not
	ops.AddBead("gt-queued", "open", []string{QueueLabel})
	ops.AddBead("gt-notqueued", "open", []string{})

	q := New(ops)
	items, err := q.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}

	if items[0].BeadID != "gt-queued" {
		t.Errorf("Expected gt-queued, got %s", items[0].BeadID)
	}
}

func TestQueue_Load_Empty(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	q := New(ops)
	items, err := q.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(items))
	}
}

func TestQueue_Load_OnlyOpenBeads(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	// Add open and closed beads
	ops.AddBead("gt-open", "open", []string{QueueLabel})
	ops.AddBead("gt-closed", "closed", []string{QueueLabel})

	q := New(ops)
	items, err := q.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}

	if items[0].BeadID != "gt-open" {
		t.Errorf("Expected gt-open, got %s", items[0].BeadID)
	}
}

func TestQueue_Len(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})
	ops.AddBead("gt-3", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	if q.Len() != 3 {
		t.Errorf("Expected Len()=3, got %d", q.Len())
	}
}

func TestQueue_All(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	all := q.All()
	if len(all) != 2 {
		t.Errorf("Expected 2 items, got %d", len(all))
	}
}

func TestQueue_Clear_Basic(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})

	q := New(ops)
	cleared, err := q.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if cleared != 2 {
		t.Errorf("Expected 2 cleared, got %d", cleared)
	}

	if q.Len() != 0 {
		t.Errorf("Expected empty queue after clear, got %d", q.Len())
	}
}

func TestQueue_Clear_Empty(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	q := New(ops)
	cleared, err := q.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if cleared != 0 {
		t.Errorf("Expected 0 cleared, got %d", cleared)
	}
}
