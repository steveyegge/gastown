package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// StateManager Tests
// =============================================================================

// TestState is a simple struct for testing StateManager generics
type TestState struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	State agent.State `json:"state"`
}

func defaultTestState() *TestState {
	return &TestState{
		Name:  "default",
		Count: 0,
		State: agent.StateStopped,
	}
}

func TestStateManager_NewStateManager_CreatesManager(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "test-state.json", defaultTestState)

	assert.NotNil(t, mgr)
}

func TestStateManager_StateFile_ReturnsCorrectPath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "test-state.json", defaultTestState)

	expected := filepath.Join(tmpDir, ".runtime", "test-state.json")
	assert.Equal(t, expected, mgr.StateFile())
}

func TestStateManager_Load_WhenFileNotExists_ReturnsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "nonexistent.json", defaultTestState)

	state, err := mgr.Load()

	require.NoError(t, err)
	assert.Equal(t, "default", state.Name)
	assert.Equal(t, 0, state.Count)
	assert.Equal(t, agent.StateStopped, state.State)
}

func TestStateManager_Save_CreatesDirectoryAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "test-state.json", defaultTestState)

	state := &TestState{
		Name:  "test",
		Count: 42,
		State: agent.StateRunning,
	}

	err := mgr.Save(state)

	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(mgr.StateFile())
	assert.NoError(t, err)
}

func TestStateManager_SaveAndLoad_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "test-state.json", defaultTestState)

	original := &TestState{
		Name:  "myagent",
		Count: 123,
		State: agent.StateRunning,
	}

	err := mgr.Save(original)
	require.NoError(t, err)

	loaded, err := mgr.Load()
	require.NoError(t, err)

	assert.Equal(t, original.Name, loaded.Name)
	assert.Equal(t, original.Count, loaded.Count)
	assert.Equal(t, original.State, loaded.State)
}

func TestStateManager_Load_WhenFileCorrupted_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewStateManager(tmpDir, "corrupted.json", defaultTestState)

	// Create corrupted file
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	err := os.MkdirAll(runtimeDir, 0755)
	require.NoError(t, err)

	corruptedPath := filepath.Join(runtimeDir, "corrupted.json")
	err = os.WriteFile(corruptedPath, []byte("not valid json{{{"), 0644)
	require.NoError(t, err)

	_, err = mgr.Load()
	assert.Error(t, err)
}

func TestStateManager_Load_WhenReadError_ReturnsError(t *testing.T) {
	// Use a path that will cause a read error (directory as file)
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	err := os.MkdirAll(runtimeDir, 0755)
	require.NoError(t, err)

	// Create a directory where the file should be
	dirAsFile := filepath.Join(runtimeDir, "isadirectory.json")
	err = os.MkdirAll(dirAsFile, 0755)
	require.NoError(t, err)

	mgr := agent.NewStateManager(tmpDir, "isadirectory.json", defaultTestState)

	_, err = mgr.Load()
	assert.Error(t, err)
}

func TestStateManager_Save_WhenDirectoryCreationFails_ReturnsError(t *testing.T) {
	// Use a path where we can't create directories
	// This is platform-specific and tricky to test reliably
	// Skip if we can't set up the conditions

	if os.Getuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}

	tmpDir := t.TempDir()

	// Create a file where .runtime directory should go
	runtimePath := filepath.Join(tmpDir, ".runtime")
	err := os.WriteFile(runtimePath, []byte("file"), 0644)
	require.NoError(t, err)

	mgr := agent.NewStateManager(tmpDir, "test.json", defaultTestState)

	err = mgr.Save(&TestState{})
	assert.Error(t, err)
}

// --- State Constants Tests ---

func TestState_Constants(t *testing.T) {
	// Verify state constants have expected values
	assert.Equal(t, agent.State("stopped"), agent.StateStopped)
	assert.Equal(t, agent.State("running"), agent.StateRunning)
	assert.Equal(t, agent.State("paused"), agent.StatePaused)
}
