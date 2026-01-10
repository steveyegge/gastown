// Package beads provides merge slot management for serialized conflict resolution.
package beads

import (
	"encoding/json"
	"fmt"
	"strings"
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
	out, err := b.run("merge-slot", "create", "--json")
	if err != nil {
		return "", fmt.Errorf("creating merge slot: %w", err)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing merge-slot create output: %w", err)
	}

	return result.ID, nil
}

// MergeSlotCheck checks the availability of the merge slot.
// Returns the current status including holder and waiters if held.
func (b *Beads) MergeSlotCheck() (*MergeSlotStatus, error) {
	out, err := b.run("merge-slot", "check", "--json")
	if err != nil {
		// Check if slot doesn't exist
		if strings.Contains(err.Error(), "not found") {
			return &MergeSlotStatus{Error: "not found"}, nil
		}
		return nil, fmt.Errorf("checking merge slot: %w", err)
	}

	var status MergeSlotStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing merge-slot check output: %w", err)
	}

	return &status, nil
}

// MergeSlotAcquire attempts to acquire the merge slot for exclusive access.
// If holder is empty, defaults to BD_ACTOR environment variable.
// If addWaiter is true and the slot is held, the requester is added to the waiters queue.
// Returns the acquisition result.
func (b *Beads) MergeSlotAcquire(holder string, addWaiter bool) (*MergeSlotStatus, error) {
	args := []string{"merge-slot", "acquire", "--json"}
	if holder != "" {
		args = append(args, "--holder="+holder)
	}
	if addWaiter {
		args = append(args, "--wait")
	}

	out, err := b.run(args...)
	if err != nil {
		// Parse the output even on error - it may contain useful info
		var status MergeSlotStatus
		if jsonErr := json.Unmarshal(out, &status); jsonErr == nil {
			return &status, nil
		}
		return nil, fmt.Errorf("acquiring merge slot: %w", err)
	}

	var status MergeSlotStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing merge-slot acquire output: %w", err)
	}

	return &status, nil
}

// MergeSlotRelease releases the merge slot after conflict resolution completes.
// If holder is provided, it verifies the slot is held by that holder before releasing.
func (b *Beads) MergeSlotRelease(holder string) error {
	args := []string{"merge-slot", "release", "--json"}
	if holder != "" {
		args = append(args, "--holder="+holder)
	}

	out, err := b.run(args...)
	if err != nil {
		return fmt.Errorf("releasing merge slot: %w", err)
	}

	var result struct {
		Released bool   `json:"released"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("parsing merge-slot release output: %w", err)
	}

	if !result.Released && result.Error != "" {
		return fmt.Errorf("slot release failed: %s", result.Error)
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

	if status.Error == "not found" {
		// Create it
		return b.MergeSlotCreate()
	}

	return status.ID, nil
}
