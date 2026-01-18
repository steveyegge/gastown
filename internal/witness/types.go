// Package witness provides the polecat monitoring agent.
package witness

import (
	"time"

	"github.com/steveyegge/gastown/internal/agent"
)

// Witness represents a rig's polecat monitoring agent.
type Witness struct {
	// RigName is the rig this witness monitors.
	RigName string `json:"rig_name"`

	// State is the current running state.
	State agent.State `json:"state"`

	// StartedAt is when the witness was started.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// MonitoredPolecats tracks polecats being monitored.
	MonitoredPolecats []string `json:"monitored_polecats,omitempty"`
}

// Ensure Witness implements RigAgentState
var _ agent.RigAgentState = (*Witness)(nil)

// SetRunning updates the state to running with the given start time.
func (w *Witness) SetRunning(startedAt time.Time) {
	w.State = agent.StateRunning
	w.StartedAt = &startedAt
}

// SetStopped updates the state to stopped.
func (w *Witness) SetStopped() {
	w.State = agent.StateStopped
}

// IsRunning returns true if the state indicates running.
func (w *Witness) IsRunning() bool {
	return w.State == agent.StateRunning
}
