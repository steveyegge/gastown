// Package beads provides merge slot management for serialized conflict resolution.
package beads

import (
	"context"
	"fmt"
)

// MergeSlotStatus represents the result of checking a merge slot.
type MergeSlotStatus struct {
	ID        string   `json:"id"`
	Available bool     `json:"available"`
	Holder    string   `json:"holder,omitempty"`
	Waiters   []string `json:"waiters,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// MergeSlotCreate creates the merge slot bead for the current rig.
// The slot is used for serialized conflict resolution in the merge queue.
// Returns the slot ID if successful.
func (b *Beads) MergeSlotCreate() (string, error) {
	ctx := context.Background()
	store, err := b.openStore(ctx)
	if err != nil {
		return "", fmt.Errorf("creating merge slot: %w", err)
	}

	actor := b.getActor()
	issue, err := store.MergeSlotCreate(ctx, actor)
	if err != nil {
		return "", fmt.Errorf("creating merge slot: %w", err)
	}

	return issue.ID, nil
}

// MergeSlotCheck checks the availability of the merge slot.
// Returns the current status including holder and waiters if held.
func (b *Beads) MergeSlotCheck() (*MergeSlotStatus, error) {
	ctx := context.Background()
	store, err := b.openStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking merge slot: %w", err)
	}

	ss, err := store.MergeSlotCheck(ctx)
	if err != nil {
		return &MergeSlotStatus{Error: err.Error()}, nil
	}

	return &MergeSlotStatus{
		ID:        ss.SlotID,
		Available: ss.Available,
		Holder:    ss.Holder,
		Waiters:   ss.Waiters,
	}, nil
}

// MergeSlotAcquire attempts to acquire the merge slot for exclusive access.
// If holder is empty, defaults to BD_ACTOR environment variable.
// If addWaiter is true and the slot is held, the requester is added to the waiters queue.
// Returns the acquisition result.
func (b *Beads) MergeSlotAcquire(holder string, addWaiter bool) (*MergeSlotStatus, error) {
	ctx := context.Background()
	store, err := b.openStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquiring merge slot: %w", err)
	}

	if holder == "" {
		holder = b.getActor()
	}
	actor := b.getActor()

	result, err := store.MergeSlotAcquire(ctx, holder, actor, addWaiter)
	if err != nil {
		return nil, fmt.Errorf("acquiring merge slot: %w", err)
	}

	return &MergeSlotStatus{
		ID:        result.SlotID,
		Available: result.Acquired,
		Holder:    result.Holder,
	}, nil
}

// MergeSlotRelease releases the merge slot after conflict resolution completes.
// If holder is provided, it verifies the slot is held by that holder before releasing.
func (b *Beads) MergeSlotRelease(holder string) error {
	ctx := context.Background()
	store, err := b.openStore(ctx)
	if err != nil {
		return fmt.Errorf("releasing merge slot: %w", err)
	}

	actor := b.getActor()
	if err := store.MergeSlotRelease(ctx, holder, actor); err != nil {
		return fmt.Errorf("releasing merge slot: %w", err)
	}

	return nil
}

// MergeSlotEnsureExists creates the merge slot if it doesn't exist.
// This is idempotent - safe to call multiple times.
func (b *Beads) MergeSlotEnsureExists() (string, error) {
	// Check if slot exists first
	status, err := b.MergeSlotCheck()
	if err != nil {
		return "", err
	}

	if status.Error != "" {
		// Create it
		return b.MergeSlotCreate()
	}

	return status.ID, nil
}
