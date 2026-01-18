package agent_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Implementation-Specific Unit Tests
//
// These tests cover behaviors unique to agent.Implementation that cannot be
// tested through the Agents interface alone (zombie detection, callbacks, etc.)
//
// For interface-level tests that run against both Double and Implementation,
// see conformance_test.go.
// =============================================================================

// startCfg is a helper to create a StartConfig for tests.
func startCfg(workDir, command string) agent.StartConfig {
	return agent.StartConfig{WorkDir: workDir, Command: command}
}

// --- Zombie Detection Tests (Implementation-specific) ---

func TestImplementation_Start_DetectsAndCleansUpZombie(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	// Create zombie: session exists but process dead
	id := agent.PolecatAddress("testrig", "test-agent")
	sessionID := procs.SessionIDForAgent(id)
	_, _ = procs.Start(string(sessionID), "/tmp", "dead-command")
	_ = procs.SetRunning(sessionID, false) // Process is dead

	// Start should clean up zombie and create new session
	err := agents.StartWithConfig(id, startCfg("/tmp", "new-command"))
	require.NoError(t, err)
	assert.True(t, agents.Exists(id))
}

func TestImplementation_Start_AlreadyRunning_WithLiveProcess(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	// Create the AgentID first so we use the correct session name
	id := agent.PolecatAddress("testrig", "test-agent")
	sessionID := procs.SessionIDForAgent(id)

	// Pre-create session with live process using the correct session ID
	_, _ = procs.Start(string(sessionID), "/tmp", "running-command")
	_ = procs.SetRunning(sessionID, true)

	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	assert.ErrorIs(t, err, agent.ErrAlreadyRunning)
}

