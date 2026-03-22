package telegram

import (
	"fmt"
	"sync"
	"testing"
)

func TestMessageMap_StoreAndLookupBothDirections(t *testing.T) {
	m := NewMessageMap(10)
	m.Store(42, 100, "thread-abc")

	threadID, ok := m.ThreadID(42, 100)
	if !ok {
		t.Fatal("expected ThreadID to return ok=true")
	}
	if threadID != "thread-abc" {
		t.Fatalf("expected thread-abc, got %q", threadID)
	}

	chatID, msgID, ok := m.TelegramID("thread-abc")
	if !ok {
		t.Fatal("expected TelegramID to return ok=true")
	}
	if chatID != 42 || msgID != 100 {
		t.Fatalf("expected (42, 100), got (%d, %d)", chatID, msgID)
	}
}

func TestMessageMap_MissingKeyReturnsFalse(t *testing.T) {
	m := NewMessageMap(10)

	_, ok := m.ThreadID(99, 999)
	if ok {
		t.Fatal("expected ThreadID for missing key to return ok=false")
	}

	_, _, ok = m.TelegramID("nonexistent-thread")
	if ok {
		t.Fatal("expected TelegramID for missing thread to return ok=false")
	}
}

func TestMessageMap_FIFOEvictionAtCapacity(t *testing.T) {
	m := NewMessageMap(3)

	m.Store(1, 1, "thread-1")
	m.Store(1, 2, "thread-2")
	m.Store(1, 3, "thread-3")

	// All three should be present
	for i := 1; i <= 3; i++ {
		if _, ok := m.ThreadID(1, i); !ok {
			t.Fatalf("expected entry %d to exist before eviction", i)
		}
	}

	// Insert a 4th entry — the first should be evicted
	m.Store(1, 4, "thread-4")

	// First entry should be gone
	if _, ok := m.ThreadID(1, 1); ok {
		t.Fatal("expected first entry to be evicted (FIFO)")
	}
	if _, _, ok := m.TelegramID("thread-1"); ok {
		t.Fatal("expected thread-1 to be evicted from reverse map")
	}

	// Entries 2, 3, 4 should still be present
	for i := 2; i <= 4; i++ {
		threadID := fmt.Sprintf("thread-%d", i)
		if _, ok := m.ThreadID(1, i); !ok {
			t.Fatalf("expected entry %d to still exist after eviction", i)
		}
		if _, _, ok := m.TelegramID(threadID); !ok {
			t.Fatalf("expected %s to still exist in reverse map", threadID)
		}
	}
}

func TestMessageMap_ConcurrentAccess(t *testing.T) {
	m := NewMessageMap(200)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chatID := int64(i % 10)
			msgID := i
			threadID := fmt.Sprintf("thread-%d", i)
			m.Store(chatID, msgID, threadID)
			m.ThreadID(chatID, msgID)
			m.TelegramID(threadID)
		}(i)
	}

	wg.Wait()
}
