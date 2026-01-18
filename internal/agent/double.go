package agent

import (
	"sync"

	"github.com/steveyegge/gastown/internal/session"
)

// Note: session import used for GetInfo return type

// Double is a FAKE with SPY capabilities for the Agents interface.
//
// Test Double Taxonomy (Meszaros/Fowler):
//   - FAKE: Working in-memory implementation (no real tmux sessions)
//   - SPY: Records method calls for verification (StopCalls, NudgeLog, GetStartConfig)
//
// Use this for testing manager logic and verifying interactions.
// For error injection, wrap with AgentsStub.
type Double struct {
	mu        sync.RWMutex
	agents    map[AgentID]*doubleAgent
	stopCalls []StopCall
}

type doubleAgent struct {
	name        string
	workDir     string
	command     string
	envVars     map[string]string // Original env vars (before prepending to command)
	startConfig StartConfig       // Full config for spy verification
	nudgeLog    []string
}

// StopCall records a call to Stop() for test verification.
type StopCall struct {
	ID       AgentID
	Graceful bool
}

// NewDouble creates a new in-memory Agents test double.
func NewDouble() *Double {
	return &Double{
		agents: make(map[AgentID]*doubleAgent),
	}
}

// Ensure Double implements Agents
var _ Agents = (*Double)(nil)

// StartWithConfig creates a new agent with explicit configuration.
func (d *Double) StartWithConfig(id AgentID, cfg StartConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.agents[id]; exists {
		return ErrAlreadyRunning
	}

	// Prepend env vars to command (same as real implementation)
	command := cfg.Command
	if len(cfg.EnvVars) > 0 {
		command = prependEnvVars(cfg.EnvVars, command)
	}

	// Copy env vars to prevent mutation
	envVars := make(map[string]string, len(cfg.EnvVars))
	for k, v := range cfg.EnvVars {
		envVars[k] = v
	}

	d.agents[id] = &doubleAgent{
		name:        id.String(),
		workDir:     cfg.WorkDir,
		command:     command,
		envVars:     envVars,
		startConfig: cfg, // Store full config for spy verification
	}

	// Run the callback if provided (for test verification)
	if cfg.OnCreated != nil {
		sessionID := session.SessionID(session.SessionNameFromAgentID(id))
		_ = cfg.OnCreated(sessionID)
	}

	return nil
}

// Stop removes an agent and records the call for test verification.
func (d *Double) Stop(id AgentID, graceful bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopCalls = append(d.stopCalls, StopCall{ID: id, Graceful: graceful})
	delete(d.agents, id)
	return nil
}

// Exists checks if an agent exists.
func (d *Double) Exists(id AgentID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.agents[id]
	return exists
}

// Respawn simulates restarting an agent (clears state, keeps command).
func (d *Double) Respawn(id AgentID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	agent, exists := d.agents[id]
	if !exists {
		return ErrNotRunning
	}

	// Clear nudge log (simulates fresh start), keep same command
	agent.nudgeLog = nil
	return nil
}

// WaitReady returns nil if the agent exists, ErrNotRunning otherwise.
func (d *Double) WaitReady(id AgentID) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.agents[id]; !exists {
		return ErrNotRunning
	}
	return nil
}

// GetInfo returns information about an agent's session.
func (d *Double) GetInfo(id AgentID) (*session.Info, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agent, exists := d.agents[id]
	if !exists {
		return nil, ErrNotRunning
	}

	return &session.Info{
		Name:    agent.name,
		Created: "2024-01-01T00:00:00Z",
		Windows: 1,
	}, nil
}

// Nudge sends a message to a running agent.
func (d *Double) Nudge(id AgentID, message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	agent, exists := d.agents[id]
	if !exists {
		return ErrNotRunning
	}

	agent.nudgeLog = append(agent.nudgeLog, message)
	return nil
}

// Capture returns mock output for a running agent.
func (d *Double) Capture(id AgentID, lines int) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.agents[id]; !exists {
		return "", ErrNotRunning
	}

	return "", nil // Return empty output for tests
}

// CaptureAll returns mock output for a running agent (all scrollback).
func (d *Double) CaptureAll(id AgentID) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.agents[id]; !exists {
		return "", ErrNotRunning
	}

	return "", nil // Return empty output for tests
}

func (d *Double) List() ([]AgentID, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids := make([]AgentID, 0, len(d.agents))
	for id := range d.agents {
		ids = append(ids, id)
	}
	return ids, nil
}

// Attach is a no-op in the test double.
func (d *Double) Attach(id AgentID) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.agents[id]
	if !exists {
		return ErrNotRunning
	}
	return nil
}

// --- Test helpers (for verification, not error injection) ---

// Clear removes all agents (for test cleanup).
func (d *Double) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents = make(map[AgentID]*doubleAgent)
}

// AgentCount returns the number of agents (for test verification).
func (d *Double) AgentCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.agents)
}

// CreateAgent adds an agent without going through Start (for test setup).
func (d *Double) CreateAgent(id AgentID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents[id] = &doubleAgent{name: id.String()}
}

// GetWorkDir returns the working directory passed to Start for an agent.
// Returns empty string if agent doesn't exist.
func (d *Double) GetWorkDir(id AgentID) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		return agent.workDir
	}
	return ""
}

// GetCommand returns the command passed to Start for an agent.
// Returns empty string if agent doesn't exist.
func (d *Double) GetCommand(id AgentID) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		return agent.command
	}
	return ""
}

// NudgeLog returns the nudge messages sent to an agent (for test verification).
func (d *Double) NudgeLog(id AgentID) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		// Return a copy to prevent mutation
		result := make([]string, len(agent.nudgeLog))
		copy(result, agent.nudgeLog)
		return result
	}
	return nil
}

// StopCalls returns all Stop() calls made (for test verification).
func (d *Double) StopCalls() []StopCall {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Return a copy to prevent mutation
	result := make([]StopCall, len(d.stopCalls))
	copy(result, d.stopCalls)
	return result
}

// GetStartConfig returns the StartConfig used for an agent (spy verification).
// Returns the config and true if found, or empty config and false if not found.
func (d *Double) GetStartConfig(id AgentID) (StartConfig, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		return agent.startConfig, true
	}
	return StartConfig{}, false
}

// GetEnvVars returns the environment variables passed to Start for an agent.
// Returns nil if agent doesn't exist.
func (d *Double) GetEnvVars(id AgentID) map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		// Return a copy to prevent mutation
		result := make(map[string]string, len(agent.envVars))
		for k, v := range agent.envVars {
			result[k] = v
		}
		return result
	}
	return nil
}

// HasOnCreated returns true if the agent was started with an OnCreated callback.
// This is useful for verifying that theming would be applied in production.
func (d *Double) HasOnCreated(id AgentID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if agent, exists := d.agents[id]; exists {
		return agent.startConfig.OnCreated != nil
	}
	return false
}
