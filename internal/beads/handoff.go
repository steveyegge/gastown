// Package beads provides handoff bead operations for agent workflow management.
package beads

import (
	"fmt"
	"time"
)

// StatusPinned is the status for pinned beads that never get closed.
// These are "domain table" beads like role definitions that persist permanently.
const StatusPinned = "pinned"

// StatusHooked is the status for beads on an agent's hook (work assignment).
// This is distinct from pinned - hooked beads are active work, not permanent records.
const StatusHooked = "hooked"

// HandoffBeadTitle returns the well-known title for a role's handoff bead.
func HandoffBeadTitle(role string) string {
	return role + " Handoff"
}

// FindHandoffBead finds the pinned handoff bead for a role by title.
// Returns nil if not found (not an error).
func (b *Beads) FindHandoffBead(role string) (*Issue, error) {
	issues, err := b.List(ListOptions{Status: StatusPinned, Priority: -1})
	if err != nil {
		return nil, fmt.Errorf("listing pinned issues: %w", err)
	}

	targetTitle := HandoffBeadTitle(role)
	for _, issue := range issues {
		if issue.Title == targetTitle {
			return issue, nil
		}
	}

	return nil, nil
}

// GetOrCreateHandoffBead returns the handoff bead for a role, creating it if needed.
func (b *Beads) GetOrCreateHandoffBead(role string) (*Issue, error) {
	// Check if it exists
	existing, err := b.FindHandoffBead(role)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// Create new handoff bead (type is deprecated, uses gt:task label via backward compat)
	issue, err := b.Create(CreateOptions{
		Title:       HandoffBeadTitle(role),
		Type:        "task", // Converted to gt:task label by Create()
		Priority:    2,
		Description: "", // Empty until first handoff
		Actor:       role,
	})
	if err != nil {
		return nil, fmt.Errorf("creating handoff bead: %w", err)
	}

	// Update to pinned status
	status := StatusPinned
	if err := b.Update(issue.ID, UpdateOptions{Status: &status}); err != nil {
		return nil, fmt.Errorf("setting handoff bead to pinned: %w", err)
	}

	// Re-fetch to get updated status
	return b.Show(issue.ID)
}

// UpdateHandoffContent updates the handoff bead's description with new content.
func (b *Beads) UpdateHandoffContent(role, content string) error {
	issue, err := b.GetOrCreateHandoffBead(role)
	if err != nil {
		return err
	}

	return b.Update(issue.ID, UpdateOptions{Description: &content})
}

// ClearHandoffContent clears the handoff bead's description.
func (b *Beads) ClearHandoffContent(role string) error {
	issue, err := b.FindHandoffBead(role)
	if err != nil {
		return err
	}
	if issue == nil {
		return nil // Nothing to clear
	}

	empty := ""
	return b.Update(issue.ID, UpdateOptions{Description: &empty})
}

// ClearMailResult contains statistics from a ClearMail operation.
type ClearMailResult struct {
	Closed  int // Number of messages closed
	Cleared int // Number of pinned messages cleared (content removed)
}

// ClearMail closes or clears all open messages.
// Non-pinned messages are closed with the given reason.
// Pinned messages have their description cleared but remain open.
func (b *Beads) ClearMail(reason string) (*ClearMailResult, error) {
	// List all open messages
	issues, err := b.List(ListOptions{
		Status:   "open",
		Label:    "gt:message",
		Priority: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}

	result := &ClearMailResult{}

	// Separate pinned from non-pinned
	var toClose []string
	var toClear []*Issue

	for _, issue := range issues {
		if issue.Status == StatusPinned {
			toClear = append(toClear, issue)
		} else {
			toClose = append(toClose, issue.ID)
		}
	}

	// Close non-pinned messages in batch
	if len(toClose) > 0 {
		if err := b.CloseWithReason(reason, toClose...); err != nil {
			return nil, fmt.Errorf("closing messages: %w", err)
		}
		result.Closed = len(toClose)
	}

	// Clear pinned messages
	empty := ""
	for _, issue := range toClear {
		if err := b.Update(issue.ID, UpdateOptions{Description: &empty}); err != nil {
			return nil, fmt.Errorf("clearing pinned message %s: %w", issue.ID, err)
		}
		result.Cleared++
	}

	return result, nil
}

// AttachMolecule attaches a molecule to a pinned bead by updating its description.
// The moleculeID is the root issue ID of the molecule to attach.
// Returns the updated issue.
func (b *Beads) AttachMolecule(pinnedBeadID, moleculeID string) (*Issue, error) {
	// Fetch the pinned bead
	issue, err := b.Show(pinnedBeadID)
	if err != nil {
		return nil, fmt.Errorf("fetching pinned bead: %w", err)
	}

	if issue.Status != StatusPinned {
		return nil, fmt.Errorf("issue %s is not pinned (status: %s)", pinnedBeadID, issue.Status)
	}

	// Build attachment fields with current timestamp
	fields := &AttachmentFields{
		AttachedMolecule: moleculeID,
		AttachedAt:       currentTimestamp(),
	}

	// Update description with attachment fields
	newDesc := SetAttachmentFields(issue, fields)

	// Update the issue
	if err := b.Update(pinnedBeadID, UpdateOptions{Description: &newDesc}); err != nil {
		return nil, fmt.Errorf("updating pinned bead: %w", err)
	}

	// Re-fetch to return updated state
	return b.Show(pinnedBeadID)
}

// DetachMolecule removes molecule attachment from a pinned bead.
// Returns the updated issue.
func (b *Beads) DetachMolecule(pinnedBeadID string) (*Issue, error) {
	// Fetch the pinned bead
	issue, err := b.Show(pinnedBeadID)
	if err != nil {
		return nil, fmt.Errorf("fetching pinned bead: %w", err)
	}

	// Check if there's anything to detach
	if ParseAttachmentFields(issue) == nil {
		return issue, nil // Nothing to detach
	}

	// Clear attachment fields by passing nil
	newDesc := SetAttachmentFields(issue, nil)

	// Update the issue
	if err := b.Update(pinnedBeadID, UpdateOptions{Description: &newDesc}); err != nil {
		return nil, fmt.Errorf("updating pinned bead: %w", err)
	}

	// Re-fetch to return updated state
	return b.Show(pinnedBeadID)
}

// GetAttachment returns the attachment fields from a pinned bead.
// Returns nil if no molecule is attached.
func (b *Beads) GetAttachment(pinnedBeadID string) (*AttachmentFields, error) {
	issue, err := b.Show(pinnedBeadID)
	if err != nil {
		return nil, err
	}

	return ParseAttachmentFields(issue), nil
}

// currentTimestamp returns the current time in ISO 8601 format.
func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
