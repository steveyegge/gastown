package config

import (
	"testing"
)

// TestOpencodeAgentPreset tests the OpenCode agent preset configuration
func TestOpencodeAgentPreset(t *testing.T) {
	t.Parallel()

	preset := GetAgentPreset(AgentOpencode)
	if preset == nil {
		t.Fatal("GetAgentPreset(AgentOpencode) returned nil")
	}

	// Test basic configuration
	if preset.Name != AgentOpencode {
		t.Errorf("Name = %s, want %s", preset.Name, AgentOpencode)
	}

	if preset.Command != "opencode" {
		t.Errorf("Command = %s, want opencode", preset.Command)
	}

	// Test process detection - OpenCode can appear as "node" or "opencode" depending on startup
	if len(preset.ProcessNames) != 2 {
		t.Errorf("ProcessNames length = %d, want 2", len(preset.ProcessNames))
	}
	// Should include both "node" (when started via bash -c) and "opencode"
	foundNode, foundOpencode := false, false
	for _, name := range preset.ProcessNames {
		if name == "node" {
			foundNode = true
		}
		if name == "opencode" {
			foundOpencode = true
		}
	}
	if !foundNode {
		t.Error("ProcessNames should include 'node'")
	}
	if !foundOpencode {
		t.Error("ProcessNames should include 'opencode'")
	}

	// Test session features
	if preset.ResumeFlag != "--session" {
		t.Errorf("ResumeFlag = %s, want --session", preset.ResumeFlag)
	}

	if preset.ResumeStyle != "flag" {
		t.Errorf("ResumeStyle = %s, want flag", preset.ResumeStyle)
	}

	// Test hook support
	if !preset.SupportsHooks {
		t.Error("SupportsHooks = false, want true")
	}

	// Test fork session support (via HTTP API)
	if !preset.SupportsForkSession {
		t.Error("SupportsForkSession = false, want true (via HTTP API)")
	}

	// Test non-interactive config
	if preset.NonInteractive == nil {
		t.Fatal("NonInteractive is nil")
	}

	if preset.NonInteractive.Subcommand != "run" {
		t.Errorf("NonInteractive.Subcommand = %s, want run", preset.NonInteractive.Subcommand)
	}
}

// TestOpencodeRuntimeConfig tests OpenCode runtime configuration resolution
func TestOpencodeRuntimeConfig(t *testing.T) {
	t.Parallel()

	preset := GetAgentPreset(AgentOpencode)
	if preset == nil {
		t.Fatal("GetAgentPreset(AgentOpencode) returned nil")
	}

	cfg := RuntimeConfigFromPreset(AgentOpencode)

	// Test basic runtime configuration from preset
	if cfg.Command != "opencode" {
		t.Errorf("RuntimeConfig.Command = %s, want opencode", cfg.Command)
	}

	// RuntimeConfigFromPreset returns minimal config
	// Full config with hooks, prompt mode, etc. is populated by FillRuntimeDefaults
	// based on provider type when loading actual role config
}

// TestOpencodeAgentDetection tests process detection for OpenCode
func TestOpencodeAgentDetection(t *testing.T) {
	t.Parallel()

	preset := GetAgentPreset(AgentOpencode)
	if preset == nil {
		t.Fatal("GetAgentPreset(AgentOpencode) returned nil")
	}

	// Test that preset has process names configured
	if len(preset.ProcessNames) == 0 {
		t.Error("Preset should have ProcessNames configured")
	}

	// OpenCode runs on Node.js
	found := false
	for _, name := range preset.ProcessNames {
		if name == "node" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ProcessNames should include 'node' for OpenCode detection")
	}
}

// TestOpencodeComparedToClaude tests OpenCode has same capabilities as Claude
func TestOpencodeComparedToClaude(t *testing.T) {
	t.Parallel()

	opencode := GetAgentPreset(AgentOpencode)
	claude := GetAgentPreset(AgentClaude)

	if opencode == nil || claude == nil {
		t.Fatal("Failed to get agent presets")
	}

	// Both should support hooks
	if opencode.SupportsHooks != claude.SupportsHooks {
		t.Errorf("OpenCode SupportsHooks = %v, Claude = %v, want same", 
			opencode.SupportsHooks, claude.SupportsHooks)
	}

	// Both should support fork session
	if opencode.SupportsForkSession != claude.SupportsForkSession {
		t.Errorf("OpenCode SupportsForkSession = %v, Claude = %v, want same", 
			opencode.SupportsForkSession, claude.SupportsForkSession)
	}

	// Both should have session resume
	if (opencode.ResumeFlag != "") != (claude.ResumeFlag != "") {
		t.Error("OpenCode and Claude should both support session resume")
	}

	// Check non-interactive mode support
	hasOpencodeNonInteractive := opencode.NonInteractive != nil
	if !hasOpencodeNonInteractive {
		t.Error("OpenCode should have non-interactive mode configured")
	}
	// Note: Claude has NonInteractive: nil because it's natively non-interactive
	// OpenCode has NonInteractive configured because it requires "opencode run" subcommand
}
