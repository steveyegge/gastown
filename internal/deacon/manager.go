package deacon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Common errors
var (
	ErrNotRunning     = errors.New("deacon not running")
	ErrAlreadyRunning = errors.New("deacon already running")
)

// Manager handles deacon lifecycle operations.
type Manager struct {
	townRoot string
}

// NewManager creates a new deacon manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
	}
}

// SessionName returns the tmux session name for the deacon.
// This is a package-level function for convenience.
func SessionName() string {
	return session.DeaconSessionName()
}

// SessionName returns the tmux session name for the deacon.
func (m *Manager) SessionName() string {
	return SessionName()
}

// deaconDir returns the working directory for the deacon.
func (m *Manager) deaconDir() string {
	return filepath.Join(m.townRoot, "deacon")
}

// Start starts the deacon session.
// The deacon runs in a respawn loop for automatic recovery.
func (m *Manager) Start() error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if Claude is actually running (healthy vs zombie)
		if t.IsClaudeRunning(sessionID) {
			return ErrAlreadyRunning
		}
		// Zombie - tmux alive but Claude dead. Kill and recreate.
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Ensure deacon directory exists
	deaconDir := m.deaconDir()
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}

	// Ensure Claude settings exist
	if err := claude.EnsureSettingsForRole(deaconDir, "deacon"); err != nil {
		return fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Create new tmux session
	if err := t.NewSession(sessionID, deaconDir); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	_ = t.SetEnvironment(sessionID, "GT_ROLE", "deacon")
	_ = t.SetEnvironment(sessionID, "BD_ACTOR", "deacon")

	// Apply Deacon theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.DeaconTheme()
	_ = t.ConfigureGasTownSession(sessionID, theme, "", "Deacon", "health-check")

	// Launch Claude in a respawn loop for automatic recovery
	// The respawn loop ensures the deacon restarts if Claude crashes
	runtimeCmd := config.GetRuntimeCommand("")
	respawnCmd := fmt.Sprintf(
		`export GT_ROLE=deacon BD_ACTOR=deacon GIT_AUTHOR_NAME=deacon && while true; do echo "⛪ Starting Deacon session..."; %s; echo ""; echo "Deacon exited. Restarting in 2s... (Ctrl-C to stop)"; sleep 2; done`,
		runtimeCmd,
	)

	if err := t.SendKeysDelayed(sessionID, respawnCmd, 200); err != nil {
		_ = t.KillSession(sessionID) // best-effort cleanup
		return fmt.Errorf("starting Claude agent: %w", err)
	}

	// Wait for Claude to start (non-fatal)
	// Note: Deacon respawn loop makes this tricky - Claude restarts multiple times
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - try to continue anyway
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Note: Deacon doesn't get startup nudge due to respawn loop complexity
	// The deacon uses its own patrol pattern defined in its CLAUDE.md/prime

	return nil
}

// Stop stops the deacon session.
func (m *Manager) Stop() error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	// Check if session exists
	running, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrNotRunning
	}

	// Try graceful shutdown first (best-effort interrupt)
	_ = t.SendKeysRaw(sessionID, "C-c")
	time.Sleep(100 * time.Millisecond)

	// Kill the session
	if err := t.KillSession(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if the deacon session is active.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	return t.HasSession(m.SessionName())
}

// Status returns information about the deacon session.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	running, err := t.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return nil, ErrNotRunning
	}

	return t.GetSessionInfo(sessionID)
}
