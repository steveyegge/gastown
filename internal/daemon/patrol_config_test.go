package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPatrolConfig(t *testing.T) {
	// Create a temp dir with test config
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test config
	configJSON := `{
		"type": "daemon-patrol-config",
		"version": 1,
		"patrols": {
			"refinery": {"enabled": false},
			"witness": {"enabled": true}
		}
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "daemon.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	config := LoadPatrolConfig(tmpDir)
	if config == nil {
		t.Fatal("expected config to be loaded")
	}

	// Test enabled flags
	if IsPatrolEnabled(config, "refinery") {
		t.Error("expected refinery to be disabled")
	}
	if !IsPatrolEnabled(config, "witness") {
		t.Error("expected witness to be enabled")
	}
	if !IsPatrolEnabled(config, "deacon") {
		t.Error("expected deacon to be enabled (default)")
	}
}

func TestIsPatrolEnabled_NilConfig(t *testing.T) {
	// Should default to enabled when config is nil
	if !IsPatrolEnabled(nil, "refinery") {
		t.Error("expected default to be enabled")
	}
}

func TestLoadPatrolConfig_FileNotFound(t *testing.T) {
	// Should return nil when file doesn't exist
	config := LoadPatrolConfig(t.TempDir())
	if config != nil {
		t.Error("expected nil config when file doesn't exist")
	}
}

func TestLoadPatrolConfig_InvalidJSON(t *testing.T) {
	// Create a temp dir with invalid JSON
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write invalid JSON
	if err := os.WriteFile(filepath.Join(mayorDir, "daemon.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should return nil for invalid JSON
	config := LoadPatrolConfig(tmpDir)
	if config != nil {
		t.Error("expected nil config for invalid JSON")
	}
}

func TestIsPatrolEnabled_AllPatrols(t *testing.T) {
	tests := []struct {
		name     string
		config   *DaemonPatrolConfig
		patrol   string
		expected bool
	}{
		{
			name:     "nil config defaults to enabled",
			config:   nil,
			patrol:   "refinery",
			expected: true,
		},
		{
			name:     "nil patrols defaults to enabled",
			config:   &DaemonPatrolConfig{},
			patrol:   "witness",
			expected: true,
		},
		{
			name: "refinery explicitly disabled",
			config: &DaemonPatrolConfig{
				Patrols: &PatrolsConfig{
					Refinery: &PatrolConfig{Enabled: false},
				},
			},
			patrol:   "refinery",
			expected: false,
		},
		{
			name: "witness explicitly enabled",
			config: &DaemonPatrolConfig{
				Patrols: &PatrolsConfig{
					Witness: &PatrolConfig{Enabled: true},
				},
			},
			patrol:   "witness",
			expected: true,
		},
		{
			name: "deacon explicitly disabled",
			config: &DaemonPatrolConfig{
				Patrols: &PatrolsConfig{
					Deacon: &PatrolConfig{Enabled: false},
				},
			},
			patrol:   "deacon",
			expected: false,
		},
		{
			name: "unknown patrol defaults to enabled",
			config: &DaemonPatrolConfig{
				Patrols: &PatrolsConfig{},
			},
			patrol:   "unknown",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPatrolEnabled(tt.config, tt.patrol)
			if result != tt.expected {
				t.Errorf("IsPatrolEnabled(%v, %q) = %v, want %v", tt.config, tt.patrol, result, tt.expected)
			}
		})
	}
}

func TestPatrolConfigFile(t *testing.T) {
	path := PatrolConfigFile("/home/user/.gastown")
	expected := "/home/user/.gastown/mayor/daemon.json"
	if path != expected {
		t.Errorf("PatrolConfigFile() = %q, want %q", path, expected)
	}
}
