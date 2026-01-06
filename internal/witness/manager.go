package witness

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Common errors
var (
	ErrNotRunning     = errors.New("witness not running")
	ErrAlreadyRunning = errors.New("witness already running")
)

// Manager handles witness lifecycle and monitoring operations.
type Manager struct {
	rig          *rig.Rig
	workDir      string
	stateManager *agent.StateManager[Witness]
}

// NewManager creates a new witness manager for a rig.
func NewManager(r *rig.Rig) *Manager {
	return &Manager{
		rig:     r,
		workDir: r.Path,
		stateManager: agent.NewStateManager[Witness](r.Path, "witness.json", func() *Witness {
			return &Witness{
				RigName: r.Name,
				State:   StateStopped,
			}
		}),
	}
}

// stateFile returns the path to the witness state file.
func (m *Manager) stateFile() string {
	return m.stateManager.StateFile()
}

// loadState loads witness state from disk.
func (m *Manager) loadState() (*Witness, error) {
	return m.stateManager.Load()
}

// saveState persists witness state to disk using atomic write.
func (m *Manager) saveState(w *Witness) error {
	return m.stateManager.Save(w)
}

// sessionName returns the tmux session name for this witness.
func (m *Manager) sessionName() string {
	return fmt.Sprintf("gt-%s-witness", m.rig.Name)
}

// Status returns the current witness status.
// ZFC-compliant: trusts agent-reported state, no PID inference.
// The daemon reads agent bead state for liveness checks.
func (m *Manager) Status() (*Witness, error) {
	w, err := m.loadState()
	if err != nil {
		return nil, err
	}

	// Update monitored polecats list (still useful for display)
	w.MonitoredPolecats = m.rig.Polecats

	return w, nil
}

// Start starts the witness (marks it as running).
// Patrol logic is now handled by mol-witness-patrol molecule executed by Claude.
func (m *Manager) Start() error {
	w, err := m.loadState()
	if err != nil {
		return err
	}

	if w.State == StateRunning && w.PID > 0 && util.ProcessExists(w.PID) {
		return ErrAlreadyRunning
	}

	now := time.Now()
	w.State = StateRunning
	w.StartedAt = &now
	w.PID = os.Getpid()
	w.MonitoredPolecats = m.rig.Polecats

	return m.saveState(w)
}

// Stop stops the witness.
func (m *Manager) Stop() error {
	w, err := m.loadState()
	if err != nil {
		return err
	}

	// Check if tmux session exists
	t := tmux.NewTmux()
	sessionID := m.sessionName()
	sessionRunning, _ := t.HasSession(sessionID)

	// If neither state nor session indicates running, it's not running
	if w.State != StateRunning && !sessionRunning {
		return ErrNotRunning
	}

	// Kill tmux session if it exists (best-effort: may already be dead)
	if sessionRunning {
		_ = t.KillSession(sessionID)
	}

	// If we have a PID, try to stop it gracefully
	if w.PID > 0 && w.PID != os.Getpid() && util.ProcessExists(w.PID) {
		// Send SIGTERM (best-effort graceful stop)
		if proc, err := os.FindProcess(w.PID); err == nil {
			_ = proc.Signal(os.Interrupt)
		}
	}

	w.State = StateStopped
	w.PID = 0

	return m.saveState(w)
}

