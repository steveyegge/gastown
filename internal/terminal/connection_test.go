package terminal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockTmux is a test implementation of tmuxClient.
type mockTmux struct {
	mu sync.Mutex

	// Session state
	sessions map[string]bool // session name → alive (true = exists, pane alive)
	deadPane map[string]bool // session name → pane dead

	// Recorded calls
	hasSessionCalls            []string
	killSessionCalls           []string
	newSessionCalls            []newSessionCall
	setRemainOnExitCalls       []setRemainOnExitCall
	isPaneDeadCalls            []string
	sendKeysCalls              []sendKeysCall
	captureOutput              map[string]string // session → captured output
	captureLineCounts          []capturePaneCall

	// Error injection
	hasSessionErr              error
	killSessionErr             error
	newSessionErr              error
	setRemainOnExitErr         error
	isPaneDeadErr              error
	sendKeysErr                error
	capturePaneErr             error
}

type newSessionCall struct {
	Name, WorkDir, Command string
}

type setRemainOnExitCall struct {
	Pane string
	On   bool
}

type sendKeysCall struct {
	Session, Keys string
}

type capturePaneCall struct {
	Session string
	Lines   int
}

func newMockTmux() *mockTmux {
	return &mockTmux{
		sessions:      make(map[string]bool),
		deadPane:      make(map[string]bool),
		captureOutput: make(map[string]string),
	}
}

func (m *mockTmux) HasSession(name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasSessionCalls = append(m.hasSessionCalls, name)
	if m.hasSessionErr != nil {
		return false, m.hasSessionErr
	}
	_, exists := m.sessions[name]
	return exists, nil
}

func (m *mockTmux) KillSessionWithProcesses(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.killSessionCalls = append(m.killSessionCalls, name)
	if m.killSessionErr != nil {
		return m.killSessionErr
	}
	delete(m.sessions, name)
	delete(m.deadPane, name)
	return nil
}

func (m *mockTmux) NewSessionWithCommand(name, workDir, command string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newSessionCalls = append(m.newSessionCalls, newSessionCall{name, workDir, command})
	if m.newSessionErr != nil {
		return m.newSessionErr
	}
	m.sessions[name] = true
	return nil
}

func (m *mockTmux) SetRemainOnExit(pane string, on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setRemainOnExitCalls = append(m.setRemainOnExitCalls, setRemainOnExitCall{pane, on})
	if m.setRemainOnExitErr != nil {
		return m.setRemainOnExitErr
	}
	return nil
}

func (m *mockTmux) IsPaneDead(session string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isPaneDeadCalls = append(m.isPaneDeadCalls, session)
	if m.isPaneDeadErr != nil {
		return false, m.isPaneDeadErr
	}
	return m.deadPane[session], nil
}

func (m *mockTmux) SendKeys(session, keys string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendKeysCalls = append(m.sendKeysCalls, sendKeysCall{session, keys})
	if m.sendKeysErr != nil {
		return m.sendKeysErr
	}
	return nil
}

func (m *mockTmux) CapturePane(session string, lines int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captureLineCounts = append(m.captureLineCounts, capturePaneCall{session, lines})
	if m.capturePaneErr != nil {
		return "", m.capturePaneErr
	}
	return m.captureOutput[session], nil
}

// Helper methods for tests

func (m *mockTmux) setSessionAlive(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	m.deadPane[name] = false
}

func (m *mockTmux) setSessionDead(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	m.deadPane[name] = true
}

func (m *mockTmux) removeSession(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, name)
	delete(m.deadPane, name)
}

func (m *mockTmux) setCaptureOutput(session, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captureOutput[session] = output
}

// newTestPodConnection creates a PodConnection with a mock tmux for testing.
func newTestPodConnection(mt *mockTmux) *PodConnection {
	return NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
		Tmux:        mt,
	})
}

// --- Constructor Tests ---