func TestImplementation_Exists_ZombieDetection(t *testing.T) {
	procs := session.NewDouble()

	// Configure with process names for zombie detection
	cfg := &agent.Config{
		ProcessNames: []string{"claude", "node"},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	sessionID := procs.SessionIDForAgent(id)

	// Create session and mark it as running initially
	_, _ = procs.Start(string(sessionID), "/tmp", "claude --config")
	_ = procs.SetRunning(sessionID, true)

	// Should exist when process is running
	assert.True(t, agents.Exists(id), "should exist when process is running")

	// Simulate zombie: session exists but process died
	_ = procs.SetRunning(sessionID, false)

	// Exists should return false for zombie
	assert.False(t, agents.Exists(id), "should return false for zombie session")
}

func TestImplementation_Exists_NoProcessNames_FallsBackToSessionExistence(t *testing.T) {
	procs := session.NewDouble()

	// No process names configured - can't detect zombies
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	sessionID := procs.SessionIDForAgent(id)

	// Create session but don't set running state
	_, _ = procs.Start(string(sessionID), "/tmp", "echo hello")

	// Without ProcessNames, should return true just based on session existence
	assert.True(t, agents.Exists(id), "should return true when session exists (no zombie detection)")
}

func TestImplementation_List_FiltersZombies(t *testing.T) {
	procs := session.NewDouble()

	// Configure with process names for zombie detection
	cfg := &agent.Config{
		ProcessNames: []string{"claude"},
	}
	agents := agent.New(procs, cfg)

	// Create two sessions
	id1 := agent.PolecatAddress("testrig", "live")
	id2 := agent.PolecatAddress("testrig", "zombie")
	sessionID1 := procs.SessionIDForAgent(id1)
	sessionID2 := procs.SessionIDForAgent(id2)

	_, _ = procs.Start(string(sessionID1), "/tmp", "claude")
	_, _ = procs.Start(string(sessionID2), "/tmp", "claude")

	// First one is running, second is zombie
	_ = procs.SetRunning(sessionID1, true)
	_ = procs.SetRunning(sessionID2, false)

	// List should only return the live session
	ids, err := agents.List()
	require.NoError(t, err)
	assert.Len(t, ids, 1, "should only list live sessions")
	assert.Equal(t, id1, ids[0], "should only include the live session")
}

// --- Callback Tests (Implementation-specific) ---

func TestImplementation_Start_CallbackError_CleansUpSession(t *testing.T) {
	procs := session.NewDouble()

	callbackErr := errors.New("callback failed")
	cfg := &agent.Config{
		OnSessionCreated: func(id session.SessionID) error {
			return callbackErr
		},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session setup")

	// Session should be cleaned up
	exists, _ := procs.Exists(procs.SessionIDForAgent(id))
	assert.False(t, exists, "session should be cleaned up on callback failure")
}

func TestImplementation_Start_CallbackSuccess_SessionRemains(t *testing.T) {
	procs := session.NewDouble()

	callbackCalled := false
	cfg := &agent.Config{
		OnSessionCreated: func(id session.SessionID) error {
			callbackCalled = true
			return nil
		},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)

	assert.True(t, callbackCalled, "callback should be called")
	assert.True(t, agents.Exists(id), "session should exist after successful callback")
}

// --- EnvVars Tests (Implementation-specific) ---

func TestImplementation_Start_PrependsEnvVars(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{
		EnvVars: map[string]string{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Contains(t, cmd, "FOO=bar")
	assert.Contains(t, cmd, "BAZ=qux")
	assert.Contains(t, cmd, "echo hello")
}

func TestImplementation_Start_EnvVarsSorted(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{
		EnvVars: map[string]string{
			"ZZZ": "last",
			"AAA": "first",
			"MMM": "middle",
		},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	aaaIdx := indexOf(cmd, "AAA=first")
	mmmIdx := indexOf(cmd, "MMM=middle")
	zzzIdx := indexOf(cmd, "ZZZ=last")
	assert.True(t, aaaIdx < mmmIdx, "AAA should come before MMM")
	assert.True(t, mmmIdx < zzzIdx, "MMM should come before ZZZ")
}

func TestImplementation_Start_NoEnvVars_CommandUnchanged(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Equal(t, "echo hello", cmd)
}

// --- Timeout Tests (Implementation-specific) ---

func TestImplementation_Timeout_UsesConfigValue(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{
		Timeout: 5 * time.Second,
		Checker: &agent.PromptChecker{Prefix: "NEVER"},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	procs.SetBuffer(procs.SessionIDForAgent(id), []string{"no match"})

	start := time.Now()
	_ = agents.WaitReady(id)
	elapsed := time.Since(start)

	// Should timeout around 5 seconds (with some tolerance)
	assert.True(t, elapsed >= 4*time.Second && elapsed < 7*time.Second,
		"expected ~5s timeout, got %v", elapsed)
}

func TestImplementation_Timeout_DefaultsTo30Seconds(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{
		// Timeout: 0 means use default
		Checker: &agent.PromptChecker{Prefix: "NEVER"},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	procs.SetBuffer(procs.SessionIDForAgent(id), []string{"no match"})

	// We can't wait 30 seconds in a test, but we can verify it doesn't
	// return immediately when there's no match
	start := time.Now()
	done := make(chan bool)
	go func() {
		_ = agents.WaitReady(id)
		done <- true
	}()

	select {
	case <-done:
		// If it completed, check it took at least some time
		elapsed := time.Since(start)
		assert.True(t, elapsed >= 100*time.Millisecond, "should not return immediately")
	case <-time.After(500 * time.Millisecond):
		// Expected - it's still waiting (would wait 30s)
	}
}

// --- Graceful Stop Tests (Implementation-specific) ---

func TestImplementation_Stop_Graceful_TakesLongerThanNonGraceful(t *testing.T) {
	// Graceful stop sends Ctrl-C and waits before killing
	// We verify this by timing the operation

	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	// Non-graceful stop
	id1 := agent.PolecatAddress("testrig", "agent1")
	_ = agents.StartWithConfig(id1, startCfg("/tmp", "echo hello"))
	start1 := time.Now()
	_ = agents.Stop(id1, false) // graceful=false
	elapsed1 := time.Since(start1)

	// Graceful stop (includes 100ms sleep after Ctrl-C)
	id2 := agent.PolecatAddress("testrig", "agent2")
	_ = agents.StartWithConfig(id2, startCfg("/tmp", "echo hello"))
	start2 := time.Now()
	_ = agents.Stop(id2, true) // graceful=true
	elapsed2 := time.Since(start2)

	// Graceful should take noticeably longer due to the sleep
	assert.True(t, elapsed2 > elapsed1, "graceful stop should take longer")
	assert.True(t, elapsed2 >= 100*time.Millisecond, "graceful stop should wait at least 100ms")
}

func TestImplementation_Stop_NotGraceful_SkipsCtrlC(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	sessionID := procs.SessionIDForAgent(id)

	err := agents.Stop(id, false) // graceful=false
	require.NoError(t, err)

	// Verify no Ctrl-C was sent
	controls := procs.ControlLog(sessionID)
	assert.Empty(t, controls)
}

// --- Nudge Tests (Implementation-specific) ---

func TestImplementation_Nudge_SendsMessageToSession(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	err := agents.Nudge(id, "HEALTH_CHECK: are you alive?")

	assert.NoError(t, err)

	// Verify message was logged in session double
	nudges := procs.NudgeLog(procs.SessionIDForAgent(id))
	assert.Contains(t, nudges, "HEALTH_CHECK: are you alive?")
}

func TestImplementation_Nudge_WhenSessionNotExists_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "nonexistent")
	err := agents.Nudge(id, "hello")

	assert.Error(t, err)
}

func TestImplementation_Nudge_MultipleCalls_AllRecorded(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	_ = agents.Nudge(id, "message 1")
	_ = agents.Nudge(id, "message 2")
	_ = agents.Nudge(id, "message 3")

	nudges := procs.NudgeLog(procs.SessionIDForAgent(id))
	assert.Len(t, nudges, 3)
	assert.Equal(t, "message 1", nudges[0])
	assert.Equal(t, "message 2", nudges[1])
	assert.Equal(t, "message 3", nudges[2])
}

// --- StartupHook Tests (Implementation-specific) ---

func TestImplementation_StartupHook_CalledOnStart(t *testing.T) {
	procs := session.NewDouble()

	hookCalled := false
	var hookedSessionID session.SessionID
	cfg := &agent.Config{
		StartupHook: func(sess session.Sessions, id session.SessionID) error {
			hookCalled = true
			hookedSessionID = id
			return nil
		},
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)

	// Give the goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	assert.True(t, hookCalled, "startup hook should be called")
	assert.Equal(t, procs.SessionIDForAgent(id), hookedSessionID)
}

func TestImplementation_StartupHook_ErrorIsNonFatal(t *testing.T) {
	procs := session.NewDouble()

	hookErr := errors.New("hook failed")
	cfg := &agent.Config{
		StartupHook: func(sess session.Sessions, id session.SessionID) error {
			return hookErr
		},
	}
	agents := agent.New(procs, cfg)

	// Start should still succeed even if startup hook fails
	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))
	require.NoError(t, err)
	assert.True(t, agents.Exists(id))
}

// --- doWaitForReady Tests (Implementation-specific) ---

func TestImplementation_WaitReady_WithStartupDelay(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{
		StartupDelay: 100 * time.Millisecond,
		// No Checker - falls back to StartupDelay
	}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))

	start := time.Now()
	err := agents.WaitReady(id)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, elapsed >= 100*time.Millisecond, "should wait at least StartupDelay")
}

