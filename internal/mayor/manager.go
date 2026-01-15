package mayor

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
	ErrNotRunning     = errors.New("mayor not running")
	ErrAlreadyRunning = errors.New("mayor already running")
)

// Manager handles mayor lifecycle operations.
type Manager struct {
	townRoot string
}

// NewManager creates a new mayor manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
	}
}

// SessionName returns the tmux session name for the mayor.
// This is a package-level function for convenience.
func SessionName() string {
	return session.MayorSessionName()
}

// SessionName returns the tmux session name for the mayor.
func (m *Manager) SessionName() string {
	return SessionName()
}

// mayorDir returns the working directory for the mayor.
func (m *Manager) mayorDir() string {
	return filepath.Join(m.townRoot, "mayor")
}

// Start starts the mayor session.
// agentOverride optionally specifies a different agent alias to use.
func (m *Manager) Start(agentOverride string) error {
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

	// Ensure mayor directory exists (for Claude settings)
	mayorDir := m.mayorDir()
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		return fmt.Errorf("creating mayor directory: %w", err)
	}

	// Ensure Claude settings exist
	if err := claude.EnsureSettingsForRole(mayorDir, "mayor"); err != nil {
		return fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Build startup beacon with explicit instructions (matches gt handoff behavior)
	// This ensures the agent has clear context immediately, not after nudges arrive
	beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
		Recipient: "mayor",
		Sender:    "human",
		Topic:     "cold-start",
	})

	// Build startup command WITH the beacon prompt - the startup hook handles 'gt prime' automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	startupCmd, err := config.BuildAgentStartupCommandWithAgentOverride("mayor", "", m.townRoot, "", beacon, agentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// Create session in townRoot (not mayorDir) to match gt handoff behavior
	// This ensures Mayor works from the town root where all tools work correctly
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, m.townRoot, startupCmd); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     "mayor",
		TownRoot: m.townRoot,
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Apply Mayor theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.MayorTheme()
	_ = t.ConfigureGasTownSession(sessionID, theme, "", "Mayor", "coordinator")

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - try to continue anyway
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Startup beacon with instructions is now included in the initial command,
	// so no separate nudge needed. The agent starts with full context immediately.

	return nil
}

// Stop stops the mayor session.
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

// IsRunning checks if the mayor session is active.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	return t.HasSession(m.SessionName())
}

// Status returns information about the mayor session.
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
