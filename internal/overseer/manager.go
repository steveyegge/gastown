package overseer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Common errors
var (
	ErrNotRunning     = errors.New("overseer not running")
	ErrAlreadyRunning = errors.New("overseer already running")
)

// tmuxOps abstracts tmux operations for testing.
type tmuxOps interface {
	HasSession(name string) (bool, error)
	IsAgentAlive(session string) bool
	KillSessionWithProcesses(name string) error
	NewSessionWithCommand(name, workDir, command string) error
	SetRemainOnExit(pane string, on bool) error
	SetEnvironment(session, key, value string) error
	GetPaneID(session string) (string, error)
	ConfigureGasTownSession(session string, theme tmux.Theme, rig, worker, role string) error
	WaitForCommand(session string, excludeCommands []string, timeout time.Duration) error
	SetAutoRespawnHook(session string) error
	AcceptStartupDialogs(session string) error
	SendKeysRaw(session, keys string) error
	GetSessionInfo(name string) (*tmux.SessionInfo, error)
}

// Manager handles overseer lifecycle operations.
type Manager struct {
	townRoot string
	tmux     tmuxOps
}

// NewManager creates a new overseer manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
	}
}

// SessionName returns the tmux session name for the overseer.
func SessionName() string {
	return session.OverseerSessionName()
}

// SessionName returns the tmux session name for the overseer.
func (m *Manager) SessionName() string {
	return SessionName()
}

// overseerDir returns the working directory for the overseer.
func (m *Manager) overseerDir() string {
	return filepath.Join(m.townRoot, "overseer")
}

// Start starts the overseer session.
// agentOverride allows specifying an alternate agent alias (e.g., for testing).
func (m *Manager) Start(agentOverride string) error {
	t := m.tmux
	sessionID := m.SessionName()

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if agent is actually running (healthy vs zombie)
		if t.IsAgentAlive(sessionID) {
			return ErrAlreadyRunning
		}

		// Session exists but agent is dead. Kill and recreate.
		if err := t.KillSessionWithProcesses(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Ensure overseer directory exists
	overseerDir := m.overseerDir()
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		return fmt.Errorf("creating overseer directory: %w", err)
	}

	// Ensure runtime settings exist in overseerDir where session runs.
	runtimeConfig := config.ResolveRoleAgentConfig("overseer", m.townRoot, overseerDir)
	if err := runtime.EnsureSettingsForRole(overseerDir, overseerDir, "overseer", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: "overseer",
		Sender:    "daemon",
		Topic:     "patrol",
	}, "I am Overseer. Check gt hook. If no hook, create mol-overseer-patrol wisp and execute it.")
	startupCmd, err := config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
		Role:        "overseer",
		TownRoot:    m.townRoot,
		Prompt:      initialPrompt,
		Topic:       "patrol",
		SessionName: sessionID,
	}, "", initialPrompt, agentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// Create session with command directly to avoid send-keys race condition.
	if err := t.NewSessionWithCommand(sessionID, overseerDir, startupCmd); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set remain-on-exit immediately after session creation.
	_ = t.SetRemainOnExit(sessionID, true)

	// Set environment variables (non-fatal)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "overseer",
		TownRoot:    m.townRoot,
		Agent:       agentOverride,
		SessionName: sessionID,
	})
	envVars = session.MergeRuntimeLivenessEnv(envVars, runtimeConfig)
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Record agent's pane_id for ZFC-compliant liveness checks.
	if paneID, err := t.GetPaneID(sessionID); err == nil {
		_ = t.SetEnvironment(sessionID, "GT_PANE_ID", paneID)
	}

	// Apply Overseer theming (non-fatal)
	theme := tmux.OverseerTheme()
	_ = t.ConfigureGasTownSession(sessionID, theme, "", "Overseer", "patrol")

	// Wait for Claude to start - fatal if Claude fails to launch
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		_ = t.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for overseer to start: %w", err)
	}

	// Track PID for defense-in-depth orphan cleanup (non-fatal)
	if realTmux, ok := t.(*tmux.Tmux); ok {
		_ = session.TrackSessionPID(m.townRoot, sessionID, realTmux)
	}

	// Set auto-respawn hook for resilience.
	if err := t.SetAutoRespawnHook(sessionID); err != nil {
		fmt.Printf("warning: failed to set auto-respawn hook for overseer: %v\n", err)
	}

	// Accept startup dialogs if they appear.
	_ = t.AcceptStartupDialogs(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	return nil
}

// Stop stops the overseer session.
func (m *Manager) Stop() error {
	t := m.tmux
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

	// Kill the session with all descendant processes.
	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if the overseer session is active.
func (m *Manager) IsRunning() (bool, error) {
	return m.tmux.HasSession(m.SessionName())
}

// Status returns information about the overseer session.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := m.tmux
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