// --- Error Path Tests (using sessionsStub) ---

func TestImplementation_Start_WhenSessionStartFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	stub := newSessionsStub(procs)
	stub.StartErr = errors.New("session start failed")

	agents := agent.New(stub, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	err := agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "starting session")
}

func TestImplementation_Start_WhenZombieCleanupFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()

	// Create a zombie session using the proper session ID
	id := agent.PolecatAddress("testrig", "test-agent")
	sessionID := procs.SessionIDForAgent(id)
	_, _ = procs.Start(string(sessionID), "/tmp", "dead-command")
	_ = procs.SetRunning(sessionID, false) // Process is dead

	// Now wrap with stub that fails on Stop (zombie cleanup)
	stub := newSessionsStub(procs)
	stub.StopErr = errors.New("stop failed")

	agents := agent.New(stub, nil)

	err := agents.StartWithConfig(id, startCfg("/tmp", "new-command"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "killing zombie")
}

func TestImplementation_Stop_WhenSessionStopFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	stub := newSessionsStub(procs)

	agents := agent.New(stub, nil)

	id := agent.PolecatAddress("testrig", "test-agent")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))

	// Now inject Stop error
	stub.StopErr = errors.New("stop failed")

	err := agents.Stop(id, false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stopping session")
}

// Helper function
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// --- Self() Tests ---

