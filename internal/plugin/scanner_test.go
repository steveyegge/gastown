package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePluginMD(t *testing.T) {
	content := []byte(`+++
name = "test-plugin"
description = "A test plugin"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["test:label"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
+++

# Test Plugin

These are the instructions.
`)

	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}

	if plugin.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", plugin.Name)
	}
	if plugin.Description != "A test plugin" {
		t.Errorf("expected description 'A test plugin', got %q", plugin.Description)
	}
	if plugin.Version != 1 {
		t.Errorf("expected version 1, got %d", plugin.Version)
	}
	if plugin.Location != LocationTown {
		t.Errorf("expected location 'town', got %q", plugin.Location)
	}
	if plugin.Gate == nil {
		t.Fatal("expected gate to be non-nil")
	}
	if plugin.Gate.Type != GateCooldown {
		t.Errorf("expected gate type 'cooldown', got %q", plugin.Gate.Type)
	}
	if plugin.Gate.Duration != "1h" {
		t.Errorf("expected gate duration '1h', got %q", plugin.Gate.Duration)
	}
	if plugin.Tracking == nil {
		t.Fatal("expected tracking to be non-nil")
	}
	if len(plugin.Tracking.Labels) != 1 || plugin.Tracking.Labels[0] != "test:label" {
		t.Errorf("expected labels ['test:label'], got %v", plugin.Tracking.Labels)
	}
	if !plugin.Tracking.Digest {
		t.Error("expected digest to be true")
	}
	if plugin.Execution == nil {
		t.Fatal("expected execution to be non-nil")
	}
	if plugin.Execution.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", plugin.Execution.Timeout)
	}
	if !plugin.Execution.NotifyOnFailure {
		t.Error("expected notify_on_failure to be true")
	}
	if plugin.Instructions == "" {
		t.Error("expected instructions to be non-empty")
	}
}

func TestParsePluginMD_MissingName(t *testing.T) {
	content := []byte(`+++
description = "No name"
+++

# No Name Plugin
`)

	_, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParsePluginMD_MissingFrontmatter(t *testing.T) {
	content := []byte(`# No Frontmatter

Just instructions.
`)

	_, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParsePluginMD_ManualGate(t *testing.T) {
	// Plugin with no gate section should have nil Gate
	content := []byte(`+++
name = "manual-plugin"
description = "A manual plugin"
version = 1
+++

# Manual Plugin
`)

	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}

	if plugin.Gate != nil {
		t.Error("expected gate to be nil for manual plugin")
	}

	// Summary should report gate type as manual
	summary := plugin.Summary()
	if summary.GateType != GateManual {
		t.Errorf("expected gate type 'manual', got %q", summary.GateType)
	}
}

func TestParsePluginMD_MalformedTOML(t *testing.T) {
	content := []byte(`+++
name = "broken
description = no quotes
+++

# Broken Plugin
`)
	_, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err == nil {
		t.Error("expected error for malformed TOML")
	}
}

func TestParsePluginMD_UnclosedFrontmatter(t *testing.T) {
	content := []byte(`+++
name = "unclosed"
description = "No closing delimiter"
`)
	_, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err == nil {
		t.Error("expected error for unclosed frontmatter")
	}
}

func TestParsePluginMD_EmptyFrontmatter(t *testing.T) {
	content := []byte(`+++
+++

# Empty Plugin
`)
	_, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err == nil {
		t.Error("expected error for empty frontmatter (missing name)")
	}
}

func TestParsePluginMD_CooldownGateNoDuration(t *testing.T) {
	content := []byte(`+++
name = "no-duration"
description = "Cooldown without duration"
version = 1

[gate]
type = "cooldown"
+++

# No Duration
`)
	// Currently accepted (no validation on gate fields beyond parsing)
	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}
	if plugin.Gate == nil {
		t.Fatal("expected gate to be non-nil")
	}
	if plugin.Gate.Type != GateCooldown {
		t.Errorf("expected gate type 'cooldown', got %q", plugin.Gate.Type)
	}
	if plugin.Gate.Duration != "" {
		t.Errorf("expected empty duration, got %q", plugin.Gate.Duration)
	}
}

func TestParsePluginMD_UnknownGateType(t *testing.T) {
	content := []byte(`+++
name = "unknown-gate"
description = "Unknown gate type"
version = 1

[gate]
type = "never-heard-of-this"
+++

# Unknown Gate
`)
	// Currently accepted (no validation on gate type values)
	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}
	if plugin.Gate == nil {
		t.Fatal("expected gate to be non-nil")
	}
	if plugin.Gate.Type != "never-heard-of-this" {
		t.Errorf("expected unknown gate type preserved, got %q", plugin.Gate.Type)
	}
}

func TestParsePluginMD_CronGate(t *testing.T) {
	content := []byte(`+++
name = "cron-plugin"
description = "Runs on schedule"
version = 1

[gate]
type = "cron"
schedule = "*/5 * * * *"
+++

# Cron Plugin
`)
	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}
	if plugin.Gate == nil {
		t.Fatal("expected gate to be non-nil")
	}
	if plugin.Gate.Type != GateCron {
		t.Errorf("expected gate type 'cron', got %q", plugin.Gate.Type)
	}
	if plugin.Gate.Schedule != "*/5 * * * *" {
		t.Errorf("expected schedule '*/5 * * * *', got %q", plugin.Gate.Schedule)
	}
}

