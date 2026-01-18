package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestParseProtectedBranches(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single branch",
			input:    "main",
			expected: []string{"main"},
		},
		{
			name:     "two branches",
			input:    "main,master",
			expected: []string{"main", "master"},
		},
		{
			name:     "with spaces",
			input:    "main, master, develop",
			expected: []string{"main", "master", "develop"},
		},
		{
			name:     "trailing comma",
			input:    "main,master,",
			expected: []string{"main", "master"},
		},
		{
			name:     "empty entries",
			input:    "main,,master",
			expected: []string{"main", "master"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProtectedBranches(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseProtectedBranches(%q) returned %d items, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseProtectedBranches(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestGeneratePrePushHook(t *testing.T) {
	tests := []struct {
		name             string
		branches         []string
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			name:     "main and master",
			branches: []string{"main", "master"},
			shouldContain: []string{
				"main|master",
				"Protected branches",
				"main, master",
				"polecat/*",
				"beads-sync",
			},
		},
		{
			name:     "single branch",
			branches: []string{"main"},
			shouldContain: []string{
				"main)",
				"Protected branches",
			},
		},
		{
			name:     "multiple branches",
			branches: []string{"main", "master", "production"},
			shouldContain: []string{
				"main|master|production",
				"main, master, production",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := generatePrePushHook(tt.branches)

			for _, s := range tt.shouldContain {
				if !strings.Contains(hook, s) {
					t.Errorf("generatePrePushHook(%v) should contain %q", tt.branches, s)
				}
			}

			for _, s := range tt.shouldNotContain {
				if strings.Contains(hook, s) {
					t.Errorf("generatePrePushHook(%v) should not contain %q", tt.branches, s)
				}
			}

			// Verify it's a valid bash script
			if !strings.HasPrefix(hook, "#!/bin/bash") {
				t.Error("hook should start with #!/bin/bash")
			}
		})
	}
}

func TestInstallProtectedBranchesHook(t *testing.T) {
	tmpDir := t.TempDir()

	branches := []string{"main", "master"}
	err := installProtectedBranchesHook(tmpDir, branches)
	if err != nil {
		t.Fatalf("installProtectedBranchesHook failed: %v", err)
	}

	// Verify .githooks directory was created
	hooksDir := filepath.Join(tmpDir, ".githooks")
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		t.Error(".githooks directory was not created")
	}

	// Verify pre-push hook was created
	hookPath := filepath.Join(hooksDir, "pre-push")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed to read pre-push hook: %v", err)
	}

	// Verify hook content
	hookStr := string(content)
	if !strings.Contains(hookStr, "main|master") {
		t.Error("hook should contain the protected branches pattern")
	}
	if !strings.Contains(hookStr, "Gas Town pre-push hook") {
		t.Error("hook should be identified as Gas Town hook")
	}

	// Verify hook is executable
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("failed to stat hook: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("hook should be executable")
	}
}

func TestTownSettingsProtectedBranches(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings directory
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("failed to create settings dir: %v", err)
	}

	settingsPath := filepath.Join(settingsDir, "config.json")

	// Create and save settings with protected branches
	settings := config.NewTownSettings()
	settings.ProtectedBranches = []string{"main", "master"}

	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Load and verify
	loaded, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}

	if len(loaded.ProtectedBranches) != 2 {
		t.Errorf("expected 2 protected branches, got %d", len(loaded.ProtectedBranches))
	}
	if loaded.ProtectedBranches[0] != "main" {
		t.Errorf("expected first branch to be 'main', got %q", loaded.ProtectedBranches[0])
	}
	if loaded.ProtectedBranches[1] != "master" {
		t.Errorf("expected second branch to be 'master', got %q", loaded.ProtectedBranches[1])
	}
}

func TestTownSettingsProtectedBranchesEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings directory
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("failed to create settings dir: %v", err)
	}

	settingsPath := filepath.Join(settingsDir, "config.json")

	// Create and save settings without protected branches
	settings := config.NewTownSettings()
	// Don't set ProtectedBranches

	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Load and verify
	loaded, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}

	if len(loaded.ProtectedBranches) != 0 {
		t.Errorf("expected 0 protected branches, got %d", len(loaded.ProtectedBranches))
	}
}

