package factory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// WorkDirForID Tests
// =============================================================================

func TestWorkDirForID_TownLevelAgents(t *testing.T) {
	townRoot := "/town"

	tests := []struct {
		name     string
		id       agent.AgentID
		expected string
	}{
		{"mayor", agent.MayorAddress, "/town/mayor"},
		{"deacon", agent.DeaconAddress, "/town/deacon"},
		{"boot", agent.BootAddress, "/town/deacon/dogs/boot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := WorkDirForID(townRoot, tt.id)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, dir)
		})
	}
}

func TestWorkDirForID_Witness_PrefersWitnessRigDir(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create witness/rig directory (preferred)
	witnessRigDir := filepath.Join(rigPath, "witness", "rig")
	require.NoError(t, os.MkdirAll(witnessRigDir, 0755))

	id := agent.WitnessAddress("testrig")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, witnessRigDir, dir)
}

func TestWorkDirForID_Witness_FallsBackToWitnessDir(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create witness directory (fallback), but NOT witness/rig
	witnessDir := filepath.Join(rigPath, "witness")
	require.NoError(t, os.MkdirAll(witnessDir, 0755))

	id := agent.WitnessAddress("testrig")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, witnessDir, dir)
}

func TestWorkDirForID_Witness_FallsBackToRigPath(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create only the rig directory (final fallback)
	require.NoError(t, os.MkdirAll(rigPath, 0755))

	id := agent.WitnessAddress("testrig")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, rigPath, dir)
}

func TestWorkDirForID_Refinery_PrefersRefineryRigDir(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create refinery/rig directory (preferred)
	refineryRigDir := filepath.Join(rigPath, "refinery", "rig")
	require.NoError(t, os.MkdirAll(refineryRigDir, 0755))

	id := agent.RefineryAddress("testrig")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, refineryRigDir, dir)
}

func TestWorkDirForID_Refinery_FallsBackToMayorRig(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Only create the rig path (no refinery/rig), fallback is mayor/rig
	require.NoError(t, os.MkdirAll(rigPath, 0755))

	id := agent.RefineryAddress("testrig")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	// Falls back to mayor/rig (legacy architecture)
	assert.Equal(t, filepath.Join(rigPath, "mayor", "rig"), dir)
}

func TestWorkDirForID_Polecat_PrefersNewStructure(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create new structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(rigPath, "polecats", "Toast", "testrig")
	require.NoError(t, os.MkdirAll(newPath, 0755))

	id := agent.PolecatAddress("testrig", "Toast")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, newPath, dir)
}

func TestWorkDirForID_Polecat_FallsBackToOldStructure(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create old structure: polecats/<name>/
	oldPath := filepath.Join(rigPath, "polecats", "Toast")
	require.NoError(t, os.MkdirAll(oldPath, 0755))
	// Don't create the new structure

	id := agent.PolecatAddress("testrig", "Toast")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, oldPath, dir)
}

func TestWorkDirForID_Crew(t *testing.T) {
	townRoot := "/town"

	id := agent.CrewAddress("testrig", "emma")
	dir, err := WorkDirForID(townRoot, id)

	require.NoError(t, err)
	assert.Equal(t, "/town/testrig/crew/emma", dir)
}

