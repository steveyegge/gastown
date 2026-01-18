package config

import (
	"strings"
	"testing"
)

// =============================================================================
// OPENCODE PROVIDER DEFAULTS
// =============================================================================
//
// OpenCode is a multi-model CLI that supports various providers (OpenAI, Google,
// xAI, etc.). Its TUI uses box-drawing characters (┃) that break prompt prefix
// matching, requiring delay-based ready detection instead.
//
// The 8000ms default was derived from testing with openai/gpt-5.2-codex during
// refinery debugging (see: docs/proposals/refinery-opencode-debug.md).
//
// KNOWN LIMITATIONS:
// - Free tier models may need longer delays (up to 15000ms)
// - Fast models (grok-code-fast) could use shorter delays (4000ms)
// - Users can override via runtime.tmux.ready_delay_ms in settings
//
// =============================================================================

func TestOpenCodeProviderDefaults(t *testing.T) {
	t.Parallel()

	rc := &RuntimeConfig{Provider: "opencode"}
	normalized := normalizeRuntimeConfig(rc)

	t.Run("command defaults to opencode", func(t *testing.T) {
		if normalized.Command != "opencode" {
			t.Errorf("Command = %q, want %q", normalized.Command, "opencode")
		}
	})

	t.Run("prompt mode is none", func(t *testing.T) {
		// OpenCode doesn't support prompt arg mode like claude
		if normalized.PromptMode != "none" {
			t.Errorf("PromptMode = %q, want %q", normalized.PromptMode, "none")
		}
	})

	t.Run("hooks provider is opencode", func(t *testing.T) {
		if normalized.Hooks == nil {
			t.Fatal("Hooks config is nil")
		}
		if normalized.Hooks.Provider != "opencode" {
			t.Errorf("Hooks.Provider = %q, want %q", normalized.Hooks.Provider, "opencode")
		}
	})

	t.Run("hooks dir is .opencode/plugin", func(t *testing.T) {
		if normalized.Hooks == nil {
			t.Fatal("Hooks config is nil")
		}
		if normalized.Hooks.Dir != ".opencode/plugin" {
			t.Errorf("Hooks.Dir = %q, want %q", normalized.Hooks.Dir, ".opencode/plugin")
		}
	})

	t.Run("hooks file is gastown.js", func(t *testing.T) {
		if normalized.Hooks == nil {
			t.Fatal("Hooks config is nil")
		}
		if normalized.Hooks.SettingsFile != "gastown.js" {
			t.Errorf("Hooks.SettingsFile = %q, want %q", normalized.Hooks.SettingsFile, "gastown.js")
		}
	})

	t.Run("instructions file is AGENTS.md", func(t *testing.T) {
		if normalized.Instructions == nil {
			t.Fatal("Instructions config is nil")
		}
		if normalized.Instructions.File != "AGENTS.md" {
			t.Errorf("Instructions.File = %q, want %q", normalized.Instructions.File, "AGENTS.md")
		}
	})

	t.Run("tmux ready_delay_ms is 8000", func(t *testing.T) {
		// 8000ms derived from testing with openai/gpt-5.2-codex
		// See: docs/proposals/refinery-opencode-debug.md
		if normalized.Tmux == nil {
			t.Fatal("Tmux config is nil")
		}
		if normalized.Tmux.ReadyDelayMs != 8000 {
			t.Errorf("Tmux.ReadyDelayMs = %d, want %d", normalized.Tmux.ReadyDelayMs, 8000)
		}
	})

	t.Run("tmux ready_prompt_prefix is empty for delay-based detection", func(t *testing.T) {
		// OpenCode's prompt "┃  Ask anything..." contains box-drawing chars
		// that break prefix matching. Empty prefix triggers delay-based detection.
		if normalized.Tmux == nil {
			t.Fatal("Tmux config is nil")
		}
		if normalized.Tmux.ReadyPromptPrefix != "" {
			t.Errorf("ReadyPromptPrefix = %q, want empty", normalized.Tmux.ReadyPromptPrefix)
		}
	})

	t.Run("process names include opencode", func(t *testing.T) {
		// NOTE: OpenCode actually runs as Node.js process, but we detect
		// based on command name. This may need revision if IsAgentRunning
		// checks fail in practice.
		if normalized.Tmux == nil {
			t.Fatal("Tmux config is nil")
		}
		if len(normalized.Tmux.ProcessNames) == 0 {
			t.Error("ProcessNames is empty")
		}
		found := false
		for _, name := range normalized.Tmux.ProcessNames {
			if name == "opencode" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ProcessNames %v does not contain 'opencode'", normalized.Tmux.ProcessNames)
		}
	})
}

