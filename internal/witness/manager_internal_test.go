package witness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManagerInternal(t *testing.T) (*Manager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, "witness"), 0755))

	// Create minimal Claude settings
	claudeDir := filepath.Join(rigPath, "witness", ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{}`), 0644))

	r := &rig.Rig{
		Name:     "testrig",
		Path:     rigPath,
		Polecats: []string{"p1", "p2"},
	}

	agents := agent.NewDouble()
	return NewManager(agents, r), rigPath
}

// =============================================================================
// witnessDir Tests
// =============================================================================

func TestManager_witnessDir_PrefersWitnessRig(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	// Create witness/rig directory
	witnessRigDir := filepath.Join(rigPath, "witness", "rig")
	require.NoError(t, os.MkdirAll(witnessRigDir, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	assert.Equal(t, witnessRigDir, mgr.witnessDir())
}

func TestManager_witnessDir_FallsBackToWitness(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	// Create only witness directory (not witness/rig)
	witnessDir := filepath.Join(rigPath, "witness")
	require.NoError(t, os.MkdirAll(witnessDir, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	assert.Equal(t, witnessDir, mgr.witnessDir())
}

func TestManager_witnessDir_FallsBackToRigPath(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	// Create only the rig directory (no witness subdirs)
	require.NoError(t, os.MkdirAll(rigPath, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	assert.Equal(t, rigPath, mgr.witnessDir())
}

// =============================================================================
// Status Error Path Tests
// =============================================================================

func TestManager_Status_WhenLoadStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))

	// Write invalid JSON
	stateFile := filepath.Join(rigPath, ".runtime", "witness.json")
	require.NoError(t, os.WriteFile(stateFile, []byte("invalid json"), 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	_, err := mgr.Status()
	assert.Error(t, err)
}

// =============================================================================
// LoadState/SaveState Tests
// =============================================================================

func TestManager_LoadState_ReturnsPersistedState(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	runtimeDir := filepath.Join(rigPath, ".runtime")
	require.NoError(t, os.MkdirAll(runtimeDir, 0755))

	// Write a state file
	stateFile := filepath.Join(runtimeDir, "witness.json")
	state := Witness{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.MarshalIndent(state, "", "  ")
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	loaded, err := mgr.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}

func TestManager_SaveState_PersistsState(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	runtimeDir := filepath.Join(rigPath, ".runtime")
	require.NoError(t, os.MkdirAll(runtimeDir, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	state := &Witness{RigName: "testrig", State: agent.StateRunning}
	err := mgr.SaveState(state)
	require.NoError(t, err)

	// Verify file was written
	stateFile := filepath.Join(runtimeDir, "witness.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var loaded Witness
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}
