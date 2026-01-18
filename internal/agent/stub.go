package agent

import (
	"github.com/steveyegge/gastown/internal/session"
)

// Note: session import used for GetInfo return type

// =============================================================================
// Test Stubs for Error Injection
//
// Test Double Taxonomy (Meszaros/Fowler):
//   - STUB: Provides canned responses and error injection
//   - Wraps a FAKE (Double) for normal operations
//   - Intercepts specific methods to return configured errors
//
// These stubs wrap pure fakes and allow injecting errors for testing
// error paths. This keeps the fakes as pure drop-in replacements.
//
// Exported so other packages (e.g., refinery) can use them for testing.
// =============================================================================

// AgentsStub is a STUB wrapper for testing error paths.
// Wraps an Agents implementation (typically Double) and injects errors.
//
// Example:
//
//	fake := agent.NewDouble()
//	stub := agent.NewAgentsStub(fake)
//	stub.StartErr = errors.New("start failed")
//	// Now StartWithConfig will return the injected error
type AgentsStub struct {
	Agents

	// Inject errors for specific operations
	StartErr     error
	StopErr      error
	WaitReadyErr error
	GetInfoErr   error
}

// NewAgentsStub creates a new stub wrapping the given Agents implementation.
func NewAgentsStub(wrapped Agents) *AgentsStub {
	return &AgentsStub{Agents: wrapped}
}

// StartWithConfig creates a new agent with config, or returns StartErr if set.
func (s *AgentsStub) StartWithConfig(id AgentID, cfg StartConfig) error {
	if s.StartErr != nil {
		return s.StartErr
	}
	return s.Agents.StartWithConfig(id, cfg)
}

// Stop terminates an agent, or returns StopErr if set.
func (s *AgentsStub) Stop(id AgentID, graceful bool) error {
	if s.StopErr != nil {
		return s.StopErr
	}
	return s.Agents.Stop(id, graceful)
}

// WaitReady blocks until ready, or returns WaitReadyErr if set.
func (s *AgentsStub) WaitReady(id AgentID) error {
	if s.WaitReadyErr != nil {
		return s.WaitReadyErr
	}
	return s.Agents.WaitReady(id)
}

// Delegate all other methods to the wrapped Agents

// Exists checks if an agent exists.
func (s *AgentsStub) Exists(id AgentID) bool {
	return s.Agents.Exists(id)
}

// GetInfo returns information about an agent's session, or returns GetInfoErr if set.
func (s *AgentsStub) GetInfo(id AgentID) (*session.Info, error) {
	if s.GetInfoErr != nil {
		return nil, s.GetInfoErr
	}
	return s.Agents.GetInfo(id)
}
