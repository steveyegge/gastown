package witness

import "sync"

// MessageDeduplicator tracks processed message IDs to prevent duplicate handling.
// If the witness crashes and restarts, re-reading the mailbox could process the
// same message twice (e.g., POLECAT_DONE creating duplicate cleanup wisps).
// This provides in-memory idempotency within a single witness session.
//
// Thread-safe for concurrent patrol goroutines.
type MessageDeduplicator struct {
	mu        sync.Mutex
	processed map[string]bool
	maxSize   int
}

// NewMessageDeduplicator creates a deduplicator with the given max capacity.
// When capacity is reached, the oldest entries are not evicted (simple set).
// For witness sessions that process hundreds of messages at most, this is fine.
func NewMessageDeduplicator(maxSize int) *MessageDeduplicator {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MessageDeduplicator{
		processed: make(map[string]bool),
		maxSize:   maxSize,
	}
}

// AlreadyProcessed returns true if this message ID has been seen before.
// If not seen, marks it as processed and returns false.
// This is an atomic check-and-set operation.
func (d *MessageDeduplicator) AlreadyProcessed(messageID string) bool {
	if messageID == "" {
		return false // Empty IDs can't be deduped; allow processing
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.processed[messageID] {
		return true
	}

	// Don't grow unboundedly
	if len(d.processed) >= d.maxSize {
		return false // At capacity, allow processing (safe: worst case is a dup)
	}

	d.processed[messageID] = true
	return false
}

// Size returns the number of tracked message IDs.
func (d *MessageDeduplicator) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.processed)
}
