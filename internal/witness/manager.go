package witness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
)

// Manager handles witness status and monitoring operations.
// Start/Stop operations are handled via factory.Start()/factory.Agents().Stop().
type Manager struct {
	stateManager *agent.StateManager[Witness]
	agents       agent.AgentObserver // Only needs Exists() for status checks
	rigName      string
	rigPath      string
	address      agent.AgentID
	rig          *rig.Rig
}

// NewManager creates a new witness manager for a rig.
// The manager handles status queries and state reconciliation.
// Lifecycle operations (Start/Stop) should use factory.Start()/factory.Agents().Stop().
//
// The agents parameter only needs to implement AgentObserver (Exists, GetInfo, List).
// In production, pass factory.Agents(). In tests, use agent.NewObserverDouble().
func NewManager(agents agent.AgentObserver, r *rig.Rig) *Manager {
	stateFactory := func() *Witness {
		return &Witness{
			RigName: r.Name,
			State:   agent.StateStopped,
		}
	}
	return &Manager{
		stateManager: agent.NewStateManager[Witness](r.Path, "witness.json", stateFactory),
		agents:       agents,
		rigName:      r.Name,
		rigPath:      r.Path,
		address:      agent.WitnessAddress(r.Name),
		rig:          r,
	}
}

// witnessDir returns the working directory for the witness.
// Prefers witness/rig/, falls back to witness/, then rig root.
func (m *Manager) witnessDir() string {
	witnessRigDir := filepath.Join(m.rig.Path, "witness", "rig")
	if _, err := os.Stat(witnessRigDir); err == nil {
		return witnessRigDir
	}

	witnessDir := filepath.Join(m.rig.Path, "witness")
	if _, err := os.Stat(witnessDir); err == nil {
		return witnessDir
	}

	return m.rig.Path
}

// Status returns the current witness status.
// Reconciles persisted state with actual agent existence.
func (m *Manager) Status() (*Witness, error) {
	w, err := m.stateManager.Load()
	if err != nil {
		return nil, err
	}

	// Reconcile state with reality (don't persist, just report accurately)
	if w.IsRunning() && !m.agents.Exists(m.address) {
		w.SetStopped() // Agent crashed
	}

	// Update monitored polecats list (still useful for display)
	w.MonitoredPolecats = m.rig.Polecats

	return w, nil
}

// SessionName returns the tmux session name for this witness.
func (m *Manager) SessionName() string {
	return fmt.Sprintf("gt-%s-witness", m.rigName)
}

// IsRunning checks if the witness session is currently active.
func (m *Manager) IsRunning() bool {
	return m.agents.Exists(m.address)
}

// Address returns the agent's AgentID.
func (m *Manager) Address() agent.AgentID {
	return m.address
}

// LoadState loads the witness state from disk.
func (m *Manager) LoadState() (*Witness, error) {
	return m.stateManager.Load()
}

// SaveState persists the witness state to disk.
func (m *Manager) SaveState(w *Witness) error {
	return m.stateManager.Save(w)
}
