package witness_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Witness Manager Unit Tests
// Using agent.Double for testable abstraction
//
// Note: Start/Stop operations are handled by factory.Start()/factory.Agents().Stop()
// The Manager only handles status queries and state persistence.
// =============================================================================

func setupTestRig(t *testing.T) (*rig.Rig, string) {
	t.Helper()
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, "witness"), 0755))

	// Create minimal Claude settings
	claudeDir := filepath.Join(rigPath, "witness", ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{}`), 0644))

	return &rig.Rig{
		Name:     "testrig",
		Path:     rigPath,
		Polecats: []string{"p1", "p2"},
	}, rigPath
}

// --- Status() Tests ---

func TestManager_Status_WhenAgentRunning_ReportsRunning(t *testing.T) {
	r, rigPath := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	// Simulate running agent
	agentID := agent.WitnessAddress(r.Name)
	agents.CreateAgent(agentID)

	// Write state file that says running
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	state := witness.Witness{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.Marshal(state)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateRunning, status.State)
	assert.Equal(t, []string{"p1", "p2"}, status.MonitoredPolecats)
}

func TestManager_Status_WhenAgentCrashed_DetectsMismatch(t *testing.T) {
	// Scenario: State says running but agent doesn't exist (crashed).
	// Status() should detect mismatch and report stopped.
	r, rigPath := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	// Write state that says running (but don't create agent)
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	staleState := witness.Witness{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.Marshal(staleState)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	// Agent doesn't exist
	agentID := agent.WitnessAddress(r.Name)
	assert.False(t, agents.Exists(agentID), "agent should not exist")

	// Status() detects the mismatch and reports stopped
	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateStopped, status.State, "should detect crashed agent")
}

func TestManager_Status_WhenStateStopped_ReportsStopped(t *testing.T) {
	r, rigPath := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	// Write state that says stopped
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	state := witness.Witness{RigName: "testrig", State: agent.StateStopped}
	data, _ := json.Marshal(state)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateStopped, status.State)
}

// --- IsRunning() Tests ---

func TestManager_IsRunning_WhenAgentExists_ReturnsTrue(t *testing.T) {
	r, _ := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	agentID := agent.WitnessAddress(r.Name)
	agents.CreateAgent(agentID)

	assert.True(t, mgr.IsRunning())
}

func TestManager_IsRunning_WhenAgentNotExists_ReturnsFalse(t *testing.T) {
	r, _ := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	assert.False(t, mgr.IsRunning())
}

// --- SessionName() Tests ---

func TestManager_SessionName_Format(t *testing.T) {
	r, _ := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	assert.Equal(t, "gt-testrig-witness", mgr.SessionName())
}

// --- Address() Tests ---

func TestManager_Address_ReturnsCorrectAgentID(t *testing.T) {
	r, _ := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	expected := agent.WitnessAddress(r.Name)
	assert.Equal(t, expected, mgr.Address())
}

// --- LoadState/SaveState Tests ---

func TestManager_LoadState_ReturnsPersistedState(t *testing.T) {
	r, rigPath := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	// Write a state file
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	state := witness.Witness{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.MarshalIndent(state, "", "  ")
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	loaded, err := mgr.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}

func TestManager_SaveState_PersistsState(t *testing.T) {
	r, rigPath := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	state := &witness.Witness{RigName: "testrig", State: agent.StateRunning}
	err := mgr.SaveState(state)
	require.NoError(t, err)

	// Verify file was written
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var loaded witness.Witness
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}

func TestManager_LoadState_WhenNoFile_ReturnsDefaultState(t *testing.T) {
	r, _ := setupTestRig(t)
	agents := agent.NewDouble()
	mgr := witness.NewManager(agents, r)

	// Don't create state file - should return default
	state, err := mgr.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "testrig", state.RigName)
	assert.Equal(t, agent.StateStopped, state.State)
}
