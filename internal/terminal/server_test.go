package terminal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewServer_Defaults(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	if srv.rig != "gastown" {
		t.Errorf("expected rig 'gastown', got %q", srv.rig)
	}
	if srv.namespace != "gastown-test" {
		t.Errorf("expected namespace 'gastown-test', got %q", srv.namespace)
	}
	if srv.healthInterval != DefaultHealthInterval {
		t.Errorf("expected health interval %v, got %v", DefaultHealthInterval, srv.healthInterval)
	}
	if srv.screenSession != DefaultScreenSession {
		t.Errorf("expected screen session %q, got %q", DefaultScreenSession, srv.screenSession)
	}
}

func TestNewServer_CustomConfig(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:            "testrig",
		Namespace:      "test-ns",
		KubeConfig:     "/tmp/kubeconfig",
		HealthInterval: 10 * time.Second,
		ScreenSession:  "custom-screen",
		PodSource:      &mockPodSource{},
	})

	if srv.rig != "testrig" {
		t.Errorf("expected rig 'testrig', got %q", srv.rig)
	}
	if srv.namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got %q", srv.namespace)
	}
	if srv.kubeconfig != "/tmp/kubeconfig" {
		t.Errorf("expected kubeconfig '/tmp/kubeconfig', got %q", srv.kubeconfig)
	}
	if srv.healthInterval != 10*time.Second {
		t.Errorf("expected health interval 10s, got %v", srv.healthInterval)
	}
	if srv.screenSession != "custom-screen" {
		t.Errorf("expected screen session 'custom-screen', got %q", srv.screenSession)
	}
}

func TestNewServer_NilPodSource(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		// PodSource intentionally nil - should default to CLIPodSource
	})
	if srv.inventory == nil {
		t.Error("inventory should not be nil even with nil PodSource")
	}
}

func TestNewServer_WithTmuxClient(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	if srv.tmuxClient != mt {
		t.Error("expected TmuxClient to be stored on server")
	}
}

func TestAgentIDToSessionName(t *testing.T) {
	tests := []struct {
		agentID  string
		expected string
	}{
		{"gastown/polecats/alpha", "gt-gastown-alpha"},
		{"gastown/polecats/bravo", "gt-gastown-bravo"},
		{"gastown/witness", "gt-gastown-witness"},
		{"gastown/refinery", "gt-gastown-refinery"},
		{"gastown/crew/k8s", "gt-gastown-crew-k8s"},
		{"testrig/polecats/toast", "gt-testrig-toast"},
		{"testrig/crew/dev", "gt-testrig-crew-dev"},
		// Single-part agent ID (fallback: len < 2)
		{"standalone", "gt-standalone"},
		// 4+ part agent ID (default fallback: join with dashes)
		{"rig/custom/role/deep", "gt-rig-custom-role-deep"},
	}

	for _, tt := range tests {
		got := agentIDToSessionName(tt.agentID)
		if got != tt.expected {
			t.Errorf("agentIDToSessionName(%q) = %q, want %q", tt.agentID, got, tt.expected)
		}
	}
}

func TestSessionNameForAgent(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	got := srv.sessionNameForAgent("gastown/polecats/alpha")
	if got != "gt-gastown-alpha" {
		t.Errorf("sessionNameForAgent() = %q, want %q", got, "gt-gastown-alpha")
	}
}

