// Package deacon provides the Deacon agent infrastructure.
// The Deacon is a Claude agent that monitors Mayor and Witnesses,
// handles lifecycle requests, and keeps Gas Town running.
package deacon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Heartbeat represents the Deacon's heartbeat file contents.
// Written by the Deacon on each wake cycle.
// Read by the Go daemon to decide whether to poke.
type Heartbeat struct {
	// Timestamp is when the heartbeat was written.
	Timestamp time.Time `json:"timestamp"`

	// Cycle is the current wake cycle number.
	Cycle int64 `json:"cycle"`

	// LastAction describes what the Deacon did in this cycle.
	LastAction string `json:"last_action,omitempty"`

	// HealthyAgents is the count of healthy agents observed.
	HealthyAgents int `json:"healthy_agents"`

	// UnhealthyAgents is the count of unhealthy agents observed.
	UnhealthyAgents int `json:"unhealthy_agents"`
}

// HeartbeatFile returns the path to the Deacon heartbeat file.
func HeartbeatFile(townRoot string) string {
	return filepath.Join(townRoot, "deacon", "heartbeat.json")
}

// WriteHeartbeat writes a new heartbeat to disk.
// Called by the Deacon at the start of each wake cycle.
func WriteHeartbeat(townRoot string, hb *Heartbeat) error {
	hbFile := HeartbeatFile(townRoot)

	// Ensure deacon directory exists
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

// ReadHeartbeat reads the Deacon heartbeat from disk.
// Returns nil if the file doesn't exist or can't be read.
func ReadHeartbeat(townRoot string) *Heartbeat {
	hbFile := HeartbeatFile(townRoot)

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
// A fresh heartbeat means the Deacon is actively working or recently finished.
func (hb *Heartbeat) IsFresh() bool {
	return hb != nil && hb.Age() < 5*time.Minute
}

// IsStale returns true if the heartbeat is 5-15 minutes old.
// A stale heartbeat may indicate the Deacon is doing a long operation.
func (hb *Heartbeat) IsStale() bool {
	if hb == nil {
		return false
	}
	age := hb.Age()
	return age >= 5*time.Minute && age < 15*time.Minute
}

// IsVeryStale returns true if the heartbeat is more than 15 minutes old.
// A very stale heartbeat means the Deacon should be poked.
func (hb *Heartbeat) IsVeryStale() bool {
	return hb == nil || hb.Age() >= 15*time.Minute
}

// ShouldPoke returns true if the daemon should poke the Deacon.
// The Deacon should be poked if:
// - No heartbeat exists
// - Heartbeat is very stale (>5 minutes)
func (hb *Heartbeat) ShouldPoke() bool {
	return hb.IsVeryStale()
}

// Touch writes a minimal heartbeat with just the timestamp.
// This is a convenience function for simple heartbeat updates.
func Touch(townRoot string) error {
	// Read existing heartbeat to increment cycle
	existing := ReadHeartbeat(townRoot)
	cycle := int64(1)
	if existing != nil {
		cycle = existing.Cycle + 1
	}

	return WriteHeartbeat(townRoot, &Heartbeat{
		Timestamp: time.Now().UTC(),
		Cycle:     cycle,
	})
}

// TouchWithAction writes a heartbeat with an action description.
func TouchWithAction(townRoot, action string, healthy, unhealthy int) error {
	existing := ReadHeartbeat(townRoot)
	cycle := int64(1)
	if existing != nil {
		cycle = existing.Cycle + 1
	}

	return WriteHeartbeat(townRoot, &Heartbeat{
		Timestamp:       time.Now().UTC(),
		Cycle:           cycle,
		LastAction:      action,
		HealthyAgents:   healthy,
		UnhealthyAgents: unhealthy,
	})
}
