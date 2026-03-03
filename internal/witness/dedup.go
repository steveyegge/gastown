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
}

// NewMessageDeduplicator creates a deduplicator.
// The map grows without bound — 10k string keys is negligible memory,
// and correctness (no duplicate processing) matters more than a soft cap.
func NewMessageDeduplicator() *MessageDeduplicator {
	return &MessageDeduplicator{
		processed: make(map[string]bool),
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

	d.processed[messageID] = true
	return false
}

// Size returns the number of tracked message IDs.
func (d *MessageDeduplicator) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.processed)
}