func TestParsePluginMD_InstructionsOnly(t *testing.T) {
	content := []byte(`+++
name = "minimal"
+++

These are the instructions.
Multiple lines.
`)
	plugin, err := parsePluginMD(content, "/test/path", LocationTown, "")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}
	if plugin.Instructions == "" {
		t.Error("expected non-empty instructions")
	}
	if plugin.Description != "" {
		t.Errorf("expected empty description, got %q", plugin.Description)
	}
	if plugin.Version != 0 {
		t.Errorf("expected version 0, got %d", plugin.Version)
	}
}

func TestScanner_DiscoverAll(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "plugin-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create town plugins directory
	townPluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(townPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create town plugins dir: %v", err)
	}

	// Create a town plugin
	townPlugin := filepath.Join(townPluginsDir, "town-plugin")
	if err := os.MkdirAll(townPlugin, 0755); err != nil {
		t.Fatalf("failed to create town plugin dir: %v", err)
	}
	townPluginContent := []byte(`+++
name = "town-plugin"
description = "Town level plugin"
version = 1
+++

# Town Plugin
`)
	if err := os.WriteFile(filepath.Join(townPlugin, "plugin.md"), townPluginContent, 0644); err != nil {
		t.Fatalf("failed to write town plugin: %v", err)
	}

	// Create rig plugins directory
	rigPluginsDir := filepath.Join(tmpDir, "testrig", "plugins")
	if err := os.MkdirAll(rigPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create rig plugins dir: %v", err)
	}

	// Create a rig plugin
	rigPlugin := filepath.Join(rigPluginsDir, "rig-plugin")
	if err := os.MkdirAll(rigPlugin, 0755); err != nil {
		t.Fatalf("failed to create rig plugin dir: %v", err)
	}
	rigPluginContent := []byte(`+++
name = "rig-plugin"
description = "Rig level plugin"
version = 1
+++

# Rig Plugin
`)
	if err := os.WriteFile(filepath.Join(rigPlugin, "plugin.md"), rigPluginContent, 0644); err != nil {
		t.Fatalf("failed to write rig plugin: %v", err)
	}

	// Create scanner
	scanner := NewScanner(tmpDir, []string{"testrig"})

	// Discover all plugins
	plugins, err := scanner.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}

	// Check that we have both plugins
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Name] = true
	}

	if !names["town-plugin"] {
		t.Error("expected to find 'town-plugin'")
	}
	if !names["rig-plugin"] {
		t.Error("expected to find 'rig-plugin'")
	}
}

func TestParsePluginMD_GitHubSheriff(t *testing.T) {
	// Verify the actual github-sheriff plugin.md parses correctly.
	// This catches frontmatter regressions in the shipped plugin.
	content, err := os.ReadFile(filepath.Join("..", "..", "plugins", "github-sheriff", "plugin.md"))
	if err != nil {
		t.Skipf("github-sheriff plugin not found (expected in plugins/): %v", err)
	}

	plugin, err := parsePluginMD(content, "/test/github-sheriff", LocationRig, "gastown")
	if err != nil {
		t.Fatalf("parsePluginMD failed: %v", err)
	}

	if plugin.Name != "github-sheriff" {
		t.Errorf("expected name 'github-sheriff', got %q", plugin.Name)
	}
	if plugin.Gate == nil {
		t.Fatal("expected gate to be non-nil")
	}
	if plugin.Gate.Type != GateCooldown {
		t.Errorf("expected gate type 'cooldown', got %q", plugin.Gate.Type)
	}
	if plugin.Gate.Duration != "5m" {
		t.Errorf("expected gate duration '5m', got %q", plugin.Gate.Duration)
	}
	if plugin.Tracking == nil {
		t.Fatal("expected tracking to be non-nil")
	}
	if !plugin.Tracking.Digest {
		t.Error("expected digest to be true")
	}
	if plugin.Execution == nil {
		t.Fatal("expected execution to be non-nil")
	}
	if plugin.Execution.Timeout != "2m" {
		t.Errorf("expected timeout '2m', got %q", plugin.Execution.Timeout)
	}
	if !plugin.Execution.NotifyOnFailure {
		t.Error("expected notify_on_failure to be true")
	}
	if plugin.Instructions == "" {
		t.Error("expected non-empty instructions")
	}
}

func TestScanner_RigOverridesTown(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "plugin-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create town plugins directory with a plugin
	townPluginsDir := filepath.Join(tmpDir, "plugins", "shared-plugin")
	if err := os.MkdirAll(townPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create town plugins dir: %v", err)
	}
	townPluginContent := []byte(`+++
name = "shared-plugin"
description = "Town version"
version = 1
+++

# Town Version
`)
	if err := os.WriteFile(filepath.Join(townPluginsDir, "plugin.md"), townPluginContent, 0644); err != nil {
		t.Fatalf("failed to write town plugin: %v", err)
	}

	// Create rig plugins directory with same-named plugin
	rigPluginsDir := filepath.Join(tmpDir, "testrig", "plugins", "shared-plugin")
	if err := os.MkdirAll(rigPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create rig plugins dir: %v", err)
	}
	rigPluginContent := []byte(`+++
name = "shared-plugin"
description = "Rig version"
version = 1
+++

# Rig Version
`)
	if err := os.WriteFile(filepath.Join(rigPluginsDir, "plugin.md"), rigPluginContent, 0644); err != nil {
		t.Fatalf("failed to write rig plugin: %v", err)
	}

	// Create scanner
	scanner := NewScanner(tmpDir, []string{"testrig"})

	// Discover all plugins
	plugins, err := scanner.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	// Should only have one plugin (rig overrides town)
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin (rig override), got %d", len(plugins))
	}

	if plugins[0].Description != "Rig version" {
		t.Errorf("expected rig version description, got %q", plugins[0].Description)
	}
	if plugins[0].Location != LocationRig {
		t.Errorf("expected location 'rig', got %q", plugins[0].Location)
	}
}