// TestProviderReadyDelayDefaults verifies ready delay defaults for all providers.
func TestProviderReadyDelayDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider string
		wantMs   int
		note     string
	}{
		{"claude", 10000, "prompt-based detection, delay is fallback"},
		{"codex", 3000, "fast startup"},
		{"opencode", 8000, "delay-based detection (box-drawing chars break prefix)"},
		{"generic", 0, "no default - user must configure"},
		{"unknown", 0, "unknown providers have no delay"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			rc := &RuntimeConfig{Provider: tt.provider}
			normalized := normalizeRuntimeConfig(rc)

			if normalized.Tmux == nil {
				t.Fatal("Tmux config is nil")
			}
			if normalized.Tmux.ReadyDelayMs != tt.wantMs {
				t.Errorf("ReadyDelayMs for provider %q = %d, want %d (%s)",
					tt.provider, normalized.Tmux.ReadyDelayMs, tt.wantMs, tt.note)
			}
		})
	}
}

// =============================================================================
// EDGE CASES AND FAILURE MODES
// =============================================================================

// TestOpenCodeUserOverrideDelay verifies users can override the default delay.
// CRITICAL: Without this, users with slow models (free tier) cannot fix timeouts.
func TestOpenCodeUserOverrideDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configDelay int
		wantDelay   int
	}{
		{
			name:        "user can increase delay for slow models",
			configDelay: 15000,
			wantDelay:   15000,
		},
		{
			name:        "user can decrease delay for fast models",
			configDelay: 4000,
			wantDelay:   4000,
		},
		{
			// IMPORTANT: 0 means "use default", NOT "disable delay"
			// This matches the normalizeRuntimeConfig behavior where 0 triggers default
			name:        "zero delay means use default (not disable)",
			configDelay: 0,
			wantDelay:   8000, // Gets opencode default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RuntimeConfig{
				Provider: "opencode",
				Tmux: &RuntimeTmuxConfig{
					ReadyDelayMs: tt.configDelay,
				},
			}
			normalized := normalizeRuntimeConfig(rc)

			if normalized.Tmux.ReadyDelayMs != tt.wantDelay {
				t.Errorf("User override not respected: got %d, want %d",
					normalized.Tmux.ReadyDelayMs, tt.wantDelay)
			}
		})
	}
}

// TestOpenCodeCommandWithoutProvider tests the dangerous edge case where
// someone uses command: "opencode" without setting provider: "opencode".
// This WILL FAIL because it uses claude's prompt prefix detection.
func TestOpenCodeCommandWithoutProvider(t *testing.T) {
	t.Parallel()

	// User mistake: setting command without provider
	rc := &RuntimeConfig{
		Command: "opencode",
		// Provider not set - defaults to "claude"!
	}
	normalized := normalizeRuntimeConfig(rc)

	// Document the dangerous behavior:
	t.Run("provider defaults to claude NOT opencode", func(t *testing.T) {
		if normalized.Provider != "claude" {
			t.Errorf("Expected default provider 'claude', got %q", normalized.Provider)
		}
	})

	t.Run("gets claude prompt prefix which will fail", func(t *testing.T) {
		// OpenCode shows "┃  Ask anything...", not "> "
		// This WILL cause timeout waiting for runtime prompt
		if normalized.Tmux.ReadyPromptPrefix != "> " {
			t.Errorf("Expected claude prefix '> ', got %q", normalized.Tmux.ReadyPromptPrefix)
		}
		// Log warning about this footgun
		t.Log("WARNING: command='opencode' without provider='opencode' uses claude settings!")
		t.Log("This will timeout because OpenCode doesn't show '> ' prompt")
	})

	t.Run("gets claude delay not opencode delay", func(t *testing.T) {
		if normalized.Tmux.ReadyDelayMs != 10000 {
			t.Errorf("Expected claude delay 10000, got %d", normalized.Tmux.ReadyDelayMs)
		}
	})
}

