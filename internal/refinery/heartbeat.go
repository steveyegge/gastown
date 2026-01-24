package refinery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Heartbeat represents the Refinery's heartbeat file contents.
// Written by the Refinery on each patrol cycle.
// Read by the Go daemon to decide whether to poke or restart.
type Heartbeat struct {
	// Timestamp is when the heartbeat was written.
	Timestamp time.Time `json:"timestamp"`

	// Cycle is the current patrol cycle number.
	Cycle int64 `json:"cycle"`

	// LastAction describes what the Refinery did in this cycle.
	LastAction string `json:"last_action,omitempty"`

	// QueueLength is the number of MRs in the queue.
	QueueLength int `json:"queue_length"`

	// ProcessedCount is MRs processed this cycle.
	ProcessedCount int `json:"processed_count"`
}

// HeartbeatFile returns the path to the Refinery heartbeat file for a rig.
func HeartbeatFile(townRoot, rigName string) string {
	return filepath.Join(townRoot, rigName, "refinery", "heartbeat.json")
}

// WriteHeartbeat writes a new heartbeat to disk.
// Called by the Refinery at the start of each patrol cycle.
func WriteHeartbeat(townRoot, rigName string, hb *Heartbeat) error {
	hbFile := HeartbeatFile(townRoot, rigName)

	// Ensure refinery directory exists
	if err := os.MkdirAll(filepath.Dir(hbFile), 0755); err != nil {
		return err
	}

	// Set timestamp if not already set
	if hb.Timestamp.IsZero() {
		hb.Timestamp = time.Now().UTC()
	}

	data, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(hbFile, data, 0600)
}

// ReadHeartbeat reads the Refinery heartbeat from disk.
// Returns nil if the file doesn't exist or can't be read.
func ReadHeartbeat(townRoot, rigName string) *Heartbeat {
	hbFile := HeartbeatFile(townRoot, rigName)

	data, err := os.ReadFile(hbFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		return nil
	}

	var hb Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil
	}

	return &hb
}

// Age returns how old the heartbeat is.
// Returns a very large duration if the heartbeat is nil.
func (hb *Heartbeat) Age() time.Duration {
	if hb == nil {
		return 24 * time.Hour * 365 // Very stale
	}
	return time.Since(hb.Timestamp)
}

// IsFresh returns true if the heartbeat is less than 5 minutes old.
// A fresh heartbeat means the Refinery is actively working or recently finished.
func (hb *Heartbeat) IsFresh() bool {
	return hb != nil && hb.Age() < 5*time.Minute
}

// IsStale returns true if the heartbeat is 5-15 minutes old.
// A stale heartbeat may indicate the Refinery is doing a long operation.
func (hb *Heartbeat) IsStale() bool {
	if hb == nil {
		return false
	}
	age := hb.Age()
	return age >= 5*time.Minute && age < 15*time.Minute
}

// IsVeryStale returns true if the heartbeat is more than 15 minutes old.
// A very stale heartbeat means the Refinery should be poked.
func (hb *Heartbeat) IsVeryStale() bool {
	return hb == nil || hb.Age() >= 15*time.Minute
}

// ShouldPoke returns true if the daemon should poke the Refinery.
// The Refinery should be poked if the heartbeat is very stale (>15 minutes).
func (hb *Heartbeat) ShouldPoke() bool {
	return hb.IsVeryStale()
}

// Touch writes a minimal heartbeat with just the timestamp.
// This is a convenience function for simple heartbeat updates.
func Touch(townRoot, rigName string) error {
	// Read existing heartbeat to increment cycle
	existing := ReadHeartbeat(townRoot, rigName)
	cycle := int64(1)
	if existing != nil {
		cycle = existing.Cycle + 1
	}

	return WriteHeartbeat(townRoot, rigName, &Heartbeat{
		Timestamp: time.Now().UTC(),
		Cycle:     cycle,
	})
}

// TouchWithAction writes a heartbeat with an action description and queue info.
func TouchWithAction(townRoot, rigName, action string, queueLen, processed int) error {
	existing := ReadHeartbeat(townRoot, rigName)
	cycle := int64(1)
	if existing != nil {
		cycle = existing.Cycle + 1
	}

	return WriteHeartbeat(townRoot, rigName, &Heartbeat{
		Timestamp:      time.Now().UTC(),
		Cycle:          cycle,
		LastAction:     action,
		QueueLength:    queueLen,
		ProcessedCount: processed,
	})
}
