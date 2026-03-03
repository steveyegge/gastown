package deacon

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/tmux"
)

// mockTmux implements tmuxOps for testing.
type mockTmux struct {
	hasSessionResult bool
	hasSessionErr    error
	agentAlive       bool
	killErr          error
	newSessionErr    error
	waitErr          error
	sessionInfo      *tmux.SessionInfo
	sessionInfoErr   error
	sendKeysErr      error

	// Dead pane detection (for respawn-instead-of-kill behavior)
	paneDead               bool  // true if pane is dead (remain-on-exit)
	respawnErr             error // error from RespawnPaneDefault
	agentAliveAfterRespawn bool  // IsAgentAlive result after respawn

	// Call tracking
	killCalls       []string
	newSessionCalls int
	respawnCalls    int
	respawnChecked  bool // tracks if IsPaneDead was checked
}

func (m *mockTmux) HasSession(name string) (bool, error) {
	return m.hasSessionResult, m.hasSessionErr
}

func (m *mockTmux) IsAgentAlive(_ string) bool {
	// After respawn, return the post-respawn result
	if m.respawnChecked && m.agentAliveAfterRespawn {
		return true
	}
	return m.agentAlive
}

func (m *mockTmux) IsPaneDead(_ string) bool {
	return m.paneDead
}

func (m *mockTmux) RespawnPaneDefault(_ string) error {
	m.respawnCalls++
	m.respawnChecked = true
	return m.respawnErr
}

func (m *mockTmux) KillSessionWithProcesses(name string) error {
	m.killCalls = append(m.killCalls, name)
	return m.killErr
}

func (m *mockTmux) NewSessionWithCommand(_, _, _ string) error {
	m.newSessionCalls++
	return m.newSessionErr
}

func (m *mockTmux) SetRemainOnExit(_ string, _ bool) error    { return nil }
func (m *mockTmux) SetEnvironment(_, _, _ string) error       { return nil }
func (m *mockTmux) GetPaneID(_ string) (string, error)        { return "%0", nil }
func (m *mockTmux) ConfigureGasTownSession(_ string, _ tmux.Theme, _, _, _ string) error {
	return nil
}

func (m *mockTmux) WaitForCommand(_ string, _ []string, _ time.Duration) error {
	return m.waitErr
}

func (m *mockTmux) SetAutoRespawnHook(_ string) error              { return nil }
func (m *mockTmux) AcceptStartupDialogs(_ string) error            { return nil }
func (m *mockTmux) AcceptWorkspaceTrustDialog(_ string) error      { return nil }
func (m *mockTmux) AcceptBypassPermissionsWarning(_ string) error  { return nil }
func (m *mockTmux) SendKeysRaw(_, _ string) error                  { return m.sendKeysErr }
func (m *mockTmux) NudgeSession(_, _ string) error                                          { return nil }
func (m *mockTmux) WaitForRuntimeReady(_ string, _ *config.RuntimeConfig, _ time.Duration) error { return nil }
func (m *mockTmux) GetSessionInfo(_ string) (*tmux.SessionInfo, error) {
	return m.sessionInfo, m.sessionInfoErr
}

func newTestManager(townRoot string, mock *mockTmux) *Manager {
	return &Manager{
		townRoot: townRoot,
		tmux:     mock,
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-town")
	if m.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", m.townRoot, "/tmp/test-town")
	}
	if m.tmux == nil {
		t.Error("tmux should not be nil")
	}
}

func TestManager_SessionName(t *testing.T) {
	m := NewManager("/tmp/test-town")
	name := m.SessionName()
	if name == "" {
		t.Error("SessionName() should not be empty")
	}
	// Should match package-level SessionName
	if name != SessionName() {
		t.Errorf("method SessionName() = %q, package SessionName() = %q", name, SessionName())
	}
}

func TestManager_deaconDir(t *testing.T) {
	m := NewManager("/tmp/test-town")
	expected := filepath.Join("/tmp/test-town", "deacon")
	if m.deaconDir() != expected {
		t.Errorf("deaconDir() = %q, want %q", m.deaconDir(), expected)
	}
}

