// Package agent provides interfaces and implementations for managing agent processes.
package agent

import "github.com/steveyegge/gastown/internal/session"

// =============================================================================
// Segregated Interfaces
//
// These smaller interfaces allow consumers to depend only on the capabilities
// they actually need, following the Interface Segregation Principle.
//
// Usage patterns:
//   - Managers that only query status: use AgentObserver
//   - Managers that need cleanup: use AgentObserver + AgentStopper
//   - Factory/lifecycle code: use AgentStarter
//   - Interactive/communication: use AgentCommunicator
//   - Handoff operations: use AgentRespawner
//   - Full control: use Agents (composes all)
// =============================================================================

// AgentObserver provides read-only observation of agents.
// Used by managers that need to check agent state without controlling lifecycle.
//
// Implementations:
//   - agent.Implementation (production)
//   - agent.Double (full fake for testing)
//   - agent.ObserverDouble (minimal fake for testing)
type AgentObserver interface {
	// Exists checks if an agent is running (session exists AND process is alive).
	// Returns false for zombie sessions (tmux exists but agent process died).
	Exists(id AgentID) bool

	// GetInfo returns information about an agent's session.
	GetInfo(id AgentID) (*session.Info, error)

	// List returns all agent addresses.
	List() ([]AgentID, error)
}

// AgentStopper can stop agents.
// Combined with AgentObserver for managers that need cleanup capabilities.
type AgentStopper interface {
	// Stop terminates an agent process.
	Stop(id AgentID, graceful bool) error
}

// AgentStarter can start agents and wait for readiness.
// Used by factory for lifecycle control.
type AgentStarter interface {
	// StartWithConfig launches an agent process with explicit configuration.
	StartWithConfig(id AgentID, cfg StartConfig) error

	// WaitReady blocks until the agent is ready for input or times out.
	WaitReady(id AgentID) error
}

// AgentCommunicator can interact with running agents.
// Used for nudging, capturing output, and attaching.
type AgentCommunicator interface {
	// Nudge sends a message to a running agent reliably.
	Nudge(id AgentID, message string) error

	// Capture returns the recent output from an agent's session.
	Capture(id AgentID, lines int) (string, error)

	// CaptureAll returns the entire scrollback history from an agent's session.
	CaptureAll(id AgentID) (string, error)

	// Attach attaches to a running agent's session (exec into terminal).
	Attach(id AgentID) error
}

// AgentRespawner can atomically restart agents.
// Used for handoff operations.
type AgentRespawner interface {
	// Respawn atomically kills the agent process and starts a new one.
	Respawn(id AgentID) error
}

// =============================================================================
// Composite Interfaces
// =============================================================================

// AgentObserverStopper combines observation and stop capabilities.
// Used by managers that need to check status and clean up agents.
type AgentObserverStopper interface {
	AgentObserver
	AgentStopper
}

// Ensure that the full Agents interface (defined in agent.go) composes all
// the smaller interfaces. This is verified at compile time by the type
// assertion in agent.go: var _ Agents = (*Implementation)(nil)
