package terminal

import (
	"context"
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
	}

	for _, tt := range tests {
		got := agentIDToSessionName(tt.agentID)
		if got != tt.expected {
			t.Errorf("agentIDToSessionName(%q) = %q, want %q", tt.agentID, got, tt.expected)
		}
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

func TestHandlePodEvent_Added(t *testing.T) {
	// Track what events the server handles.
	// We can't test actual tmux connections in unit tests,
	// but we can verify the event handler dispatches correctly.
	var addedCount atomic.Int32

	source := &mockPodSource{}
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: source,
	})

	// Override handlePodEvent to track calls without needing tmux
	// Since we can't easily mock tmux in unit tests, we verify the
	// server's connection management logic through the status API.

	// Verify initial state is empty
	status := srv.Status()
	if len(status.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(status.Connections))
	}

	// The actual connectPod will fail without tmux, but let's verify
	// the event flow by checking that inventory events fire correctly.
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

func TestHandlePodEvent_Removed(t *testing.T) {
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

func TestHandlePodEvent_Updated(t *testing.T) {
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

func TestServerShutdown_NoConnections(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Shutdown with no connections should not panic
	srv.shutdown()
}

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

func TestServerDisconnectPod_NotFound(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Disconnecting a pod that doesn't exist should not panic
	srv.disconnectPod("nonexistent/agent")
}

func TestServerCheckHealth_EmptyConnections(t *testing.T) {
	srv := NewServer(ServerConfig{
		Rig:       "gastown",
		Namespace: "gastown-test",
		PodSource: &mockPodSource{},
	})

	// Health check with no connections should not panic
	srv.checkHealth(context.Background())
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
