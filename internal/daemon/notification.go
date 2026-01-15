package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NotificationSlot tracks a pending notification for deduplication.
// Only the latest notification per slot matters - earlier ones are replaced.
type NotificationSlot struct {
	Slot      string    `json:"slot"`
	Session   string    `json:"session"`
	Message   string    `json:"message"`
	SentAt    time.Time `json:"sent_at"`
	Consumed  bool      `json:"consumed"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
}

// NotificationManager handles slot-based notification deduplication.
// It ensures that for a given (session, slot) pair, only one notification
// is pending at a time. Sending a new notification to the same slot
// replaces the previous one.
type NotificationManager struct {
	stateDir string // Directory for slot state files
	maxAge   time.Duration // Max age before considering a slot stale
}

// NewNotificationManager creates a new notification manager.
// stateDir is where slot state files are stored (e.g., ~/gt/daemon/notifications/)
func NewNotificationManager(stateDir string, maxAge time.Duration) *NotificationManager {
	return &NotificationManager{
		stateDir: stateDir,
		maxAge:   maxAge,
	}
}

// slotPath returns the path to the slot state file.
func (m *NotificationManager) slotPath(session, slot string) string {
	// Sanitize session name (replace / with -)
	safeSession := session
	for i := range safeSession {
		if safeSession[i] == '/' {
			safeSession = safeSession[:i] + "-" + safeSession[i+1:]
		}
	}
	return filepath.Join(m.stateDir, fmt.Sprintf("slot-%s-%s.json", safeSession, slot))
}

// GetSlot reads the current state of a notification slot.
func (m *NotificationManager) GetSlot(session, slot string) (*NotificationSlot, error) {
	path := m.slotPath(session, slot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No slot state
		}
		return nil, err
	}

	var ns NotificationSlot
	if err := json.Unmarshal(data, &ns); err != nil {
		return nil, err
	}

	return &ns, nil
}

// ShouldSend checks if a notification should be sent for this slot.
// Returns true if:
// - No pending notification exists for this slot
// - The pending notification is stale (older than maxAge)
// - The pending notification was consumed
func (m *NotificationManager) ShouldSend(session, slot string) (bool, error) {
	ns, err := m.GetSlot(session, slot)
	if err != nil {
		return true, err // On error, allow sending
	}

	if ns == nil {
		return true, nil // No pending notification
	}

	if ns.Consumed {
		return true, nil // Previous was consumed
	}

	// Check if stale
	if time.Since(ns.SentAt) > m.maxAge {
		return true, nil // Stale, allow new send
	}

	return false, nil // Recent pending notification exists
}

// RecordSend records that a notification was sent for a slot.
func (m *NotificationManager) RecordSend(session, slot, message string) error {
	// Ensure directory exists
	if err := os.MkdirAll(m.stateDir, 0755); err != nil {
		return err
	}

	ns := &NotificationSlot{
		Slot:     slot,
		Session:  session,
		Message:  message,
		SentAt:   time.Now(),
		Consumed: false,
	}

	data, err := json.Marshal(ns)
	if err != nil {
		return err
	}

	return os.WriteFile(m.slotPath(session, slot), data, 0600)
}

// MarkConsumed marks a slot's notification as consumed (agent responded).
func (m *NotificationManager) MarkConsumed(session, slot string) error {
	ns, err := m.GetSlot(session, slot)
	if err != nil {
		return err
	}

	if ns == nil {
		return nil // Nothing to mark
	}

	ns.Consumed = true
	ns.ConsumedAt = time.Now()

	data, err := json.Marshal(ns)
	if err != nil {
		return err
	}

	return os.WriteFile(m.slotPath(session, slot), data, 0600)
}

// MarkSessionActive marks all slots for a session as consumed.
// Call this when the session shows activity (keepalive update).
func (m *NotificationManager) MarkSessionActive(session string) error {
	// List all slot files for this session
	pattern := filepath.Join(m.stateDir, fmt.Sprintf("slot-%s-*.json", session))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var ns NotificationSlot
		if err := json.Unmarshal(data, &ns); err != nil {
			continue
		}

		if !ns.Consumed {
			ns.Consumed = true
			ns.ConsumedAt = time.Now()
			if data, err := json.Marshal(&ns); err == nil {
				_ = os.WriteFile(path, data, 0644) // non-fatal: state file update
			}
		}
	}

	return nil
}

// ClearSlot removes the state file for a slot.
func (m *NotificationManager) ClearSlot(session, slot string) error {
	path := m.slotPath(session, slot)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ClearStaleSlots removes slot files older than maxAge.
func (m *NotificationManager) ClearStaleSlots() error {
	pattern := filepath.Join(m.stateDir, "slot-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > m.maxAge {
			_ = os.Remove(path) // best-effort cleanup
		}
	}

	return nil
}

// Common notification slots
const (
	SlotHeartbeat = "heartbeat"
	SlotStatus    = "status"
)
