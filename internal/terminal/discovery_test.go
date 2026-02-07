package terminal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockPodSource is a test implementation of PodSource.
type mockPodSource struct {
	mu   sync.Mutex
	pods []*PodInfo
	err  error
}

func (m *mockPodSource) ListPods(_ context.Context) ([]*PodInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*PodInfo, len(m.pods))
	for i, p := range m.pods {
		copied := *p
		result[i] = &copied
	}
	return result, nil
}

func (m *mockPodSource) setPods(pods []*PodInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pods = pods
}

func (m *mockPodSource) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func TestNewPodInventory_DefaultInterval(t *testing.T) {
	pi := NewPodInventory(PodInventoryConfig{
		Source: &mockPodSource{},
	})
	if pi.pollInterval != DefaultPollInterval {
		t.Errorf("expected default poll interval %v, got %v", DefaultPollInterval, pi.pollInterval)
	}
}

func TestNewPodInventory_CustomInterval(t *testing.T) {
	pi := NewPodInventory(PodInventoryConfig{
		Source:       &mockPodSource{},
		PollInterval: 10 * time.Second,
	})
	if pi.pollInterval != 10*time.Second {
		t.Errorf("expected poll interval 10s, got %v", pi.pollInterval)
	}
}

func TestRefresh_EmptyInventory(t *testing.T) {
	source := &mockPodSource{}
	pi := NewPodInventory(PodInventoryConfig{Source: source})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if pi.Count() != 0 {
		t.Errorf("expected 0 pods, got %d", pi.Count())
	}
	if pods := pi.ListPods(); len(pods) != 0 {
		t.Errorf("expected empty pod list, got %d", len(pods))
	}
}

func TestRefresh_DiscoversPods(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod-abc", PodIP: "10.0.0.1", PodNode: "node-1", PodStatus: "running", ScreenSession: "agent"},
			{AgentID: "gt-gastown-bravo", PodName: "bravo-pod-def", PodIP: "10.0.0.2", PodNode: "node-2", PodStatus: "running", ScreenSession: "agent"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if pi.Count() != 2 {
		t.Errorf("expected 2 pods, got %d", pi.Count())
	}

	alpha := pi.GetPod("gt-gastown-alpha")
	if alpha == nil {
		t.Fatal("expected alpha pod, got nil")
	}
	if alpha.PodName != "alpha-pod-abc" {
		t.Errorf("expected alpha pod name 'alpha-pod-abc', got %q", alpha.PodName)
	}
	if alpha.PodIP != "10.0.0.1" {
		t.Errorf("expected alpha pod IP '10.0.0.1', got %q", alpha.PodIP)
	}
}

func TestRefresh_DetectsAddedPods(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}

	var events []PodEvent
	var eventMu sync.Mutex
	pi := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			eventMu.Lock()
			events = append(events, e)
			eventMu.Unlock()
		},
	})

	// First refresh: alpha appears
	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	eventMu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != PodAdded {
		t.Errorf("expected PodAdded event, got %v", events[0].Type)
	}
	if events[0].Pod.AgentID != "gt-gastown-alpha" {
		t.Errorf("expected agent ID 'gt-gastown-alpha', got %q", events[0].Pod.AgentID)
	}
	eventMu.Unlock()

	// Add a second pod
	events = nil
	source.setPods([]*PodInfo{
		{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		{AgentID: "gt-gastown-bravo", PodName: "bravo-pod", PodStatus: "running"},
	})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	eventMu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event (added bravo), got %d", len(events))
	}
	if events[0].Type != PodAdded {
		t.Errorf("expected PodAdded, got %v", events[0].Type)
	}
	if events[0].Pod.AgentID != "gt-gastown-bravo" {
		t.Errorf("expected bravo added, got %q", events[0].Pod.AgentID)
	}
	eventMu.Unlock()
}

