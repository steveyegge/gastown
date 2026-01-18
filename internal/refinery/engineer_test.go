package refinery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestDefaultMergeQueueConfig(t *testing.T) {
	cfg := DefaultMergeQueueConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.TargetBranch != "main" {
		t.Errorf("expected TargetBranch to be 'main', got %q", cfg.TargetBranch)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("expected PollInterval to be 30s, got %v", cfg.PollInterval)
	}
	if cfg.MaxConcurrent != 1 {
		t.Errorf("expected MaxConcurrent to be 1, got %d", cfg.MaxConcurrent)
	}
	if cfg.OnConflict != "assign_back" {
		t.Errorf("expected OnConflict to be 'assign_back', got %q", cfg.OnConflict)
	}
}

func TestEngineer_LoadConfig_NoFile(t *testing.T) {
	// Create a temp directory without config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	// Should not error with missing config file
	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error with missing config: %v", err)
	}

	// Should use defaults
	if e.config.PollInterval != 30*time.Second {
		t.Errorf("expected default PollInterval, got %v", e.config.PollInterval)
	}
}

func TestEngineer_LoadConfig_WithMergeQueue(t *testing.T) {
	// Create a temp directory with config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
		"merge_queue": map[string]interface{}{
			"enabled":        true,
			"target_branch":  "develop",
			"poll_interval":  "10s",
			"max_concurrent": 2,
			"run_tests":      false,
			"test_command":   "make test",
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error loading config: %v", err)
	}

	// Check that config values were loaded
	if e.config.TargetBranch != "develop" {
		t.Errorf("expected TargetBranch 'develop', got %q", e.config.TargetBranch)
	}
	if e.config.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", e.config.PollInterval)
	}
	if e.config.MaxConcurrent != 2 {
		t.Errorf("expected MaxConcurrent 2, got %d", e.config.MaxConcurrent)
	}
	if e.config.RunTests != false {
		t.Errorf("expected RunTests false, got %v", e.config.RunTests)
	}
	if e.config.TestCommand != "make test" {
		t.Errorf("expected TestCommand 'make test', got %q", e.config.TestCommand)
	}

	// Check that defaults are preserved for unspecified fields
	if e.config.OnConflict != "assign_back" {
		t.Errorf("expected OnConflict default 'assign_back', got %q", e.config.OnConflict)
	}
}

func TestEngineer_LoadConfig_NoMergeQueueSection(t *testing.T) {
	// Create a temp directory with config.json without merge_queue
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file without merge_queue
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error loading config: %v", err)
	}

	// Should use all defaults
	if e.config.PollInterval != 30*time.Second {
		t.Errorf("expected default PollInterval, got %v", e.config.PollInterval)
	}
}