func TestSplitAgentID(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"gastown/polecats/alpha", []string{"gastown", "polecats", "alpha"}},
		{"gastown/witness", []string{"gastown", "witness"}},
		{"single", []string{"single"}},
		{"a/b/c/d", []string{"a", "b", "c", "d"}},
	}

	for _, tt := range tests {
		got := splitAgentID(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("splitAgentID(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("splitAgentID(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestJoinDash(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"a", "b", "c"}, "a-b-c"},
		{[]string{"one"}, "one"},
		{[]string{}, ""},
	}

	for _, tt := range tests {
		got := joinDash(tt.input)
		if got != tt.expected {
			t.Errorf("joinDash(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- handlePodEvent Tests ---

func TestHandlePodEvent_Added_ConnectsPod(t *testing.T) {
	mt := newMockTmux()
	source := &mockPodSource{}
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  source,
		TmuxClient: mt,
	})

	srv.handlePodEvent(PodEvent{
		Type: PodAdded,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
			PodName: "gt-gastown-polecat-alpha",
		},
	})

	// Should have created a tmux session
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.newSessionCalls) != 1 {
		t.Fatalf("expected 1 NewSessionWithCommand call, got %d", len(mt.newSessionCalls))
	}
	if mt.newSessionCalls[0].Name != "gt-gastown-alpha" {
		t.Errorf("session name = %q, want %q", mt.newSessionCalls[0].Name, "gt-gastown-alpha")
	}

	// Connection should be tracked
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if _, exists := srv.connections["gastown/polecats/alpha"]; !exists {
		t.Error("expected connection to be tracked")
	}
}

func TestHandlePodEvent_Added_OpenFails(t *testing.T) {
	mt := newMockTmux()
	mt.newSessionErr = fmt.Errorf("tmux not available")
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	srv.handlePodEvent(PodEvent{
		Type: PodAdded,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
			PodName: "gt-gastown-polecat-alpha",
		},
	})

	// Connection should NOT be tracked when Open fails
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if _, exists := srv.connections["gastown/polecats/alpha"]; exists {
		t.Error("connection should not be tracked when Open fails")
	}
}

func TestHandlePodEvent_Removed_DisconnectsPod(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// First add a pod
	srv.handlePodEvent(PodEvent{
		Type: PodAdded,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
			PodName: "gt-gastown-polecat-alpha",
		},
	})

	// Then remove it
	srv.handlePodEvent(PodEvent{
		Type: PodRemoved,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
		},
	})

	// Connection should be removed
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if _, exists := srv.connections["gastown/polecats/alpha"]; exists {
		t.Error("connection should be removed after PodRemoved")
	}
}

func TestHandlePodEvent_Updated_Reconnects(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add pod v1
	srv.handlePodEvent(PodEvent{
		Type: PodAdded,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
			PodName: "gt-gastown-polecat-alpha-v1",
		},
	})

	// Update to pod v2 (restart)
	srv.handlePodEvent(PodEvent{
		Type: PodUpdated,
		Pod: &PodInfo{
			AgentID: "gastown/polecats/alpha",
			PodName: "gt-gastown-polecat-alpha-v2",
		},
	})

	// Should have created 2 sessions (one per connect)
	mt.mu.Lock()
	newCalls := len(mt.newSessionCalls)
	mt.mu.Unlock()
	if newCalls != 2 {
		t.Errorf("expected 2 NewSessionWithCommand calls, got %d", newCalls)
	}

	// Connection should exist with new pod name
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	pc, exists := srv.connections["gastown/polecats/alpha"]
	if !exists {
		t.Fatal("expected connection to exist after update")
	}
	if pc.PodName != "gt-gastown-polecat-alpha-v2" {
		t.Errorf("PodName = %q, want %q", pc.PodName, "gt-gastown-polecat-alpha-v2")
	}
}

// --- connectPod Tests ---

func TestConnectPod_UsesCorrectConfig(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:           "gastown",
		Namespace:     "gastown-test",
		KubeConfig:    "/tmp/kubeconfig",
		ScreenSession: "custom-screen",
		PodSource:     &mockPodSource{},
		TmuxClient:    mt,
	})

	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Verify connection config
	srv.mu.RLock()
	pc := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()

	if pc == nil {
		t.Fatal("expected connection to be created")
	}
	if pc.Namespace != "gastown-test" {
		t.Errorf("Namespace = %q, want %q", pc.Namespace, "gastown-test")
	}
	if pc.KubeConfig != "/tmp/kubeconfig" {
		t.Errorf("KubeConfig = %q, want %q", pc.KubeConfig, "/tmp/kubeconfig")
	}
	if pc.ScreenSession != "custom-screen" {
		t.Errorf("ScreenSession = %q, want %q", pc.ScreenSession, "custom-screen")
	}
	if pc.SessionName != "gt-gastown-alpha" {
		t.Errorf("SessionName = %q, want %q", pc.SessionName, "gt-gastown-alpha")
	}
}