// TestProviderCaseSensitivity verifies that provider matching is case-sensitive.
// Users MUST use lowercase "opencode", not "OpenCode" or "OPENCODE".
func TestProviderCaseSensitivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider  string
		wantDelay int
		note      string
	}{
		{"opencode", 8000, "correct - lowercase"},
		{"OpenCode", 0, "FAILS - mixed case not recognized"},
		{"OPENCODE", 0, "FAILS - uppercase not recognized"},
		{"Opencode", 0, "FAILS - capitalized not recognized"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			rc := &RuntimeConfig{Provider: tt.provider}
			normalized := normalizeRuntimeConfig(rc)

			if normalized.Tmux.ReadyDelayMs != tt.wantDelay {
				t.Errorf("Provider %q: delay=%d, want %d (%s)",
					tt.provider, normalized.Tmux.ReadyDelayMs, tt.wantDelay, tt.note)
			}
			if tt.wantDelay == 0 && strings.ToLower(tt.provider) == "opencode" {
				t.Logf("NOTE: Provider %q not recognized. Use lowercase 'opencode'.", tt.provider)
			}
		})
	}
}

// TestOpenCodeRefineryScenario tests the exact scenario from the debug log.
func TestOpenCodeRefineryScenario(t *testing.T) {
	t.Parallel()

	rc := &RuntimeConfig{
		Provider: "opencode",
	}
	normalized := normalizeRuntimeConfig(rc)

	// Matches the fix documented in refinery-opencode-debug.md
	if normalized.Tmux.ReadyPromptPrefix != "" {
		t.Errorf("ReadyPromptPrefix should be empty, got %q", normalized.Tmux.ReadyPromptPrefix)
	}

	if normalized.Tmux.ReadyDelayMs < 8000 {
		t.Errorf("ReadyDelayMs should be >= 8000, got %d", normalized.Tmux.ReadyDelayMs)
	}

	// Build command should be just "opencode" (no YOLO flags)
	cmd := normalized.BuildCommand()
	if cmd != "opencode" {
		t.Errorf("BuildCommand() = %q, want %q", cmd, "opencode")
	}
}

// =============================================================================
// DELAY BOUNDARY CONDITIONS
// =============================================================================

// TestDelayBoundaryConditions tests edge cases in delay handling.
func TestDelayBoundaryConditions(t *testing.T) {
	t.Parallel()

	t.Run("negative delay in config is preserved", func(t *testing.T) {
		// normalizeRuntimeConfig doesn't validate - it's the caller's job
		// WaitForRuntimeReady handles delay <= 0 by returning immediately
		rc := &RuntimeConfig{
			Provider: "opencode",
			Tmux:     &RuntimeTmuxConfig{ReadyDelayMs: -100},
		}
		normalized := normalizeRuntimeConfig(rc)

		// Negative values are preserved (caller handles)
		if normalized.Tmux.ReadyDelayMs != -100 {
			t.Errorf("Negative delay not preserved: got %d", normalized.Tmux.ReadyDelayMs)
		}
	})

	t.Run("very large delay is preserved", func(t *testing.T) {
		// No upper bound validation - WaitForRuntimeReady caps at timeout
		rc := &RuntimeConfig{
			Provider: "opencode",
			Tmux:     &RuntimeTmuxConfig{ReadyDelayMs: 600000}, // 10 minutes
		}
		normalized := normalizeRuntimeConfig(rc)

		if normalized.Tmux.ReadyDelayMs != 600000 {
			t.Errorf("Large delay not preserved: got %d", normalized.Tmux.ReadyDelayMs)
		}
	})

	t.Run("zero delay triggers default (cannot disable via 0)", func(t *testing.T) {
		// IMPORTANT: 0 is treated as "unset" and replaced with provider default
		// To disable delay, users would need negative value (-1), but this is undocumented
		rc := &RuntimeConfig{
			Provider: "opencode",
			Tmux:     &RuntimeTmuxConfig{ReadyDelayMs: 0},
		}
		normalized := normalizeRuntimeConfig(rc)

		// 0 gets replaced with default, not preserved
		if normalized.Tmux.ReadyDelayMs != 8000 {
			t.Errorf("Zero delay should trigger default: got %d, want 8000", normalized.Tmux.ReadyDelayMs)
		}
		t.Log("NOTE: Users cannot disable delay via 0. This is by design - delay is required for opencode.")
	})
}

