package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureExistingAgentHooks_NoWorkspace(t *testing.T) {
	// Save and change to temp dir (no workspace)
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	count := configureExistingAgentHooks()
	if count != 0 {
		t.Errorf("configureExistingAgentHooks() = %d, want 0 (no workspace)", count)
	}
}

func TestConfigureExistingAgentHooks_MayorOnly(t *testing.T) {
	// Create minimal workspace with just mayor
	tmpDir := t.TempDir()
	setupMinimalWorkspace(t, tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	count := configureExistingAgentHooks()
	// Should configure mayor (deacon dir doesn't exist in minimal setup)
	if count < 1 {
		t.Errorf("configureExistingAgentHooks() = %d, want >= 1 (at least mayor)", count)
	}
}

func TestConfigureExistingAgentHooks_WithRig(t *testing.T) {
	// Create workspace with mayor, deacon, and a rig
	tmpDir := t.TempDir()
	setupWorkspaceWithRig(t, tmpDir, "testrig")

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	count := configureExistingAgentHooks()
	// Should configure: mayor, deacon, witness, refinery = 4
	if count < 4 {
		t.Errorf("configureExistingAgentHooks() = %d, want >= 4", count)
	}
}

func TestConfigureExistingAgentHooks_WithPolecats(t *testing.T) {
	tmpDir := t.TempDir()
	setupWorkspaceWithRig(t, tmpDir, "testrig")

	// Add polecats
	polecatsDir := filepath.Join(tmpDir, "testrig", "polecats")
	os.MkdirAll(filepath.Join(polecatsDir, "alpha"), 0755)
	os.MkdirAll(filepath.Join(polecatsDir, "beta"), 0755)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	count := configureExistingAgentHooks()
	// mayor + deacon + witness + refinery + 2 polecats = 6
	if count < 6 {
		t.Errorf("configureExistingAgentHooks() = %d, want >= 6", count)
	}
}

func TestConfigureExistingAgentHooks_WithCrew(t *testing.T) {
	tmpDir := t.TempDir()
	setupWorkspaceWithRig(t, tmpDir, "testrig")

	// Add crew members
	crewDir := filepath.Join(tmpDir, "testrig", "crew")
	os.MkdirAll(filepath.Join(crewDir, "alice"), 0755)
	os.MkdirAll(filepath.Join(crewDir, "bob"), 0755)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	count := configureExistingAgentHooks()
	// mayor + deacon + witness + refinery + 2 crew = 6
	if count < 6 {
		t.Errorf("configureExistingAgentHooks() = %d, want >= 6", count)
	}
}

// setupMinimalWorkspace creates a minimal Gas Town workspace structure
func setupMinimalWorkspace(t *testing.T, root string) {
	t.Helper()

	// Create mayor directory with town.json
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("Failed to create mayor dir: %v", err)
	}

	// Create town.json (required for workspace detection)
	townConfig := map[string]interface{}{
		"name":    "test-town",
		"version": 2,
	}
	townData, _ := json.Marshal(townConfig)
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), townData, 0644); err != nil {
		t.Fatalf("Failed to create town.json: %v", err)
	}

	// Create empty rigs.json
	rigsConfig := map[string]interface{}{
		"version": 1,
		"rigs":    map[string]interface{}{},
	}
	rigsData, _ := json.Marshal(rigsConfig)
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), rigsData, 0644); err != nil {
		t.Fatalf("Failed to create rigs.json: %v", err)
	}
}

// setupWorkspaceWithRig creates a workspace with a registered rig
func setupWorkspaceWithRig(t *testing.T, root, rigName string) {
	t.Helper()

	// Create mayor directory
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("Failed to create mayor dir: %v", err)
	}

	// Create deacon directory
	deaconDir := filepath.Join(root, "deacon")
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		t.Fatalf("Failed to create deacon dir: %v", err)
	}
	_ = deaconDir // used for side effect

	// Create town.json
	townConfig := map[string]interface{}{
		"name":    "test-town",
		"version": 2,
	}
	townData, _ := json.Marshal(townConfig)
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), townData, 0644); err != nil {
		t.Fatalf("Failed to create town.json: %v", err)
	}

	// Create rigs.json with the rig registered
	rigsConfig := map[string]interface{}{
		"version": 1,
		"rigs": map[string]interface{}{
			rigName: map[string]interface{}{
				"git_url":    "https://github.com/test/repo",
				"local_repo": filepath.Join(root, rigName),
			},
		},
	}
	rigsData, _ := json.Marshal(rigsConfig)
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), rigsData, 0644); err != nil {
		t.Fatalf("Failed to create rigs.json: %v", err)
	}

	// Create rig directories
	// Note: witness and refinery run from their rig subdirectory (e.g., witness/rig)
	// The configureExistingAgentHooks function looks for these paths
	rigDir := filepath.Join(root, rigName)
	os.MkdirAll(filepath.Join(rigDir, "witness", "rig"), 0755)
	os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755)
	os.MkdirAll(filepath.Join(rigDir, "polecats"), 0755)
	os.MkdirAll(filepath.Join(rigDir, "crew"), 0755)
}