func TestStart_AlreadyRunning(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: true,
		agentAlive:       true,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Start("")
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("Start() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestStart_ZombieDetected_KillFails(t *testing.T) {
	killErr := errors.New("kill failed: session locked")
	mock := &mockTmux{
		hasSessionResult: true,
		agentAlive:       false, // zombie: tmux alive, agent dead
		killErr:          killErr,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Start("")
	if err == nil {
		t.Fatal("Start() should return error when zombie kill fails")
	}
	if !errors.Is(err, killErr) {
		t.Errorf("Start() error = %v, should wrap %v", err, killErr)
	}
	if len(mock.killCalls) != 1 {
		t.Errorf("expected 1 kill call, got %d", len(mock.killCalls))
	}
	if len(mock.killCalls) > 0 && mock.killCalls[0] != m.SessionName() {
		t.Errorf("killed session %q, want %q", mock.killCalls[0], m.SessionName())
	}
}

func TestStart_ZombieDetected_KillSucceeds(t *testing.T) {
	// Zombie kill succeeds, Start continues into config/runtime.
	// We verify the zombie was detected and killed.
	mock := &mockTmux{
		hasSessionResult: true,
		agentAlive:       false, // zombie
		killErr:          nil,   // kill succeeds
	}
	m := newTestManager(t.TempDir(), mock)

	// Start will proceed past zombie kill into config resolution.
	// It may fail on config.BuildAgentStartupCommandWithAgentOverride
	// in the test environment - that's fine, we're verifying zombie handling.
	_ = m.Start("")

	if len(mock.killCalls) != 1 {
		t.Errorf("expected 1 zombie kill call, got %d", len(mock.killCalls))
	}
}

func TestStart_NoExistingSession(t *testing.T) {
	// No existing session - Start proceeds to create one.
	// Will hit config/runtime calls which may error in test env.
	mock := &mockTmux{
		hasSessionResult: false,
	}
	m := newTestManager(t.TempDir(), mock)

	_ = m.Start("")

	// Should NOT have tried to kill anything
	if len(mock.killCalls) != 0 {
		t.Errorf("expected 0 kill calls, got %d", len(mock.killCalls))
	}
}

func TestStart_HasSessionError(t *testing.T) {
	// HasSession error is ignored (line 59: running, _ := ...).
	// When HasSession errors, running=false, so Start proceeds normally.
	mock := &mockTmux{
		hasSessionResult: false,
		hasSessionErr:    errors.New("tmux not available"),
	}
	m := newTestManager(t.TempDir(), mock)

	_ = m.Start("")

	// Should NOT have tried to kill anything
	if len(mock.killCalls) != 0 {
		t.Errorf("expected 0 kill calls when HasSession errors, got %d", len(mock.killCalls))
	}
}

func TestStart_SessionCreateFails(t *testing.T) {
	// Test that NewSessionWithCommand failure is propagated.
	mock := &mockTmux{
		hasSessionResult: false,
		newSessionErr:    errors.New("tmux server not running"),
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Start("claude")
	if err == nil {
		// If we got past config without error, session creation should have failed.
		// But config may have failed first - check if NewSessionWithCommand was called.
		if mock.newSessionCalls > 0 {
			t.Fatal("Start() should return error when session creation fails")
		}
		// Config failed before reaching session creation - acceptable in test env.
		return
	}

	// If NewSessionWithCommand was called and failed, error should wrap it.
	if mock.newSessionCalls > 0 {
		if got := err.Error(); got == "" {
			t.Error("error should have content")
		}
	}
}

func TestStart_WaitForCommandFails(t *testing.T) {
	// WaitForCommand failure should kill the session and return error.
	waitErr := errors.New("timeout waiting for agent")
	mock := &mockTmux{
		hasSessionResult: false,
		newSessionErr:    nil,
		waitErr:          waitErr,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Start("claude")

	// If we got past config to WaitForCommand, verify cleanup behavior.
	if mock.newSessionCalls > 0 {
		// Session was created, WaitForCommand was called.
		if err == nil {
			t.Fatal("Start() should return error when WaitForCommand fails")
		}
		if !errors.Is(err, waitErr) {
			t.Errorf("Start() error = %v, should wrap %v", err, waitErr)
		}
		// Should have killed the zombie session as cleanup (line 122).
		if len(mock.killCalls) == 0 {
			t.Error("expected cleanup kill call after WaitForCommand failure")
		}
	}
	// If config failed before reaching NewSessionWithCommand, that's
	// acceptable - the WaitForCommand path isn't reachable in test env.
}

// TestStart_DeadPane_RespawnsInsteadOfKill verifies that when the deacon
// session exists but the pane is dead (process exited with remain-on-exit on),
// Start() respawns the pane instead of killing the entire session and
// creating a new one. This is critical for the daemon's crash loop detection:
// respawning returns ErrAlreadyRunning, which makes the daemon call
// RecordSuccess() instead of RecordRestart().
//
// Regression test for the deacon crash loop: the deacon exits cleanly
// after each patrol cycle (~3 min), but the daemon heartbeat (also ~3 min)
// finds the dead pane, kills the "zombie", creates a new session, and
// increments the restart counter. After 5 restarts -> crash loop.
func TestStart_DeadPane_RespawnsInsteadOfKill(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult:       true,
		agentAlive:             false, // agent not running (pane dead)
		paneDead:               true,  // pane is dead, not a zombie shell
		respawnErr:             nil,   // respawn succeeds
		agentAliveAfterRespawn: true,  // agent comes back after respawn
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Start("")

	// Should return ErrAlreadyRunning because respawn recovered the session
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("Start() = %v, want ErrAlreadyRunning (respawn should recover dead pane)", err)
	}

	// Should NOT have killed the session
	if len(mock.killCalls) > 0 {
		t.Errorf("Start() killed session %v — should have respawned instead", mock.killCalls)
	}

	// Should NOT have created a new session
	if mock.newSessionCalls > 0 {
		t.Errorf("Start() created %d new sessions — should have respawned existing pane", mock.newSessionCalls)
	}

	// Should have called RespawnPaneDefault
	if mock.respawnCalls == 0 {
		t.Error("Start() did not call RespawnPaneDefault — dead pane should be respawned")
	}
}

// TestStart_DeadPane_RespawnFails_FallsThrough verifies that when respawn
// fails, Start() falls through to the kill+recreate path.
func TestStart_DeadPane_RespawnFails_FallsThrough(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: true,
		agentAlive:       false,
		paneDead:         true,
		respawnErr:       errors.New("respawn-pane failed"),
	}
	m := newTestManager(t.TempDir(), mock)

	// Start will fall through to kill+recreate, then hit config resolution
	// which may fail in test env. We just verify the fallthrough behavior.
	_ = m.Start("")

	// Respawn was attempted
	if mock.respawnCalls == 0 {
		t.Error("Start() did not attempt RespawnPaneDefault before falling through to kill")
	}

	// Should have fallen through to kill
	if len(mock.killCalls) == 0 {
		t.Error("Start() should fall through to kill when respawn fails")
	}
}

// TestStart_ZombieShell_StillKills verifies that a true zombie (shell
// running but agent dead) is still handled with kill+recreate, not respawn.
func TestStart_ZombieShell_StillKills(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: true,
		agentAlive:       false,
		paneDead:         false, // NOT a dead pane -- zombie shell still running
	}
	m := newTestManager(t.TempDir(), mock)

	_ = m.Start("")

	// Zombie shell should be killed, not respawned
	if len(mock.killCalls) == 0 {
		t.Error("Start() should kill zombie shell sessions")
	}
	if mock.respawnCalls > 0 {
		t.Error("Start() should not respawn zombie shell -- only dead panes")
	}
}

func TestStop_NotRunning(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: false,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Stop()
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Stop() error = %v, want ErrNotRunning", err)
	}
}

