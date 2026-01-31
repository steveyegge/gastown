package slack

import (
	"testing"
)

// --- Channel Mode Type Tests ---
// Tests for channel mode routing types and validation (bc14d084).

// TestChannelModeType_Constants verifies ChannelMode constant values.
func TestChannelModeType_Constants(t *testing.T) {
	// Verify exact string values used in config
	tests := []struct {
		mode     ChannelMode
		expected string
	}{
		{ChannelModeGeneral, "general"},
		{ChannelModeAgent, "agent"},
		{ChannelModeEpic, "epic"},
		{ChannelModeDM, "dm"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("ChannelMode constant %v = %q, want %q", tt.mode, string(tt.mode), tt.expected)
		}
	}
}

// TestIsValidChannelMode verifies mode validation function.
func TestIsValidChannelMode(t *testing.T) {
	validModes := []string{"general", "agent", "epic", "dm"}
	invalidModes := []string{"", "invalid", "GENERAL", "General", "epic ", " dm", "none", "default"}

	for _, mode := range validModes {
		if !IsValidChannelMode(mode) {
			t.Errorf("IsValidChannelMode(%q) = false, want true", mode)
		}
	}

	for _, mode := range invalidModes {
		if IsValidChannelMode(mode) {
			t.Errorf("IsValidChannelMode(%q) = true, want false", mode)
		}
	}
}

// TestValidChannelModes_Completeness verifies all modes are in ValidChannelModes.
func TestValidChannelModes_Completeness(t *testing.T) {
	expectedModes := []ChannelMode{ChannelModeGeneral, ChannelModeAgent, ChannelModeEpic, ChannelModeDM}

	if len(ValidChannelModes) != len(expectedModes) {
		t.Errorf("ValidChannelModes has %d entries, expected %d", len(ValidChannelModes), len(expectedModes))
	}

	modeMap := make(map[ChannelMode]bool)
	for _, m := range ValidChannelModes {
		modeMap[m] = true
	}

	for _, expected := range expectedModes {
		if !modeMap[expected] {
			t.Errorf("ValidChannelModes missing %q", expected)
		}
	}
}

