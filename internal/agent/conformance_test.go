package agent_test

import (
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAgent creates a test agent ID with the given name.
func testAgent(name string) agent.AgentID {
	return agent.PolecatAddress("testrig", name)
}

// =============================================================================
// Conformance Tests for Agents Interface
//
// These tests verify that both implementations behave identically:
// 1. agent.Double (test double)
// 2. agent.Implementation backed by session.Double
//
// Run these tests to ensure the test double is a faithful stand-in for the
// real implementation.
// =============================================================================

// agentsFactory creates an Agents implementation for testing.
type agentsFactory func() agent.Agents

// testCases returns the factories for both implementations.
func testCases() map[string]agentsFactory {
	return map[string]agentsFactory{
		"Double": func() agent.Agents {
			return agent.NewDouble()
		},
		"Implementation": func() agent.Agents {
			return agent.New(session.NewDouble(), nil)
		},
	}
}

// --- Start/Exists Conformance ---

func TestConformance_Start_CreatesAgent(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			require.NoError(t, err)
			assert.True(t, agents.Exists(id))
		})
	}
}

func TestConformance_Start_AlreadyRunning_ReturnsError(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			// Start first agent
			id := testAgent("test-agent")
			err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			require.NoError(t, err)

			// Try to start again with same name
			err = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			assert.ErrorIs(t, err, agent.ErrAlreadyRunning)

			// Original agent should still exist
			assert.True(t, agents.Exists(id))
		})
	}
}

func TestConformance_Exists_ReturnsFalse_WhenNoAgent(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			assert.False(t, agents.Exists(testAgent("nonexistent")))
		})
	}
}

// --- Stop Conformance ---

func TestConformance_Stop_TerminatesAgent(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			err := agents.Stop(id, false)
			require.NoError(t, err)

			assert.False(t, agents.Exists(id))
		})
	}
}

func TestConformance_Stop_Idempotent(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			// Stop non-existent agent should not error
			err := agents.Stop(testAgent("nonexistent"), false)
			assert.NoError(t, err)
		})
	}
}

func TestConformance_Stop_Graceful(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			err := agents.Stop(id, true) // graceful=true
			require.NoError(t, err)

			assert.False(t, agents.Exists(id))
		})
	}
}

// --- SessionID Conformance ---

func TestConformance_GetInfo_ReturnsName(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			info, err := agents.GetInfo(id)

			require.NoError(t, err)
			// The info.Name reflects the session name which is derived from the AgentID
			assert.NotEmpty(t, info.Name)
		})
	}
}

// --- WaitReady Conformance ---

func TestConformance_WaitReady_NotRunning_ReturnsError(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			err := agents.WaitReady(testAgent("nonexistent"))
			assert.ErrorIs(t, err, agent.ErrNotRunning)
		})
	}
}

func TestConformance_WaitReady_WhenRunning_ReturnsNil(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			err := agents.WaitReady(id)
			assert.NoError(t, err)
		})
	}
}

// --- GetInfo Conformance ---

func TestConformance_GetInfo_ReturnsInfo(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			id := testAgent("test-agent")
			_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			info, err := agents.GetInfo(id)

			require.NoError(t, err)
			assert.NotNil(t, info)
			// Name is derived from AgentID.String(), which includes the full path
			assert.NotEmpty(t, info.Name)
		})
	}
}

func TestConformance_GetInfo_NotRunning_ReturnsError(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			_, err := agents.GetInfo(testAgent("nonexistent"))
			assert.Error(t, err)
		})
	}
}

// --- Full Lifecycle Conformance ---

func TestConformance_FullLifecycle(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			// Start
			id := testAgent("lifecycle-test")
			err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
			require.NoError(t, err)
			assert.True(t, agents.Exists(id))

			// Get info
			info, err := agents.GetInfo(id)
			require.NoError(t, err)
			assert.NotEmpty(t, info.Name)

			// Stop
			err = agents.Stop(id, true)
			require.NoError(t, err)
			assert.False(t, agents.Exists(id))

			// Start again (should work after stop)
			err = agents.StartWithConfig(id, startCfg("/tmp", "echo hello again"))
			require.NoError(t, err)
			assert.True(t, agents.Exists(id))
		})
	}
}

// --- Multiple Agents Conformance ---

func TestConformance_MultipleAgents(t *testing.T) {
	for name, factory := range testCases() {
		t.Run(name, func(t *testing.T) {
			agents := factory()

			// Start multiple agents
			id1 := testAgent("agent-1")
			id2 := testAgent("agent-2")
			id3 := testAgent("agent-3")
			err := agents.StartWithConfig(id1, startCfg("/tmp", "echo 1"))
			require.NoError(t, err)
			err = agents.StartWithConfig(id2, startCfg("/tmp", "echo 2"))
			require.NoError(t, err)
			err = agents.StartWithConfig(id3, startCfg("/tmp", "echo 3"))
			require.NoError(t, err)

			// All should exist
			assert.True(t, agents.Exists(id1))
			assert.True(t, agents.Exists(id2))
			assert.True(t, agents.Exists(id3))

			// Stop one
			_ = agents.Stop(id2, false)

			// Only stopped one should be gone
			assert.True(t, agents.Exists(id1))
			assert.False(t, agents.Exists(id2))
			assert.True(t, agents.Exists(id3))
		})
	}
}
