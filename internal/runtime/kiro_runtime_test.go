package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// TestKiroHooksProviderSelection verifies that when the hooks provider is "kiro",
// EnsureSettingsForRole delegates to the kiro package and creates settings.
func TestKiroHooksProviderSelection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider:     "kiro",
			Dir:          ".kiro",
			SettingsFile: "settings.json",
		},
	}

	err := EnsureSettingsForRole(dir, "polecat", rc)
	if err != nil {
		t.Fatalf("EnsureSettingsForRole() with kiro provider error = %v", err)
	}

	// Verify the settings file was created in .kiro/
	settingsPath := filepath.Join(dir, ".kiro", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("expected settings file at %s, got error: %v", settingsPath, err)
	}
}

// TestKiroHooksProviderSelection_NotClaude verifies that selecting "kiro" does
// NOT create files in the .claude directory.
func TestKiroHooksProviderSelection_NotClaude(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider:     "kiro",
			Dir:          ".kiro",
			SettingsFile: "settings.json",
		},
	}

	err := EnsureSettingsForRole(dir, "polecat", rc)
	if err != nil {
		t.Fatalf("EnsureSettingsForRole() error = %v", err)
	}

	// .claude directory should NOT be created
	claudeDir := filepath.Join(dir, ".claude")
	if _, err := os.Stat(claudeDir); err == nil {
		t.Errorf(".claude directory should not exist when kiro provider is selected")
	}
}

// TestKiroFallbackCommand verifies that when hooks provider is set to "kiro",
// StartupFallbackCommands returns nil (because hooks are active, no fallback needed).
func TestKiroFallbackCommand(t *testing.T) {
	t.Parallel()

	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "kiro",
		},
	}

	commands := StartupFallbackCommands("polecat", rc)
	if commands != nil {
		t.Errorf("StartupFallbackCommands() with kiro hooks should return nil, got %v", commands)
	}
}

// TestKiroFallbackCommand_NoHooks verifies fallback commands when there is no hooks
// provider (simulating kiro without hook support).
func TestKiroFallbackCommand_NoHooks(t *testing.T) {
	t.Parallel()

	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	tests := []struct {
		role             string
		wantPrime        bool
		wantMailCheck    bool
		wantNudgeDeacon  bool
	}{
		{"polecat", true, true, true},
		{"mayor", true, false, true},
		{"crew", true, false, true},
		{"witness", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			commands := StartupFallbackCommands(tt.role, rc)
			if commands == nil || len(commands) == 0 {
				t.Fatal("StartupFallbackCommands() should return commands when hooks are 'none'")
			}

			joined := strings.Join(commands, " ")

			if tt.wantPrime && !strings.Contains(joined, "gt prime") {
				t.Errorf("fallback for %s should contain 'gt prime'", tt.role)
			}
			if tt.wantMailCheck && !strings.Contains(joined, "gt mail check --inject") {
				t.Errorf("fallback for %s should contain 'gt mail check --inject'", tt.role)
			}
			if !tt.wantMailCheck && strings.Contains(joined, "gt mail check --inject") {
				t.Errorf("fallback for %s should NOT contain 'gt mail check --inject'", tt.role)
			}
			if tt.wantNudgeDeacon && !strings.Contains(joined, "gt nudge deacon session-started") {
				t.Errorf("fallback for %s should contain 'gt nudge deacon session-started'", tt.role)
			}
		})
	}
}

// TestKiroEnsureSettings verifies EnsureSettingsForRole works correctly for
// different roles when the provider is "kiro".
func TestKiroEnsureSettings(t *testing.T) {
	t.Parallel()

	roles := []struct {
		role           string
		wantMailInject bool // autonomous roles have mail check --inject in SessionStart
	}{
		{"polecat", true},
		{"witness", true},
		{"refinery", true},
		{"deacon", true},
		{"mayor", false},
		{"crew", false},
	}

	for _, tt := range roles {
		t.Run(tt.role, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			rc := &config.RuntimeConfig{
				Hooks: &config.RuntimeHooksConfig{
					Provider:     "kiro",
					Dir:          ".kiro",
					SettingsFile: "settings.json",
				},
			}

			err := EnsureSettingsForRole(dir, tt.role, rc)
			if err != nil {
				t.Fatalf("EnsureSettingsForRole(%q) error = %v", tt.role, err)
			}

			settingsPath := filepath.Join(dir, ".kiro", "settings.json")
			data, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("reading settings file: %v", err)
			}
			content := string(data)

			// All roles should have gt prime --hook
			if !strings.Contains(content, "gt prime --hook") {
				t.Errorf("settings for role %q should contain 'gt prime --hook'", tt.role)
			}

			// Check for mail check --inject in SessionStart section
			// Autonomous roles should have it in SessionStart; interactive roles should not
			// We look for the pattern in the whole file (both have it in UserPromptSubmit)
			// but only autonomous roles have it in SessionStart alongside gt prime --hook
			sessionStartIdx := strings.Index(content, "SessionStart")
			if sessionStartIdx < 0 {
				t.Fatalf("settings for role %q should contain SessionStart hook", tt.role)
			}
			// Find the command string in SessionStart block
			preCompactIdx := strings.Index(content, "PreCompact")
			if preCompactIdx < 0 {
				t.Fatalf("settings for role %q should contain PreCompact hook", tt.role)
			}
			sessionStartBlock := content[sessionStartIdx:preCompactIdx]

			hasMailInSessionStart := strings.Contains(sessionStartBlock, "gt mail check --inject")
			if tt.wantMailInject && !hasMailInSessionStart {
				t.Errorf("autonomous role %q should have 'gt mail check --inject' in SessionStart", tt.role)
			}
			if !tt.wantMailInject && hasMailInSessionStart {
				t.Errorf("interactive role %q should NOT have 'gt mail check --inject' in SessionStart", tt.role)
			}
		})
	}
}

// TestKiroSettingsDirectory verifies that Kiro settings are created in the .kiro/
// directory, not some other location.
func TestKiroSettingsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider:     "kiro",
			Dir:          ".kiro",
			SettingsFile: "settings.json",
		},
	}

	err := EnsureSettingsForRole(dir, "polecat", rc)
	if err != nil {
		t.Fatalf("EnsureSettingsForRole() error = %v", err)
	}

	// Verify .kiro directory exists
	kiroDir := filepath.Join(dir, ".kiro")
	info, err := os.Stat(kiroDir)
	if err != nil {
		t.Fatalf(".kiro directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".kiro should be a directory")
	}

	// Verify settings.json exists within .kiro
	settingsPath := filepath.Join(kiroDir, "settings.json")
	finfo, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found in .kiro: %v", err)
	}
	if finfo.IsDir() {
		t.Errorf("settings.json should be a file, not a directory")
	}

	// Verify file has appropriate permissions (0600 for sensitive config)
	perm := finfo.Mode().Perm()
	if perm != 0600 {
		t.Errorf("settings.json permissions = %o, want 0600", perm)
	}
}

// TestKiroSettingsDirectory_CustomDir verifies that a custom settings directory
// is respected when provided.
func TestKiroSettingsDirectory_CustomDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider:     "kiro",
			Dir:          ".custom-kiro",
			SettingsFile: "custom-settings.json",
		},
	}

	err := EnsureSettingsForRole(dir, "crew", rc)
	if err != nil {
		t.Fatalf("EnsureSettingsForRole() error = %v", err)
	}

	customPath := filepath.Join(dir, ".custom-kiro", "custom-settings.json")
	if _, err := os.Stat(customPath); err != nil {
		t.Errorf("settings file not created at custom path %s: %v", customPath, err)
	}
}