// TestPrePushHookBlocksProtectedBranches verifies the generated hook script
// correctly blocks pushes to protected branches and allows others.
func TestPrePushHookBlocksProtectedBranches(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tmpDir := t.TempDir()
	branches := []string{"main", "master"}

	// Install the hook
	if err := installProtectedBranchesHook(tmpDir, branches); err != nil {
		t.Fatalf("installProtectedBranchesHook failed: %v", err)
	}

	hookPath := filepath.Join(tmpDir, ".githooks", "pre-push")

	tests := []struct {
		name       string
		branch     string
		shouldFail bool
	}{
		{"blocks main", "main", true},
		{"blocks master", "master", true},
		{"allows polecat branch", "polecat/toast", false},
		{"allows beads-sync", "beads-sync", false},
		{"blocks feature branch", "feature/foo", true},
		{"blocks arbitrary branch", "my-branch", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate git pre-push input: local_ref local_sha remote_ref remote_sha
			input := "refs/heads/" + tt.branch + " abc123 refs/heads/" + tt.branch + " def456\n"

			cmd := exec.Command("bash", hookPath)
			cmd.Stdin = strings.NewReader(input)
			output, err := cmd.CombinedOutput()

			if tt.shouldFail {
				if err == nil {
					t.Errorf("expected hook to block push to %s, but it succeeded", tt.branch)
				}
				if !strings.Contains(string(output), "ERROR") {
					t.Errorf("expected error message for blocked push, got: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("expected hook to allow push to %s, but it failed: %v\nOutput: %s", tt.branch, err, output)
				}
			}
		})
	}
}

// TestPrePushHookWithNoBranches verifies behavior when no branches are protected.
func TestPrePushHookWithNoBranches(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tmpDir := t.TempDir()

	// Generate hook with empty branches - should still block arbitrary branches
	// but allow polecat/* and beads-sync
	hook := generatePrePushHook([]string{})

	hooksDir := filepath.Join(tmpDir, ".githooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := os.WriteFile(hookPath, []byte(hook), 0755); err != nil {
		t.Fatalf("failed to write hook: %v", err)
	}

	// With empty protected branches, the pattern becomes just ")" which matches nothing
	// So main should be allowed through the protected check but blocked by the catchall
	input := "refs/heads/main abc123 refs/heads/main def456\n"
	cmd := exec.Command("bash", hookPath)
	cmd.Stdin = strings.NewReader(input)
	_, err := cmd.CombinedOutput()

	// With empty branches, main is NOT in protected list, so falls through to catchall
	// which blocks arbitrary branches
	if err == nil {
		t.Error("expected hook to block main when it's not in allowed list")
	}
}

// TestHookUpdatesWhenBranchesChange verifies the hook is regenerated correctly.
func TestHookUpdatesWhenBranchesChange(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, ".githooks", "pre-push")

	// First install with main,master
	if err := installProtectedBranchesHook(tmpDir, []string{"main", "master"}); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Verify main is blocked
	input := "refs/heads/main abc123 refs/heads/main def456\n"
	cmd := exec.Command("bash", hookPath)
	cmd.Stdin = strings.NewReader(input)
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Error("main should be blocked")
	}

	// Now update to only protect production
	if err := installProtectedBranchesHook(tmpDir, []string{"production"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	// Verify production is now blocked
	input = "refs/heads/production abc123 refs/heads/production def456\n"
	cmd = exec.Command("bash", hookPath)
	cmd.Stdin = strings.NewReader(input)
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Error("production should be blocked")
	}

	// main is no longer protected, but still blocked by catchall (not polecat/*)
	input = "refs/heads/main abc123 refs/heads/main def456\n"
	cmd = exec.Command("bash", hookPath)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("main should still be blocked by catchall")
	}
	// Should be blocked by catchall, not protected branches error
	if strings.Contains(string(output), "protected branch") {
		t.Error("main should be blocked by catchall, not protected branches")
	}
}