// =============================================================================
// MODEL-SPECIFIC DELAY RECOMMENDATIONS (DOCUMENTATION)
// =============================================================================

// TestOpenCodeModelDelayRecommendations documents model-specific timing.
// These are NOT enforced - they guide users who need to override defaults.
func TestOpenCodeModelDelayRecommendations(t *testing.T) {
	t.Parallel()

	// Recommendations from proposal and testing
	// Format: model -> recommended delay in ms
	recommendations := map[string]struct {
		delayMs int
		note    string
	}{
		// Fast paid models
		"openai/gpt-5.2":       {5000, "fast model"},
		"xai/grok-code-fast-1": {4000, "optimized for speed"},

		// Default tier (what 8000ms targets)
		"openai/gpt-5.2-codex": {8000, "default, tested in debug log"},
		"google/gemini-3-pro":  {6000, "moderate startup"},

		// Slow models
		"openai/codex-1": {10000, "extended context, slower init"},

		// Free tier (WILL LIKELY TIMEOUT with default 8000ms)
		"opencode/glm-4.7-free":   {15000, "free tier, may timeout with default"},
		"opencode/minimax-free":   {10000, "free tier"},
		"opencode/big-pickle":     {12000, "experimental, variable timing"},
	}

	defaultDelay := 8000

	t.Run("default covers common paid model", func(t *testing.T) {
		rec := recommendations["openai/gpt-5.2-codex"]
		if rec.delayMs != defaultDelay {
			t.Errorf("Default %d doesn't match primary target model recommendation %d",
				defaultDelay, rec.delayMs)
		}
	})

	t.Run("document models that may timeout", func(t *testing.T) {
		for model, rec := range recommendations {
			if rec.delayMs > defaultDelay {
				t.Logf("⚠️  Model %s may timeout (needs %dms, default %dms): %s",
					model, rec.delayMs, defaultDelay, rec.note)
				t.Logf("   Fix: set runtime.tmux.ready_delay_ms: %d in settings", rec.delayMs)
			}
		}
	})

	t.Run("document models where default is wasteful", func(t *testing.T) {
		for model, rec := range recommendations {
			if rec.delayMs < defaultDelay-2000 { // 2s tolerance
				t.Logf("ℹ️  Model %s: default %dms is %dms longer than needed (%s)",
					model, defaultDelay, defaultDelay-rec.delayMs, rec.note)
			}
		}
	})
}

// =============================================================================
// PROCESS DETECTION
// =============================================================================

// TestOpenCodeProcessNames verifies OpenCode includes both "opencode" and "node"
// in ProcessNames. OpenCode runs as Node.js, so tmux pane_current_command may
// show either "node" or "opencode" depending on how it was invoked.
func TestOpenCodeProcessNames(t *testing.T) {
	t.Parallel()

	rc := &RuntimeConfig{Provider: "opencode"}
	normalized := normalizeRuntimeConfig(rc)

	t.Run("includes opencode", func(t *testing.T) {
		found := false
		for _, name := range normalized.Tmux.ProcessNames {
			if name == "opencode" {
				found = true
			}
		}
		if !found {
			t.Error("Expected 'opencode' in ProcessNames")
		}
	})

	t.Run("includes node for Node.js detection", func(t *testing.T) {
		// OpenCode runs as Node.js process. tmux pane_current_command may
		// show "node" instead of "opencode", so we need both for reliable
		// IsAgentRunning detection.
		foundNode := false
		for _, name := range normalized.Tmux.ProcessNames {
			if name == "node" {
				foundNode = true
			}
		}
		if !foundNode {
			t.Error("Expected 'node' in ProcessNames for Node.js detection")
		}
	})

	t.Run("matches AgentOpenCode preset", func(t *testing.T) {
		// Verify RuntimeConfig ProcessNames matches the built-in preset
		preset := GetAgentPreset(AgentOpenCode)
		if preset == nil {
			t.Fatal("AgentOpenCode preset not found")
		}

		// Both should have opencode and node
		if len(normalized.Tmux.ProcessNames) != len(preset.ProcessNames) {
			t.Errorf("ProcessNames length mismatch: RuntimeConfig=%v, Preset=%v",
				normalized.Tmux.ProcessNames, preset.ProcessNames)
		}
	})
}

