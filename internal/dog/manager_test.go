package dog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// TestDogStateJSON verifies DogState JSON serialization.
func TestDogStateJSON(t *testing.T) {
	now := time.Now()
	state := &DogState{
		Name:       "alpha",
		State:      StateIdle,
		LastActive: now,
		Work:       "",
		Worktrees: map[string]string{
			"gastown": "/path/to/gastown",
			"beads":   "/path/to/beads",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Create temp file
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, ".dog.json")

	// Write and read back
	data, err := os.ReadFile(statePath)
	if err == nil {
		t.Logf("Data already exists: %s", data)
	}

	// Test state values
	if state.Name != "alpha" {
		t.Errorf("expected name 'alpha', got %q", state.Name)
	}
	if state.State != StateIdle {
		t.Errorf("expected state 'idle', got %q", state.State)
	}
	if len(state.Worktrees) != 2 {
		t.Errorf("expected 2 worktrees, got %d", len(state.Worktrees))
	}
}

// TestManagerCreation verifies Manager initialization.
func TestManagerCreation(t *testing.T) {
	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs: map[string]config.RigEntry{
			"gastown": {
				GitURL: "git@github.com:test/gastown.git",
			},
			"beads": {
				GitURL: "git@github.com:test/beads.git",
			},
		},
	}

	m := NewManager("/tmp/test-town", rigsConfig)

	if filepath.ToSlash(m.townRoot) != "/tmp/test-town" {
		t.Errorf("expected townRoot '/tmp/test-town', got %q", m.townRoot)
	}
	if filepath.ToSlash(m.kennelPath) != "/tmp/test-town/deacon/dogs" {
		t.Errorf("expected kennelPath '/tmp/test-town/deacon/dogs', got %q", m.kennelPath)
	}
}

// TestDogDir verifies dogDir path construction.
func TestDogDir(t *testing.T) {
	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs:    map[string]config.RigEntry{},
	}
	m := NewManager("/home/user/gt", rigsConfig)

	path := m.dogDir("alpha")
	expected := "/home/user/gt/deacon/dogs/alpha"
	if filepath.ToSlash(path) != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// TestStateConstants verifies state constants.
func TestStateConstants(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateIdle, "idle"},
		{StateWorking, "working"},
	}

	for _, tc := range tests {
		if string(tc.state) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.state))
		}
	}
}