// TestResolveProtectedBranches verifies the rig-level override behavior.
func TestResolveProtectedBranches(t *testing.T) {
	tests := []struct {
		name         string
		townBranches []string
		rigBranches  []string // nil means no override, empty slice means explicitly empty
		expected     []string
	}{
		{
			name:         "town only",
			townBranches: []string{"main", "master"},
			rigBranches:  nil,
			expected:     []string{"main", "master"},
		},
		{
			name:         "rig override",
			townBranches: []string{"main", "master"},
			rigBranches:  []string{"develop"},
			expected:     []string{"develop"},
		},
		{
			name:         "rig empty override clears protection",
			townBranches: []string{"main", "master"},
			rigBranches:  []string{},
			expected:     []string{},
		},
		{
			name:         "no town no rig",
			townBranches: nil,
			rigBranches:  nil,
			expected:     nil,
		},
		{
			name:         "rig only no town",
			townBranches: nil,
			rigBranches:  []string{"staging"},
			expected:     []string{"staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var townSettings *config.TownSettings
			if tt.townBranches != nil {
				townSettings = config.NewTownSettings()
				townSettings.ProtectedBranches = tt.townBranches
			}

			var rigSettings *config.RigSettings
			if tt.rigBranches != nil {
				rigSettings = config.NewRigSettings()
				rigSettings.ProtectedBranches = tt.rigBranches
			}

			result := config.ResolveProtectedBranches(townSettings, rigSettings)

			if len(result) != len(tt.expected) {
				t.Errorf("ResolveProtectedBranches() returned %v, want %v", result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ResolveProtectedBranches()[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

// TestRigSettingsProtectedBranches verifies RigSettings save/load for protected branches.
func TestRigSettingsProtectedBranches(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rig directory structure
	rigDir := filepath.Join(tmpDir, "rigs", "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	settingsPath := config.RigSettingsPath(rigDir)

	// Test 1: Create and save rig settings with protected branches
	settings := config.NewRigSettings()
	settings.ProtectedBranches = []string{"staging", "production"}

	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatalf("failed to save rig settings: %v", err)
	}

	// Load and verify
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load rig settings: %v", err)
	}

	if len(loaded.ProtectedBranches) != 2 {
		t.Errorf("expected 2 protected branches, got %d", len(loaded.ProtectedBranches))
	}
	if loaded.ProtectedBranches[0] != "staging" {
		t.Errorf("expected first branch to be 'staging', got %q", loaded.ProtectedBranches[0])
	}
	if loaded.ProtectedBranches[1] != "production" {
		t.Errorf("expected second branch to be 'production', got %q", loaded.ProtectedBranches[1])
	}

	// Test 2: Clear protected branches (inherit from town)
	settings.ProtectedBranches = nil
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatalf("failed to save rig settings: %v", err)
	}

	loaded, err = config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load rig settings: %v", err)
	}

	if loaded.ProtectedBranches != nil {
		t.Errorf("expected nil protected branches after clearing, got %v", loaded.ProtectedBranches)
	}
}

// TestPerRigProtectionResolution verifies end-to-end resolution with files.
func TestPerRigProtectionResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town structure
	townRoot := tmpDir
	settingsDir := filepath.Join(townRoot, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("failed to create settings dir: %v", err)
	}

	// Create rig structure
	rigDir := filepath.Join(townRoot, "rigs", "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	townSettingsPath := config.TownSettingsPath(townRoot)
	rigSettingsPath := config.RigSettingsPath(rigDir)

	// Scenario 1: Town has protection, rig has no override
	townSettings := config.NewTownSettings()
	townSettings.ProtectedBranches = []string{"main", "master"}
	if err := config.SaveTownSettings(townSettingsPath, townSettings); err != nil {
		t.Fatalf("failed to save town settings: %v", err)
	}

	// Load both and resolve
	loadedTown, _ := config.LoadOrCreateTownSettings(townSettingsPath)
	loadedRig, _ := config.LoadRigSettings(rigSettingsPath) // Will return nil/error since no file

	result := config.ResolveProtectedBranches(loadedTown, loadedRig)
	if len(result) != 2 || result[0] != "main" || result[1] != "master" {
		t.Errorf("expected [main, master], got %v", result)
	}

	// Scenario 2: Add rig override
	rigSettings := config.NewRigSettings()
	rigSettings.ProtectedBranches = []string{"develop"}
	if err := config.SaveRigSettings(rigSettingsPath, rigSettings); err != nil {
		t.Fatalf("failed to save rig settings: %v", err)
	}

	loadedRig, err := config.LoadRigSettings(rigSettingsPath)
	if err != nil {
		t.Fatalf("failed to load rig settings: %v", err)
	}

	result = config.ResolveProtectedBranches(loadedTown, loadedRig)
	if len(result) != 1 || result[0] != "develop" {
		t.Errorf("expected [develop], got %v", result)
	}

	// Scenario 3: Rig clears protection (empty list override)
	rigSettings.ProtectedBranches = []string{}
	if err := config.SaveRigSettings(rigSettingsPath, rigSettings); err != nil {
		t.Fatalf("failed to save rig settings: %v", err)
	}

	loadedRig, err = config.LoadRigSettings(rigSettingsPath)
	if err != nil {
		t.Fatalf("failed to load rig settings: %v", err)
	}

	result = config.ResolveProtectedBranches(loadedTown, loadedRig)
	if len(result) != 0 {
		t.Errorf("expected empty list, got %v", result)
	}
}
