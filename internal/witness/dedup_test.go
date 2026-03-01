package witness

import (
	"fmt"
	"sync"
	"testing"
)

func TestMessageDeduplicator_BasicDedup(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(100)

	// First time: not a duplicate
	if d.AlreadyProcessed("msg-001") {
		t.Error("first call should return false (not a duplicate)")
	}

	// Second time: is a duplicate
	if !d.AlreadyProcessed("msg-001") {
		t.Error("second call should return true (duplicate)")
	}

	// Different ID: not a duplicate
	if d.AlreadyProcessed("msg-002") {
		t.Error("different ID should return false")
	}
}

func TestMessageDeduplicator_EmptyID(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(100)

	// Empty IDs should always return false (can't deduplicate)
	if d.AlreadyProcessed("") {
		t.Error("empty ID should return false")
	}
	if d.AlreadyProcessed("") {
		t.Error("empty ID should always return false")
	}
}

func TestMessageDeduplicator_Size(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(100)

	if d.Size() != 0 {
		t.Errorf("Size() = %d, want 0", d.Size())
	}

	d.AlreadyProcessed("msg-001")
	d.AlreadyProcessed("msg-002")
	d.AlreadyProcessed("msg-001") // duplicate, shouldn't increase size

	if d.Size() != 2 {
		t.Errorf("Size() = %d, want 2", d.Size())
	}
}

func TestMessageDeduplicator_MaxSize(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(3)

	d.AlreadyProcessed("msg-001")
	d.AlreadyProcessed("msg-002")
	d.AlreadyProcessed("msg-003")

	// At capacity — new messages should still be allowed (not deduped)
	if d.AlreadyProcessed("msg-004") {
		t.Error("at capacity, new messages should be allowed through")
	}

	// But existing ones should still be detected
	if !d.AlreadyProcessed("msg-001") {
		t.Error("existing message should still be detected as duplicate")
	}
}

func TestMessageDeduplicator_DefaultMaxSize(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(0)

	if d.maxSize != 10000 {
		t.Errorf("maxSize = %d, want 10000 for zero input", d.maxSize)
	}

	d2 := NewMessageDeduplicator(-5)
	if d2.maxSize != 10000 {
		t.Errorf("maxSize = %d, want 10000 for negative input", d2.maxSize)
	}
}

func TestMessageDeduplicator_Concurrent(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator(1000)
	var wg sync.WaitGroup

	// Spawn 100 goroutines, each processing 10 unique messages
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				d.AlreadyProcessed(fmt.Sprintf("msg-%d-%d", id, j))
			}
		}(i)
	}
	wg.Wait()

	if d.Size() != 1000 {
		t.Errorf("Size() = %d, want 1000 after concurrent inserts", d.Size())
	}
}
