package feed

import (
	"sync"
)

// MultiSource combines events from multiple EventSources into a single stream.
type MultiSource struct {
	sources []EventSource
	events  chan Event
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewMultiSource creates a new multi-source that combines events from all given sources.
func NewMultiSource(sources ...EventSource) *MultiSource {
	m := &MultiSource{
		sources: sources,
		events:  make(chan Event, 100),
		done:    make(chan struct{}),
	}

	// Start a goroutine for each source to forward events
	for _, src := range sources {
		if src == nil {
			continue
		}
		m.wg.Add(1)
		go m.forwardEvents(src)
	}

	// Close events channel when all sources are done
	go func() {
		m.wg.Wait()
		close(m.events)
	}()

	return m
}

// forwardEvents reads from a source and forwards to the combined channel.
func (m *MultiSource) forwardEvents(src EventSource) {
	defer m.wg.Done()

	srcEvents := src.Events()
	for {
		select {
		case event, ok := <-srcEvents:
			if !ok {
				return
			}
			select {
			case m.events <- event:
			case <-m.done:
				return
			}
		case <-m.done:
			return
		}
	}
}

// Events returns the combined event channel.
func (m *MultiSource) Events() <-chan Event {
	return m.events
}

// Close stops all sources.
func (m *MultiSource) Close() error {
	close(m.done)
	var lastErr error
	for _, src := range m.sources {
		if src != nil {
			if err := src.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}
