package cmd

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDeaconStatusJSON_Schema verifies the JSON output schema for gt deacon status --json.
// This catches schema changes that would break witness parsing.
func TestDeaconStatusJSON_Schema(t *testing.T) {
	now := time.Now().UTC()

	out := DeaconStatusOutput{
		Running: true,
		Paused:  false,
		Session: "gt-deacon",
		Heartbeat: &HeartbeatStatus{
			Timestamp:  now,
			AgeSec:     42.5,
			Cycle:      12,
			LastAction: "patrol complete",
			Fresh:      true,
			Stale:      false,
			VeryStale:  false,
		},
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal DeaconStatusOutput: %v", err)
	}

	// Parse back as generic map to verify field names
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	// Top-level fields
	for _, key := range []string{"running", "paused", "session", "heartbeat"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing top-level key %q in JSON output", key)
		}
	}

	// Heartbeat fields
	hb, ok := m["heartbeat"].(map[string]interface{})
	if !ok {
		t.Fatal("heartbeat is not an object")
	}
	for _, key := range []string{"timestamp", "age_seconds", "cycle", "last_action", "fresh", "stale", "very_stale"} {
		if _, ok := hb[key]; !ok {
			t.Errorf("missing heartbeat key %q in JSON output", key)
		}
	}

	// Verify values round-trip correctly
	if hb["fresh"] != true {
		t.Errorf("fresh = %v, want true", hb["fresh"])
	}
	if hb["cycle"].(float64) != 12 {
		t.Errorf("cycle = %v, want 12", hb["cycle"])
	}
	if hb["last_action"] != "patrol complete" {
		t.Errorf("last_action = %v, want 'patrol complete'", hb["last_action"])
	}
}

// TestDeaconStatusJSON_NoHeartbeat verifies heartbeat is omitted when nil.
func TestDeaconStatusJSON_NoHeartbeat(t *testing.T) {
	out := DeaconStatusOutput{
		Running:   false,
		Paused:    false,
		Session:   "gt-deacon",
		Heartbeat: nil,
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := m["heartbeat"]; ok {
		t.Error("heartbeat should be omitted when nil")
	}

	if m["running"] != false {
		t.Errorf("running = %v, want false", m["running"])
	}
}

// TestDeaconStatusJSON_Roundtrip verifies the struct can be marshaled and unmarshaled.
func TestDeaconStatusJSON_Roundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second) // Truncate for JSON precision

	original := DeaconStatusOutput{
		Running: true,
		Paused:  true,
		Session: "gt-deacon",
		Heartbeat: &HeartbeatStatus{
			Timestamp:  now,
			AgeSec:     120.0,
			Cycle:      42,
			LastAction: "checking witnesses",
			Fresh:      false,
			Stale:      true,
			VeryStale:  false,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded DeaconStatusOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Running != original.Running {
		t.Errorf("Running = %v, want %v", decoded.Running, original.Running)
	}
	if decoded.Paused != original.Paused {
		t.Errorf("Paused = %v, want %v", decoded.Paused, original.Paused)
	}
	if decoded.Session != original.Session {
		t.Errorf("Session = %q, want %q", decoded.Session, original.Session)
	}
	if decoded.Heartbeat == nil {
		t.Fatal("Heartbeat is nil after roundtrip")
	}
	if decoded.Heartbeat.Cycle != 42 {
		t.Errorf("Heartbeat.Cycle = %d, want 42", decoded.Heartbeat.Cycle)
	}
	if decoded.Heartbeat.LastAction != "checking witnesses" {
		t.Errorf("Heartbeat.LastAction = %q, want 'checking witnesses'", decoded.Heartbeat.LastAction)
	}
	if decoded.Heartbeat.Fresh != false {
		t.Errorf("Heartbeat.Fresh = %v, want false", decoded.Heartbeat.Fresh)
	}
	if decoded.Heartbeat.Stale != true {
		t.Errorf("Heartbeat.Stale = %v, want true", decoded.Heartbeat.Stale)
	}
}

// TestDeaconStatusJSON_FreshnessStates verifies the three freshness states are mutually exclusive
// in typical usage (fresh, stale, very_stale).
func TestDeaconStatusJSON_FreshnessStates(t *testing.T) {
	tests := []struct {
		name      string
		fresh     bool
		stale     bool
		veryStale bool
	}{
		{"fresh heartbeat", true, false, false},
		{"stale heartbeat", false, true, false},
		{"very stale heartbeat", false, false, true},
		{"no heartbeat (very stale)", false, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := DeaconStatusOutput{
				Running: true,
				Session: "gt-deacon",
				Heartbeat: &HeartbeatStatus{
					Fresh:     tc.fresh,
					Stale:     tc.stale,
					VeryStale: tc.veryStale,
				},
			}

			data, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded DeaconStatusOutput
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Heartbeat.Fresh != tc.fresh {
				t.Errorf("Fresh = %v, want %v", decoded.Heartbeat.Fresh, tc.fresh)
			}
			if decoded.Heartbeat.Stale != tc.stale {
				t.Errorf("Stale = %v, want %v", decoded.Heartbeat.Stale, tc.stale)
			}
			if decoded.Heartbeat.VeryStale != tc.veryStale {
				t.Errorf("VeryStale = %v, want %v", decoded.Heartbeat.VeryStale, tc.veryStale)
			}
		})
	}
}

// TestDeaconStatusJSON_LastActionOmitEmpty verifies last_action is omitted when empty.
func TestDeaconStatusJSON_LastActionOmitEmpty(t *testing.T) {
	out := DeaconStatusOutput{
		Running: true,
		Session: "gt-deacon",
		Heartbeat: &HeartbeatStatus{
			Cycle:      1,
			LastAction: "", // empty — should be omitted
		},
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	hb := m["heartbeat"].(map[string]interface{})
	if _, ok := hb["last_action"]; ok {
		t.Error("last_action should be omitted when empty")
	}
}
