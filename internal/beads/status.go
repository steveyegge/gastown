// Package beads provides typed enums for agent states and issue statuses.
//
// ZFC: Hardcoded string comparisons for states and statuses are replaced by typed
// constants with semantic metadata methods. State properties (protectsFromCleanup,
// blocksRemoval, isTerminal) belong on the type, not scattered as conditionals.
// See gt-4d7p.
package beads

// AgentState represents the lifecycle state of an agent bead.
// These values are stored in the agent_state field and used by the witness,
// polecat manager, and sling for lifecycle decisions.
type AgentState string

const (
	AgentStateSpawning     AgentState = "spawning"
	AgentStateWorking      AgentState = "working"
	AgentStateDone         AgentState = "done"
	AgentStateStuck        AgentState = "stuck"
	AgentStateEscalated    AgentState = "escalated"
	AgentStateIdle         AgentState = "idle"
	AgentStateRunning      AgentState = "running"
	AgentStateNuked        AgentState = "nuked"
	AgentStateAwaitingGate AgentState = "awaiting-gate"
)

// ProtectsFromCleanup returns true if this agent state indicates an intentional
// pause that should prevent the polecat from being cleaned up as stale.
// States like "stuck" and "awaiting-gate" mean the polecat is paused on purpose.
func (s AgentState) ProtectsFromCleanup() bool {
	switch s {
	case AgentStateStuck, AgentStateAwaitingGate:
		return true
	default:
		return false
	}
}

// IsActive returns true if the agent is actively doing work.
func (s AgentState) IsActive() bool {
	switch s {
	case AgentStateWorking, AgentStateRunning, AgentStateSpawning:
		return true
	default:
		return false
	}
}

// IssueStatus represents the lifecycle status of a beads issue.
// These values are stored in the status field and govern issue workflow transitions.
//
// StatusPinned and StatusHooked are defined in handoff.go for historical reasons
// and re-exported here as typed constants.
type IssueStatus string

const (
	StatusOpen       IssueStatus = "open"
	StatusClosed     IssueStatus = "closed"
	StatusInProgress IssueStatus = "in_progress"
	StatusTombstone  IssueStatus = "tombstone"
	StatusBlocked    IssueStatus = "blocked"
	// StatusPinned and StatusHooked are defined as untyped string constants in
	// handoff.go. Use IssueStatusPinned/IssueStatusHooked for typed comparisons.
	IssueStatusPinned IssueStatus = "pinned"
	IssueStatusHooked IssueStatus = "hooked"
)

// BlocksRemoval returns true if this status should prevent removal of the
// associated resource (e.g., an MR bead in "open" status blocks polecat removal).
func (s IssueStatus) BlocksRemoval() bool {
	return s == StatusOpen
}

// IsTerminal returns true if this status represents a final state that cannot
// transition further (closed, tombstone).
func (s IssueStatus) IsTerminal() bool {
	switch s {
	case StatusClosed, StatusTombstone:
		return true
	default:
		return false
	}
}

// IsAssigned returns true if this status indicates the issue is actively
// assigned to an agent (hooked or in_progress).
func (s IssueStatus) IsAssigned() bool {
	switch s {
	case IssueStatusHooked, StatusInProgress:
		return true
	default:
		return false
	}
}