func TestRefresh_DetectsRemovedPods(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
			{AgentID: "gt-gastown-bravo", PodName: "bravo-pod", PodStatus: "running"},
		},
	}

	var events []PodEvent
	var eventMu sync.Mutex
	pi := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			eventMu.Lock()
			events = append(events, e)
			eventMu.Unlock()
		},
	})

	// Initial refresh
	_ = pi.Refresh(context.Background())
	events = nil

	// Remove bravo
	source.setPods([]*PodInfo{
		{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
	})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	eventMu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != PodRemoved {
		t.Errorf("expected PodRemoved, got %v", events[0].Type)
	}
	if events[0].Pod.AgentID != "gt-gastown-bravo" {
		t.Errorf("expected bravo removed, got %q", events[0].Pod.AgentID)
	}
	eventMu.Unlock()

	if pi.Count() != 1 {
		t.Errorf("expected 1 pod remaining, got %d", pi.Count())
	}
	if pi.GetPod("gt-gastown-bravo") != nil {
		t.Error("bravo should be removed from inventory")
	}
}

func TestRefresh_DetectsUpdatedPods(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod-v1", PodIP: "10.0.0.1", PodStatus: "running"},
		},
	}

	var events []PodEvent
	var eventMu sync.Mutex
	pi := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(e PodEvent) {
			eventMu.Lock()
			events = append(events, e)
			eventMu.Unlock()
		},
	})

	_ = pi.Refresh(context.Background())
	events = nil

	// Pod restarted with new name and IP
	source.setPods([]*PodInfo{
		{AgentID: "gt-gastown-alpha", PodName: "alpha-pod-v2", PodIP: "10.0.0.5", PodStatus: "running"},
	})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	eventMu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != PodUpdated {
		t.Errorf("expected PodUpdated, got %v", events[0].Type)
	}
	if events[0].Pod.PodName != "alpha-pod-v2" {
		t.Errorf("expected new pod name 'alpha-pod-v2', got %q", events[0].Pod.PodName)
	}
	eventMu.Unlock()
}

func TestRefresh_FiltersStaleEntries(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
			{AgentID: "gt-gastown-bravo", PodName: "bravo-pod", PodStatus: "failed"},
			{AgentID: "gt-gastown-charlie", PodName: "charlie-pod", PodStatus: "terminated"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})

	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if pi.Count() != 1 {
		t.Errorf("expected 1 pod (only running), got %d", pi.Count())
	}
	if pi.GetPod("gt-gastown-alpha") == nil {
		t.Error("running pod should be in inventory")
	}
	if pi.GetPod("gt-gastown-bravo") != nil {
		t.Error("failed pod should be filtered")
	}
	if pi.GetPod("gt-gastown-charlie") != nil {
		t.Error("terminated pod should be filtered")
	}
}

func TestRefresh_SourceError(t *testing.T) {
	source := &mockPodSource{
		err: fmt.Errorf("daemon unreachable"),
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})

	err := pi.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if pi.Count() != 0 {
		t.Errorf("expected 0 pods after error, got %d", pi.Count())
	}
}

func TestRefresh_SourceErrorPreservesExisting(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})

	// First refresh succeeds
	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if pi.Count() != 1 {
		t.Fatalf("expected 1 pod, got %d", pi.Count())
	}

	// Second refresh fails - existing inventory should be preserved
	source.setError(fmt.Errorf("daemon down"))
	err := pi.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if pi.Count() != 1 {
		t.Errorf("expected inventory preserved (1 pod), got %d", pi.Count())
	}
}

func TestGetPod_NotFound(t *testing.T) {
	pi := NewPodInventory(PodInventoryConfig{Source: &mockPodSource{}})
	if pod := pi.GetPod("nonexistent"); pod != nil {
		t.Errorf("expected nil for nonexistent pod, got %+v", pod)
	}
}

func TestGetPod_ReturnsCopy(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodIP: "10.0.0.1", PodStatus: "running"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})
	_ = pi.Refresh(context.Background())

	pod := pi.GetPod("gt-gastown-alpha")
	pod.PodIP = "MODIFIED"

	// Original should be unchanged
	original := pi.GetPod("gt-gastown-alpha")
	if original.PodIP != "10.0.0.1" {
		t.Errorf("GetPod should return copy; original was modified to %q", original.PodIP)
	}
}