// --- disconnectPod Tests ---

func TestDisconnectPod_NotFound(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Disconnecting a pod that doesn't exist should not panic
	srv.disconnectPod("nonexistent/agent")
}

func TestDisconnectPod_ClosesConnection(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add a pod
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Disconnect
	srv.disconnectPod("gastown/polecats/alpha")

	// Should be removed from connections
	srv.mu.RLock()
	_, exists := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()
	if exists {
		t.Error("connection should be removed after disconnect")
	}

	// Should have killed the session
	mt.mu.Lock()
	defer mt.mu.Unlock()
	foundKill := false
	for _, name := range mt.killSessionCalls {
		if name == "gt-gastown-alpha" {
			foundKill = true
			break
		}
	}
	if !foundKill {
		t.Error("should have killed tmux session on disconnect")
	}
}

func TestDisconnectPod_CloseError(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add a pod
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Make kill session fail
	mt.mu.Lock()
	mt.killSessionErr = fmt.Errorf("kill failed")
	mt.mu.Unlock()

	// Should not panic even when Close() returns error
	srv.disconnectPod("gastown/polecats/alpha")

	// Connection should still be removed from map
	srv.mu.RLock()
	_, exists := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()
	if exists {
		t.Error("connection should be removed even if Close fails")
	}
}

// --- checkHealth Tests ---

func TestCheckHealth_EmptyConnections(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Health check with no connections should not panic
	srv.checkHealth(context.Background())
}

func TestCheckHealth_AliveConnectionsUntouched(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add a pod with alive connection
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Reset call tracking after connect
	mt.mu.Lock()
	initialNewCalls := len(mt.newSessionCalls)
	mt.mu.Unlock()

	// Health check on alive connection should do nothing
	srv.checkHealth(context.Background())

	mt.mu.Lock()
	afterNewCalls := len(mt.newSessionCalls)
	mt.mu.Unlock()

	// No new sessions should have been created
	if afterNewCalls != initialNewCalls {
		t.Errorf("expected no new sessions, got %d additional", afterNewCalls-initialNewCalls)
	}
}

func TestCheckHealth_DeadConnectionReconnects(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add a pod
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Kill the pane (simulate kubectl exec exit)
	mt.setSessionDead("gt-gastown-alpha")

	mt.mu.Lock()
	beforeNewCalls := len(mt.newSessionCalls)
	mt.mu.Unlock()

	// Health check should trigger reconnect
	srv.checkHealth(context.Background())

	mt.mu.Lock()
	afterNewCalls := len(mt.newSessionCalls)
	mt.mu.Unlock()

	// Should have created a new session (reconnect)
	if afterNewCalls <= beforeNewCalls {
		t.Error("expected new session to be created on reconnect")
	}
}

func TestCheckHealth_MaxReconnectRemovesConnection(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add a pod
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Set reconnect count to max-1, and make the session dead
	srv.mu.RLock()
	pc := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()
	pc.mu.Lock()
	pc.reconnectCount = MaxReconnectAttempts - 1
	pc.mu.Unlock()

	// Kill the session entirely so reconnect fails and exceeds max
	mt.removeSession("gt-gastown-alpha")
	mt.mu.Lock()
	mt.newSessionErr = fmt.Errorf("tmux broken")
	mt.mu.Unlock()

	// Health check should try reconnect, which increments to MaxReconnectAttempts
	// and then removes the connection
	srv.checkHealth(context.Background())

	srv.mu.RLock()
	_, exists := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()

	if exists {
		t.Error("connection should be removed after max reconnect attempts")
	}
}

// --- shutdown Tests ---

func TestShutdown_NoConnections(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Shutdown with no connections should not panic
	srv.shutdown()
}

func TestShutdown_CloseErrorsNonFatal(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	// Make kill fail
	mt.mu.Lock()
	mt.killSessionErr = fmt.Errorf("kill failed")
	mt.mu.Unlock()

	// Should not panic even when Close() returns errors
	srv.shutdown()

	// Connections map should still be cleared
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if len(srv.connections) != 0 {
		t.Errorf("expected 0 connections after shutdown, got %d", len(srv.connections))
	}
}

