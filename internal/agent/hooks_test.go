package agent_test

import (
	"errors"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Hooks and Config Tests
// =============================================================================

// --- PromptChecker Tests ---

func TestPromptChecker_IsReady_WhenPrefixMatches_ReturnsTrue(t *testing.T) {
	checker := &agent.PromptChecker{Prefix: ">"}
	assert.True(t, checker.IsReady("> some prompt"))
	assert.True(t, checker.IsReady("line1\n> prompt"))
	assert.True(t, checker.IsReady("  >  with spaces"))
}

func TestPromptChecker_IsReady_WhenPrefixNotFound_ReturnsFalse(t *testing.T) {
	checker := &agent.PromptChecker{Prefix: ">"}
	assert.False(t, checker.IsReady("no prompt here"))
	assert.False(t, checker.IsReady(""))
}

// --- WaitForReady Function Tests ---

func TestWaitForReady_WhenCheckerIsNil_ReturnsImmediately(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")

	start := time.Now()
	err := agent.WaitForReady(procs, id, time.Second, nil)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "should return immediately when checker is nil")
}

func TestWaitForReady_WhenReadyImmediately_ReturnsNoError(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")
	procs.SetBuffer(id, []string{"> ready"})

	checker := &agent.PromptChecker{Prefix: ">"}
	err := agent.WaitForReady(procs, id, time.Second, checker)

	assert.NoError(t, err)
}

func TestWaitForReady_WhenTimeout_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")
	procs.SetBuffer(id, []string{"not ready"})

	checker := &agent.PromptChecker{Prefix: ">"}
	err := agent.WaitForReady(procs, id, 300*time.Millisecond, checker)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestWaitForReady_WhenCaptureFailsInitially_Retries(t *testing.T) {
	procs := session.NewDouble()
	// Don't start a session - Capture will fail
	// Then create it after a delay

	checker := &agent.PromptChecker{Prefix: ">"}

	go func() {
		time.Sleep(100 * time.Millisecond)
		id, _ := procs.Start("test", "/tmp", "cmd")
		procs.SetBuffer(id, []string{"> ready"})
	}()

	// Start WaitForReady before session exists
	err := agent.WaitForReady(procs, "test", 500*time.Millisecond, checker)

	assert.NoError(t, err, "should eventually succeed after session is created")
}

// --- Config Builder Tests ---

func TestConfig_WithOnSessionCreated_SetsCallback(t *testing.T) {
	base := &agent.Config{Name: "test"}
	called := false
	callback := func(id session.SessionID) error {
		called = true
		return nil
	}

	result := base.WithOnSessionCreated(callback)

	assert.NotNil(t, result.OnSessionCreated)
	assert.Equal(t, "test", result.Name, "should preserve other fields")

	// Verify callback is the one we set
	_ = result.OnSessionCreated("")
	assert.True(t, called)
}

func TestConfig_WithStartupHook_SetsHook(t *testing.T) {
	base := &agent.Config{Name: "test"}
	called := false
	hook := func(sess session.Sessions, id session.SessionID) error {
		called = true
		return nil
	}

	result := base.WithStartupHook(hook)

	assert.NotNil(t, result.StartupHook)
	assert.Equal(t, "test", result.Name, "should preserve other fields")

	// Verify hook is the one we set
	_ = result.StartupHook(nil, "")
	assert.True(t, called)
}

func TestConfig_WithEnvVars_SetsEnvVars(t *testing.T) {
	base := &agent.Config{Name: "test"}
	envVars := map[string]string{"FOO": "bar", "BAZ": "qux"}

	result := base.WithEnvVars(envVars)

	assert.Equal(t, envVars, result.EnvVars)
	assert.Equal(t, "test", result.Name, "should preserve other fields")
}

func TestConfig_Chaining(t *testing.T) {
	callback := func(id session.SessionID) error { return nil }
	hook := func(sess session.Sessions, id session.SessionID) error { return nil }
	envVars := map[string]string{"KEY": "value"}

	result := agent.FromPreset("claude").
		WithOnSessionCreated(callback).
		WithStartupHook(hook).
		WithEnvVars(envVars)

	assert.Equal(t, "claude", result.Name)
	assert.NotNil(t, result.OnSessionCreated)
	assert.NotNil(t, result.StartupHook)
	assert.Equal(t, envVars, result.EnvVars)
}