func TestSelf_Mayor(t *testing.T) {
	// Save and restore env
	orig := os.Getenv("GT_ROLE")
	defer os.Setenv("GT_ROLE", orig)

	os.Setenv("GT_ROLE", "mayor")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.MayorAddress, id)
}

func TestSelf_Deacon(t *testing.T) {
	orig := os.Getenv("GT_ROLE")
	defer os.Setenv("GT_ROLE", orig)

	os.Setenv("GT_ROLE", "deacon")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.DeaconAddress, id)
}

func TestSelf_Witness(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
	}()

	os.Setenv("GT_ROLE", "witness")
	os.Setenv("GT_RIG", "myrig")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.WitnessAddress("myrig"), id)
}

func TestSelf_Refinery(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
	}()

	os.Setenv("GT_ROLE", "refinery")
	os.Setenv("GT_RIG", "myrig")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.RefineryAddress("myrig"), id)
}

func TestSelf_Crew(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	origCrew := os.Getenv("GT_CREW")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
		os.Setenv("GT_CREW", origCrew)
	}()

	os.Setenv("GT_ROLE", "crew")
	os.Setenv("GT_RIG", "myrig")
	os.Setenv("GT_CREW", "max")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.CrewAddress("myrig", "max"), id)
}

func TestSelf_Polecat(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	origPolecat := os.Getenv("GT_POLECAT")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
		os.Setenv("GT_POLECAT", origPolecat)
	}()

	os.Setenv("GT_ROLE", "polecat")
	os.Setenv("GT_RIG", "myrig")
	os.Setenv("GT_POLECAT", "Toast")
	id, err := agent.Self()

	assert.NoError(t, err)
	assert.Equal(t, agent.PolecatAddress("myrig", "Toast"), id)
}

func TestSelf_MissingRole_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	defer os.Setenv("GT_ROLE", origRole)

	os.Setenv("GT_ROLE", "")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_UnknownRole_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	defer os.Setenv("GT_ROLE", origRole)

	os.Setenv("GT_ROLE", "bogus")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_WitnessMissingRig_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
	}()

	os.Setenv("GT_ROLE", "witness")
	os.Setenv("GT_RIG", "")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_CrewMissingName_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	origCrew := os.Getenv("GT_CREW")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
		os.Setenv("GT_CREW", origCrew)
	}()

	os.Setenv("GT_ROLE", "crew")
	os.Setenv("GT_RIG", "myrig")
	os.Setenv("GT_CREW", "")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_Polecat_BothMissing_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	origPolecat := os.Getenv("GT_POLECAT")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
		os.Setenv("GT_POLECAT", origPolecat)
	}()

	os.Setenv("GT_ROLE", "polecat")
	os.Setenv("GT_RIG", "")
	os.Setenv("GT_POLECAT", "")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_Crew_RigMissingNamePresent_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	origCrew := os.Getenv("GT_CREW")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
		os.Setenv("GT_CREW", origCrew)
	}()

	os.Setenv("GT_ROLE", "crew")
	os.Setenv("GT_RIG", "")
	os.Setenv("GT_CREW", "emma")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

func TestSelf_Refinery_MissingRig_ReturnsError(t *testing.T) {
	origRole := os.Getenv("GT_ROLE")
	origRig := os.Getenv("GT_RIG")
	defer func() {
		os.Setenv("GT_ROLE", origRole)
		os.Setenv("GT_RIG", origRig)
	}()

	os.Setenv("GT_ROLE", "refinery")
	os.Setenv("GT_RIG", "")
	_, err := agent.Self()

	assert.Error(t, err)
	assert.ErrorIs(t, err, agent.ErrUnknownRole)
}

// --- AgentID.Parse() Tests ---

func TestAgentID_Parse_TownLevel(t *testing.T) {
	id := agent.MayorAddress
	role, rig, worker := id.Parse()
	assert.Equal(t, "mayor", role)
	assert.Empty(t, rig)
	assert.Empty(t, worker)
}

func TestAgentID_Parse_RigLevel(t *testing.T) {
	id := agent.WitnessAddress("testrig")
	role, rig, worker := id.Parse()
	assert.Equal(t, "witness", role)
	assert.Equal(t, "testrig", rig)
	assert.Empty(t, worker)
}

