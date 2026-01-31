// Package eventbus provides an in-process pub/sub event bus for decision events.
// This enables real-time notification of decision creation/resolution to subscribers
// like the WatchDecisions RPC stream.
package eventbus

import (
	"sync"
)

// EventType identifies the type of event.
type EventType string

const (
	EventDecisionCreated  EventType = "decision_created"
	EventDecisionResolved EventType = "decision_resolved"
	EventDecisionCanceled EventType = "decision_canceled"
)

// Event represents a decision event in the bus.
type Event struct {
	Type       EventType
	DecisionID string
	Data       interface{} // Type-specific payload (e.g., *Decision for created events)
}

// Subscriber is a function that handles events.
type Subscriber func(Event)

// Metrics tracks event bus statistics.
type Metrics struct {
	EventsPublished   int64 // Total events published
	EventsDelivered   int64 // Total events delivered to subscribers
	EventsDropped     int64 // Events dropped due to full subscriber channels
	SubscribersActive int   // Current active subscribers
	SubscribersTotal  int64 // Total subscribers created over time
}

// Bus is an in-process event bus for decision events.
// It uses a simple broadcast pattern where all subscribers receive all events.
// Thread-safe for concurrent publish/subscribe operations.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event // subscriber ID → event channel
	nextID      int
	closed      bool

	// Metrics
	metricsLock       sync.Mutex
	eventsPublished   int64
	eventsDelivered   int64
	eventsDropped     int64
	subscribersTotal  int64
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[string]chan Event),
	}
}

// Subscribe creates a new subscription and returns a channel for receiving events.
// The returned unsubscribe function must be called to clean up when done.
// The channel has a buffer to avoid blocking publishers.
func (b *Bus) Subscribe() (events <-chan Event, unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		// Return closed channel if bus is closed
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}

	b.nextID++
	id := string(rune(b.nextID))
	ch := make(chan Event, 100) // Buffer to avoid blocking publishers
	b.subscribers[id] = ch

	b.metricsLock.Lock()
	b.subscribersTotal++
	b.metricsLock.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if ch, ok := b.subscribers[id]; ok {
			close(ch)
			delete(b.subscribers, id)
		}
	}
}

// Publish sends an event to all subscribers.
// Non-blocking: if a subscriber's channel is full, the event is dropped for that subscriber.
func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	b.metricsLock.Lock()
	b.eventsPublished++
	b.metricsLock.Unlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
			b.metricsLock.Lock()
			b.eventsDelivered++
			b.metricsLock.Unlock()
		default:
			// Channel full, drop event for this subscriber
			// This prevents slow subscribers from blocking the bus
			b.metricsLock.Lock()
			b.eventsDropped++
			b.metricsLock.Unlock()
		}
	}
}

// PublishDecisionCreated is a convenience method for publishing decision created events.
func (b *Bus) PublishDecisionCreated(decisionID string, data interface{}) {
	b.Publish(Event{
		Type:       EventDecisionCreated,
		DecisionID: decisionID,
		Data:       data,
	})
}

// PublishDecisionResolved is a convenience method for publishing decision resolved events.
func (b *Bus) PublishDecisionResolved(decisionID string, data interface{}) {
	b.Publish(Event{
		Type:       EventDecisionResolved,
		DecisionID: decisionID,
		Data:       data,
	})
}

// PublishDecisionCanceled is a convenience method for publishing decision canceled events.
func (b *Bus) PublishDecisionCanceled(decisionID string) {
	b.Publish(Event{
		Type:       EventDecisionCanceled,
		DecisionID: decisionID,
	})
}

// Close shuts down the bus and closes all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

// SubscriberCount returns the current number of subscribers.
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// Metrics returns a snapshot of event bus statistics.
func (b *Bus) Metrics() Metrics {
	b.mu.RLock()
	subscriberCount := len(b.subscribers)
	b.mu.RUnlock()

	b.metricsLock.Lock()
	defer b.metricsLock.Unlock()

	return Metrics{
		EventsPublished:   b.eventsPublished,
		EventsDelivered:   b.eventsDelivered,
		EventsDropped:     b.eventsDropped,
		SubscribersActive: subscriberCount,
		SubscribersTotal:  b.subscribersTotal,
	}
}
