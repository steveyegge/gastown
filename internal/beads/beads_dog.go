// Package beads provides dog agent bead management.
package beads

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CreateDogAgentBead creates an agent bead for a dog.
// Dogs use a different schema than other agents - they use labels for metadata.
// Returns the created issue or an error.
func (b *Beads) CreateDogAgentBead(name, location string) (*Issue, error) {
	title := fmt.Sprintf("Dog: %s", name)
	beadID := DogBeadIDTown(name) // Use canonical ID: hq-dog-<name>
	labels := []string{
		"gt:agent",
		"role_type:dog",
		"rig:town",
		"location:" + location,
	}

	description := formatDogDescription(name, location)

	args := []string{
		"create", "--json",
		"--id=" + beadID,
		"--type=agent",
		"--role-type=dog",
		"--title=" + title,
		"--description=" + description,
		"--labels=" + strings.Join(labels, ","),
	}
	if NeedsForceForID(beadID) {
		args = append(args, "--force")
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	// Uses getActor() to respect isolated mode (tests)
	if actor := b.getActor(); actor != "" {
		args = append(args, "--actor="+actor)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}

	return &issue, nil
}

// FindDogAgentBead finds the agent bead for a dog by name.
// Searches for agent beads with role_type:dog and matching title.
// Returns nil if not found.
func (b *Beads) FindDogAgentBead(name string) (*Issue, error) {
	// List all agent beads and filter by role_type:dog label
	issues, err := b.List(ListOptions{
		Label:    "gt:agent",
		Status:   "all",
		Priority: -1, // No priority filter
	})
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	expectedTitle := fmt.Sprintf("Dog: %s", name)
	for _, issue := range issues {
		// Check title match and role_type:dog label
		if issue.Title == expectedTitle {
			for _, label := range issue.Labels {
				if label == "role_type:dog" {
					return issue, nil
				}
			}
		}
	}

	return nil, nil
}

// ResetDogAgentBead finds and resets the agent bead for a dog, preserving
// persistent identity. Dogs, like polecats, have persistent agent beads that
// accumulate work history across assignments. Removal transitions state to
// "nuked" rather than deleting the bead.
// Returns nil if the bead doesn't exist (idempotent).
func (b *Beads) ResetDogAgentBead(name string) error {
	issue, err := b.FindDogAgentBead(name)
	if err != nil {
		return fmt.Errorf("finding dog bead: %w", err)
	}
	if issue == nil {
		return nil // Already doesn't exist - idempotent
	}

	err = b.ResetAgentBeadForReuse(issue.ID, "dog removed")
	if err != nil {
		return fmt.Errorf("resetting bead %s: %w", issue.ID, err)
	}
	return nil
}

// formatDogDescription creates a description for a dog agent bead.
// Includes role_type, rig, and location metadata so the mail router
// can resolve the agent address from the description.
func formatDogDescription(name, location string) string {
	return strings.Join([]string{
		fmt.Sprintf("Dog: %s", name),
		"",
		"role_type: dog",
		"rig: town",
		fmt.Sprintf("location: %s", location),
	}, "\n")
}