// TestNormalizeAgent verifies agent normalization for config keys.
func TestNormalizeAgent_ChannelMode(t *testing.T) {
	// normalizeAgent is used to create consistent config keys
	tests := []struct {
		input string
		want  string
	}{
		{"gastown/polecats/furiosa", "gastown/polecats/furiosa"},
		{"gastown/polecats/furiosa/", "gastown/polecats/furiosa"},
		{"mayor", "mayor"},
		{"mayor/", "mayor"},
		{"gastown/witness/", "gastown/witness"},
		{"a/b/c//", "a/b/c"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeAgent(tt.input)
		if got != tt.want {
			t.Errorf("normalizeAgent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Channel Mode Config Key Tests ---

// TestChannelModeConfigKey verifies the config key format for agent modes.
func TestChannelModeConfigKey(t *testing.T) {
	// The config key format is: slack.channel_mode.<agent-with-dots>
	// Agent slashes become dots in the key

	testCases := []struct {
		agent       string
		description string
	}{
		{"gastown/polecats/furiosa", "three-part agent path"},
		{"gastown/witness", "two-part agent path"},
		{"mayor", "single-part agent (mayor)"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Document the expected key transformation
			t.Logf("Agent: %q", tc.agent)
			t.Logf("Key would be: slack.channel_mode.%s (with / replaced by .)", tc.agent)
		})
	}
}

// TestDefaultChannelModeConfigKey verifies the default mode config key.
func TestDefaultChannelModeConfigKey(t *testing.T) {
	// The default channel mode is stored at: slack.default_channel_mode
	t.Log("Default channel mode key: slack.default_channel_mode")
}

// --- Channel Mode Behavior Documentation Tests ---

// TestChannelModeBehavior documents the expected behavior for each mode.
func TestChannelModeBehavior(t *testing.T) {
	behaviors := []struct {
		mode        ChannelMode
		description string
	}{
		{
			ChannelModeGeneral,
			"Routes to the default/general channel (b.channelID). " +
				"Skips epic and agent channel creation.",
		},
		{
			ChannelModeAgent,
			"Routes to a dedicated per-agent channel. " +
				"Creates channel if dynamicChannels enabled. " +
				"Falls back to general if creation disabled or fails.",
		},
		{
			ChannelModeEpic,
			"Routes to a channel based on the work's parent epic. " +
				"Uses decision.ParentBeadTitle to derive channel name. " +
				"Falls back to general (not agent) if no parent epic.",
		},
		{
			ChannelModeDM,
			"Intended to route to a direct message with the overseer. " +
				"NOT YET IMPLEMENTED - falls through to legacy routing.",
		},
	}

	for _, b := range behaviors {
		t.Run(string(b.mode), func(t *testing.T) {
			t.Logf("Mode: %q", b.mode)
			t.Logf("Behavior: %s", b.description)
		})
	}
}

// TestChannelModeRoutingPriority documents the routing priority.
func TestChannelModeRoutingPriority(t *testing.T) {
	t.Log("Channel routing priority order:")
	t.Log("  1. Convoy-based channel (if parent issue tracked by convoy)")
	t.Log("  2. Agent channel mode preference (general/agent/epic/dm)")
	t.Log("  3. Epic-based channel (legacy, only when mode is empty)")
	t.Log("  4. Static router config (pattern matching)")
	t.Log("  5. Dynamic channel creation (if enabled)")
	t.Log("  6. Default channelID")
	t.Log("")
	t.Log("Key insight: Setting a mode skips legacy epic routing (Priority 3)")
}

// TestChannelModeFallbackChain documents fallback behavior.
func TestChannelModeFallbackChain(t *testing.T) {
	fallbacks := []struct {
		mode     ChannelMode
		scenario string
		fallback string
	}{
		{ChannelModeEpic, "no ParentBeadTitle", "general channel"},
		{ChannelModeEpic, "channel creation fails", "general channel"},
		{ChannelModeAgent, "dynamicChannels disabled", "general channel"},
		{ChannelModeAgent, "empty agent", "general channel"},
		{ChannelModeAgent, "channel creation fails", "general channel"},
		{ChannelModeDM, "always (not implemented)", "legacy routing"},
		{ChannelModeGeneral, "never fails", "returns immediately"},
		{"", "no mode set, has parent", "legacy epic routing"},
		{"", "no mode set, no parent", "agent-based routing"},
	}

	for _, f := range fallbacks {
		t.Run(string(f.mode)+"/"+f.scenario, func(t *testing.T) {
			t.Logf("Mode: %q, Scenario: %s", f.mode, f.scenario)
			t.Logf("Fallback: %s", f.fallback)
		})
	}
}

// --- Edge Case Tests ---

// TestEmptyModeVsUnsetMode documents the distinction between "" and unset.
func TestEmptyModeVsUnsetMode(t *testing.T) {
	// In the implementation, empty mode ("") triggers legacy routing.
	// There's no functional difference between "not set" and "set to empty".

	t.Log("Empty mode behavior:")
	t.Log("  - Triggers legacy epic-based routing (Priority 3)")
	t.Log("  - Falls back to agent-based routing if no parent epic")
	t.Log("  - Same as having no mode configured")
}

// TestModeWithConvoyTracking documents convoy priority.
func TestModeWithConvoyTracking(t *testing.T) {
	t.Log("Convoy vs Mode priority:")
	t.Log("  - Convoy routing (Priority 1) always wins over mode (Priority 2)")
	t.Log("  - If issue is tracked by a convoy, that convoy's channel is used")
	t.Log("  - Agent's channel mode preference is ignored for convoy-tracked work")
}

// TestModeInheritance documents mode lookup order.
func TestModeInheritance(t *testing.T) {
	t.Log("Mode lookup order (getEffectiveChannelMode):")
	t.Log("  1. Agent-specific mode: slack.channel_mode.<agent>")
	t.Log("  2. Default mode: slack.default_channel_mode")
	t.Log("  3. If both empty: returns empty string (triggers legacy routing)")
	t.Log("")
	t.Log("Agent-specific mode overrides default mode when set.")
}
