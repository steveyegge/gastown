// Package queue provides a work queue for dispatching beads to polecats.
package queue

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
)

// QueueLabel is the label used to mark beads as queued.
const QueueLabel = "queued"

// QueueItem represents a bead in the queue with its target rig.
type QueueItem struct {
	BeadID  string // Bead ID
	Title   string // Bead title
	RigName string // Target rig for dispatch
}

// Queue manages the work queue for dispatching beads.
type Queue struct {
	ops   beads.BeadsOps
	items []QueueItem
}

// New creates a new Queue with the given BeadsOps implementation.
func New(ops beads.BeadsOps) *Queue {
	return &Queue{
		ops:   ops,
		items: []QueueItem{},
	}
}

// Add adds a bead to the queue by applying the "queued" label.
// Returns error if the bead is a town-level bead or has no routable prefix.
func (q *Queue) Add(beadID string) error {
	// Reject town-level beads
	if q.ops.IsTownLevelBead(beadID) {
		return fmt.Errorf("cannot queue town-level bead %s: polecats are rig-local", beadID)
	}

	// Determine target rig from bead prefix
	rigName := q.ops.GetRigForBead(beadID)
	if rigName == "" {
		return fmt.Errorf("cannot queue bead %s: no rig route for prefix", beadID)
	}

	// Add the queued label
	if err := q.ops.LabelAdd(beadID, QueueLabel); err != nil {
		return fmt.Errorf("adding queue label: %w", err)
	}

	return nil
}

// Load loads all READY queued beads from all rigs.
// Ready means: status=open AND no open blockers (dependencies satisfied).
// Returns the loaded items and updates internal state.
func (q *Queue) Load() ([]QueueItem, error) {
	// Query all rigs for ready queued beads (excludes blocked beads)
	rigBeads, err := q.ops.ListReadyByLabel(QueueLabel)
	if err != nil {
		return nil, err
	}

	// Build queue items
	q.items = []QueueItem{}
	for rigName, beadList := range rigBeads {
		for _, bead := range beadList {
			q.items = append(q.items, QueueItem{
				BeadID:  bead.ID,
				Title:   bead.Title,
				RigName: rigName,
			})
		}
	}

	return q.items, nil
}

// Len returns the number of items in the queue.
func (q *Queue) Len() int {
	return len(q.items)
}

// All returns all items in the queue.
func (q *Queue) All() []QueueItem {
	return q.items
}

// Remove removes a bead from the queue by removing the "queued" label.
// Also updates the internal items slice to reflect the removal.
func (q *Queue) Remove(beadID string) error {
	if err := q.ops.LabelRemove(beadID, QueueLabel); err != nil {
		return err
	}

	// Update internal items slice
	newItems := make([]QueueItem, 0, len(q.items))
	for _, item := range q.items {
		if item.BeadID != beadID {
			newItems = append(newItems, item)
		}
	}
	q.items = newItems

	return nil
}

// Clear removes all beads from the queue.
// Returns the number of items cleared.
func (q *Queue) Clear() (int, error) {
	if _, err := q.Load(); err != nil {
		return 0, err
	}

	cleared := 0
	for _, item := range q.items {
		if err := q.Remove(item.BeadID); err != nil {
			continue // Best effort
		}
		cleared++
	}

	q.items = []QueueItem{}
	return cleared, nil
}
