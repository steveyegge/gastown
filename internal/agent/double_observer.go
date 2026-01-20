package agent

import (
	"sync"

	"github.com/steveyegge/gastown/internal/session"
)

// ObserverDouble is a minimal test double for AgentObserver.
// FAKE: working in-memory implementation for observation-only tests.
//
// Use this instead of the full Double when testing code that only needs
// to check agent existence and status (e.g., witness.Manager, refinery.Manager).
//
// For full lifecycle testing, use agent.Double instead.
type ObserverDouble struct {
	mu     sync.RWMutex
	agents map[AgentID]*observerAgent
}

type observerAgent struct {
	info *session.Info
}

// NewObserverDouble creates a new minimal Agents test double for observation.
func NewObserverDouble() *ObserverDouble {
	return &ObserverDouble{
		agents: make(map[AgentID]*observerAgent),
	}
}

// Ensure ObserverDouble implements AgentObserver
var _ AgentObserver = (*ObserverDouble)(nil)

// Exists checks if an agent exists in the double.
func (d *ObserverDouble) Exists(id AgentID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.agents[id]
	return exists
}

// GetInfo returns information about an agent.
func (d *ObserverDouble) GetInfo(id AgentID) (*session.Info, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agent, exists := d.agents[id]
	if !exists {
		return nil, ErrNotRunning
	}

	if agent.info != nil {
		return agent.info, nil
	}

	// Default info
	return &session.Info{
		Name:    id.String(),
		Created: "2024-01-01T00:00:00Z",
		Windows: 1,
	}, nil
}

// List returns all agent IDs.
func (d *ObserverDouble) List() ([]AgentID, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids := make([]AgentID, 0, len(d.agents))
	for id := range d.agents {
		ids = append(ids, id)
	}
	return ids, nil
}

// =============================================================================
// Test Helpers
// =============================================================================

// SetExists adds or removes an agent from the double.
// This is the primary way to set up test scenarios.
func (d *ObserverDouble) SetExists(id AgentID, exists bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if exists {
		if _, ok := d.agents[id]; !ok {
			d.agents[id] = &observerAgent{}
		}
	} else {
		delete(d.agents, id)
	}
}

// SetInfo sets custom session info for an agent.
// The agent must already exist (call SetExists first).
func (d *ObserverDouble) SetInfo(id AgentID, info *session.Info) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if agent, exists := d.agents[id]; exists {
		agent.info = info
	}
}

// Clear removes all agents.
func (d *ObserverDouble) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents = make(map[AgentID]*observerAgent)
}

// AgentCount returns the number of agents.
func (d *ObserverDouble) AgentCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.agents)
}
