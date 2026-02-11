// Package dog provides dog session management for Deacon's helper workers.
package dog

import (
	"github.com/steveyegge/gastown/internal/cli"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Session errors
var (
	ErrSessionRunning  = errors.New("session already running")
	ErrSessionNotFound = errors.New("session not found")
)

// SessionManager handles dog session lifecycle.
type SessionManager struct {
	tmux     *tmux.Tmux
	mgr      *Manager
	townRoot string
	townName string
}

// NewSessionManager creates a new dog session manager.
// The Manager parameter is used to sync persistent dog state (idle/working)
// when sessions start and stop.
func NewSessionManager(t *tmux.Tmux, townRoot string, mgr *Manager) *SessionManager {
	townName, _ := workspace.GetTownName(townRoot)
	return &SessionManager{
		tmux:     t,
		mgr:      mgr,
		townRoot: townRoot,
		townName: townName,
	}
}

// SessionStartOptions configures dog session startup.
type SessionStartOptions struct {
	// WorkDesc is the work description (formula or bead ID) for the startup prompt.
	WorkDesc string

	// AgentOverride specifies an alternate agent (e.g., "gemini", "claude-haiku").
	AgentOverride string
}

// SessionInfo contains information about a running dog session.
type SessionInfo struct {
	// DogName is the dog name.
	DogName string `json:"dog_name"`

	// SessionID is the tmux session identifier.
	SessionID string `json:"session_id"`

	// Running indicates if the session is currently active.
	Running bool `json:"running"`

	// Attached indicates if someone is attached to the session.
	Attached bool `json:"attached,omitempty"`

	// Created is when the session was created.
	Created time.Time `json:"created,omitempty"`
}

// SessionName generates the tmux session name for a dog.
// Pattern: gt-{town}-deacon-{name}
func (m *SessionManager) SessionName(dogName string) string {
	return fmt.Sprintf("gt-%s-deacon-%s", m.townName, dogName)
}

// kennelPath returns the path to the dog's kennel directory.
func (m *SessionManager) kennelPath(dogName string) string {
	return filepath.Join(m.townRoot, "deacon", "dogs", dogName)
}

// Start creates and starts a new session for a dog.
// Dogs run agent sessions that check mail for work and execute formulas.
func (m *SessionManager) Start(dogName string, opts SessionStartOptions) error {
	kennelDir := m.kennelPath(dogName)
	if _, err := os.Stat(kennelDir); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrDogNotFound, dogName)
	}

	sessionID := m.SessionName(dogName)

	// Check if session already exists
	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		// Session exists - check if agent is actually running (healthy vs zombie)
		if m.tmux.IsAgentAlive(sessionID) {
			return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
		}
		// Zombie - tmux alive but agent dead. Kill and recreate.
		if err := m.tmux.KillSessionWithProcesses(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Ensure runtime settings exist for dogs
	runtimeConfig := config.ResolveRoleAgentConfig("dog", m.townRoot, kennelDir)
	if err := runtime.EnsureSettingsForRole(kennelDir, "dog", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Build startup prompt - dogs check mail for work
	address := fmt.Sprintf("deacon/dogs/%s", dogName)
	workInfo := ""
	if opts.WorkDesc != "" {
		workInfo = fmt.Sprintf(" Work assigned: %s.", opts.WorkDesc)
	}
	beacon := session.FormatStartupBeacon(session.BeaconConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "assigned",
	})
	initialPrompt := fmt.Sprintf("I am Dog %s.%s Check mail for work: `" + cli.Name() + " mail inbox`. Execute assigned formula/bead. When done, send DOG_DONE mail to deacon/ and return to idle.", dogName, workInfo)

	// Build startup command
	startupCmd, err := config.BuildAgentStartupCommandWithAgentOverride("dog", "", m.townRoot, "", beacon+"\n"+initialPrompt, opts.AgentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// Create session with command
	if err := m.tmux.NewSessionWithCommand(sessionID, kennelDir, startupCmd); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     "dog",
		TownRoot: m.townRoot,
	})
	for k, v := range envVars {
		_ = m.tmux.SetEnvironment(sessionID, k, v)
	}

	// Apply dog theming
	theme := tmux.DogTheme()
	_ = m.tmux.ConfigureGasTownSession(sessionID, theme, "", dogName, "dog")

	// Wait for agent to start
	if err := m.tmux.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		_ = m.tmux.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for dog to start: %w", err)
	}

	// Accept bypass permissions warning if it appears
	_ = m.tmux.AcceptBypassPermissionsWarning(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Verify session survived startup
	running, err = m.tmux.HasSession(sessionID)
	if err != nil {
		// Clean up the session we just created to prevent orphans
		_ = m.tmux.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("verifying session: %w", err)
	}
	if !running {
		return fmt.Errorf("session %s died during startup", sessionID)
	}

	// Track PID for defense-in-depth orphan cleanup (non-fatal)
	_ = session.TrackSessionPID(m.townRoot, sessionID, m.tmux)

	// Update persistent state to working
	if m.mgr != nil {
		if err := m.mgr.SetState(dogName, StateWorking); err != nil {
			// Log but don't fail - session is running, state sync is best-effort
			fmt.Fprintf(os.Stderr, "warning: failed to set dog %s state to working: %v\n", dogName, err)
		}
	}

	return nil
}

// Stop terminates a dog session.
func (m *SessionManager) Stop(dogName string, force bool) error {
	sessionID := m.SessionName(dogName)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	// Try graceful shutdown first
	if !force {
		_ = m.tmux.SendKeysRaw(sessionID, "C-c")
		session.WaitForSessionExit(m.tmux, sessionID, constants.GracefulShutdownTimeout)
	}

	if err := m.tmux.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	// Update persistent state to idle so dog is available for reassignment
	if m.mgr != nil {
		if err := m.mgr.SetState(dogName, StateIdle); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to set dog %s state to idle: %v\n", dogName, err)
		}
	}

	return nil
}

// IsRunning checks if a dog session is active.
func (m *SessionManager) IsRunning(dogName string) (bool, error) {
	sessionID := m.SessionName(dogName)
	return m.tmux.HasSession(sessionID)
}

// Status returns detailed status for a dog session.
func (m *SessionManager) Status(dogName string) (*SessionInfo, error) {
	sessionID := m.SessionName(dogName)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}

	info := &SessionInfo{
		DogName:   dogName,
		SessionID: sessionID,
		Running:   running,
	}

	if !running {
		return info, nil
	}

	tmuxInfo, err := m.tmux.GetSessionInfo(sessionID)
	if err != nil {
		return info, nil
	}

	info.Attached = tmuxInfo.Attached

	return info, nil
}

// GetPane returns the pane ID for a dog session.
func (m *SessionManager) GetPane(dogName string) (string, error) {
	sessionID := m.SessionName(dogName)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	// Get pane ID from session
	pane, err := m.tmux.GetPaneID(sessionID)
	if err != nil {
		return "", fmt.Errorf("getting pane: %w", err)
	}

	return pane, nil
}

// EnsureRunning ensures a dog session is running, starting it if needed.
// Returns the pane ID.
func (m *SessionManager) EnsureRunning(dogName string, opts SessionStartOptions) (string, error) {
	running, err := m.IsRunning(dogName)
	if err != nil {
		return "", err
	}

	if !running {
		if err := m.Start(dogName, opts); err != nil {
			return "", err
		}
	}

	return m.GetPane(dogName)
}
