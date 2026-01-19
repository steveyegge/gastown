package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRegistry(t *testing.T) {
	// All providers should be registered via init()
	names := Names()
	if len(names) < 3 {
		t.Errorf("Expected at least 3 providers (claude, opencode, none), got %d", len(names))
	}

	// Check specific providers exist
	providers := []string{"claude", "opencode", "none"}
	for _, name := range providers {
		if Get(name) == nil {
			t.Errorf("Provider %q not registered", name)
		}
	}
}

func TestGet_UnknownProvider(t *testing.T) {
	p := Get("nonexistent")
	if p != nil {
		t.Errorf("Get() for unknown provider should return nil, got %v", p)
	}
}

func TestClaudeProvider_Name(t *testing.T) {
	p := Get("claude")
	if p == nil {
		t.Fatal("Claude provider not registered")
	}
	if p.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", p.Name(), "claude")
	}
}

func TestOpencodeProvider_Name(t *testing.T) {
	p := Get("opencode")
	if p == nil {
		t.Fatal("OpenCode provider not registered")
	}
	if p.Name() != "opencode" {
		t.Errorf("Name() = %q, want %q", p.Name(), "opencode")
	}
}

func TestNoneProvider_Name(t *testing.T) {
	p := Get("none")
	if p == nil {
		t.Fatal("None provider not registered")
	}
	if p.Name() != "none" {
		t.Errorf("Name() = %q, want %q", p.Name(), "none")
	}
}

func TestNoneProvider_EnsureHooks(t *testing.T) {
	p := Get("none")
	if p == nil {
		t.Fatal("None provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider:     "none",
		Dir:          ".test",
		SettingsFile: "test.json",
	}

	err := p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("EnsureHooks() = %v, want nil", err)
	}

	// None provider should not create any files
	testDir := filepath.Join(tmpDir, ".test")
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Errorf("None provider should not create directory, but %s exists", testDir)
	}
}

func TestClaudeProvider_EnsureHooks(t *testing.T) {
	p := Get("claude")
	if p == nil {
		t.Fatal("Claude provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider:     "claude",
		Dir:          ".claude",
		SettingsFile: "settings.json",
	}

	err := p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("EnsureHooks() = %v, want nil", err)
	}

	// Should create settings file
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Errorf("Claude provider should create %s", settingsPath)
	}
}

func TestClaudeProvider_EnsureHooks_Idempotent(t *testing.T) {
	p := Get("claude")
	if p == nil {
		t.Fatal("Claude provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider:     "claude",
		Dir:          ".claude",
		SettingsFile: "settings.json",
	}

	// Call twice - should not error
	err := p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("First EnsureHooks() = %v, want nil", err)
	}

	err = p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("Second EnsureHooks() = %v, want nil", err)
	}
}

func TestClaudeProvider_EnsureHooks_DefaultPaths(t *testing.T) {
	p := Get("claude")
	if p == nil {
		t.Fatal("Claude provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider: "claude",
		// Empty Dir and SettingsFile - should use defaults
	}

	err := p.EnsureHooks(tmpDir, "mayor", cfg)
	if err != nil {
		t.Errorf("EnsureHooks() = %v, want nil", err)
	}

	// Should create default settings file
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Errorf("Claude provider should create default %s", settingsPath)
	}
}

func TestOpencodeProvider_EnsureHooks(t *testing.T) {
	p := Get("opencode")
	if p == nil {
		t.Fatal("OpenCode provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider:     "opencode",
		Dir:          ".opencode/plugin",
		SettingsFile: "gastown.js",
	}

	err := p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("EnsureHooks() = %v, want nil", err)
	}

	// Should create plugin file
	pluginPath := filepath.Join(tmpDir, ".opencode/plugin", "gastown.js")
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Errorf("OpenCode provider should create %s", pluginPath)
	}
}

func TestOpencodeProvider_EnsureHooks_EmptyConfig(t *testing.T) {
	p := Get("opencode")
	if p == nil {
		t.Fatal("OpenCode provider not registered")
	}

	tmpDir := t.TempDir()
	cfg := &config.RuntimeHooksConfig{
		Provider: "opencode",
		// Empty Dir and SettingsFile - opencode returns nil
	}

	err := p.EnsureHooks(tmpDir, "polecat", cfg)
	if err != nil {
		t.Errorf("EnsureHooks() with empty config = %v, want nil", err)
	}
}