func TestEngineer_LoadConfig_InvalidPollInterval(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := map[string]interface{}{
		"merge_queue": map[string]interface{}{
			"poll_interval": "not-a-duration",
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	err = e.LoadConfig()
	if err == nil {
		t.Error("expected error for invalid poll_interval")
	}
}

func TestNewEngineer(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}

	e := NewEngineer(r)

	if e.rig != r {
		t.Error("expected rig to be set")
	}
	if e.beads == nil {
		t.Error("expected beads client to be initialized")
	}
	if e.git == nil {
		t.Error("expected git client to be initialized")
	}
	if e.config == nil {
		t.Error("expected config to be initialized with defaults")
	}
}

func TestEngineer_DeleteMergedBranchesConfig(t *testing.T) {
	// Test that DeleteMergedBranches is true by default
	cfg := DefaultMergeQueueConfig()
	if !cfg.DeleteMergedBranches {
		t.Error("expected DeleteMergedBranches to be true by default")
	}
}

func TestEngineer_GetProtectedBranches(t *testing.T) {
	// Create a temp town structure
	tmpDir, err := os.MkdirTemp("", "engineer-protection-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create town structure: tmpDir/rigs/testrig
	townRoot := tmpDir
	rigPath := filepath.Join(townRoot, "rigs", "testrig")
	settingsDir := filepath.Join(townRoot, "settings")

	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Helper to write town settings
	writeTownSettings := func(branches []string) {
		settings := map[string]interface{}{
			"type":               "town-settings",
			"version":            1,
			"protected_branches": branches,
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644)
	}

	// Helper to write rig settings
	// Rig settings path is: rigPath/settings/config.json
	rigSettingsDir := filepath.Join(rigPath, "settings")
	writeRigSettings := func(branches []string) {
		os.MkdirAll(rigSettingsDir, 0755)
		settings := map[string]interface{}{
			"type":               "rig-settings",
			"version":            1,
			"protected_branches": branches,
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(filepath.Join(rigSettingsDir, "config.json"), data, 0644)
	}

	// Helper to remove rig settings
	removeRigSettings := func() {
		os.Remove(filepath.Join(rigSettingsDir, "config.json"))
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}
	e := NewEngineer(r)

	t.Run("uses town settings when no rig override", func(t *testing.T) {
		writeTownSettings([]string{"main", "master"})
		removeRigSettings()

		branches := e.getProtectedBranches()
		if len(branches) != 2 {
			t.Errorf("expected 2 branches, got %d", len(branches))
		}
		if len(branches) > 0 && branches[0] != "main" {
			t.Errorf("expected first branch to be 'main', got %q", branches[0])
		}
	})

	t.Run("rig override takes precedence", func(t *testing.T) {
		writeTownSettings([]string{"main", "master"})
		writeRigSettings([]string{"develop", "staging"})

		branches := e.getProtectedBranches()
		if len(branches) != 2 {
			t.Errorf("expected 2 branches, got %d", len(branches))
		}
		if len(branches) > 0 && branches[0] != "develop" {
			t.Errorf("expected first branch to be 'develop', got %q", branches[0])
		}
	})

	t.Run("rig can disable protection with empty list", func(t *testing.T) {
		writeTownSettings([]string{"main", "master"})
		writeRigSettings([]string{})

		branches := e.getProtectedBranches()
		if len(branches) != 0 {
			t.Errorf("expected 0 branches (disabled), got %d: %v", len(branches), branches)
		}
	})

	t.Run("no protection when neither town nor rig set", func(t *testing.T) {
		// Write town settings without protected_branches
		settings := map[string]interface{}{
			"type":    "town-settings",
			"version": 1,
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644)
		removeRigSettings()

		branches := e.getProtectedBranches()
		if len(branches) != 0 {
			t.Errorf("expected 0 branches, got %d", len(branches))
		}
	})
}

func TestEngineer_CheckMergePolicy_ProtectedBranches(t *testing.T) {
	// Create a temp town structure
	tmpDir, err := os.MkdirTemp("", "engineer-policy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create town structure
	townRoot := tmpDir
	rigPath := filepath.Join(townRoot, "rigs", "testrig")
	settingsDir := filepath.Join(townRoot, "settings")
	rigSettingsDir := filepath.Join(rigPath, "settings")

	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write town settings with protected branches
	townSettings := map[string]interface{}{
		"type":               "town-settings",
		"version":            1,
		"protected_branches": []string{"main", "master"},
	}
	data, _ := json.MarshalIndent(townSettings, "", "  ")
	os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644)

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}
	e := NewEngineer(r)

	// Note: CheckMergePolicy calls checkExistingGate when NeedsGate=true,
	// which tries to fetch the MR bead. Without a beads DB, this fails.
	// We test the core logic through getProtectedBranches tests above.
	// Here we just verify non-protected branches work (no bead fetch needed).

	t.Run("no gate required for non-protected branch", func(t *testing.T) {
		mr := &MRInfo{
			ID:     "gt-test456",
			Branch: "polecat/feature",
			Target: "develop", // Not protected
			Worker: "testrig/polecat/nux",
		}

		result, err := e.CheckMergePolicy(mr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// NeedsGate should be false since develop is not protected
		if result.NeedsGate {
			t.Error("expected NeedsGate=false for non-protected branch")
		}
	})

	t.Run("rig override makes main unprotected", func(t *testing.T) {
		// Write rig settings that only protect 'staging'
		os.MkdirAll(rigSettingsDir, 0755)
		rigSettings := map[string]interface{}{
			"type":               "rig-settings",
			"version":            1,
			"protected_branches": []string{"staging"},
		}
		data, _ := json.MarshalIndent(rigSettings, "", "  ")
		os.WriteFile(filepath.Join(rigSettingsDir, "config.json"), data, 0644)

		// main is no longer protected (rig override)
		mr := &MRInfo{
			ID:     "gt-test789",
			Branch: "polecat/feature",
			Target: "main",
			Worker: "testrig/polecat/nux",
		}

		result, err := e.CheckMergePolicy(mr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.NeedsGate {
			t.Error("expected NeedsGate=false since rig override doesn't protect main")
		}

		// Cleanup
		os.Remove(filepath.Join(rigSettingsDir, "config.json"))
	})

	t.Run("rig empty override disables all protection", func(t *testing.T) {
		// Write rig settings with empty protection (disables town protection)
		os.MkdirAll(rigSettingsDir, 0755)
		rigSettings := map[string]interface{}{
			"type":               "rig-settings",
			"version":            1,
			"protected_branches": []string{},
		}
		data, _ := json.MarshalIndent(rigSettings, "", "  ")
		os.WriteFile(filepath.Join(rigSettingsDir, "config.json"), data, 0644)

		// main is no longer protected (rig empty override)
		mr := &MRInfo{
			ID:     "gt-testABC",
			Branch: "polecat/feature",
			Target: "main",
			Worker: "testrig/polecat/nux",
		}

		result, err := e.CheckMergePolicy(mr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.NeedsGate {
			t.Error("expected NeedsGate=false since rig override disables all protection")
		}

		// Cleanup
		os.Remove(filepath.Join(rigSettingsDir, "config.json"))
	})
}
