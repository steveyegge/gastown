// Package polecat provides polecat lifecycle management.
package polecat

import "time"

// State represents the current state of a polecat.
// In the transient model, polecats exist only while working.
type State string

const (
	// StateWorking means the polecat is actively working on an issue.
	// This is the initial and primary state for transient polecats.
	StateWorking State = "working"

	// StateDone means the polecat has completed its assigned work
	// and is ready for cleanup by the Witness.
	StateDone State = "done"

	// StateStuck means the polecat needs assistance.
	StateStuck State = "stuck"

	// StateActive is deprecated: use StateWorking.
	// Kept only for backward compatibility with existing data.
	StateActive State = "active"
)

// IsWorking returns true if the polecat is currently working.
func (s State) IsWorking() bool {
	return s == StateWorking
}

// IsActive returns true if the polecat session is actively working.
// For transient polecats, this is true for working state and
// legacy active state (treated as working).
func (s State) IsActive() bool {
	return s == StateWorking || s == StateActive
}

// Polecat represents a worker agent in a rig.
type Polecat struct {
	// Name is the polecat identifier.
	Name string `json:"name"`

	// Rig is the rig this polecat belongs to.
	Rig string `json:"rig"`

	// State is the current lifecycle state.
	State State `json:"state"`

	// ClonePath is the path to the polecat's clone of the rig.
	ClonePath string `json:"clone_path"`

	// Branch is the current git branch.
	Branch string `json:"branch"`

	// Issue is the currently assigned issue ID (if any).
	Issue string `json:"issue,omitempty"`

	// CreatedAt is when the polecat was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the polecat was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// Summary provides a concise view of polecat status.
type Summary struct {
	Name  string `json:"name"`
	State State  `json:"state"`
	Issue string `json:"issue,omitempty"`
}

// Summary returns a Summary for this polecat.
func (p *Polecat) Summary() Summary {
	return Summary{
		Name:  p.Name,
		State: p.State,
		Issue: p.Issue,
	}
}
