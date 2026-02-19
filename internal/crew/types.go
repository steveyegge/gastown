// Package crew provides crew workspace management for overseer workspaces.
package crew

import "time"

// CrewWorker represents a user-managed workspace in a rig.
type CrewWorker struct {
	// Name is the crew worker identifier.
	Name string `json:"name"`

	// Rig is the rig this crew worker belongs to.
	Rig string `json:"rig"`

	// ClonePath is the path to the crew worker's clone of the rig.
	ClonePath string `json:"clone_path"`

	// Branch is the current git branch.
	Branch string `json:"branch"`

	// CreatedAt is when the crew worker was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the crew worker was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// Persona is an optional override for the persona name.
	// When set, prime loads the named persona instead of using crew name.
	// Set via `gt crew persona set`.
	Persona string `json:"persona,omitempty"`

	// Identity is kept for JSON migration only (original field name).
	// Do not use — read via Persona after loadState migrates it.
	Identity string `json:"identity,omitempty"`
}

// Summary provides a concise view of crew worker status.
type Summary struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
}

// Summary returns a Summary for this crew worker.
func (c *CrewWorker) Summary() Summary {
	return Summary{
		Name:   c.Name,
		Branch: c.Branch,
	}
}