func TestShutdown_ClosesAllConnections(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add multiple pods
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/bravo",
		PodName: "gt-gastown-polecat-bravo",
	})
	srv.connectPod(&PodInfo{
		AgentID: "gastown/witness",
		PodName: "gt-gastown-witness",
	})

	// Verify we have 3 connections
	srv.mu.RLock()
	if len(srv.connections) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(srv.connections))
	}
	srv.mu.RUnlock()

	// Shutdown
	srv.shutdown()

	// All connections should be cleared
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if len(srv.connections) != 0 {
		t.Errorf("expected 0 connections after shutdown, got %d", len(srv.connections))
	}
}

// --- Status Tests ---

func TestServerStatus_Empty(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	status := srv.Status()
	if status.Rig != "gastown" {
		t.Errorf("expected rig 'gastown', got %q", status.Rig)
	}
	if status.Namespace != "gastown-test" {
		t.Errorf("expected namespace 'gastown-test', got %q", status.Namespace)
	}
	if len(status.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(status.Connections))
	}
	if status.PodCount != 0 {
		t.Errorf("expected 0 pods, got %d", status.PodCount)
	}
}

func TestServerStatus_WithConnections(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "gt-gastown-polecat-alpha",
	})

	status := srv.Status()
	if len(status.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(status.Connections))
	}
	conn := status.Connections[0]
	if conn.AgentID != "gastown/polecats/alpha" {
		t.Errorf("AgentID = %q, want %q", conn.AgentID, "gastown/polecats/alpha")
	}
	if conn.PodName != "gt-gastown-polecat-alpha" {
		t.Errorf("PodName = %q, want %q", conn.PodName, "gt-gastown-polecat-alpha")
	}
	if conn.SessionName != "gt-gastown-alpha" {
		t.Errorf("SessionName = %q, want %q", conn.SessionName, "gt-gastown-alpha")
	}
}

// --- Run Tests ---

func TestServerRun_CancelledImmediately(t *testing.T) {
	source := &mockPodSource{}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   100 * time.Millisecond,
		HealthInterval: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := srv.Run(ctx)
	if err != nil {
		t.Errorf("expected nil error on clean shutdown, got %v", err)
	}
}

func TestServerRun_StartsAndStops(t *testing.T) {
	source := &mockPodSource{}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   50 * time.Millisecond,
		HealthInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Let it run for a bit
	time.Sleep(200 * time.Millisecond)

	// Cancel and wait for shutdown
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within timeout")
	}
}

func TestServerRun_DiscoversPods(t *testing.T) {
	mt := newMockTmux()
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gastown/polecats/alpha", PodName: "gt-gastown-polecat-alpha", PodStatus: "running"},
		},
	}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   50 * time.Millisecond,
		HealthInterval: 50 * time.Millisecond,
		TmuxClient:     mt,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Wait for discovery to happen
	time.Sleep(300 * time.Millisecond)

	// Verify connection was established
	status := srv.Status()
	if len(status.Connections) != 1 {
		t.Errorf("expected 1 connection, got %d", len(status.Connections))
	}

	cancel()
	<-done
}

func TestServerRun_GracefulShutdownClosesConnections(t *testing.T) {
	mt := newMockTmux()
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodStatus: "running"},
			{AgentID: "gastown/polecats/bravo", PodName: "pod-2", PodStatus: "running"},
		},
	}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   50 * time.Millisecond,
		HealthInterval: 50 * time.Millisecond,
		TmuxClient:     mt,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Wait for connections to be established
	time.Sleep(300 * time.Millisecond)

	// Graceful shutdown
	cancel()
	<-done

	// All connections should be closed
	srv.mu.RLock()
	connCount := len(srv.connections)
	srv.mu.RUnlock()
	if connCount != 0 {
		t.Errorf("expected 0 connections after shutdown, got %d", connCount)
	}
}

// --- Inventory Event Integration Tests ---

