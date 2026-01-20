package queue

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// fakeSpawner implements Spawner for testing.
type fakeSpawner struct {
	spawned   []string
	errOnBead string // Bead ID to return an error for
}

func (s *fakeSpawner) SpawnIn(rigName, beadID string) error {
	if s.errOnBead == beadID {
		return errors.New("spawn failed")
	}
	s.spawned = append(s.spawned, beadID)
	return nil
}

func TestDispatcher_Dispatch_Basic(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner)

	result, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(result.Dispatched) != 2 {
		t.Errorf("Expected 2 dispatched, got %d", len(result.Dispatched))
	}

	if len(spawner.spawned) != 2 {
		t.Errorf("Expected 2 spawned, got %d", len(spawner.spawned))
	}
}

func TestDispatcher_Dispatch_Empty(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	q := New(ops)
	q.Load()

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner)

	result, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(result.Dispatched) != 0 {
		t.Errorf("Expected 0 dispatched, got %d", len(result.Dispatched))
	}
}

func TestDispatcher_Dispatch_DryRun(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner).WithDryRun(true)

	result, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(result.Dispatched) != 2 {
		t.Errorf("Expected 2 dispatched in dry run, got %d", len(result.Dispatched))
	}

	// In dry run, spawner should not be called
	if len(spawner.spawned) != 0 {
		t.Errorf("Expected 0 spawned in dry run, got %d", len(spawner.spawned))
	}
}

func TestDispatcher_Dispatch_WithLimit(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-2", "open", []string{QueueLabel})
	ops.AddBead("gt-3", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner).WithLimit(2)

	result, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(result.Dispatched) != 2 {
		t.Errorf("Expected 2 dispatched, got %d", len(result.Dispatched))
	}

	if len(result.Skipped) != 1 {
		t.Errorf("Expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestDispatcher_Dispatch_WithErrors(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})
	ops.AddBead("gt-fail", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	spawner := &fakeSpawner{errOnBead: "gt-fail"}
	d := NewDispatcher(q, spawner)

	result, err := d.Dispatch()
	if err == nil {
		t.Error("Expected error from Dispatch")
	}

	if len(result.Dispatched) != 1 {
		t.Errorf("Expected 1 dispatched, got %d", len(result.Dispatched))
	}

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
}

func TestDispatcher_Dispatch_NoRigName(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	// Add a bead but manually manipulate to have no rig
	ops.AddBead("gt-norigs", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	// Manually corrupt the queue to have a bead with no rig name
	q.items[0].RigName = ""

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner)

	result, err := d.Dispatch()
	// Dispatch returns an error when there are any errors (validation or dispatch)
	if err == nil {
		t.Fatal("Expected error from Dispatch for invalid items")
	}

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error for no-rig bead, got %d", len(result.Errors))
	}
}

func TestDispatcher_Dispatch_Parallel(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	for i := 0; i < 10; i++ {
		ops.AddBead(string(rune('a'+i))+"gt-"+string(rune('0'+i)), "open", []string{QueueLabel})
	}

	q := New(ops)
	q.Load()

	var spawnCount int32
	spawner := &RealSpawner{
		SpawnInFunc: func(rigName, beadID string) error {
			atomic.AddInt32(&spawnCount, 1)
			return nil
		},
	}

	d := NewDispatcher(q, spawner).WithParallelism(4)

	result, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(result.Dispatched) != 10 {
		t.Errorf("Expected 10 dispatched, got %d", len(result.Dispatched))
	}

	if spawnCount != 10 {
		t.Errorf("Expected 10 spawn calls, got %d", spawnCount)
	}
}

func TestDispatcher_RemovesFromQueue(t *testing.T) {
	ops := beads.NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)

	ops.AddBead("gt-1", "open", []string{QueueLabel})

	q := New(ops)
	q.Load()

	if q.Len() != 1 {
		t.Fatalf("Expected 1 item in queue, got %d", q.Len())
	}

	spawner := &fakeSpawner{}
	d := NewDispatcher(q, spawner)

	_, err := d.Dispatch()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Queue should be empty after dispatch
	if q.Len() != 0 {
		t.Errorf("Expected 0 items in queue after dispatch, got %d", q.Len())
	}
}

func TestRealSpawner_NilFunc(t *testing.T) {
	spawner := &RealSpawner{}
	err := spawner.SpawnIn("rig", "bead")
	if err == nil {
		t.Error("Expected error for nil SpawnInFunc")
	}
}
