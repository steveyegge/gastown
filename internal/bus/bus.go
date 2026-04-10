// Package bus provides a pub/sub event bus for step transition events.
// Supports both a local in-process bus (for testing and single-machine use)
// and a Redis-backed bus for distributed deacon aggregation.
package bus

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// StepEventType classifies step transition events.
type StepEventType string

const (
	StepAdvanced  StepEventType = "advanced"
	StepCompleted StepEventType = "completed"
	StepFailed    StepEventType = "failed"
	StepRetried   StepEventType = "retried"
	StepTriaged   StepEventType = "triaged"
	StepEscalated StepEventType = "escalated"
)

// StepEvent is a step transition event published to the bus.
type StepEvent struct {
	Rig       string        `json:"rig"`
	Polecat   string        `json:"polecat"`
	StepID    string        `json:"step_id"`
	Type      StepEventType `json:"type"`
	Detail    string        `json:"detail,omitempty"`
	Formula   string        `json:"formula,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// NewStepEvent creates a StepEvent with the current timestamp.
func NewStepEvent(rig, polecat, stepID string, eventType StepEventType) StepEvent {
	return StepEvent{
		Rig:       rig,
		Polecat:   polecat,
		StepID:    stepID,
		Type:      eventType,
		Timestamp: time.Now(),
	}
}

// Channel returns the Redis channel name for this event.
func (e StepEvent) Channel() string {
	return fmt.Sprintf("orchestrator:step:%s", e.Rig)
}

// Marshal serializes the event to JSON.
func (e StepEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalStepEvent deserializes a StepEvent from JSON.
func UnmarshalStepEvent(data []byte) (StepEvent, error) {
	var ev StepEvent
	err := json.Unmarshal(data, &ev)
	return ev, err
}

// Bus is the interface for publishing and subscribing to step events.
type Bus interface {
	// Publish sends an event to all subscribers of the event's channel.
	Publish(ev StepEvent) error

	// Subscribe registers a callback for events on the given channel.
	// Returns an unsubscribe function.
	Subscribe(channel string, fn func(StepEvent)) func()

	// Close shuts down the bus.
	Close() error
}

// subscriber is a registered event handler.
type subscriber struct {
	id int
	fn func(StepEvent)
}

// LocalBus is an in-process pub/sub bus for single-machine use and testing.
type LocalBus struct {
	mu          sync.RWMutex
	subscribers map[string][]subscriber
	nextID      int
}

// NewLocalBus creates a new in-process bus.
func NewLocalBus() *LocalBus {
	return &LocalBus{
		subscribers: make(map[string][]subscriber),
	}
}

// Publish sends an event to all subscribers on the event's channel.
func (b *LocalBus) Publish(ev StepEvent) error {
	b.mu.RLock()
	subs := b.subscribers[ev.Channel()]
	// Copy slice under read lock to avoid holding lock during callbacks.
	copied := make([]subscriber, len(subs))
	copy(copied, subs)
	b.mu.RUnlock()

	for _, s := range copied {
		s.fn(ev)
	}
	return nil
}

// Subscribe registers a callback for the given channel.
func (b *LocalBus) Subscribe(channel string, fn func(StepEvent)) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subscribers[channel] = append(b.subscribers[channel], subscriber{id: id, fn: fn})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[channel]
		for i, s := range subs {
			if s.id == id {
				b.subscribers[channel] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}
}

// Close is a no-op for the local bus.
func (b *LocalBus) Close() error {
	return nil
}