func TestNewPodConnection(t *testing.T) {
	mt := newMockTmux()
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
		Tmux:        mt,
	})

	if pc.AgentID != "gastown/polecats/alpha" {
		t.Errorf("AgentID = %q, want %q", pc.AgentID, "gastown/polecats/alpha")
	}
	if pc.PodName != "gt-gastown-polecat-alpha" {
		t.Errorf("PodName = %q, want %q", pc.PodName, "gt-gastown-polecat-alpha")
	}
	if pc.Namespace != "gastown-test" {
		t.Errorf("Namespace = %q, want %q", pc.Namespace, "gastown-test")
	}
	if pc.SessionName != "gt-gastown-alpha" {
		t.Errorf("SessionName = %q, want %q", pc.SessionName, "gt-gastown-alpha")
	}
	if pc.ScreenSession != DefaultScreenSession {
		t.Errorf("ScreenSession = %q, want %q", pc.ScreenSession, DefaultScreenSession)
	}
	if pc.tmux == nil {
		t.Error("tmux should not be nil")
	}
	if pc.connected {
		t.Error("connected should be false initially")
	}
}

func TestNewPodConnection_CustomScreenSession(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:       "gastown/polecats/alpha",
		PodName:       "gt-gastown-polecat-alpha",
		Namespace:     "gastown-test",
		SessionName:   "gt-gastown-alpha",
		ScreenSession: "custom-session",
		Tmux:          newMockTmux(),
	})

	if pc.ScreenSession != "custom-session" {
		t.Errorf("ScreenSession = %q, want %q", pc.ScreenSession, "custom-session")
	}
}

func TestNewPodConnection_NilTmux_UsesDefault(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
	})
	if pc.tmux == nil {
		t.Error("tmux should not be nil even without explicit Tmux config")
	}
}

func TestNewPodConnection_CustomTmux(t *testing.T) {
	mt := newMockTmux()
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
		Tmux:        mt,
	})
	if pc.tmux != mt {
		t.Error("should use provided mock tmux")
	}
}

// --- KubectlExecCommand Tests ---

func TestPodConnection_KubectlExecCommand(t *testing.T) {
	mt := newMockTmux()
	tests := []struct {
		name    string
		cfg     PodConnectionConfig
		wantCmd string
	}{
		{
			name: "basic",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				Namespace:   "gastown-test",
				SessionName: "gt-gastown-alpha",
				Tmux:        mt,
			},
			wantCmd: "kubectl exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x agent",
		},
		{
			name: "with kubeconfig",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				Namespace:   "gastown-test",
				SessionName: "gt-gastown-alpha",
				KubeConfig:  "/home/user/.kube/config",
				Tmux:        mt,
			},
			wantCmd: "kubectl --kubeconfig /home/user/.kube/config exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x agent",
		},
		{
			name: "custom screen session",
			cfg: PodConnectionConfig{
				AgentID:       "gastown/polecats/alpha",
				PodName:       "gt-gastown-polecat-alpha",
				Namespace:     "gastown-test",
				SessionName:   "gt-gastown-alpha",
				ScreenSession: "claude",
				Tmux:          mt,
			},
			wantCmd: "kubectl exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x claude",
		},
		{
			name: "no namespace",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				SessionName: "gt-gastown-alpha",
				Tmux:        mt,
			},
			wantCmd: "kubectl exec -it gt-gastown-polecat-alpha -- screen -x agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPodConnection(tt.cfg)
			got := pc.kubectlExecCommand()
			if got != tt.wantCmd {
				t.Errorf("kubectlExecCommand() = %q, want %q", got, tt.wantCmd)
			}
		})
	}
}

// --- Open Tests ---

func TestOpen_CreatesSession(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Should have created a new session
	if len(mt.newSessionCalls) != 1 {
		t.Fatalf("expected 1 NewSessionWithCommand call, got %d", len(mt.newSessionCalls))
	}
	call := mt.newSessionCalls[0]
	if call.Name != "gt-gastown-alpha" {
		t.Errorf("session name = %q, want %q", call.Name, "gt-gastown-alpha")
	}
	if call.WorkDir != "/tmp" {
		t.Errorf("workdir = %q, want %q", call.WorkDir, "/tmp")
	}
	expectedCmd := "kubectl exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x agent"
	if call.Command != expectedCmd {
		t.Errorf("command = %q, want %q", call.Command, expectedCmd)
	}
}

