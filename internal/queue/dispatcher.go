package queue

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/parallel"
)

// Spawner is the interface for spawning polecats.
type Spawner interface {
	// SpawnIn spawns a polecat in the given rig for the given bead.
	SpawnIn(rigName, beadID string) error
}

// RealSpawner implements Spawner using a callback function.
type RealSpawner struct {
	SpawnInFunc func(rigName, beadID string) error
}

// SpawnIn implements Spawner.
func (s *RealSpawner) SpawnIn(rigName, beadID string) error {
	if s.SpawnInFunc == nil {
		return fmt.Errorf("SpawnInFunc not set")
	}
	return s.SpawnInFunc(rigName, beadID)
}

// DispatchResult contains the results of a dispatch operation.
type DispatchResult struct {
	Dispatched []string // Bead IDs that were dispatched
	Skipped    []string // Bead IDs that were skipped (limit reached)
	Errors     []error  // Errors encountered
}

// Dispatcher dispatches queued beads to polecats.
type Dispatcher struct {
	queue       *Queue
	spawner     Spawner
	dryRun      bool
	limit       int // 0 = unlimited
	parallelism int // 0 or 1 = sequential
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(queue *Queue, spawner Spawner) *Dispatcher {
	return &Dispatcher{
		queue:       queue,
		spawner:     spawner,
		parallelism: 1, // Default to sequential
	}
}

// WithDryRun sets dry-run mode.
func (d *Dispatcher) WithDryRun(dryRun bool) *Dispatcher {
	d.dryRun = dryRun
	return d
}

// WithLimit sets the maximum number of items to dispatch.
func (d *Dispatcher) WithLimit(limit int) *Dispatcher {
	d.limit = limit
	return d
}

// WithParallelism sets the number of concurrent dispatches.
func (d *Dispatcher) WithParallelism(parallelism int) *Dispatcher {
	d.parallelism = parallelism
	return d
}

// Dispatch dispatches queued beads to polecats.
func (d *Dispatcher) Dispatch() (*DispatchResult, error) {
	result := &DispatchResult{
		Dispatched: []string{},
		Skipped:    []string{},
		Errors:     []error{},
	}

	items := d.queue.All()
	if len(items) == 0 {
		return result, nil
	}

	// Filter out items without rig names
	validItems := []QueueItem{}
	for _, item := range items {
		if item.RigName == "" {
			result.Errors = append(result.Errors, fmt.Errorf("bead %s has no rig name", item.BeadID))
			continue
		}
		validItems = append(validItems, item)
	}

	// Apply limit
	toDispatch := validItems
	if d.limit > 0 && len(validItems) > d.limit {
		toDispatch = validItems[:d.limit]
		for _, item := range validItems[d.limit:] {
			result.Skipped = append(result.Skipped, item.BeadID)
		}
	}

	if d.dryRun {
		// Dry run - just record what would be dispatched
		for _, item := range toDispatch {
			result.Dispatched = append(result.Dispatched, item.BeadID)
		}
		return result, nil
	}

	// Dispatch using parallel executor
	parallelResults := parallel.Execute(toDispatch, d.parallelism, func(item QueueItem) error {
		return d.dispatchOne(item)
	})

	// Process results
	for _, r := range parallelResults {
		if r.Success {
			result.Dispatched = append(result.Dispatched, r.Input.BeadID)
		} else {
			result.Errors = append(result.Errors, fmt.Errorf("dispatch %s: %w", r.Input.BeadID, r.Error))
		}
	}

	// Remove dispatched items from queue
	for _, beadID := range result.Dispatched {
		_ = d.queue.Remove(beadID) // Best effort
	}

	if len(result.Errors) > 0 {
		return result, fmt.Errorf("%d dispatch errors", len(result.Errors))
	}

	return result, nil
}

func (d *Dispatcher) dispatchOne(item QueueItem) error {
	return d.spawner.SpawnIn(item.RigName, item.BeadID)
}