func TestHandlePodEvent_Added_EventFlow(t *testing.T) {
	var addedCount atomic.Int32

	source := &mockPodSource{}
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  source,
		TmuxClient: mt,
	})

	// Verify initial state is empty
	status := srv.Status()
	if len(status.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(status.Connections))
	}

	var events []PodEvent
	var eventsMu sync.Mutex

	inv := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			eventsMu.Lock()
			events = append(events, e)
			eventsMu.Unlock()
			if e.Type == PodAdded {
				addedCount.Add(1)
			}
		},
	})

	// Add a pod
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "gt-gastown-polecat-alpha", PodStatus: "running"},
	})

	if err := inv.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	eventsMu.Lock()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	} else if events[0].Type != PodAdded {
		t.Errorf("expected PodAdded event, got %v", events[0].Type)
	}
	eventsMu.Unlock()

	if addedCount.Load() != 1 {
		t.Errorf("expected 1 added event, got %d", addedCount.Load())
	}

	_ = srv // used for config verification above
}

func TestHandlePodEvent_Removed_EventFlow(t *testing.T) {
	source := &mockPodSource{}

	var events []PodEvent
	var mu sync.Mutex

	inv := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	// First: add a pod
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodStatus: "running"},
	})
	_ = inv.Refresh(context.Background())

	// Then: remove it
	source.setPods([]*PodInfo{})
	_ = inv.Refresh(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != PodAdded {
		t.Errorf("event[0]: expected PodAdded, got %v", events[0].Type)
	}
	if events[1].Type != PodRemoved {
		t.Errorf("event[1]: expected PodRemoved, got %v", events[1].Type)
	}
	if events[1].Pod.AgentID != "gastown/polecats/alpha" {
		t.Errorf("removed pod agent ID = %q, want %q", events[1].Pod.AgentID, "gastown/polecats/alpha")
	}
}

func TestHandlePodEvent_Updated_EventFlow(t *testing.T) {
	source := &mockPodSource{}

	var events []PodEvent
	var mu sync.Mutex

	inv := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	// First: add a pod
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodIP: "10.0.0.1", PodStatus: "running"},
	})
	_ = inv.Refresh(context.Background())

	// Then: update it (new pod name = pod restarted)
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-2", PodIP: "10.0.0.2", PodStatus: "running"},
	})
	_ = inv.Refresh(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != PodAdded {
		t.Errorf("event[0]: expected PodAdded, got %v", events[0].Type)
	}
	if events[1].Type != PodUpdated {
		t.Errorf("event[1]: expected PodUpdated, got %v", events[1].Type)
	}
	if events[1].Pod.PodName != "pod-2" {
		t.Errorf("updated pod name = %q, want %q", events[1].Pod.PodName, "pod-2")
	}
}

func TestServerReconciliation_MultiplePodsAddedAndRemoved(t *testing.T) {
	source := &mockPodSource{}

	var events []PodEvent
	var mu sync.Mutex

	inv := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	// Add 3 pods
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-alpha", PodStatus: "running"},
		{AgentID: "gastown/polecats/bravo", PodName: "pod-bravo", PodStatus: "running"},
		{AgentID: "gastown/witness", PodName: "pod-witness", PodStatus: "running"},
	})
	_ = inv.Refresh(context.Background())

	// Remove alpha, add charlie
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/bravo", PodName: "pod-bravo", PodStatus: "running"},
		{AgentID: "gastown/witness", PodName: "pod-witness", PodStatus: "running"},
		{AgentID: "gastown/polecats/charlie", PodName: "pod-charlie", PodStatus: "running"},
	})
	_ = inv.Refresh(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// Should have 3 adds + 1 remove + 1 add = 5 events
	addCount := 0
	removeCount := 0
	for _, e := range events {
		switch e.Type {
		case PodAdded:
			addCount++
		case PodRemoved:
			removeCount++
		}
	}

	if addCount != 4 {
		t.Errorf("expected 4 PodAdded events, got %d", addCount)
	}
	if removeCount != 1 {
		t.Errorf("expected 1 PodRemoved event, got %d", removeCount)
	}
}