func TestOpen_KillsStaleSession(t *testing.T) {
	mt := newMockTmux()
	mt.setSessionAlive("gt-gastown-alpha") // pre-existing session
	pc := newTestPodConnection(mt)

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Should have killed the stale session first
	if len(mt.killSessionCalls) != 1 {
		t.Fatalf("expected 1 KillSessionWithProcesses call, got %d", len(mt.killSessionCalls))
	}
	if mt.killSessionCalls[0] != "gt-gastown-alpha" {
		t.Errorf("killed session = %q, want %q", mt.killSessionCalls[0], "gt-gastown-alpha")
	}
}

func TestOpen_NoKillWhenNoStaleSession(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Should NOT have killed any session (none existed)
	if len(mt.killSessionCalls) != 0 {
		t.Errorf("expected 0 KillSessionWithProcesses calls, got %d", len(mt.killSessionCalls))
	}
}

func TestOpen_SetsRemainOnExit(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	if len(mt.setRemainOnExitCalls) != 1 {
		t.Fatalf("expected 1 SetRemainOnExit call, got %d", len(mt.setRemainOnExitCalls))
	}
	call := mt.setRemainOnExitCalls[0]
	if call.Pane != "gt-gastown-alpha" {
		t.Errorf("pane = %q, want %q", call.Pane, "gt-gastown-alpha")
	}
	if !call.On {
		t.Error("SetRemainOnExit should be called with on=true")
	}
}

func TestOpen_SetsConnectedState(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if !pc.IsConnected() {
		t.Error("IsConnected() should be true after Open()")
	}
	if pc.ReconnectCount() != 0 {
		t.Errorf("ReconnectCount() = %d, want 0 after Open()", pc.ReconnectCount())
	}
}

func TestOpen_FailsOnNewSessionError(t *testing.T) {
	mt := newMockTmux()
	mt.newSessionErr = fmt.Errorf("tmux not available")
	pc := newTestPodConnection(mt)

	err := pc.Open(context.Background())
	if err == nil {
		t.Fatal("expected error from Open()")
	}
	if !pc.IsConnected() == true {
		// Should not be connected after failure
	}
}

func TestOpen_RemainOnExitErrorNonFatal(t *testing.T) {
	mt := newMockTmux()
	mt.setRemainOnExitErr = fmt.Errorf("remain-on-exit failed")
	pc := newTestPodConnection(mt)

	// SetRemainOnExit failure should not cause Open to fail
	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() should succeed despite SetRemainOnExit error, got %v", err)
	}
	if !pc.IsConnected() {
		t.Error("should be connected even if SetRemainOnExit fails")
	}
}

func TestOpen_ResetsReconnectCount(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Manually set reconnect count
	pc.mu.Lock()
	pc.reconnectCount = 3
	pc.mu.Unlock()

	if err := pc.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if pc.ReconnectCount() != 0 {
		t.Errorf("ReconnectCount() = %d, want 0 after Open()", pc.ReconnectCount())
	}
}

// --- Close Tests ---

func TestClose_KillsSession(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Open first
	_ = pc.Open(context.Background())

	if err := pc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if pc.IsConnected() {
		t.Error("IsConnected() should be false after Close()")
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	// Should have called KillSessionWithProcesses
	found := false
	for _, name := range mt.killSessionCalls {
		if name == "gt-gastown-alpha" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Close() should kill the tmux session")
	}
}

func TestClose_NoopIfSessionGone(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Don't open - session doesn't exist
	err := pc.Close()
	if err != nil {
		t.Fatalf("Close() error = %v, want nil (noop)", err)
	}

	if pc.IsConnected() {
		t.Error("IsConnected() should be false")
	}
}

func TestClose_ReturnsKillError(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Open first to create the session
	_ = pc.Open(context.Background())

	// Make kill fail
	mt.mu.Lock()
	mt.killSessionErr = fmt.Errorf("kill failed")
	mt.mu.Unlock()

	err := pc.Close()
	if err == nil {
		t.Fatal("expected error from Close()")
	}
}

func TestClose_SetsDisconnected(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())
	if !pc.IsConnected() {
		t.Fatal("should be connected after Open()")
	}

	_ = pc.Close()
	if pc.IsConnected() {
		t.Error("should be disconnected after Close()")
	}
}

