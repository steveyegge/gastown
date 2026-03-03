package witness

import (
	"fmt"
	"sync"
	"testing"
)

func TestMessageDeduplicator_BasicDedup(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator()

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
	d := NewMessageDeduplicator()

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
	d := NewMessageDeduplicator()

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

func TestMessageDeduplicator_ManyMessages(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator()

	// Add many messages
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("msg-%03d", i)
		if d.AlreadyProcessed(id) {
			t.Errorf("first call for %s should return false", id)
		}
	}

	// All should be tracked as duplicates
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("msg-%03d", i)
		if !d.AlreadyProcessed(id) {
			t.Errorf("second call for %s should return true", id)
		}
	}
}

func TestMessageDeduplicator_Concurrent(t *testing.T) {
	t.Parallel()
	d := NewMessageDeduplicator()
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
