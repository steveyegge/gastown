// Package polecat provides polecat lifecycle management.
package polecat

import "time"

// State represents the current session state of a polecat.
//
// IMPORTANT: There is NO idle state. Polecats have three operating conditions:
//
//   - Working: Session active, doing assigned work (normal operation)
//   - Stalled: Session stopped unexpectedly, was never nudged back to life
//   - Zombie: Session called 'gt done' but cleanup failed - tried to die but couldn't
//
// The distinction matters: zombies completed their work; stalled polecats did not.
// Neither is "idle" - stalled polecats are SUPPOSED to be working, zombies are
// SUPPOSED to be dead. There is no idle pool where polecats wait for work.
//
// Note: These are SESSION states. The polecat IDENTITY (CV chain, mailbox, work
// history) persists across sessions. A stalled or zombie session doesn't destroy
// the polecat's identity - it just means the session needs intervention.
//
// "Stalled" and "zombie" are detected conditions, not stored states. The Witness
// detects them through monitoring (tmux state, age in StateDone, etc.).
type State string

const (
	// StateWorking means the polecat session is actively working on an issue.
	// This is the initial and primary state for transient polecats.
	// Working is the ONLY healthy operating state - there is no idle pool.
	StateWorking State = "working"

	// StateDone means the polecat has completed its assigned work and called
	// 'gt done'. This is normally a transient state - the session should exit
	// immediately after. If a polecat remains in StateDone, it's a "zombie":
	// the cleanup failed and the session is stuck.
	StateDone State = "done"

	// StateStuck means the polecat has explicitly signaled it needs assistance.
	// This is an intentional request for help from the polecat itself.
	// Different from "stalled" (detected externally when session stops working).
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

// CleanupStatus represents the git state of a polecat for cleanup decisions.
// The Witness uses this to determine whether it's safe to nuke a polecat worktree.
type CleanupStatus string

const (
	// CleanupClean means the worktree has no uncommitted work and is safe to remove.
	CleanupClean CleanupStatus = "clean"

	// CleanupUncommitted means there are uncommitted changes in the worktree.
	CleanupUncommitted CleanupStatus = "has_uncommitted"

	// CleanupStash means there are stashed changes that would be lost.
	CleanupStash CleanupStatus = "has_stash"

	// CleanupUnpushed means there are commits not pushed to the remote.
	CleanupUnpushed CleanupStatus = "has_unpushed"

	// CleanupUnknown means the status could not be determined.
	CleanupUnknown CleanupStatus = "unknown"
)

// IsSafe returns true if the status indicates it's safe to remove the worktree
// without losing any work.
func (s CleanupStatus) IsSafe() bool {
	return s == CleanupClean
}

// RequiresRecovery returns true if the status indicates there is work that
// needs to be recovered before removal. This includes uncommitted changes,
// stashes, and unpushed commits.
func (s CleanupStatus) RequiresRecovery() bool {
	switch s {
	case CleanupUncommitted, CleanupStash, CleanupUnpushed:
		return true
	default:
		return false
	}
}

// CanForceRemove returns true if the status allows forced removal.
// Uncommitted changes can be force-removed, but stashes and unpushed commits cannot.
func (s CleanupStatus) CanForceRemove() bool {
	return s == CleanupClean || s == CleanupUncommitted
}
