package deacon

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
	ErrNotRunning     = errors.New("deacon not running")
	ErrAlreadyRunning = errors.New("deacon already running")
)

// tmuxOps abstracts tmux operations for testing.
type tmuxOps interface {
	HasSession(name string) (bool, error)
	IsAgentAlive(session string) bool
	KillSessionWithProcesses(name string) error
	NewSessionWithCommand(name, workDir, command string) error
	SetRemainOnExit(pane string, on bool) error
	SetEnvironment(session, key, value string) error
	ConfigureGasTownSession(session string, theme tmux.Theme, rig, worker, role string) error
	WaitForCommand(session string, excludeCommands []string, timeout time.Duration) error
	SetAutoRespawnHook(session string) error
	AcceptBypassPermissionsWarning(session string) error
	SendKeysRaw(session, keys string) error
	GetSessionInfo(name string) (*tmux.SessionInfo, error)
	NudgeSession(session, message string) error
}

// Manager handles deacon lifecycle operations.
type Manager struct {
	townRoot string
	tmux     tmuxOps
}

// NewManager creates a new deacon manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
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
// agentOverride allows specifying an alternate agent alias (e.g., for testing).
// Restarts are handled by daemon via ensureDeaconRunning on each heartbeat.
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
		// Zombie - tmux alive but agent dead. Kill and recreate.
		// Use KillSessionWithProcesses to ensure all descendant processes are killed.
		if err := t.KillSessionWithProcesses(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Ensure deacon directory exists
	deaconDir := m.deaconDir()
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}

	// Ensure runtime settings exist in deaconDir where session runs.
	runtimeConfig := config.ResolveRoleAgentConfig("deacon", m.townRoot, deaconDir)
	if err := runtime.EnsureSettingsForRole(deaconDir, deaconDir, "deacon", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Build startup prompt with beacon and instructions.
	beacon := session.FormatStartupBeacon(session.BeaconConfig{
		Recipient: "deacon",
		Sender:    "daemon",
		Topic:     "patrol",
	})
	initialPrompt := beacon + "\n\nI am Deacon running in PERSISTENT PATROL MODE. My patrol loop: 1. Run gt deacon heartbeat. 2. Check gt hook - if exists, execute it. 3. If no hook, create and execute: gt wisp create mol-deacon-patrol --hook --execute. 4. After patrol completes, use await-signal to wait for next cycle. 5. Return to step 1. I NEVER exit voluntarily."
	startupCmd, err := config.BuildAgentStartupCommandWithAgentOverride("deacon", "", m.townRoot, "", initialPrompt, agentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// Create session using unified StartSession (benefits from tmuxinator
	// when available). Type-assert for production path; fall back to manual
	// session creation for test mocks.
	theme := tmux.DeaconTheme()
	if realTmux, ok := t.(*tmux.Tmux); ok {
		_, err := session.StartSession(realTmux, session.SessionConfig{
			SessionID:    sessionID,
			WorkDir:      deaconDir,
			Role:         "deacon",
			TownRoot:     m.townRoot,
			AgentName:    "Deacon",
			Command:      startupCmd,
			Theme:        &theme,
			RemainOnExit: true,
			AutoRespawn:  true,
			WaitForAgent: true,
			WaitFatal:    true,
			AcceptBypass: true,
			TrackPID:     true,
		})
		if err != nil {
			return err
		}
	} else {
		// Mock/test path: manual session creation
		if err := t.NewSessionWithCommand(sessionID, deaconDir, startupCmd); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}
		_ = t.SetRemainOnExit(sessionID, true)
		envVars := config.AgentEnv(config.AgentEnvConfig{
			Role:     "deacon",
			TownRoot: m.townRoot,
		})
		for k, v := range envVars {
			_ = t.SetEnvironment(sessionID, k, v)
		}
		_ = t.ConfigureGasTownSession(sessionID, theme, "", "Deacon", "health-check")
		if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
			_ = t.KillSessionWithProcesses(sessionID)
			return fmt.Errorf("waiting for deacon to start: %w", err)
		}
		if err := t.SetAutoRespawnHook(sessionID); err != nil {
			fmt.Printf("warning: failed to set auto-respawn hook for deacon: %v\n", err)
		}
		_ = t.AcceptBypassPermissionsWarning(sessionID)
	}

	time.Sleep(constants.ShutdownNotifyDelay)

	// Wait for runtime to be fully ready at the prompt (not just started)
	runtime.SleepForReadyDelay(runtimeConfig)

	return nil
}

// Stop stops the deacon session.
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

	// Kill the session.
	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	// This prevents orphan bash processes from Claude's Bash tool surviving session termination.
	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if the deacon session is active.
func (m *Manager) IsRunning() (bool, error) {
	return m.tmux.HasSession(m.SessionName())
}

// Status returns information about the deacon session.
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
