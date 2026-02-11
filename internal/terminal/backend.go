// Package terminal provides a backend abstraction for terminal I/O operations.
//
// This enables the same peek/nudge commands to work with both local tmux
// sessions and remote K8s pods via SSH+tmux or Coop.
package terminal

import "errors"

// ErrNotSupported is returned by backend methods that are not available
// for a particular backend type. For example, environment variable
// operations are only supported by the Coop backend.
var ErrNotSupported = errors.New("operation not supported by this backend")

// Backend provides terminal capture and input for agent sessions.
// Implementations include local tmux (TmuxBackend), remote SSH+tmux
// for K8s-hosted polecats (SSHBackend), and Coop (CoopBackend).
type Backend interface {
	// HasSession checks if a terminal session exists and is running.
	HasSession(session string) (bool, error)

	// CapturePane captures the last N lines of terminal output from a session.
	CapturePane(session string, lines int) (string, error)

	// CapturePaneAll captures the full scrollback history from a session.
	CapturePaneAll(session string) (string, error)

	// CapturePaneLines captures the last N lines as a string slice.
	CapturePaneLines(session string, lines int) ([]string, error)

	// NudgeSession sends a message to a terminal session with proper
	// serialization and Enter key handling.
	NudgeSession(session string, message string) error

	// SendKeys sends raw keystrokes to a terminal session.
	SendKeys(session string, keys string) error

	// IsPaneDead checks if the session's pane process has exited.
	// For tmux: checks pane_dead flag. For coop: checks if agent state is "exited" or "crashed".
	IsPaneDead(session string) (bool, error)

	// SetPaneDiedHook sets up a callback/hook for when an agent's pane dies.
	// For tmux: sets a tmux pane-died hook. For coop: this is a no-op (coop manages its own lifecycle).
	SetPaneDiedHook(session, agentID string) error

	// --- Coop-first methods (Phase 2) ---
	// These are fully implemented by CoopBackend; TmuxBackend and SSHBackend
	// return ErrNotSupported.

	// KillSession terminates an agent session.
	// For coop: sends SIGTERM via POST /api/v1/signal.
	KillSession(session string) error

	// IsAgentRunning returns true if the agent process is actively running
	// (not exited, not crashed).
	// For coop: checks /api/v1/status process state.
	IsAgentRunning(session string) (bool, error)

	// GetAgentState returns the structured agent state string.
	// For coop: returns the state field from /api/v1/agent/state.
	GetAgentState(session string) (string, error)

	// SetEnvironment sets an environment variable for the session.
	// The value is staged and applied on the next session switch.
	// For coop: PUT /api/v1/env/:key.
	SetEnvironment(session, key, value string) error

	// GetEnvironment retrieves the value of an environment variable.
	// For coop: GET /api/v1/env/:key (checks pending env first, then /proc).
	GetEnvironment(session, key string) (string, error)

	// GetPaneWorkDir returns the working directory of the session's process.
	// For coop: GET /api/v1/session/cwd.
	GetPaneWorkDir(session string) (string, error)

	// SendInput sends text input to the terminal, optionally followed by Enter.
	// For coop: POST /api/v1/input.
	SendInput(session string, text string, enter bool) error

	// RespawnPane restarts the agent process within the same session.
	// For coop: PUT /api/v1/session/switch (switch-to-self restarts).
	RespawnPane(session string) error

	// SwitchSession switches the agent session to use new credentials/env.
	// For coop: PUT /api/v1/session/switch with the provided config.
	SwitchSession(session string, cfg SwitchConfig) error
}

// SwitchConfig holds parameters for a session switch operation.
type SwitchConfig struct {
	// ExtraEnv are environment variables to merge into the new session.
	ExtraEnv map[string]string `json:"extra_env,omitempty"`
}
