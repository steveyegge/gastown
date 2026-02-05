package daemon

import (
	"encoding/json"
	"testing"
)

func TestDeepMergeDaemonConfig(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple override",
			dst: map[string]interface{}{
				"type":    "daemon-patrol-config",
				"version": float64(1),
			},
			src: map[string]interface{}{
				"version": float64(2),
			},
			expected: map[string]interface{}{
				"type":    "daemon-patrol-config",
				"version": float64(2),
			},
		},
		{
			name: "nested map merge",
			dst: map[string]interface{}{
				"patrols": map[string]interface{}{
					"refinery": map[string]interface{}{
						"enabled":  true,
						"interval": "5m",
					},
					"witness": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			src: map[string]interface{}{
				"patrols": map[string]interface{}{
					"refinery": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			expected: map[string]interface{}{
				"patrols": map[string]interface{}{
					"refinery": map[string]interface{}{
						"enabled":  false,
						"interval": "5m",
					},
					"witness": map[string]interface{}{
						"enabled": true,
					},
				},
			},
		},
		{
			name: "add new key",
			dst: map[string]interface{}{
				"patrols": map[string]interface{}{
					"refinery": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			src: map[string]interface{}{
				"patrols": map[string]interface{}{
					"dolt_server": map[string]interface{}{
						"enabled":  true,
						"port":     float64(3306),
						"data_dir": "/home/ubuntu/gt11/dolt",
					},
				},
			},
			expected: map[string]interface{}{
				"patrols": map[string]interface{}{
					"refinery": map[string]interface{}{
						"enabled": true,
					},
					"dolt_server": map[string]interface{}{
						"enabled":  true,
						"port":     float64(3306),
						"data_dir": "/home/ubuntu/gt11/dolt",
					},
				},
			},
		},
		{
			name:     "empty dst",
			dst:      map[string]interface{}{},
			src:      map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "empty src",
			dst:      map[string]interface{}{"key": "value"},
			src:      map[string]interface{}{},
			expected: map[string]interface{}{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deepMergeDaemonConfig(tt.dst, tt.src)

			dstJSON, _ := json.Marshal(tt.dst)
			expectedJSON, _ := json.Marshal(tt.expected)
			if string(dstJSON) != string(expectedJSON) {
				t.Errorf("deepMergeDaemonConfig mismatch:\n  got:    %s\n  want:   %s", dstJSON, expectedJSON)
			}
		})
	}
}

func TestDaemonConfigRoundTrip(t *testing.T) {
	// Verify that a DaemonPatrolConfig can round-trip through JSON
	// (simulating seed → bead metadata → load from beads)
	original := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Heartbeat: &PatrolConfig{
			Enabled:  true,
			Interval: "3m",
		},
		Patrols: &PatrolsConfig{
			Refinery: &PatrolConfig{Enabled: true, Interval: "5m", Agent: "refinery"},
			Witness:  &PatrolConfig{Enabled: true, Interval: "5m", Agent: "witness"},
			Deacon:   &PatrolConfig{Enabled: true, Interval: "5m", Agent: "deacon"},
		},
	}

	// Serialize (like seed does)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Deserialize (like LoadDaemonConfigFromBeads does)
	var restored DaemonPatrolConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("type: got %q, want %q", restored.Type, original.Type)
	}
	if restored.Version != original.Version {
		t.Errorf("version: got %d, want %d", restored.Version, original.Version)
	}
	if restored.Heartbeat == nil || !restored.Heartbeat.Enabled {
		t.Error("heartbeat should be enabled")
	}
	if restored.Heartbeat.Interval != "3m" {
		t.Errorf("heartbeat interval: got %q, want %q", restored.Heartbeat.Interval, "3m")
	}
	if restored.Patrols == nil {
		t.Fatal("patrols should not be nil")
	}
	if restored.Patrols.Refinery == nil || !restored.Patrols.Refinery.Enabled {
		t.Error("refinery patrol should be enabled")
	}
	if restored.Patrols.Witness == nil || !restored.Patrols.Witness.Enabled {
		t.Error("witness patrol should be enabled")
	}
	if restored.Patrols.Deacon == nil || !restored.Patrols.Deacon.Enabled {
		t.Error("deacon patrol should be enabled")
	}
}

func TestIsPatrolEnabledWithBeadConfig(t *testing.T) {
	// Simulate a config loaded from beads
	config := &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: &PatrolsConfig{
			Refinery: &PatrolConfig{Enabled: false},
			Witness:  &PatrolConfig{Enabled: true},
		},
	}

	if IsPatrolEnabled(config, "refinery") {
		t.Error("refinery should be disabled")
	}
	if !IsPatrolEnabled(config, "witness") {
		t.Error("witness should be enabled")
	}
	// Deacon not set → default enabled
	if !IsPatrolEnabled(config, "deacon") {
		t.Error("deacon should default to enabled when not configured")
	}
}
