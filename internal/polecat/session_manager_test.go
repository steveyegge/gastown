package polecat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
)

// =============================================================================
// Polecat SessionManager Unit Tests
// Using agent.Double for testable abstraction
//
// Note: Start operations are handled by factory.Start().
// The SessionManager handles status queries, enumeration, and session operations.
// =============================================================================

func TestSessionName(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	name := m.SessionName("Toast")
	if name != "gt-gastown-Toast" {
		t.Errorf("sessionName = %q, want gt-gastown-Toast", name)
	}
}

func TestSessionManagerPolecatDir(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Path:     "/home/user/ai/gastown",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	dir := m.polecatDir("Toast")
	expected := "/home/user/ai/gastown/polecats/Toast"
	if dir != expected {
		t.Errorf("polecatDir = %q, want %q", dir, expected)
	}
}

func TestHasPolecat(t *testing.T) {
	root := t.TempDir()
	// hasPolecat checks filesystem, so create actual directories
	for _, name := range []string{"Toast", "Cheedo"} {
		if err := os.MkdirAll(filepath.Join(root, "polecats", name), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{"Toast", "Cheedo"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	if !m.hasPolecat("Toast") {
		t.Error("expected hasPolecat(Toast) = true")
	}
	if !m.hasPolecat("Cheedo") {
		t.Error("expected hasPolecat(Cheedo) = true")
	}
	if m.hasPolecat("Unknown") {
		t.Error("expected hasPolecat(Unknown) = false")
	}
}

func TestIsRunningNoSession(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Error("expected IsRunning = false for non-existent session")
	}
}

func TestIsRunning_WhenAgentExists_ReturnsTrue(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create agent
	agentID := agent.PolecatAddress("gastown", "Toast")
	agents.CreateAgent(agentID)

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !running {
		t.Error("expected IsRunning = true when agent exists")
	}
}

func TestSessionManagerListEmpty(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig-unlikely-name",
		Polecats: []string{},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("infos count = %d, want 0", len(infos))
	}
}

func TestSessionManagerList_WithRunningAgents(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast", "Cheedo"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create some running agents
	agents.CreateAgent(agent.PolecatAddress("testrig", "Toast"))
	agents.CreateAgent(agent.PolecatAddress("testrig", "Cheedo"))

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("infos count = %d, want 2", len(infos))
	}
}

func TestStopNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	err := m.Stop("Toast", false)
	if err != ErrSessionNotFound {
		t.Errorf("Stop = %v, want ErrSessionNotFound", err)
	}
}

func TestStop_WhenAgentExists_StopsIt(t *testing.T) {
	root := t.TempDir()
	// Create polecat directory
	if err := os.MkdirAll(filepath.Join(root, "polecats", "Toast"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	r := &rig.Rig{
		Name:     "testrig",
		Path:     root,
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create running agent
	agentID := agent.PolecatAddress("testrig", "Toast")
	agents.CreateAgent(agentID)

	// Stop should succeed
	err := m.Stop("Toast", false)
	if err != nil {
		t.Errorf("Stop = %v, want nil", err)
	}

	// Agent should be gone
	if agents.Exists(agentID) {
		t.Error("agent should not exist after Stop")
	}
}

func TestCaptureNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	_, err := m.Capture("Toast", 50)
	if err != ErrSessionNotFound {
		t.Errorf("Capture = %v, want ErrSessionNotFound", err)
	}
}

func TestInjectNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	err := m.Inject("Toast", "hello")
	if err != ErrSessionNotFound {
		t.Errorf("Inject = %v, want ErrSessionNotFound", err)
	}
}

func TestAttachNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	err := m.Attach("Toast")
	if err != ErrSessionNotFound {
		t.Errorf("Attach = %v, want ErrSessionNotFound", err)
	}
}

func TestRigName(t *testing.T) {
	r := &rig.Rig{
		Name:     "myrig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	if m.RigName() != "myrig" {
		t.Errorf("RigName = %q, want myrig", m.RigName())
	}
}

func TestStatus(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Status when not running
	info, err := m.Status("Toast")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if info.Running {
		t.Error("expected Running = false")
	}
	if info.Polecat != "Toast" {
		t.Errorf("Polecat = %q, want Toast", info.Polecat)
	}
	if info.RigName != "testrig" {
		t.Errorf("RigName = %q, want testrig", info.RigName)
	}
}

func TestStatus_WhenRunning(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create running agent
	agentID := agent.PolecatAddress("testrig", "Toast")
	agents.CreateAgent(agentID)

	info, err := m.Status("Toast")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !info.Running {
		t.Error("expected Running = true")
	}
}

func TestClonePath_NewStructure(t *testing.T) {
	root := t.TempDir()
	rigName := "testrig"

	// Create new structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(root, "polecats", "Toast", rigName)
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	r := &rig.Rig{
		Name:     rigName,
		Path:     root,
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	path := m.clonePath("Toast")
	if path != newPath {
		t.Errorf("clonePath = %q, want %q", path, newPath)
	}
}

func TestClonePath_OldStructure(t *testing.T) {
	root := t.TempDir()

	// Create old structure: polecats/<name>/ with .git file
	oldPath := filepath.Join(root, "polecats", "Toast")
	if err := os.MkdirAll(oldPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create .git file to mark it as a git worktree
	if err := os.WriteFile(filepath.Join(oldPath, ".git"), []byte("gitdir: /some/path"), 0644); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	r := &rig.Rig{
		Name:     "testrig",
		Path:     root,
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	path := m.clonePath("Toast")
	if path != oldPath {
		t.Errorf("clonePath = %q, want %q (old structure)", path, oldPath)
	}
}

func TestStopAll(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast", "Cheedo"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create running agents
	agents.CreateAgent(agent.PolecatAddress("testrig", "Toast"))
	agents.CreateAgent(agent.PolecatAddress("testrig", "Cheedo"))

	// Stop all
	err := m.StopAll(false)
	if err != nil {
		t.Errorf("StopAll: %v", err)
	}

	// All agents should be stopped
	if agents.Exists(agent.PolecatAddress("testrig", "Toast")) {
		t.Error("Toast should be stopped")
	}
	if agents.Exists(agent.PolecatAddress("testrig", "Cheedo")) {
		t.Error("Cheedo should be stopped")
	}
}

func TestCaptureSession_Deprecated(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// Create running agent
	agentID := agent.PolecatAddress("testrig", "Toast")
	agents.CreateAgent(agentID)

	// CaptureSession with raw session name
	_, err := m.CaptureSession("gt-testrig-Toast", 50)
	if err != nil {
		t.Errorf("CaptureSession: %v", err)
	}
}

func TestCaptureSession_WrongPrefix(t *testing.T) {
	r := &rig.Rig{
		Name:     "testrig",
		Polecats: []string{"Toast"},
	}
	agents := agent.NewDouble()
	m := NewSessionManager(agents, r, "")

	// CaptureSession with wrong prefix
	_, err := m.CaptureSession("gt-wrongrig-Toast", 50)
	if err != ErrSessionNotFound {
		t.Errorf("CaptureSession = %v, want ErrSessionNotFound", err)
	}
}