// --- FromPreset Tests ---

func TestFromPreset_Claude_ReturnsCorrectConfig(t *testing.T) {
	cfg := agent.FromPreset("claude")

	assert.Equal(t, "claude", cfg.Name)
	assert.NotNil(t, cfg.StartupHook, "claude should have startup hook")
	assert.NotNil(t, cfg.Checker, "claude should have a checker")
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

func TestFromPreset_OpenCode_ReturnsCorrectConfig(t *testing.T) {
	cfg := agent.FromPreset("opencode")

	assert.Equal(t, "opencode", cfg.Name)
	assert.Nil(t, cfg.StartupHook, "opencode should not have startup hook")
	assert.Nil(t, cfg.Checker, "opencode should not have a checker")
	assert.Equal(t, 500*time.Millisecond, cfg.StartupDelay)
	assert.Equal(t, 10*time.Second, cfg.Timeout)
}

func TestFromPreset_Gemini_ReturnsCorrectConfig(t *testing.T) {
	cfg := agent.FromPreset("gemini")

	assert.Equal(t, "gemini", cfg.Name)
	assert.NotNil(t, cfg.Checker, "gemini should have a checker")
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestFromPreset_Unknown_ReturnsDefaultConfig(t *testing.T) {
	cfg := agent.FromPreset("unknown-agent")

	assert.Equal(t, "unknown-agent", cfg.Name)
	assert.Nil(t, cfg.Checker, "unknown agent should not have a checker")
	assert.Equal(t, 1*time.Second, cfg.StartupDelay, "unknown agent should have default startup delay")
	assert.Equal(t, 30*time.Second, cfg.Timeout, "unknown agent should have default timeout")
}

// --- Factory Function Tests ---

func TestClaude_ReturnsClaudeConfig(t *testing.T) {
	cfg := agent.Claude()

	assert.Equal(t, "claude", cfg.Name)
	assert.NotNil(t, cfg.StartupHook)
}

func TestOpenCode_ReturnsOpenCodeConfig(t *testing.T) {
	cfg := agent.OpenCode()

	assert.Equal(t, "opencode", cfg.Name)
	assert.Equal(t, 500*time.Millisecond, cfg.StartupDelay)
}

// --- ClaudeStartupHook Tests ---

func TestClaudeStartupHook_WhenNoWarning_DoesNothing(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")
	procs.SetBuffer(id, []string{"Normal output", "No warning here"})

	err := agent.ClaudeStartupHook(procs, id)

	assert.NoError(t, err)
	// Verify no control sequences were sent (no dialog to dismiss)
	controls := procs.ControlLog(id)
	assert.Empty(t, controls)
}

func TestClaudeStartupHook_WhenWarningPresent_DismissesDialog(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")
	procs.SetBuffer(id, []string{"Bypass Permissions mode is enabled"})

	err := agent.ClaudeStartupHook(procs, id)

	assert.NoError(t, err)
	// Verify Down and Enter were sent to dismiss the dialog
	controls := procs.ControlLog(id)
	assert.Contains(t, controls, "Down")
	assert.Contains(t, controls, "Enter")
}

func TestClaudeStartupHook_WhenCaptureErrors_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	// Don't start a session - Capture will fail

	err := agent.ClaudeStartupHook(procs, "nonexistent")

	assert.Error(t, err)
}

func TestClaudeStartupHook_WhenSendControlFails_ReturnsError(t *testing.T) {
	procs := session.NewDouble()
	id, _ := procs.Start("test", "/tmp", "cmd")
	procs.SetBuffer(id, []string{"Bypass Permissions mode is enabled"})

	// Create a stub that wraps the double and fails on SendControl
	stub := &sendControlFailStub{Sessions: procs}

	err := agent.ClaudeStartupHook(stub, id)

	assert.Error(t, err)
}

// sendControlFailStub is a minimal stub that fails on SendControl
type sendControlFailStub struct {
	session.Sessions
}

func (s *sendControlFailStub) SendControl(id session.SessionID, key string) error {
	return errors.New("sendcontrol failed")
}
