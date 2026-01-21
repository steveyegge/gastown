package feed

import (
	"context"
)

// MQEventSource was used to read MQ lifecycle events from mq_events.jsonl.
// The mrqueue package has been removed, so this is now a no-op stub.
// MR events can be observed via beads activity instead.
type MQEventSource struct {
	events chan Event
	cancel context.CancelFunc
}

// NewMQEventSource creates a stub source that produces no events.
// The mrqueue event log is no longer written.
func NewMQEventSource(beadsDir string) (*MQEventSource, error) {
	ctx, cancel := context.WithCancel(context.Background())

	source := &MQEventSource{
		events: make(chan Event, 1),
		cancel: cancel,
	}

	// Start a goroutine that just waits for cancellation
	go func() {
		<-ctx.Done()
		close(source.events)
	}()

	return source, nil
}

// NewMQEventSourceFromWorkDir creates an MQ event source (stub).
func NewMQEventSourceFromWorkDir(workDir string) (*MQEventSource, error) {
	return NewMQEventSource("")
}

// Events returns the event channel (always empty).
func (s *MQEventSource) Events() <-chan Event {
	return s.events
}

// Close stops the source.
func (s *MQEventSource) Close() error {
	s.cancel()
	return nil
}
