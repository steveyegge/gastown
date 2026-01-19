package agent

import (
	"time"
)

// RigAgentState is the interface that rig-level agent states must implement.
// This allows managers to update running state generically for state reconciliation.
// Implementations should define methods on pointer receivers.
//
// Rig-level state types (Witness, Refinery) implement this interface to provide
// consistent state management across different agent types.
type RigAgentState interface {
	// SetRunning updates the state to running with the given start time.
	SetRunning(startedAt time.Time)
	// SetStopped updates the state to stopped.
	SetStopped()
	// IsRunning returns true if the state indicates running.
	IsRunning() bool
}
