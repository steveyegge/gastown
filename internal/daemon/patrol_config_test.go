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
