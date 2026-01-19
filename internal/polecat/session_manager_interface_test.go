package polecat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
)

// =============================================================================
// Polecat SessionManager Interface Tests
// Using agent.Double for testable abstraction
//
// Note: Start operations are handled by factory.Start().
// These tests verify the SessionManager's status, enumeration, and operations.
// =============================================================================

// testPolecat returns a test polecat AgentID for the given name.
func testPolecat(name string) agent.AgentID {
	return agent.PolecatAddress("testrig", name)
}

func TestStop_TerminatesSession(t *testing.T) {
	root := t.TempDir()

	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewSessionManager(agents, r, "")

	// Stop the agent
	err := m.Stop("Toast", false)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify agent was stopped
	if agents.Exists(testPolecat("Toast")) {
		t.Error("agent should be stopped")
	}
}

func TestStop_ForceSkipsGracefulShutdown(t *testing.T) {
	root := t.TempDir()

	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewSessionManager(agents, r, "")

	// Force stop
	err := m.Stop("Toast", true)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify agent was stopped
	if agents.Exists(testPolecat("Toast")) {
		t.Error("agent should be stopped")
	}
}

func TestIsRunning_ReturnsTrue(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning error: %v", err)
	}
	if !running {
		t.Error("expected IsRunning = true")
	}
}

func TestIsRunning_ReturnsFalse(t *testing.T) {
	agents := agent.NewDouble()
	// Don't create the agent

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning error: %v", err)
	}
	if running {
		t.Error("expected IsRunning = false")
	}
}

func TestCapture_WhenAgentExists_Succeeds(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	// Capture should succeed (returns empty from Double)
	_, err := m.Capture("Toast", 50)
	if err != nil {
		t.Fatalf("Capture error: %v", err)
	}
}

func TestCapture_WhenAgentNotExists_ReturnsError(t *testing.T) {
	agents := agent.NewDouble()
	// Don't create the agent

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	_, err := m.Capture("Toast", 50)
	if err != ErrSessionNotFound {
		t.Errorf("Capture = %v, want ErrSessionNotFound", err)
	}
}

func TestInject_WhenAgentExists_Succeeds(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	message := "Hello, polecat!"
	err := m.Inject("Toast", message)
	if err != nil {
		t.Fatalf("Inject error: %v", err)
	}

	// Verify message was nudged
	nudges := agents.NudgeLog(testPolecat("Toast"))
	if len(nudges) != 1 || nudges[0] != message {
		t.Errorf("expected nudge log [%q], got %v", message, nudges)
	}
}

func TestInject_WhenAgentNotExists_ReturnsError(t *testing.T) {
	agents := agent.NewDouble()
	// Don't create the agent

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	err := m.Inject("Toast", "hello")
	if err != ErrSessionNotFound {
		t.Errorf("Inject = %v, want ErrSessionNotFound", err)
	}
}

func TestList_FiltersByRigPrefix(t *testing.T) {
	agents := agent.NewDouble()
	// Add agents for different rigs using proper AgentID format
	agents.CreateAgent(agent.PolecatAddress("testrig", "Toast"))
	agents.CreateAgent(agent.PolecatAddress("testrig", "Furiosa"))
	agents.CreateAgent(agent.PolecatAddress("otherrig", "Max")) // Different rig
	agents.CreateAgent(agent.MayorAddress)                      // Not a polecat

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	// Should only return testrig polecats
	if len(infos) != 2 {
		t.Fatalf("expected 2 sessions for testrig, got %d", len(infos))
	}

	names := make(map[string]bool)
	for _, info := range infos {
		names[info.Polecat] = true
	}

	if !names["Toast"] {
		t.Error("expected Toast in list")
	}
	if !names["Furiosa"] {
		t.Error("expected Furiosa in list")
	}
	if names["Max"] {
		t.Error("Max should not be in list (different rig)")
	}
}

func TestStatus_WhenAgentExists_ReturnsRunning(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	info, err := m.Status("Toast")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	if !info.Running {
		t.Error("expected Running = true")
	}
	if info.Polecat != "Toast" {
		t.Errorf("Polecat = %q, want Toast", info.Polecat)
	}
	if info.SessionID != "gt-testrig-Toast" {
		t.Errorf("SessionID = %q, want gt-testrig-Toast", info.SessionID)
	}
	if info.RigName != "testrig" {
		t.Errorf("RigName = %q, want testrig", info.RigName)
	}
}

func TestStatus_WhenAgentNotExists_ReturnsNotRunning(t *testing.T) {
	agents := agent.NewDouble()
	// Don't create the agent

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	info, err := m.Status("Toast")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	if info.Running {
		t.Error("expected Running = false")
	}
}

func TestStopAll_StopsAllSessions(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))
	agents.CreateAgent(testPolecat("Furiosa"))
	agents.CreateAgent(testPolecat("Max"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	err := m.StopAll(false)
	if err != nil {
		t.Fatalf("StopAll error: %v", err)
	}

	// Verify all agents were stopped
	if agents.AgentCount() != 0 {
		t.Errorf("expected 0 agents after StopAll, got %d", agents.AgentCount())
	}
}

func TestStopAll_EmptyList_Succeeds(t *testing.T) {
	agents := agent.NewDouble()
	// No agents added

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	err := m.StopAll(false)
	if err != nil {
		t.Errorf("StopAll on empty list should succeed, got: %v", err)
	}
}

func TestStopAll_Force_SkipsGracefulShutdown(t *testing.T) {
	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	err := m.StopAll(true) // force=true
	if err != nil {
		t.Fatalf("StopAll error: %v", err)
	}

	// Verify agent was stopped
	if agents.AgentCount() != 0 {
		t.Error("expected all agents to be stopped")
	}
}

func TestAttach_WhenAgentExists_Succeeds(t *testing.T) {
	root := t.TempDir()
	// Create polecat directory
	polecatDir := filepath.Join(root, "polecats", "Toast")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}

	agents := agent.NewDouble()
	agents.CreateAgent(testPolecat("Toast"))

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewSessionManager(agents, r, "")

	// Attach should succeed (agent.Double returns nil for Attach)
	err := m.Attach("Toast")
	if err != nil {
		t.Fatalf("Attach error: %v", err)
	}
}

func TestAttach_WhenAgentNotExists_ReturnsError(t *testing.T) {
	agents := agent.NewDouble()
	// Don't create the agent

	r := &rig.Rig{Name: "testrig", Path: "/tmp"}
	m := NewSessionManager(agents, r, "")

	err := m.Attach("Toast")
	if err != ErrSessionNotFound {
		t.Errorf("Attach = %v, want ErrSessionNotFound", err)
	}
}