func TestWorkDirForID_UnknownRole_ReturnsError(t *testing.T) {
	townRoot := "/town"

	_, err := WorkDirForID(townRoot, agent.AgentID{Role: "unknown"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown role")
}

func TestWorkDirForID_WitnessMissingRig_ReturnsError(t *testing.T) {
	townRoot := "/town"

	// Malformed ID: witness without rig
	_, err := WorkDirForID(townRoot, agent.AgentID{Role: constants.RoleWitness})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires rig name")
}

func TestWorkDirForID_PolecatMissingWorker_ReturnsError(t *testing.T) {
	townRoot := "/town"

	// Malformed ID: polecat with rig but no worker
	_, err := WorkDirForID(townRoot, agent.AgentID{Role: "polecat", Rig: "testrig"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires rig and worker name")
}

// =============================================================================
// buildCommand Tests
// =============================================================================

func TestBuildCommand_Basic(t *testing.T) {
	cfg := &startConfig{}
	cmd := buildCommand(constants.RoleMayor, "claude", cfg)

	// Should contain the base claude command (config.BuildAgentCommand)
	assert.Contains(t, cmd, "claude")
	assert.Contains(t, cmd, "--dangerously-skip-permissions")
}

func TestBuildCommand_WithTopic(t *testing.T) {
	cfg := &startConfig{topic: "patrol"}
	cmd := buildCommand(constants.RoleCrew, "claude", cfg)

	// Topic becomes the beacon in the command
	assert.Contains(t, cmd, "patrol")
}

func TestBuildCommand_Interactive_RemovesSkipPermissions(t *testing.T) {
	cfg := &startConfig{interactive: true}
	cmd := buildCommand(constants.RoleCrew, "claude", cfg)

	// Interactive mode should remove --dangerously-skip-permissions
	assert.NotContains(t, cmd, "--dangerously-skip-permissions")
}

func TestBuildCommand_Interactive_WithTopic(t *testing.T) {
	cfg := &startConfig{topic: "review", interactive: true}
	cmd := buildCommand(constants.RoleCrew, "claude", cfg)

	// Should have topic but not --dangerously-skip-permissions
	assert.Contains(t, cmd, "review")
	assert.NotContains(t, cmd, "--dangerously-skip-permissions")
}

// =============================================================================
// StartOption Tests
// =============================================================================

func TestWithTopic(t *testing.T) {
	cfg := &startConfig{}
	WithTopic("patrol")(cfg)

	assert.Equal(t, "patrol", cfg.topic)
}

func TestWithInteractive(t *testing.T) {
	cfg := &startConfig{}
	WithInteractive()(cfg)

	assert.True(t, cfg.interactive)
}

func TestWithKillExisting(t *testing.T) {
	cfg := &startConfig{}
	WithKillExisting()(cfg)

	assert.True(t, cfg.killExisting)
}

func TestWithEnvOverrides(t *testing.T) {
	cfg := &startConfig{}
	overrides := map[string]string{"FOO": "bar", "BAZ": "qux"}
	WithEnvOverrides(overrides)(cfg)

	assert.Equal(t, overrides, cfg.envOverrides)
}

// =============================================================================
// parseEnvOverrides Tests
// =============================================================================

func TestParseEnvOverrides_Empty(t *testing.T) {
	result := parseEnvOverrides(nil)
	assert.Empty(t, result)
}

func TestParseEnvOverrides_SinglePair(t *testing.T) {
	result := parseEnvOverrides([]string{"FOO=bar"})
	assert.Equal(t, map[string]string{"FOO": "bar"}, result)
}

func TestParseEnvOverrides_MultiplePairs(t *testing.T) {
	result := parseEnvOverrides([]string{"FOO=bar", "BAZ=qux"})
	assert.Equal(t, map[string]string{"FOO": "bar", "BAZ": "qux"}, result)
}

func TestParseEnvOverrides_ValueWithEquals(t *testing.T) {
	result := parseEnvOverrides([]string{"FOO=bar=baz"})
	assert.Equal(t, map[string]string{"FOO": "bar=baz"}, result)
}

func TestParseEnvOverrides_InvalidPair_Ignored(t *testing.T) {
	result := parseEnvOverrides([]string{"NOEQUALS", "FOO=bar"})
	assert.Equal(t, map[string]string{"FOO": "bar"}, result)
}

// =============================================================================
// StartWithAgents Tests
//
// These tests use agent.Double (a Fake with spy capabilities) to verify
// Start() behavior without requiring real tmux sessions.
//
// Test double taxonomy (Meszaros/Fowler):
// - Fake: Working in-memory implementation (agent.Double)
// - Spy: Records method calls for verification (StopCalls(), GetStartConfig())
// - Stub: Used for error injection (AgentsStub wrapping Double)
// =============================================================================

func TestStartWithAgents_Basic(t *testing.T) {
	// Uses FAKE (agent.Double) with STATE VERIFICATION
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	require.NoError(t, os.MkdirAll(mayorDir, 0755))

	agents := agent.NewDouble() // FAKE: working in-memory implementation

	id, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude")

	// STATE VERIFICATION: check the result, not how we got there
	require.NoError(t, err)
	assert.Equal(t, agent.MayorAddress, id)
	assert.True(t, agents.Exists(agent.MayorAddress)) // Agent exists in fake
}

func TestStartWithAgents_SetsCorrectWorkDir(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	witnessDir := filepath.Join(rigPath, "witness", "rig")
	require.NoError(t, os.MkdirAll(witnessDir, 0755))

	agents := agent.NewDouble()
	id := agent.WitnessAddress("testrig")

	_, err := StartWithAgents(agents, nil, townRoot, id, "claude")

	require.NoError(t, err)
	assert.Equal(t, witnessDir, agents.GetWorkDir(id))
}

func TestStartWithAgents_SetsCorrectEnvVars(t *testing.T) {
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()
	_, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude")

	require.NoError(t, err)
	cmd := agents.GetCommand(agent.MayorAddress)

	// Verify env vars are prepended to command
	assert.Contains(t, cmd, "GT_ROLE=mayor")
	assert.Contains(t, cmd, "GT_ROOT="+townRoot)
}

func TestStartWithAgents_CrewAgent_SetsRigAndWorkerEnvVars(t *testing.T) {
	townRoot := t.TempDir()
	crewDir := filepath.Join(townRoot, "testrig", "crew", "emma")
	require.NoError(t, os.MkdirAll(crewDir, 0755))

	agents := agent.NewDouble()
	id := agent.CrewAddress("testrig", "emma")

	_, err := StartWithAgents(agents, nil, townRoot, id, "claude")

	require.NoError(t, err)
	cmd := agents.GetCommand(id)

	assert.Contains(t, cmd, "GT_ROLE=crew")
	assert.Contains(t, cmd, "GT_RIG=testrig")
	assert.Contains(t, cmd, "GT_CREW=emma") // Crew uses GT_CREW for agent name
}

func TestStartWithAgents_WithTopic(t *testing.T) {
	townRoot := t.TempDir()
	crewDir := filepath.Join(townRoot, "testrig", "crew", "emma")
	require.NoError(t, os.MkdirAll(crewDir, 0755))

	agents := agent.NewDouble()
	id := agent.CrewAddress("testrig", "emma")

	_, err := StartWithAgents(agents, nil, townRoot, id, "claude", WithTopic("patrol"))

	require.NoError(t, err)
	cmd := agents.GetCommand(id)
	assert.Contains(t, cmd, "patrol")
}

func TestStartWithAgents_WithInteractive(t *testing.T) {
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()
	_, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude", WithInteractive())

	require.NoError(t, err)
	cmd := agents.GetCommand(agent.MayorAddress)
	assert.NotContains(t, cmd, "--dangerously-skip-permissions")
}

func TestStartWithAgents_WithKillExisting(t *testing.T) {
	// Uses FAKE with SPY feature for BEHAVIOR VERIFICATION
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()            // FAKE with spy capability
	agents.CreateAgent(agent.MayorAddress) // Pre-existing session

	_, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude", WithKillExisting())

	// STATE VERIFICATION: agent exists after operation
	require.NoError(t, err)
	assert.True(t, agents.Exists(agent.MayorAddress))

	// SPY VERIFICATION: check that Stop was called correctly
	stops := agents.StopCalls() // Spy feature: recorded calls
	require.Len(t, stops, 1)
	assert.Equal(t, agent.MayorAddress, stops[0].ID)
	assert.True(t, stops[0].Graceful) // Should be graceful stop
}

func TestStartWithAgents_AlreadyRunning(t *testing.T) {
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()
	agents.CreateAgent(agent.MayorAddress) // Already running

	_, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude")

	// Without WithKillExisting, should fail with ErrAlreadyRunning
	assert.ErrorIs(t, err, agent.ErrAlreadyRunning)
}

func TestStartWithAgents_WithEnvOverrides(t *testing.T) {
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()
	overrides := map[string]string{"CUSTOM_VAR": "custom_value"}

	_, err := StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude", WithEnvOverrides(overrides))

	require.NoError(t, err)
	cmd := agents.GetCommand(agent.MayorAddress)
	assert.Contains(t, cmd, "CUSTOM_VAR=custom_value")
}

func TestStartWithAgents_InvalidAgentID(t *testing.T) {
	townRoot := t.TempDir()
	agents := agent.NewDouble()

	_, err := StartWithAgents(agents, nil, townRoot, agent.AgentID{Role: "invalid"}, "claude")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown role")
}

func TestStartWithAgents_PolecatRequiresRigAndWorker(t *testing.T) {
	townRoot := t.TempDir()
	agents := agent.NewDouble()

	// Malformed polecat ID (missing worker)
	_, err := StartWithAgents(agents, nil, townRoot, agent.AgentID{Role: "polecat", Rig: "testrig"}, "claude")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires rig and worker name")
}

func TestStartWithAgents_StartError_Propagates(t *testing.T) {
	// Uses STUB wrapping FAKE for ERROR INJECTION
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()         // FAKE: working implementation
	stub := agent.NewAgentsStub(agents) // STUB: wraps fake, injects errors
	stub.StartErr = assert.AnError      // Inject canned error

	_, err := StartWithAgents(stub, nil, townRoot, agent.MayorAddress, "claude")

	// STATE VERIFICATION: error propagated correctly
	assert.Error(t, err)
}

func TestStartWithAgents_CallsOnCreatedCallback(t *testing.T) {
	townRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755))

	agents := agent.NewDouble()

	// Pass a non-nil callback to verify it gets recorded
	themer := func(_ session.SessionID) error {
		return nil
	}

	_, err := StartWithAgents(agents, themer, townRoot, agent.MayorAddress, "claude")

	require.NoError(t, err)
	// Verify callback was passed (Double records it but doesn't execute real callbacks)
	assert.True(t, agents.HasOnCreated(agent.MayorAddress))
}

func TestStartWithAgents_RefineryAgent(t *testing.T) {
	townRoot := t.TempDir()
	refineryDir := filepath.Join(townRoot, "testrig", "refinery", "rig")
	require.NoError(t, os.MkdirAll(refineryDir, 0755))

	agents := agent.NewDouble()
	id := agent.RefineryAddress("testrig")

	_, err := StartWithAgents(agents, nil, townRoot, id, "claude")

	require.NoError(t, err)
	assert.True(t, agents.Exists(id))
	assert.Equal(t, refineryDir, agents.GetWorkDir(id))
}

func TestStartWithAgents_PolecatAgent(t *testing.T) {
	townRoot := t.TempDir()
	polecatDir := filepath.Join(townRoot, "testrig", "polecats", "Toast")
	require.NoError(t, os.MkdirAll(polecatDir, 0755))

	agents := agent.NewDouble()
	id := agent.PolecatAddress("testrig", "Toast")

	_, err := StartWithAgents(agents, nil, townRoot, id, "claude")

	require.NoError(t, err)
	assert.True(t, agents.Exists(id))
	cmd := agents.GetCommand(id)
	assert.Contains(t, cmd, "GT_ROLE=polecat")
	assert.Contains(t, cmd, "GT_RIG=testrig")
	assert.Contains(t, cmd, "GT_POLECAT=Toast") // Polecat uses GT_POLECAT for agent name
}

func TestStartWithAgents_DeaconAgent(t *testing.T) {
	townRoot := t.TempDir()
	deaconDir := filepath.Join(townRoot, "deacon")
	require.NoError(t, os.MkdirAll(deaconDir, 0755))

	agents := agent.NewDouble()

	id, err := StartWithAgents(agents, nil, townRoot, agent.DeaconAddress, "claude")

	require.NoError(t, err)
	assert.Equal(t, agent.DeaconAddress, id)
	assert.True(t, agents.Exists(agent.DeaconAddress))
	assert.Equal(t, deaconDir, agents.GetWorkDir(id))
}

func TestStartWithAgents_BootAgent(t *testing.T) {
	townRoot := t.TempDir()
	bootDir := filepath.Join(townRoot, "deacon", "dogs", "boot")
	require.NoError(t, os.MkdirAll(bootDir, 0755))

	agents := agent.NewDouble()

	id, err := StartWithAgents(agents, nil, townRoot, agent.BootAddress, "claude")

	require.NoError(t, err)
	assert.Equal(t, agent.BootAddress, id)
	assert.True(t, agents.Exists(agent.BootAddress))
	assert.Equal(t, bootDir, agents.GetWorkDir(id))
}