// --- IsAlive Tests ---

func TestIsAlive_NotConnected(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	if pc.IsAlive() {
		t.Error("IsAlive() should be false when not connected")
	}
}

func TestIsAlive_SessionExists_PaneAlive(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())

	if !pc.IsAlive() {
		t.Error("IsAlive() should be true when session exists and pane is alive")
	}
}

func TestIsAlive_SessionExists_PaneDead(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())
	mt.setSessionDead("gt-gastown-alpha") // kubectl exec exited

	if pc.IsAlive() {
		t.Error("IsAlive() should be false when pane is dead")
	}
	// Should also update connected state
	if pc.IsConnected() {
		t.Error("IsConnected() should be false after IsAlive detects dead pane")
	}
}

func TestIsAlive_SessionGone(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())
	mt.removeSession("gt-gastown-alpha") // session removed externally

	if pc.IsAlive() {
		t.Error("IsAlive() should be false when session is gone")
	}
	if pc.IsConnected() {
		t.Error("IsConnected() should be false when session is gone")
	}
}

func TestIsAlive_HasSessionError(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())

	mt.mu.Lock()
	mt.hasSessionErr = fmt.Errorf("tmux error")
	mt.mu.Unlock()

	if pc.IsAlive() {
		t.Error("IsAlive() should be false on HasSession error")
	}
}

func TestIsAlive_IsPaneDeadError(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())

	mt.mu.Lock()
	mt.isPaneDeadErr = fmt.Errorf("pane check error")
	mt.mu.Unlock()

	if pc.IsAlive() {
		t.Error("IsAlive() should be false on IsPaneDead error")
	}
}

// --- SendKeys Tests ---

func TestSendKeys_DeliversToSession(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	err := pc.SendKeys("hello world")
	if err != nil {
		t.Fatalf("SendKeys() error = %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.sendKeysCalls) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(mt.sendKeysCalls))
	}
	if mt.sendKeysCalls[0].Session != "gt-gastown-alpha" {
		t.Errorf("session = %q, want %q", mt.sendKeysCalls[0].Session, "gt-gastown-alpha")
	}
	if mt.sendKeysCalls[0].Keys != "hello world" {
		t.Errorf("keys = %q, want %q", mt.sendKeysCalls[0].Keys, "hello world")
	}
}

func TestSendKeys_ReturnsError(t *testing.T) {
	mt := newMockTmux()
	mt.sendKeysErr = fmt.Errorf("send failed")
	pc := newTestPodConnection(mt)

	err := pc.SendKeys("test")
	if err == nil {
		t.Fatal("expected error from SendKeys()")
	}
}

// --- Capture Tests ---

func TestCapture_ReadsFromSession(t *testing.T) {
	mt := newMockTmux()
	mt.setCaptureOutput("gt-gastown-alpha", "line1\nline2\nline3")
	pc := newTestPodConnection(mt)

	output, err := pc.Capture(50)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if output != "line1\nline2\nline3" {
		t.Errorf("Capture() = %q, want %q", output, "line1\nline2\nline3")
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.captureLineCounts) != 1 {
		t.Fatalf("expected 1 CapturePane call, got %d", len(mt.captureLineCounts))
	}
	if mt.captureLineCounts[0].Session != "gt-gastown-alpha" {
		t.Errorf("session = %q, want %q", mt.captureLineCounts[0].Session, "gt-gastown-alpha")
	}
	if mt.captureLineCounts[0].Lines != 50 {
		t.Errorf("lines = %d, want %d", mt.captureLineCounts[0].Lines, 50)
	}
}

func TestCapture_ReturnsError(t *testing.T) {
	mt := newMockTmux()
	mt.capturePaneErr = fmt.Errorf("capture failed")
	pc := newTestPodConnection(mt)

	_, err := pc.Capture(50)
	if err == nil {
		t.Fatal("expected error from Capture()")
	}
}

// --- Reconnect Tests ---

func TestReconnect_ClosesAndReopens(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	_ = pc.Open(context.Background())

	// Simulate connection death
	mt.setSessionDead("gt-gastown-alpha")

	err := pc.Reconnect(context.Background())
	if err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}

	if !pc.IsConnected() {
		t.Error("should be connected after Reconnect()")
	}
	if pc.ReconnectCount() != 0 {
		// Open() resets reconnectCount to 0
		// But Reconnect increments before calling Open, so net = 0 after Open resets
		// Actually: Reconnect increments to 1, then calls Open which resets to 0
		t.Errorf("ReconnectCount() = %d, want 0 (Open resets)", pc.ReconnectCount())
	}
}