func TestListPods_ReturnsCopies(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})
	_ = pi.Refresh(context.Background())

	pods := pi.ListPods()
	pods[0].PodName = "MODIFIED"

	// Original should be unchanged
	original := pi.GetPod("gt-gastown-alpha")
	if original.PodName != "alpha-pod" {
		t.Errorf("ListPods should return copies; original was modified to %q", original.PodName)
	}
}

func TestRefresh_NoEventsForUnchanged(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodIP: "10.0.0.1", PodStatus: "running"},
		},
	}

	var eventCount int32
	pi := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(_ PodEvent) {
			atomic.AddInt32(&eventCount, 1)
		},
	})

	// First refresh: 1 added event
	_ = pi.Refresh(context.Background())
	if atomic.LoadInt32(&eventCount) != 1 {
		t.Fatalf("expected 1 event after first refresh, got %d", atomic.LoadInt32(&eventCount))
	}

	// Second refresh with same data: no events
	_ = pi.Refresh(context.Background())
	if atomic.LoadInt32(&eventCount) != 1 {
		t.Errorf("expected still 1 event (no change), got %d", atomic.LoadInt32(&eventCount))
	}
}

func TestRefresh_CallbackCanAccessInventory(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}

	var callbackCount int
	pi := NewPodInventory(PodInventoryConfig{
		Source: source,
		OnChange: func(_ PodEvent) {
			callbackCount++
		},
	})

	// Verify the callback gets called and inventory is accessible after
	if err := pi.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if callbackCount != 1 {
		t.Errorf("expected callback called once, got %d", callbackCount)
	}

	// GetPod should work after callback
	if pi.GetPod("gt-gastown-alpha") == nil {
		t.Error("expected pod accessible after callback")
	}
}

func TestWatch_CancelsOnContext(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}

	pi := NewPodInventory(PodInventoryConfig{
		Source:       source,
		PollInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- pi.Watch(ctx)
	}()

	// Let it run for a few ticks
	time.Sleep(200 * time.Millisecond)

	// Verify pods were discovered
	if pi.Count() != 1 {
		t.Errorf("expected 1 pod from Watch, got %d", pi.Count())
	}

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not exit after context cancellation")
	}
}

func TestWatch_DetectsChanges(t *testing.T) {
	source := &mockPodSource{}

	var events []PodEvent
	var eventMu sync.Mutex
	pi := NewPodInventory(PodInventoryConfig{
		Source:       source,
		PollInterval: 50 * time.Millisecond,
		OnChange: func(e PodEvent) {
			eventMu.Lock()
			events = append(events, e)
			eventMu.Unlock()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pi.Watch(ctx) //nolint:errcheck

	// Wait for initial poll
	time.Sleep(100 * time.Millisecond)

	// Add a pod
	source.setPods([]*PodInfo{
		{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
	})

	// Wait for next poll
	time.Sleep(200 * time.Millisecond)

	eventMu.Lock()
	foundAdd := false
	for _, e := range events {
		if e.Type == PodAdded && e.Pod.AgentID == "gt-gastown-alpha" {
			foundAdd = true
		}
	}
	eventMu.Unlock()

	if !foundAdd {
		t.Error("expected PodAdded event for alpha after Watch poll")
	}
}

func TestConcurrentAccess(t *testing.T) {
	source := &mockPodSource{
		pods: []*PodInfo{
			{AgentID: "gt-gastown-alpha", PodName: "alpha-pod", PodStatus: "running"},
		},
	}
	pi := NewPodInventory(PodInventoryConfig{Source: source})
	_ = pi.Refresh(context.Background())

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent readers
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = pi.GetPod("gt-gastown-alpha")
				_ = pi.ListPods()
				_ = pi.Count()
			}
		}()
	}

	// Concurrent writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			_ = pi.Refresh(ctx)
		}
	}()

	wg.Wait()
}

func TestCLIPodSource_ImplementsInterface(t *testing.T) {
	var _ PodSource = (*CLIPodSource)(nil)
}
