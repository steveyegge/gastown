package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateConstants(t *testing.T) {
	tests := []struct {
		name  string
		state State
		value string
	}{
		{"StateStopped", StateStopped, "stopped"},
		{"StateRunning", StateRunning, "running"},
		{"StatePaused", StatePaused, "paused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.value {
				t.Errorf("State constant = %q, want %q", tt.state, tt.value)
			}
		})
	}
}

func TestStateManager_StateFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewStateManager[TestState](tmpDir, "test-state.json", func() *TestState {
		return &TestState{Value: "default"}
	})

	expectedPath := filepath.Join(tmpDir, ".runtime", "test-state.json")
	if manager.StateFile() != expectedPath {
		t.Errorf("StateFile() = %q, want %q", manager.StateFile(), expectedPath)
	}
}

func TestStateManager_Load_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewStateManager[TestState](tmpDir, "nonexistent.json", func() *TestState {
		return &TestState{Value: "default"}
	})

	state, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.Value != "default" {
		t.Errorf("Load() value = %q, want %q", state.Value, "default")
	}
}

func TestStateManager_Load_Save_Load(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewStateManager[TestState](tmpDir, "test-state.json", func() *TestState {
		return &TestState{Value: "default"}
	})

	// Save initial state
	state := &TestState{Value: "test-value", Count: 42}
	if err := manager.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load it back
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Value != state.Value {
		t.Errorf("Load() value = %q, want %q", loaded.Value, state.Value)
	}
	if loaded.Count != state.Count {
		t.Errorf("Load() count = %d, want %d", loaded.Count, state.Count)
	}
}

func TestStateManager_Load_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewStateManager[TestState](tmpDir, "test-state.json", func() *TestState {
		return &TestState{Value: "default"}
	})

	// Save should create .runtime directory
	state := &TestState{Value: "test"}
	if err := manager.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify directory was created
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if _, err := os.Stat(runtimeDir); err != nil {
		t.Errorf("Save() should create .runtime directory: %v", err)
	}
}

func TestStateManager_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewStateManager[TestState](tmpDir, "test-state.json", func() *TestState {
		return &TestState{Value: "default"}
	})

	// Write invalid JSON
	statePath := manager.StateFile()
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err := manager.Load()
	if err == nil {
		t.Error("Load() with invalid JSON should return error")
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateStopped, "stopped"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.want {
			t.Errorf("State(%q) = %q, want %q", tt.state, string(tt.state), tt.want)
		}
	}
}

func TestStateManager_GenericType(t *testing.T) {
	// Test that StateManager works with different types

	type ComplexState struct {
		Name      string   `json:"name"`
		Values    []int    `json:"values"`
		Enabled   bool     `json:"enabled"`
		Nested    struct {
			X int `json:"x"`
		} `json:"nested"`
	}

	tmpDir := t.TempDir()
	manager := NewStateManager[ComplexState](tmpDir, "complex.json", func() *ComplexState {
		return &ComplexState{Name: "default", Values: []int{}}
	})

	original := &ComplexState{
		Name:    "test",
		Values:  []int{1, 2, 3},
		Enabled: true,
	}
	original.Nested.X = 42

	if err := manager.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if len(loaded.Values) != len(original.Values) {
		t.Errorf("Values length = %d, want %d", len(loaded.Values), len(original.Values))
	}
	if loaded.Enabled != original.Enabled {
		t.Errorf("Enabled = %v, want %v", loaded.Enabled, original.Enabled)
	}
	if loaded.Nested.X != original.Nested.X {
		t.Errorf("Nested.X = %d, want %d", loaded.Nested.X, original.Nested.X)
	}
}

// TestState is a simple type for testing
type TestState struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}