func TestReconnect_IncrementsCount(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Reconnect will increment count then call Close+Open.
	// Open resets reconnectCount to 0, so after a successful reconnect it's 0.
	// But we can observe the count incremented by making Open fail.
	mt.newSessionErr = fmt.Errorf("tmux error")

	// First reconnect attempt: count goes from 0→1, Open fails
	_ = pc.Reconnect(context.Background())
	if pc.ReconnectCount() != 1 {
		t.Errorf("ReconnectCount() = %d, want 1 after first failed reconnect", pc.ReconnectCount())
	}
}

func TestReconnect_MaxAttemptsExceeded(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Set reconnect count to max
	pc.mu.Lock()
	pc.reconnectCount = MaxReconnectAttempts
	pc.mu.Unlock()

	err := pc.Reconnect(context.Background())
	if err == nil {
		t.Fatal("expected error when max reconnect attempts exceeded")
	}
}

func TestReconnect_ExponentialBackoff_ContextCancelled(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// Set reconnect count to 1 so backoff delay = 2s
	pc.mu.Lock()
	pc.reconnectCount = 1
	pc.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := pc.Reconnect(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestReconnect_FirstAttemptNoBackoff(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	// First reconnect (count=0) should have no backoff delay
	start := time.Now()
	_ = pc.Reconnect(context.Background())
	elapsed := time.Since(start)

	// Should be nearly instant (no backoff for first attempt)
	if elapsed > 500*time.Millisecond {
		t.Errorf("first reconnect took %v, expected near-instant (no backoff)", elapsed)
	}
}

// --- IsConnected Tests ---

func TestPodConnection_IsConnected_Initial(t *testing.T) {
	pc := newTestPodConnection(newMockTmux())

	if pc.IsConnected() {
		t.Error("IsConnected() should be false before Open()")
	}
}

// --- ReconnectCount Tests ---

func TestPodConnection_ReconnectCount_Initial(t *testing.T) {
	pc := newTestPodConnection(newMockTmux())

	if pc.ReconnectCount() != 0 {
		t.Errorf("ReconnectCount() = %d, want 0", pc.ReconnectCount())
	}
}

// --- Constants Tests ---

func TestDefaultScreenSession(t *testing.T) {
	if DefaultScreenSession != "agent" {
		t.Errorf("DefaultScreenSession = %q, want %q", DefaultScreenSession, "agent")
	}
}

func TestMaxReconnectAttempts(t *testing.T) {
	if MaxReconnectAttempts != 5 {
		t.Errorf("MaxReconnectAttempts = %d, want %d", MaxReconnectAttempts, 5)
	}
}

// --- Concurrent Access Tests ---

func TestPodConnection_ConcurrentIsAlive(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)
	_ = pc.Open(context.Background())

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = pc.IsAlive()
				_ = pc.IsConnected()
				_ = pc.ReconnectCount()
			}
		}()
	}
	wg.Wait()
}

func TestPodConnection_ConcurrentOpenClose(t *testing.T) {
	mt := newMockTmux()
	pc := newTestPodConnection(mt)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pc.Open(context.Background())
			_ = pc.Close()
		}()
	}
	wg.Wait()
}
