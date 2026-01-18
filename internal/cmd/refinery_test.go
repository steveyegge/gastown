package cmd

import (
	"errors"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
)

// mockExistsChecker implements AgentExistsChecker for testing.
type mockExistsChecker struct {
	exists bool
}

func (m *mockExistsChecker) Exists(id agent.AgentID) bool {
	return m.exists
}

// --- Auto-Start Helper Tests ---

func TestEnsureAgentRunning_AlreadyRunning(t *testing.T) {
	checker := &mockExistsChecker{exists: true}
	starterCalled := false
	starter := func(id agent.AgentID) error {
		starterCalled = true
		return nil
	}

	result := ensureAgentRunning(agent.RefineryAddress("testrig"), checker, starter)

	if !result.AlreadyRunning {
		t.Error("expected AlreadyRunning=true")
	}
	if result.Started {
		t.Error("expected Started=false")
	}
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if starterCalled {
		t.Error("starter should not be called when already running")
	}
}

func TestEnsureAgentRunning_NotRunning_StartsSuccessfully(t *testing.T) {
	checker := &mockExistsChecker{exists: false}
	starterCalled := false
	starter := func(id agent.AgentID) error {
		starterCalled = true
		return nil
	}

	result := ensureAgentRunning(agent.RefineryAddress("testrig"), checker, starter)

	if result.AlreadyRunning {
		t.Error("expected AlreadyRunning=false")
	}
	if !result.Started {
		t.Error("expected Started=true")
	}
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if !starterCalled {
		t.Error("starter should be called when not running")
	}
}

func TestEnsureAgentRunning_NotRunning_StartFails(t *testing.T) {
	checker := &mockExistsChecker{exists: false}
	testErr := errors.New("start failed")
	starter := func(id agent.AgentID) error {
		return testErr
	}

	result := ensureAgentRunning(agent.RefineryAddress("testrig"), checker, starter)

	if result.AlreadyRunning {
		t.Error("expected AlreadyRunning=false")
	}
	if result.Started {
		t.Error("expected Started=false when start fails")
	}
	if result.Err == nil {
		t.Error("expected error, got nil")
	}
	if result.Err != testErr {
		t.Errorf("expected test error, got %v", result.Err)
	}
}

func TestEnsureAgentRunning_PassesCorrectAgentID(t *testing.T) {
	checker := &mockExistsChecker{exists: false}
	expectedID := agent.RefineryAddress("myrig")
	var receivedID agent.AgentID
	starter := func(id agent.AgentID) error {
		receivedID = id
		return nil
	}

	_ = ensureAgentRunning(expectedID, checker, starter)

	if receivedID != expectedID {
		t.Errorf("expected agent ID %q, got %q", expectedID, receivedID)
	}
}
