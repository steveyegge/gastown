package eventbus

import (
	"sync"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	events, unsub := bus.Subscribe()
	defer unsub()

	// Publish an event
	bus.PublishDecisionCreated("test-123", "test data")

	// Should receive the event
	select {
	case event := <-events:
		if event.Type != EventDecisionCreated {
			t.Errorf("expected EventDecisionCreated, got %v", event.Type)
		}
		if event.DecisionID != "test-123" {
			t.Errorf("expected decision ID test-123, got %v", event.DecisionID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	events1, unsub1 := bus.Subscribe()
	defer unsub1()

	events2, unsub2 := bus.Subscribe()
	defer unsub2()

	// Publish an event
	bus.Publish(Event{Type: EventDecisionResolved, DecisionID: "test-456"})

	// Both subscribers should receive it
	var wg sync.WaitGroup
	wg.Add(2)

	received := make([]bool, 2)

	go func() {
		defer wg.Done()
		select {
		case <-events1:
			received[0] = true
		case <-time.After(100 * time.Millisecond):
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case <-events2:
			received[1] = true
		case <-time.After(100 * time.Millisecond):
		}
	}()

	wg.Wait()

	if !received[0] || !received[1] {
		t.Errorf("not all subscribers received event: %v", received)
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	events, unsub := bus.Subscribe()

	// Unsubscribe
	unsub()

	// Channel should be closed
	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout - channel not closed")
	}
}

func TestBusClose(t *testing.T) {
	bus := New()

	events1, _ := bus.Subscribe()
	events2, _ := bus.Subscribe()

	// Close the bus
	bus.Close()

	// All subscriber channels should be closed
	select {
	case _, ok := <-events1:
		if ok {
			t.Error("expected channel 1 to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout - channel 1 not closed")
	}

	select {
	case _, ok := <-events2:
		if ok {
			t.Error("expected channel 2 to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout - channel 2 not closed")
	}
}

func TestBusSubscriberCount(t *testing.T) {
	bus := New()
	defer bus.Close()

	if bus.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers, got %d", bus.SubscriberCount())
	}

	_, unsub1 := bus.Subscribe()
	if bus.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber, got %d", bus.SubscriberCount())
	}

	_, unsub2 := bus.Subscribe()
	if bus.SubscriberCount() != 2 {
		t.Errorf("expected 2 subscribers, got %d", bus.SubscriberCount())
	}

	unsub1()
	if bus.SubscriberCount() != 1 {
		t.Errorf("expected 1 subscriber after unsub, got %d", bus.SubscriberCount())
	}

	unsub2()
	if bus.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after unsub, got %d", bus.SubscriberCount())
	}
}

func TestBusNonBlocking(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Subscribe but don't read
	_, _ = bus.Subscribe()

	// Fill the buffer (100 events)
	for i := 0; i < 100; i++ {
		bus.PublishDecisionCreated("test", nil)
	}

	// Publishing more should not block
	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			bus.PublishDecisionCreated("overflow", nil)
		}
		done <- true
	}()

	select {
	case <-done:
		// Good - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish blocked with full subscriber buffer")
	}
}

func TestBusMetrics(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Check initial metrics
	m := bus.Metrics()
	if m.EventsPublished != 0 {
		t.Errorf("expected 0 events published, got %d", m.EventsPublished)
	}
	if m.SubscribersActive != 0 {
		t.Errorf("expected 0 active subscribers, got %d", m.SubscribersActive)
	}

	// Subscribe
	events, unsub := bus.Subscribe()

	m = bus.Metrics()
	if m.SubscribersActive != 1 {
		t.Errorf("expected 1 active subscriber, got %d", m.SubscribersActive)
	}
	if m.SubscribersTotal != 1 {
		t.Errorf("expected 1 total subscriber, got %d", m.SubscribersTotal)
	}

	// Publish and consume
	bus.PublishDecisionCreated("test-1", nil)
	<-events

	m = bus.Metrics()
	if m.EventsPublished != 1 {
		t.Errorf("expected 1 event published, got %d", m.EventsPublished)
	}
	if m.EventsDelivered != 1 {
		t.Errorf("expected 1 event delivered, got %d", m.EventsDelivered)
	}
	if m.EventsDropped != 0 {
		t.Errorf("expected 0 events dropped, got %d", m.EventsDropped)
	}

	// Unsubscribe and verify
	unsub()
	m = bus.Metrics()
	if m.SubscribersActive != 0 {
		t.Errorf("expected 0 active subscribers after unsub, got %d", m.SubscribersActive)
	}
	if m.SubscribersTotal != 1 {
		t.Errorf("expected 1 total subscriber, got %d", m.SubscribersTotal)
	}
}

func TestBusMetricsDropped(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Subscribe but don't read
	_, _ = bus.Subscribe()

	// Fill buffer (100) and overflow
	for i := 0; i < 110; i++ {
		bus.PublishDecisionCreated("test", nil)
	}

	m := bus.Metrics()
	if m.EventsPublished != 110 {
		t.Errorf("expected 110 events published, got %d", m.EventsPublished)
	}
	if m.EventsDelivered != 100 {
		t.Errorf("expected 100 events delivered (buffer size), got %d", m.EventsDelivered)
	}
	if m.EventsDropped != 10 {
		t.Errorf("expected 10 events dropped, got %d", m.EventsDropped)
	}
}
