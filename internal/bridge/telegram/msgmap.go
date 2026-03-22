package telegram

import (
	"fmt"
	"sync"
)

// MessageMap is a bidirectional, thread-safe, bounded map between
// Telegram (chatID, msgID) pairs and mail thread IDs.
// Eviction is FIFO: the oldest entry is dropped when maxSize is exceeded.
type MessageMap struct {
	mu           sync.RWMutex
	teleToThread map[string]string // teleKey -> threadID
	threadToTele map[string]string // threadID -> teleKey
	order        []string          // insertion order of teleKeys (for FIFO eviction)
	maxSize      int
}

// NewMessageMap creates a MessageMap with the given maximum capacity.
func NewMessageMap(maxSize int) *MessageMap {
	return &MessageMap{
		teleToThread: make(map[string]string),
		threadToTele: make(map[string]string),
		order:        make([]string, 0, maxSize),
		maxSize:      maxSize,
	}
}

func teleKey(chatID int64, msgID int) string {
	return fmt.Sprintf("%d:%d", chatID, msgID)
}

// Store records a mapping between (chatID, msgID) and threadID.
// If the map is at capacity, the oldest entry is evicted first.
func (m *MessageMap) Store(chatID int64, msgID int, threadID string) {
	key := teleKey(chatID, msgID)

	m.mu.Lock()
	defer m.mu.Unlock()

	// If key already exists, remove old reverse mapping before updating.
	if oldThread, exists := m.teleToThread[key]; exists {
		delete(m.threadToTele, oldThread)
		// Remove from order slice.
		for i, k := range m.order {
			if k == key {
				m.order = append(m.order[:i], m.order[i+1:]...)
				break
			}
		}
	}

	// Evict oldest entry if at capacity.
	if len(m.order) >= m.maxSize {
		oldest := m.order[0]
		m.order = m.order[1:]
		if oldThread, exists := m.teleToThread[oldest]; exists {
			delete(m.threadToTele, oldThread)
		}
		delete(m.teleToThread, oldest)
	}

	m.teleToThread[key] = threadID
	m.threadToTele[threadID] = key
	m.order = append(m.order, key)
}

// ThreadID returns the mail thread ID for a given Telegram (chatID, msgID) pair.
func (m *MessageMap) ThreadID(chatID int64, msgID int) (string, bool) {
	key := teleKey(chatID, msgID)
	m.mu.RLock()
	threadID, ok := m.teleToThread[key]
	m.mu.RUnlock()
	return threadID, ok
}

// TelegramID returns the Telegram chatID and msgID for a given mail thread ID.
func (m *MessageMap) TelegramID(threadID string) (chatID int64, msgID int, ok bool) {
	m.mu.RLock()
	key, found := m.threadToTele[threadID]
	m.mu.RUnlock()

	if !found {
		return 0, 0, false
	}

	var cid int64
	var mid int
	_, err := fmt.Sscanf(key, "%d:%d", &cid, &mid)
	if err != nil {
		return 0, 0, false
	}
	return cid, mid, true
}