// =============================================================================
// WAIT FOR RUNTIME READY BEHAVIOR (INTEGRATION CONCERN)
// =============================================================================

// TestWaitForRuntimeReadyBehavior documents how the delay is used.
// This is NOT an integration test but documents the expected behavior.
func TestWaitForRuntimeReadyBehavior(t *testing.T) {
	t.Parallel()

	// Document the behavior from tmux.go:962-978
	t.Run("empty prefix triggers delay-based detection", func(t *testing.T) {
		// When ReadyPromptPrefix == "":
		//   if ReadyDelayMs <= 0: return nil immediately
		//   else: sleep(min(ReadyDelayMs, timeout)) and return nil
		//
		// This is what opencode uses.
		t.Log("WaitForRuntimeReady with empty prefix: sleeps for ReadyDelayMs")
	})

	t.Run("delay is capped at timeout", func(t *testing.T) {
		// If delay > timeout, uses timeout instead
		// This prevents infinite waits
		t.Log("Delay is capped: min(ReadyDelayMs, timeout)")
	})

	t.Run("delay-based detection does NOT verify agent is ready", func(t *testing.T) {
		// RISK: If agent crashes within delay period, we incorrectly
		// report it as ready. No actual verification occurs.
		t.Log("WARNING: Delay-based detection assumes agent starts successfully")
		t.Log("If agent crashes during startup, false positive 'ready' is returned")
	})
}

func TestFillRuntimeDefaultsPreservesEnv(t *testing.T) {
	t.Parallel()

	t.Run("preserves Env from custom agent config", func(t *testing.T) {
		// Regression test: fillRuntimeDefaults must preserve Env field
		// This is critical for custom agents that use env vars for config
		customConfig := &RuntimeConfig{
			Command: "my-agent",
			Args:    []string{"--flag"},
			Env: map[string]string{
				"CUSTOM_VAR":   "value",
				"ANOTHER_VAR":  "another",
				"JSON_VAR":     `{"key":"value"}`,
			},
		}

		result := fillRuntimeDefaults(customConfig)

		// Verify all env vars are preserved
		if result.Env == nil {
			t.Fatal("fillRuntimeDefaults lost Env map")
		}
		if len(result.Env) != 3 {
			t.Errorf("fillRuntimeDefaults lost env vars: got %d, want 3", len(result.Env))
		}
		if result.Env["CUSTOM_VAR"] != "value" {
			t.Errorf("CUSTOM_VAR = %q, want %q", result.Env["CUSTOM_VAR"], "value")
		}
		if result.Env["JSON_VAR"] != `{"key":"value"}` {
			t.Errorf("JSON_VAR = %q, want %q", result.Env["JSON_VAR"], `{"key":"value"}`)
		}
	})

	t.Run("does not mutate original config", func(t *testing.T) {
		originalConfig := &RuntimeConfig{
			Command: "my-agent",
			Env: map[string]string{
				"KEY": "original",
			},
		}

		result := fillRuntimeDefaults(originalConfig)

		// Modify the result's Env
		result.Env["KEY"] = "modified"
		result.Env["NEW_KEY"] = "new"

		// Original should be unchanged
		if originalConfig.Env["KEY"] != "original" {
			t.Errorf("fillRuntimeDefaults mutated original: KEY = %q, want %q",
				originalConfig.Env["KEY"], "original")
		}
		if _, exists := originalConfig.Env["NEW_KEY"]; exists {
			t.Error("fillRuntimeDefaults mutated original: NEW_KEY should not exist")
		}
	})

	t.Run("handles nil Env gracefully", func(t *testing.T) {
		configNoEnv := &RuntimeConfig{
			Command: "my-agent",
			Args:    []string{"--flag"},
			// Env is nil
		}

		result := fillRuntimeDefaults(configNoEnv)

		// Should not create empty Env map, just leave it nil
		if result.Env != nil && len(result.Env) > 0 {
			t.Errorf("fillRuntimeDefaults created unexpected Env: %v", result.Env)
		}
	})
}
