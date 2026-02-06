// Package terminal provides a backend abstraction for terminal I/O operations.
//
// This enables the same peek/nudge commands to work with both local tmux
// sessions and remote K8s pods via SSH+tmux.
package terminal

// Backend provides terminal capture and input for agent sessions.
// Implementations include local tmux (TmuxBackend) and remote SSH+tmux
// for K8s-hosted polecats (SSHBackend).
type Backend interface {
	// HasSession checks if a terminal session exists and is running.
	HasSession(session string) (bool, error)

	// CapturePane captures the last N lines of terminal output from a session.
	CapturePane(session string, lines int) (string, error)

	// NudgeSession sends a message to a terminal session with proper
	// serialization and Enter key handling.
	NudgeSession(session string, message string) error

	// SendKeys sends raw keystrokes to a terminal session.
	SendKeys(session string, keys string) error
}