func TestAgentID_Parse_NamedWorker(t *testing.T) {
	id := agent.CrewAddress("testrig", "emma")
	role, rig, worker := id.Parse()
	assert.Equal(t, "crew", role)
	assert.Equal(t, "testrig", rig)
	assert.Equal(t, "emma", worker)
}

func TestAgentID_Parse_Empty(t *testing.T) {
	id := agent.AgentID{}
	role, rig, worker := id.Parse()
	assert.Equal(t, "", role)
	assert.Empty(t, rig)
	assert.Empty(t, worker)
}

// --- Respawn Tests ---

func TestImplementation_Respawn_SessionNotExists_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test")
	err := agents.Respawn(id)

	assert.ErrorIs(t, err, agent.ErrNotRunning)
}

func TestImplementation_Respawn_GetCommandFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	stub := newSessionsStub(procs)
	agents := agent.New(stub, nil)

	id := agent.PolecatAddress("testrig", "test")
	_, _ = procs.Start(session.SessionNameFromAgentID(id), "/tmp", "echo")

	// Now inject the GetStartCommand error
	stub.GetStartCommandErr = errors.New("command not found")

	err := agents.Respawn(id)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting start command")
}

func TestImplementation_Respawn_Success_SessionStillExists(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo hello"))

	err := agents.Respawn(id)

	require.NoError(t, err)
	assert.True(t, agents.Exists(id))
}

// --- mergeEnvVars Tests (via StartWithConfig) ---

func TestStartWithConfig_MergesEnvVars_BothEmpty(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil) // No Agents-level env vars

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, agent.StartConfig{
		WorkDir: "/tmp",
		Command: "echo hello",
		// No Start-level env vars
	})

	// Command should have no env var prefix
	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Equal(t, "echo hello", cmd)
}

func TestStartWithConfig_MergesEnvVars_OnlyAgentsLevel(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{EnvVars: map[string]string{"A": "1"}}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, agent.StartConfig{
		WorkDir: "/tmp",
		Command: "echo hello",
	})

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Contains(t, cmd, "A=1")
}

func TestStartWithConfig_MergesEnvVars_OnlyStartLevel(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, agent.StartConfig{
		WorkDir: "/tmp",
		Command: "echo hello",
		EnvVars: map[string]string{"B": "2"},
	})

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Contains(t, cmd, "B=2")
}

func TestStartWithConfig_MergesEnvVars_StartWinsConflict(t *testing.T) {
	procs := session.NewDouble()
	cfg := &agent.Config{EnvVars: map[string]string{"KEY": "agents"}}
	agents := agent.New(procs, cfg)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, agent.StartConfig{
		WorkDir: "/tmp",
		Command: "echo hello",
		EnvVars: map[string]string{"KEY": "start"},
	})

	cmd := procs.GetCommand(procs.SessionIDForAgent(id))
	assert.Contains(t, cmd, "KEY=start") // Start wins
	assert.NotContains(t, cmd, "KEY=agents")
}

// --- Capture/CaptureAll Tests ---

func TestImplementation_Capture_DelegatesToSession(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo"))
	_ = procs.SetBuffer(procs.SessionIDForAgent(id), []string{"line1", "line2", "line3"})

	output, err := agents.Capture(id, 10)

	require.NoError(t, err)
	assert.Contains(t, output, "line1")
	assert.Contains(t, output, "line2")
	assert.Contains(t, output, "line3")
}

func TestImplementation_CaptureAll_DelegatesToSession(t *testing.T) {
	procs := session.NewDouble()
	agents := agent.New(procs, nil)

	id := agent.PolecatAddress("testrig", "test")
	_ = agents.StartWithConfig(id, startCfg("/tmp", "echo"))
	_ = procs.SetBuffer(procs.SessionIDForAgent(id), []string{"all", "the", "lines"})

	output, err := agents.CaptureAll(id)

	require.NoError(t, err)
	assert.Contains(t, output, "all")
	assert.Contains(t, output, "the")
	assert.Contains(t, output, "lines")
}

// --- List Error Tests ---

func TestImplementation_List_SessionListFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	stub := newSessionsStub(procs)
	stub.ListErr = errors.New("list failed")
	agents := agent.New(stub, nil)

	_, err := agents.List()

	assert.Error(t, err)
}
