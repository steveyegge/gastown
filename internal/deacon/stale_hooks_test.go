package deacon

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/ids"
)

// mockAgentChecker is a test implementation of AgentChecker.
type mockAgentChecker struct {
	agents map[string]bool
}

func (m *mockAgentChecker) Exists(id agent.AgentID) bool {
	return m.agents[id.String()]
}

func TestStaleHookConfig_AgentChecker(t *testing.T) {
	// Verify that AgentChecker can be injected into the config
	// Note: agents are now keyed by their AgentID (e.g., "gastown/witness", "gastown/polecats/max")
	mock := &mockAgentChecker{
		agents: map[string]bool{
			"gastown/witness":      true,
			"gastown/polecats/max": false,
		},
	}

	cfg := &StaleHookConfig{
		MaxAge:       1 * time.Hour,
		DryRun:       true,
		AgentChecker: mock,
	}

	// Verify the mock works as expected
	if !cfg.AgentChecker.Exists(ids.ParseAddress("gastown/witness")) {
		t.Error("expected witness agent to be alive")
	}

	if cfg.AgentChecker.Exists(ids.ParseAddress("gastown/polecats/max")) {
		t.Error("expected max agent to be dead")
	}

	if cfg.AgentChecker.Exists(ids.ParseAddress("nonexistent")) {
		t.Error("expected nonexistent agent to return false")
	}
}

func TestDefaultStaleHookConfig(t *testing.T) {
	cfg := DefaultStaleHookConfig()

	if cfg.MaxAge != 1*time.Hour {
		t.Errorf("MaxAge = %v, want 1h", cfg.MaxAge)
	}

	if cfg.DryRun {
		t.Error("DryRun should be false by default")
	}

	if cfg.AgentChecker != nil {
		t.Error("AgentChecker should be nil by default")
	}
}
