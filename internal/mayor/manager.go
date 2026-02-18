package mayor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/runtime"
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

	// Kill any existing zombie session (tmux alive but agent dead).
	// Returns error if session is healthy and already running.
	_, err := session.KillExistingSession(t, sessionID, true)
	if err != nil {
		return ErrAlreadyRunning
	}

	// Ensure mayor directory exists (for Claude settings)
	mayorDir := m.mayorDir()
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		return fmt.Errorf("creating mayor directory: %w", err)
	}

	// Get fallback info to determine beacon content based on agent capabilities.
	// Non-hook agents need "Run gt prime" in beacon; work instructions come as delayed nudge.
	runtimeConfig := config.ResolveRoleAgentConfig("mayor", m.townRoot, "")
	fallbackInfo := runtime.GetStartupFallbackInfo(runtimeConfig)

	// Use unified session lifecycle for config → settings → command → create → env → theme → wait.
	// Configure beacon based on agent's hook/prompt capabilities.
	theme := tmux.MayorTheme()
	result, err := session.StartSession(t, session.SessionConfig{
		SessionID: sessionID,
		WorkDir:   mayorDir,
		Role:      "mayor",
		TownRoot:  m.townRoot,
		AgentName: "Mayor",
		Beacon: session.BeaconConfig{
			Recipient:               "mayor",
			Sender:                  "human",
			Topic:                   "cold-start",
			IncludePrimeInstruction: fallbackInfo.IncludePrimeInBeacon,
			ExcludeWorkInstructions: fallbackInfo.SendStartupNudge,
		},
		AgentOverride: agentOverride,
		Theme:         &theme,
		WaitForAgent:  true,
		WaitFatal:     true,
		AutoRespawn:   true,
		AcceptBypass:  true,
	})
	if err != nil {
		return err
	}

	// Send gt prime via tmux for non-hook agents (e.g., codex).
	// Hook-based agents (claude, opencode) get context via their hook system,
	// so RunStartupFallback is a no-op for them.
	runtime.SleepForReadyDelay(result.RuntimeConfig)
	_ = runtime.RunStartupFallback(t, sessionID, "mayor", result.RuntimeConfig)

	time.Sleep(session.ShutdownDelay())

	// Handle fallback nudges for non-prompt agents (e.g., Codex, OpenCode).
	// See StartupFallbackInfo in runtime package for the fallback matrix.
	if fallbackInfo.SendBeaconNudge && fallbackInfo.SendStartupNudge && fallbackInfo.StartupNudgeDelayMs == 0 {
		// Hooks + no prompt: Single combined nudge (hook already ran gt prime synchronously)
		beacon := session.FormatStartupBeacon(session.BeaconConfig{
			Recipient: "mayor",
			Sender:    "human",
			Topic:     "cold-start",
		})
		combined := beacon + "\n\n" + runtime.StartupNudgeContent()
		_ = t.NudgeSession(sessionID, combined)
	} else {
		if fallbackInfo.SendBeaconNudge {
			// Agent doesn't support CLI prompt - send beacon via nudge
			beacon := session.FormatStartupBeacon(session.BeaconConfig{
				Recipient: "mayor",
				Sender:    "human",
				Topic:     "cold-start",
			})
			_ = t.NudgeSession(sessionID, beacon)
		}

		if fallbackInfo.StartupNudgeDelayMs > 0 {
			// Wait for agent to run gt prime before sending work instructions
			time.Sleep(time.Duration(fallbackInfo.StartupNudgeDelayMs) * time.Millisecond)
		}

		if fallbackInfo.SendStartupNudge {
			// Send work instructions via nudge
			_ = t.NudgeSession(sessionID, runtime.StartupNudgeContent())
		}
	}

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

	// Kill the session and all its processes
	if err := t.KillSessionWithProcesses(sessionID); err != nil {
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
