package witness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
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

// SessionName returns the tmux session name for this witness.
func (m *Manager) SessionName() string {
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

// Start starts the witness.
// If foreground is true, only updates state (no tmux session - deprecated).
// Otherwise, spawns a Claude agent in a tmux session.
// agentOverride optionally specifies a different agent alias to use.
func (m *Manager) Start(foreground bool, agentOverride string) error {
	w, err := m.loadState()
	if err != nil {
		return err
	}

	t := tmux.NewTmux()
	sessionID := m.SessionName()

	if foreground {
		// Foreground mode is deprecated - patrol logic moved to mol-witness-patrol
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

	// Background mode: check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if Claude is actually running (healthy vs zombie)
		if t.IsClaudeRunning(sessionID) {
			// Healthy - Claude is running
			return ErrAlreadyRunning
		}
		// Zombie - tmux alive but Claude dead. Kill and recreate.
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Also check via PID for backwards compatibility
	if w.State == StateRunning && w.PID > 0 && util.ProcessExists(w.PID) {
		return ErrAlreadyRunning
	}

	// Working directory
	witnessDir := m.witnessDir()

	// Ensure Claude settings exist in witness/ (not witness/rig/) so we don't
	// write into the source repo. Claude walks up the tree to find settings.
	witnessParentDir := filepath.Join(m.rig.Path, "witness")
	if err := claude.EnsureSettingsForRole(witnessParentDir, "witness"); err != nil {
		return fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Create new tmux session
	if err := t.NewSession(sessionID, witnessDir); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	townRoot := filepath.Dir(m.rig.Path)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     "witness",
		Rig:      m.rig.Name,
		TownRoot: townRoot,
		BeadsDir: beads.ResolveBeadsDir(m.rig.Path),
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Apply Gas Town theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, "witness", "witness")

	// Update state to running
	now := time.Now()
	w.State = StateRunning
	w.StartedAt = &now
	w.PID = 0 // Claude agent doesn't have a PID we track
	w.MonitoredPolecats = m.rig.Polecats
	if err := m.saveState(w); err != nil {
		_ = t.KillSession(sessionID) // best-effort cleanup on state save failure
		return fmt.Errorf("saving state: %w", err)
	}

	// Launch Claude directly (no shell respawn loop)
	// Restarts are handled by daemon via LIFECYCLE mail or deacon health-scan
	// NOTE: No gt prime injection needed - SessionStart hook handles it automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	// Pass m.rig.Path so rig agent settings are honored (not town-level defaults)
	command, err := config.BuildAgentStartupCommandWithAgentOverride("witness", bdActor, m.rig.Path, "", agentOverride)
	if err != nil {
		_ = t.KillSession(sessionID)
		return fmt.Errorf("building startup command: %w", err)
	}
	// Wait for shell to be ready before sending keys (prevents "can't find pane" under load)
	if err := t.WaitForShellReady(sessionID, 5*time.Second); err != nil {
		_ = t.KillSession(sessionID)
		return fmt.Errorf("waiting for shell: %w", err)
	}
	if err := t.SendKeys(sessionID, command); err != nil {
		_ = t.KillSession(sessionID) // best-effort cleanup
		return fmt.Errorf("starting Claude agent: %w", err)
	}

	// Wait for Claude to start (non-fatal).
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - try to continue anyway
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/witness", m.rig.Name)
	_ = session.StartupNudge(t, sessionID, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "patrol",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous patrol execution.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionID, session.PropulsionNudgeForRole("witness", witnessDir)) // Non-fatal

	return nil
}

// Stop stops the witness.
func (m *Manager) Stop() error {
	w, err := m.loadState()
	if err != nil {
		return err
	}

	// Check if tmux session exists
	t := tmux.NewTmux()
	sessionID := m.SessionName()
	sessionRunning, _ := t.HasSession(sessionID)

	// If neither state nor session indicates running, it's not running
	if w.State != StateRunning && !sessionRunning {
		return ErrNotRunning
	}

	// Kill tmux session if it exists (best-effort: may already be dead)
	if sessionRunning {
		_ = t.KillSession(sessionID)
	}

	// If we have a PID and it's a different process, try to stop it gracefully
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
