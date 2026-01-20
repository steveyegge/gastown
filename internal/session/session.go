// Package session provides abstractions for running agents in terminal sessions.
// The primary implementation is tmux, but this abstraction allows for
// testing with mocks and potentially other terminal multiplexers (e.g., zellij).
package session

import (
	"time"

	"github.com/steveyegge/gastown/internal/ids"
)

// SessionID identifies a session at the interface level.
// Examples: "hq-mayor", "gt-myrig-witness", "gt-myrig-crew-bob"
type SessionID string

// Info contains information about a session.
type Info struct {
	Name         string // Session name
	Created      string // Creation time (format varies by implementation)
	Attached     bool   // Whether someone is attached
	Windows      int    // Number of windows
	Activity     string // Last activity timestamp
	LastAttached string // Last time the session was attached
}

// Sessions is the portable interface for managing a collection of terminal sessions.
// It abstracts the underlying session manager (tmux, zellij, etc.).
//
// This interface manages a collection of named sessions. Methods that operate
// on a specific session take a SessionID parameter. Use List() to get existing
// session IDs, or Start() to create a new session and get its ID.
//
// Agent-specific behavior (readiness, hooks) is handled by the agent.Agents layer.
//
// Implementation-specific extensions (like tmux theming) should be handled directly
// by the implementation (e.g., *tmux.Tmux has ConfigureGasTownSession method).
type Sessions interface {
	// Lifecycle
	Start(name, workDir, command string) (SessionID, error)
	Stop(id SessionID) error
	Exists(id SessionID) (bool, error)
	Respawn(id SessionID, command string) error // Atomic kill + restart (for handoff)

	// Communication
	Send(id SessionID, text string) error       // Send text to session (appends Enter)
	SendControl(id SessionID, key string) error // Send control sequence (no Enter, e.g., "C-c", "Down")
	Nudge(id SessionID, message string) error   // Robust message delivery (handles vim mode, retries)

	// Observation
	Capture(id SessionID, lines int) (string, error)
	CaptureAll(id SessionID) (string, error) // Capture entire scrollback history
	IsRunning(id SessionID, processNames ...string) bool
	WaitFor(id SessionID, timeout time.Duration, processNames ...string) error
	GetStartCommand(id SessionID) (string, error) // Get the command that started the session

	// Management
	List() ([]SessionID, error)
	GetInfo(id SessionID) (*Info, error)

	// Interactive
	Attach(id SessionID) error   // Attach to session (exec into terminal)
	SwitchTo(id SessionID) error // Switch to session (when inside multiplexer)

	// ID Conversion
	// SessionIDForAgent converts an agent address to its SessionID.
	// This allows the agent layer to work with AgentIDs while session layer uses SessionIDs.
	SessionIDForAgent(id ids.AgentID) SessionID
}