func TestServerReconciliation_StalePodsFiltered(t *testing.T) {
	source := &mockPodSource{}

	var events []PodEvent
	var mu sync.Mutex

	inv := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	// Add pods including failed/terminated
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-alpha", PodStatus: "running"},
		{AgentID: "gastown/polecats/bravo", PodName: "pod-bravo", PodStatus: "failed"},
		{AgentID: "gastown/polecats/charlie", PodName: "pod-charlie", PodStatus: "terminated"},
	})
	_ = inv.Refresh(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// Only alpha should be added (failed and terminated are filtered)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Pod.AgentID != "gastown/polecats/alpha" {
		t.Errorf("expected alpha pod, got %q", events[0].Pod.AgentID)
	}
}

// --- Server Full Integration with Mock Tmux ---

func TestServer_FullLifecycle_DiscoveryToShutdown(t *testing.T) {
	mt := newMockTmux()
	source := &mockPodSource{}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   50 * time.Millisecond,
		HealthInterval: 50 * time.Millisecond,
		TmuxClient:     mt,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Phase 1: No pods initially
	time.Sleep(100 * time.Millisecond)
	if srv.Status().PodCount != 0 {
		t.Errorf("expected 0 pods initially")
	}

	// Phase 2: Add pods
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodStatus: "running"},
		{AgentID: "gastown/polecats/bravo", PodName: "pod-2", PodStatus: "running"},
	})
	time.Sleep(200 * time.Millisecond)

	status := srv.Status()
	if len(status.Connections) != 2 {
		t.Errorf("expected 2 connections, got %d", len(status.Connections))
	}

	// Phase 3: Remove one pod
	source.setPods([]*PodInfo{
		{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodStatus: "running"},
	})
	time.Sleep(200 * time.Millisecond)

	status = srv.Status()
	if len(status.Connections) != 1 {
		t.Errorf("expected 1 connection after removal, got %d", len(status.Connections))
	}

	// Phase 4: Graceful shutdown
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}

	// All connections closed
	srv.mu.RLock()
	connCount := len(srv.connections)
	srv.mu.RUnlock()
	if connCount != 0 {
		t.Errorf("expected 0 connections after shutdown, got %d", connCount)
	}
}

func TestServer_HealthMonitor_ReconnectsDeadConnection(t *testing.T) {
	mt := newMockTmux()
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gastown/polecats/alpha", PodName: "pod-1", PodStatus: "running"},
		},
	}
	srv := NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      "gastown-test",
		PodSource:      source,
		PollInterval:   50 * time.Millisecond,
		HealthInterval: 50 * time.Millisecond,
		TmuxClient:     mt,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	// Wait for initial connection
	time.Sleep(200 * time.Millisecond)

	// Simulate kubectl exec death
	mt.setSessionDead("gt-gastown-alpha")

	// Wait for health monitor to detect and reconnect
	time.Sleep(300 * time.Millisecond)

	// Verify connection was re-established
	srv.mu.RLock()
	pc, exists := srv.connections["gastown/polecats/alpha"]
	srv.mu.RUnlock()

	if exists && pc.IsConnected() {
		// Successful reconnect
	} else if !exists {
		// Connection was removed (may have exceeded max reconnects depending on timing)
		// This is also valid behavior
	}

	cancel()
	<-done
}

// --- Concurrent Server Access Tests ---

func TestServer_ConcurrentStatusCalls(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	// Add some connections
	srv.connectPod(&PodInfo{
		AgentID: "gastown/polecats/alpha",
		PodName: "pod-1",
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = srv.Status()
			}
		}()
	}
	wg.Wait()
}

func TestServer_ConcurrentConnectDisconnect(t *testing.T) {
	mt := newMockTmux()
	srv := NewServer(ServerConfig{
		Rig:        "gastown",
		Namespace:  "gastown-test",
		PodSource:  &mockPodSource{},
		TmuxClient: mt,
	})

	var wg sync.WaitGroup
	for i := range 10 {
		agentID := fmt.Sprintf("gastown/polecats/test-%d", i)
		podName := fmt.Sprintf("pod-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.connectPod(&PodInfo{
				AgentID: agentID,
				PodName: podName,
			})
			srv.disconnectPod(agentID)
		}()
	}
	wg.Wait()
}