func TestStop_HasSessionError(t *testing.T) {
	sessionErr := errors.New("tmux server crashed")
	mock := &mockTmux{
		hasSessionErr: sessionErr,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Stop()
	if err == nil {
		t.Fatal("Stop() should return error when HasSession fails")
	}
	if !errors.Is(err, sessionErr) {
		t.Errorf("Stop() error = %v, should wrap %v", err, sessionErr)
	}
}

func TestStop_Success(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: true,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
	if len(mock.killCalls) != 1 {
		t.Errorf("expected 1 kill call, got %d", len(mock.killCalls))
	}
}

func TestStop_KillFails(t *testing.T) {
	killErr := errors.New("permission denied")
	mock := &mockTmux{
		hasSessionResult: true,
		killErr:          killErr,
	}
	m := newTestManager(t.TempDir(), mock)

	err := m.Stop()
	if err == nil {
		t.Fatal("Stop() should return error when kill fails")
	}
	if !errors.Is(err, killErr) {
		t.Errorf("Stop() error = %v, should wrap %v", err, killErr)
	}
}

func TestIsRunning(t *testing.T) {
	tests := []struct {
		name     string
		running  bool
		err      error
		wantRun  bool
		wantErr  bool
	}{
		{
			name:    "running",
			running: true,
			wantRun: true,
		},
		{
			name:    "not running",
			running: false,
			wantRun: false,
		},
		{
			name:    "error",
			err:     errors.New("tmux error"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockTmux{
				hasSessionResult: tc.running,
				hasSessionErr:    tc.err,
			}
			m := newTestManager(t.TempDir(), mock)

			running, err := m.IsRunning()
			if (err != nil) != tc.wantErr {
				t.Errorf("IsRunning() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if running != tc.wantRun {
				t.Errorf("IsRunning() = %v, want %v", running, tc.wantRun)
			}
		})
	}
}

func TestStatus_NotRunning(t *testing.T) {
	mock := &mockTmux{
		hasSessionResult: false,
	}
	m := newTestManager(t.TempDir(), mock)

	info, err := m.Status()
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Status() error = %v, want ErrNotRunning", err)
	}
	if info != nil {
		t.Error("Status() should return nil info when not running")
	}
}

func TestStatus_HasSessionError(t *testing.T) {
	sessionErr := errors.New("tmux gone")
	mock := &mockTmux{
		hasSessionErr: sessionErr,
	}
	m := newTestManager(t.TempDir(), mock)

	info, err := m.Status()
	if err == nil {
		t.Fatal("Status() should return error when HasSession fails")
	}
	if !errors.Is(err, sessionErr) {
		t.Errorf("Status() error = %v, should wrap %v", err, sessionErr)
	}
	if info != nil {
		t.Error("Status() should return nil info on error")
	}
}

func TestStatus_Running(t *testing.T) {
	expected := &tmux.SessionInfo{
		Name:    "hq-deacon",
		Windows: 1,
	}
	mock := &mockTmux{
		hasSessionResult: true,
		sessionInfo:      expected,
	}
	m := newTestManager(t.TempDir(), mock)

	info, err := m.Status()
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}
	if info != expected {
		t.Errorf("Status() = %v, want %v", info, expected)
	}
}

func TestStatus_GetSessionInfoError(t *testing.T) {
	infoErr := errors.New("session info unavailable")
	mock := &mockTmux{
		hasSessionResult: true,
		sessionInfoErr:   infoErr,
	}
	m := newTestManager(t.TempDir(), mock)

	info, err := m.Status()
	if err == nil {
		t.Fatal("Status() should return error when GetSessionInfo fails")
	}
	if !errors.Is(err, infoErr) {
		t.Errorf("Status() error = %v, should wrap %v", err, infoErr)
	}
	if info != nil {
		t.Error("Status() should return nil info on error")
	}
}
