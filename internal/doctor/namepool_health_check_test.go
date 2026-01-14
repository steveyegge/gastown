package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNamepoolHealthCheck_NoRigs(t *testing.T) {
	t.Parallel()

	// Create temp workspace without rigs.json
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewNamepoolHealthCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with no rigs, got %v: %s", result.Status, result.Message)
	}
}

func TestNamepoolHealthCheck_NoNamepool(t *testing.T) {
	t.Parallel()

	townRoot := setupTestWorkspace(t, "test-rig")

	check := NewNamepoolHealthCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with no namepool, got %v: %s", result.Status, result.Message)
	}
}

func TestNamepoolHealthCheck_ConsistentNamepool(t *testing.T) {
	t.Parallel()

	townRoot := setupTestWorkspace(t, "test-rig")

	// Create a polecat directory
	polecatsDir := filepath.Join(townRoot, "test-rig", "polecats")
	if err := os.MkdirAll(filepath.Join(polecatsDir, "furiosa"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create namepool state with furiosa in use
	runtimeDir := filepath.Join(townRoot, "test-rig", ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	state := namepoolState{
		RigName:      "test-rig",
		Theme:        "mad-max",
		InUse:        map[string]bool{"furiosa": true},
		OverflowNext: 51,
		MaxSize:      50,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(filepath.Join(runtimeDir, "namepool-state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	check := NewNamepoolHealthCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for consistent namepool, got %v: %s", result.Status, result.Message)
	}
}

func TestNamepoolHealthCheck_StaleEntry(t *testing.T) {
	t.Parallel()

	townRoot := setupTestWorkspace(t, "test-rig")

	// Create polecats directory (no polecat inside)
	polecatsDir := filepath.Join(townRoot, "test-rig", "polecats")
	if err := os.MkdirAll(polecatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create namepool state with ghost entry
	runtimeDir := filepath.Join(townRoot, "test-rig", ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	state := namepoolState{
		RigName:      "test-rig",
		Theme:        "mad-max",
		InUse:        map[string]bool{"ghost": true}, // No matching directory
		OverflowNext: 51,
		MaxSize:      50,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(filepath.Join(runtimeDir, "namepool-state.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	check := NewNamepoolHealthCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status == StatusOK {
		t.Error("expected non-OK status for stale namepool entry")
	}
	if len(check.staleEntries) != 1 {
		t.Errorf("expected 1 rig with stale entries, got %d", len(check.staleEntries))
	}
}

func TestNamepoolHealthCheck_Fix(t *testing.T) {
	t.Parallel()

	townRoot := setupTestWorkspace(t, "test-rig")
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create polecats directory
	if err := os.MkdirAll(filepath.Join(rigPath, "polecats"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create namepool state with stale entry
	runtimeDir := filepath.Join(rigPath, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(runtimeDir, "namepool-state.json")
	state := namepoolState{
		RigName:      "test-rig",
		Theme:        "mad-max",
		InUse:        map[string]bool{"ghost": true, "furiosa": true}, // ghost is stale
		OverflowNext: 51,
		MaxSize:      50,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Only create furiosa polecat
	if err := os.MkdirAll(filepath.Join(rigPath, "polecats", "furiosa"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewNamepoolHealthCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run to detect
	_ = check.Run(ctx)

	// Fix
	if err := check.Fix(ctx); err != nil {
		t.Errorf("Fix failed: %v", err)
	}

	// Read back and verify ghost is gone
	data, _ = os.ReadFile(statePath)
	var newState namepoolState
	_ = json.Unmarshal(data, &newState)

	if newState.InUse["ghost"] {
		t.Error("Fix did not remove ghost entry")
	}
	if !newState.InUse["furiosa"] {
		t.Error("Fix incorrectly removed valid furiosa entry")
	}
}

// setupTestWorkspace creates a minimal workspace with a rig
func setupTestWorkspace(t *testing.T, rigName string) string {
	t.Helper()

	townRoot := t.TempDir()

	// Create mayor directory with rigs.json
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	rigs := map[string]interface{}{
		rigName: map[string]interface{}{},
	}
	data, _ := json.MarshalIndent(rigs, "", "  ")
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig directory
	rigPath := filepath.Join(townRoot, rigName)
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	return townRoot
}
