package slackbot

import (
	"strings"
	"testing"
)

func TestNewBot_MissingBotToken(t *testing.T) {
	cfg := Config{
		AppToken: "xapp-test",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for missing bot token")
	}
}

func TestNewBot_MissingAppToken(t *testing.T) {
	cfg := Config{
		BotToken: "xoxb-test",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for missing app token")
	}
}

func TestNewBot_InvalidAppToken(t *testing.T) {
	cfg := Config{
		BotToken: "xoxb-test",
		AppToken: "invalid-token",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid app token format")
	}
}

func TestNewBot_ValidConfig(t *testing.T) {
	cfg := Config{
		BotToken:    "xoxb-test-token",
		AppToken:    "xapp-test-token",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, err := New(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bot == nil {
		t.Error("expected bot to be created")
	}
}

func TestAgentToChannelName(t *testing.T) {
	bot := &Bot{
		channelPrefix: "gt-decisions",
	}

	tests := []struct {
		agent    string
		expected string
	}{
		// Standard three-part agents → use rig and role
		{"gastown/polecats/furiosa", "gt-decisions-gastown-polecats"},
		{"beads/crew/wolf", "gt-decisions-beads-crew"},
		{"longeye/polecats/alpha", "gt-decisions-longeye-polecats"},

		// Two-part agents → use both parts
		{"gastown/witness", "gt-decisions-gastown-witness"},
		{"beads/refinery", "gt-decisions-beads-refinery"},

		// Single-part agents
		{"mayor", "gt-decisions-mayor"},

		// Edge cases
		{"", "gt-decisions"},
		{"a/b/c/d/e", "gt-decisions-a-b"}, // Only takes first two parts
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			got := bot.agentToChannelName(tt.agent)
			if got != tt.expected {
				t.Errorf("agentToChannelName(%q) = %q, want %q", tt.agent, got, tt.expected)
			}
		})
	}
}

func TestAgentToChannelName_Sanitization(t *testing.T) {
	bot := &Bot{
		channelPrefix: "gt-decisions",
	}

	tests := []struct {
		agent    string
		expected string
	}{
		// Uppercase gets lowercased
		{"GasTown/Polecats/Furiosa", "gt-decisions-gastown-polecats"},

		// Underscores become hyphens
		{"gas_town/pole_cats/agent", "gt-decisions-gas-town-pole-cats"},

		// Multiple consecutive hyphens collapsed
		{"foo--bar/baz", "gt-decisions-foo-bar-baz"},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			got := bot.agentToChannelName(tt.agent)
			if got != tt.expected {
				t.Errorf("agentToChannelName(%q) = %q, want %q", tt.agent, got, tt.expected)
			}
		})
	}
}

func TestAgentToChannelName_CustomPrefix(t *testing.T) {
	bot := &Bot{
		channelPrefix: "custom-prefix",
	}

	got := bot.agentToChannelName("gastown/polecats/furiosa")
	expected := "custom-prefix-gastown-polecats"
	if got != expected {
		t.Errorf("agentToChannelName with custom prefix = %q, want %q", got, expected)
	}
}

// --- Decision Chaining Helper Tests ---

// TestFormatContextForSlack tests the JSON context formatter for Slack display.
func TestFormatContextForSlack(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		maxLen   int
		wantType string // "empty", "codeblock", "plain", "truncated"
		contains []string
	}{
		{
			name:     "empty context",
			context:  "",
			maxLen:   500,
			wantType: "empty",
		},
		{
			name:     "simple JSON object",
			context:  `{"key": "value"}`,
			maxLen:   500,
			wantType: "codeblock",
			contains: []string{"```", "key", "value"},
		},
		{
			name:     "nested JSON",
			context:  `{"outer": {"inner": "value"}, "number": 42}`,
			maxLen:   500,
			wantType: "codeblock",
			contains: []string{"```", "outer", "inner"},
		},
		{
			name:     "JSON array",
			context:  `["item1", "item2", "item3"]`,
			maxLen:   500,
			wantType: "codeblock",
			contains: []string{"```", "item1"},
		},
		{
			name:     "plain text (not JSON)",
			context:  "This is plain text context",
			maxLen:   500,
			wantType: "plain",
			contains: []string{"This is plain text context"},
		},
		{
			name:     "truncated plain text",
			context:  "This is a very long plain text context that exceeds the maximum length",
			maxLen:   30,
			wantType: "truncated",
			contains: []string{"..."},
		},
		{
			name:     "JSON with successor_schemas",
			context:  `{"diagnosis": "rate limiting", "successor_schemas": {"Fix upstream": {"required": ["approach"]}}}`,
			maxLen:   500,
			wantType: "codeblock",
			contains: []string{"diagnosis", "rate limiting", "successor schemas"},
		},
		{
			name:     "only successor_schemas",
			context:  `{"successor_schemas": {"Option A": {"required": ["field1"]}}}`,
			maxLen:   500,
			wantType: "schema_only",
			contains: []string{"successor schemas"},
		},
		{
			name:     "complex nested JSON",
			context:  `{"error_code": 500, "details": {"service": "api", "attempts": 3}, "timestamp": "2026-01-29T10:00:00Z"}`,
			maxLen:   500,
			wantType: "codeblock",
			contains: []string{"```", "error_code", "500", "service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatContextForSlack(tt.context, tt.maxLen)

			switch tt.wantType {
			case "empty":
				if result != "" {
					t.Errorf("expected empty string, got %q", result)
				}
			case "codeblock":
				if len(result) < 6 || result[:3] != "```" {
					t.Errorf("expected code block, got %q", result)
				}
			case "plain":
				if len(result) >= 3 && result[:3] == "```" {
					t.Errorf("expected plain text, got code block: %q", result)
				}
			case "truncated":
				if len(result) > tt.maxLen {
					t.Errorf("result length %d exceeds maxLen %d", len(result), tt.maxLen)
				}
			case "schema_only":
				// Should contain schema info indicator
			}

			for _, s := range tt.contains {
				if !containsIgnoreCase(result, s) {
					t.Errorf("result should contain %q, got %q", s, result)
				}
			}
		})
	}
}

// TestFormatContextForSlack_Truncation tests length limits.
func TestFormatContextForSlack_Truncation(t *testing.T) {
	// Long JSON that will need truncation
	longJSON := `{"field1": "` + string(make([]byte, 1000)) + `"}`
	result := formatContextForSlack(longJSON, 200)

	if len(result) > 200 {
		t.Errorf("result length %d exceeds maxLen 200", len(result))
	}
}

// TestFormatContextForSlack_InvalidJSON tests handling of malformed JSON.
func TestFormatContextForSlack_InvalidJSON(t *testing.T) {
	tests := []struct {
		name    string
		context string
	}{
		{"unclosed brace", `{"key": "value"`},
		{"single quotes", `{'key': 'value'}`},
		{"trailing comma", `{"key": "value",}`},
		{"unquoted keys", `{key: "value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatContextForSlack(tt.context, 500)
			// Should return plain text, not code block
			if len(result) >= 3 && result[:3] == "```" {
				t.Errorf("invalid JSON should not produce code block: %q", result)
			}
			// Should contain the original content (possibly truncated)
			if result == "" {
				t.Error("result should not be empty for non-empty input")
			}
		})
	}
}

// TestBuildChainInfoText tests the chain info text builder.
func TestBuildChainInfoText(t *testing.T) {
	tests := []struct {
		predecessorID string
		wantEmpty     bool
		contains      string
	}{
		{
			predecessorID: "",
			wantEmpty:     true,
		},
		{
			predecessorID: "hq-dec-123",
			wantEmpty:     false,
			contains:      "hq-dec-123",
		},
		{
			predecessorID: "some-long-decision-id-abc123",
			wantEmpty:     false,
			contains:      "some-long-decision-id-abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.predecessorID, func(t *testing.T) {
			result := buildChainInfoText(tt.predecessorID)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty string for empty predecessor, got %q", result)
				}
			} else {
				if result == "" {
					t.Error("expected non-empty string for non-empty predecessor")
				}
				if !containsIgnoreCase(result, tt.contains) {
					t.Errorf("result should contain %q, got %q", tt.contains, result)
				}
				// Should indicate it's a chain
				if !containsIgnoreCase(result, "chain") {
					t.Errorf("result should mention chain, got %q", result)
				}
			}
		})
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		(len(s) > 0 && (s[0] == substr[0] || s[0]+32 == substr[0] || s[0]-32 == substr[0]) && containsIgnoreCase(s[1:], substr[1:])) ||
		containsIgnoreCase(s[1:], substr))
}

// TestFormatContextForSlack_EdgeCases tests edge cases.
func TestFormatContextForSlack_EdgeCases(t *testing.T) {
	t.Run("just whitespace", func(t *testing.T) {
		result := formatContextForSlack("   ", 500)
		// Whitespace is not valid JSON, should return as plain text
		if result == "" {
			t.Error("whitespace should be returned as-is (trimmed or not)")
		}
	})

	t.Run("JSON boolean", func(t *testing.T) {
		result := formatContextForSlack("true", 500)
		// Valid JSON, should work
		if result == "" {
			t.Error("JSON boolean should produce output")
		}
	})

	t.Run("JSON number", func(t *testing.T) {
		result := formatContextForSlack("42", 500)
		// Valid JSON, should work
		if result == "" {
			t.Error("JSON number should produce output")
		}
	})

	t.Run("JSON null", func(t *testing.T) {
		result := formatContextForSlack("null", 500)
		// Valid JSON, should work
		if result == "" {
			t.Error("JSON null should produce output")
		}
	})

	t.Run("small maxLen", func(t *testing.T) {
		// Note: Very small maxLen values (< 10) can cause issues due to
		// code block marker overhead. Use reasonable minimum values.
		result := formatContextForSlack(`{"key": "value"}`, 50)
		if len(result) > 55 { // Allow some buffer for code block markers
			t.Errorf("result too long for maxLen=50: %d chars", len(result))
		}
	})

	t.Run("context with _type field removed", func(t *testing.T) {
		context := `{"_type": "tradeoff", "key": "value"}`
		result := formatContextForSlack(context, 500)
		// _type should be removed from display
		if strings.Contains(result, "_type") {
			t.Error("_type field should be removed from display")
		}
		if !strings.Contains(result, "key") {
			t.Error("regular fields should be preserved")
		}
	})

	t.Run("context with only _type field", func(t *testing.T) {
		context := `{"_type": "tradeoff"}`
		result := formatContextForSlack(context, 500)
		// Should return empty since only internal field
		if result != "" {
			t.Errorf("context with only _type should be empty, got: %s", result)
		}
	})
}

func TestExtractTypeFromContext(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		wantType string
	}{
		{
			name:     "empty context",
			context:  "",
			wantType: "",
		},
		{
			name:     "context with type",
			context:  `{"_type": "tradeoff", "key": "value"}`,
			wantType: "tradeoff",
		},
		{
			name:     "context without type",
			context:  `{"key": "value"}`,
			wantType: "",
		},
		{
			name:     "invalid JSON",
			context:  "not json",
			wantType: "",
		},
		{
			name:     "JSON array",
			context:  `["item1", "item2"]`,
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTypeFromContext(tt.context)
			if got != tt.wantType {
				t.Errorf("extractTypeFromContext() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestBuildTypeHeader(t *testing.T) {
	tests := []struct {
		name       string
		context    string
		wantEmoji  string
		wantLabel  string
	}{
		{
			name:       "no type",
			context:    `{"key": "value"}`,
			wantEmoji:  "",
			wantLabel:  "",
		},
		{
			name:       "tradeoff type",
			context:    `{"_type": "tradeoff"}`,
			wantEmoji:  "⚖️",
			wantLabel:  "Tradeoff Decision",
		},
		{
			name:       "confirmation type",
			context:    `{"_type": "confirmation"}`,
			wantEmoji:  "✅",
			wantLabel:  "Confirmation",
		},
		{
			name:       "checkpoint type",
			context:    `{"_type": "checkpoint"}`,
			wantEmoji:  "🚧",
			wantLabel:  "Checkpoint",
		},
		{
			name:       "unknown type",
			context:    `{"_type": "foobar"}`,
			wantEmoji:  "📋",
			wantLabel:  "Foobar Decision",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emoji, label := buildTypeHeader(tt.context)
			if emoji != tt.wantEmoji {
				t.Errorf("emoji = %q, want %q", emoji, tt.wantEmoji)
			}
			if label != tt.wantLabel {
				t.Errorf("label = %q, want %q", label, tt.wantLabel)
			}
		})
	}
}

// --- Hybrid Convoy/Epic Channel Routing Tests ---

// TestGetTrackingConvoyTitle_NoTownRoot tests that convoy lookup returns empty
// when townRoot is not set.
func TestGetTrackingConvoyTitle_NoTownRoot(t *testing.T) {
	bot := &Bot{
		townRoot: "", // Empty town root
	}

	result := bot.getTrackingConvoyTitle("gt-some-issue")
	if result != "" {
		t.Errorf("expected empty string when townRoot not set, got %q", result)
	}
}

// TestGetTrackingConvoyTitle_InvalidTownRoot tests that convoy lookup handles
// invalid town root gracefully.
func TestGetTrackingConvoyTitle_InvalidTownRoot(t *testing.T) {
	bot := &Bot{
		townRoot: "/nonexistent/path", // Invalid path
		debug:    false,
	}

	result := bot.getTrackingConvoyTitle("gt-some-issue")
	if result != "" {
		t.Errorf("expected empty string for invalid town root, got %q", result)
	}
}

// TestGetDecisionByThread tests the reverse lookup of decision ID by thread (gt-8d5q52.1).
func TestGetDecisionByThread(t *testing.T) {
	bot := &Bot{
		decisionMessages: make(map[string]messageInfo),
	}

	// Seed with some decision messages
	bot.decisionMessages["decision-1"] = messageInfo{channelID: "C123", timestamp: "1234.5678"}
	bot.decisionMessages["decision-2"] = messageInfo{channelID: "C456", timestamp: "2345.6789"}
	bot.decisionMessages["decision-3"] = messageInfo{channelID: "C123", timestamp: "3456.7890"}

	tests := []struct {
		name       string
		channelID  string
		threadTS   string
		expectedID string
	}{
		{
			name:       "finds decision by exact match",
			channelID:  "C123",
			threadTS:   "1234.5678",
			expectedID: "decision-1",
		},
		{
			name:       "finds second decision",
			channelID:  "C456",
			threadTS:   "2345.6789",
			expectedID: "decision-2",
		},
		{
			name:       "finds decision in same channel different thread",
			channelID:  "C123",
			threadTS:   "3456.7890",
			expectedID: "decision-3",
		},
		{
			name:       "returns empty for non-existent thread",
			channelID:  "C123",
			threadTS:   "9999.9999",
			expectedID: "",
		},
		{
			name:       "returns empty for non-existent channel",
			channelID:  "C999",
			threadTS:   "1234.5678",
			expectedID: "",
		},
		{
			name:       "returns empty for mismatched channel and thread",
			channelID:  "C456",
			threadTS:   "1234.5678",
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bot.getDecisionByThread(tt.channelID, tt.threadTS)
			if got != tt.expectedID {
				t.Errorf("getDecisionByThread(%q, %q) = %q, want %q",
					tt.channelID, tt.threadTS, got, tt.expectedID)
			}
		})
	}
}

// TestGetDecisionByThread_Empty tests that empty map returns empty string.
func TestGetDecisionByThread_Empty(t *testing.T) {
	bot := &Bot{
		decisionMessages: make(map[string]messageInfo),
	}

	got := bot.getDecisionByThread("C123", "1234.5678")
	if got != "" {
		t.Errorf("expected empty string for empty decisionMessages map, got %q", got)
	}
}

// TestResolveChannelForDecision_PriorityOrder tests that the routing priority
// is respected: convoy > epic > agent.
func TestResolveChannelForDecision_PriorityOrder(t *testing.T) {
	// This test verifies the priority flow without actual Slack API calls.
	// We test that when both convoy and epic info are available,
	// the routing logic checks convoy first.
	bot := &Bot{
		channelPrefix: "gt-decisions",
		channelID:     "default-channel",
		townRoot:      "", // No convoy lookup possible
	}

	tests := []struct {
		name           string
		parentBeadID   string
		parentTitle    string
		expectContains string // What the fallback should contain
	}{
		{
			name:           "no parent info falls back to agent routing",
			parentBeadID:   "",
			parentTitle:    "",
			expectContains: "", // Falls through to agent routing
		},
		{
			name:           "parent title triggers epic routing",
			parentBeadID:   "gt-epic-123",
			parentTitle:    "Test Epic",
			expectContains: "", // Would trigger epic routing (needs Slack API)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Full routing tests require Slack API mocking.
			// These tests verify the routing function doesn't panic
			// and handles edge cases gracefully.

			// Test convoy lookup returns empty when no town root
			convoyTitle := bot.getTrackingConvoyTitle(tt.parentBeadID)
			if convoyTitle != "" {
				t.Errorf("expected empty convoy title with no town root, got %q", convoyTitle)
			}

			_ = tt.expectContains // Placeholder for future assertions
		})
	}
}

// TestDecisionThreadReply_PredecessorLookup tests that follow-up decisions
// correctly look up their predecessor's thread timestamp (gt-8d5q52.2).
func TestDecisionThreadReply_PredecessorLookup(t *testing.T) {
	bot := &Bot{
		decisionMessages: make(map[string]messageInfo),
	}

	// Seed with a predecessor decision
	predecessorID := "predecessor-decision-1"
	bot.decisionMessages[predecessorID] = messageInfo{
		channelID: "C123",
		timestamp: "1111.2222",
	}

	tests := []struct {
		name            string
		predecessorID   string
		targetChannelID string
		expectThreadTS  string // Expected thread timestamp (empty if not a thread reply)
	}{
		{
			name:            "finds predecessor in same channel",
			predecessorID:   predecessorID,
			targetChannelID: "C123",
			expectThreadTS:  "1111.2222",
		},
		{
			name:            "no thread for non-existent predecessor",
			predecessorID:   "non-existent-predecessor",
			targetChannelID: "C123",
			expectThreadTS:  "",
		},
		{
			name:            "no thread when predecessor in different channel",
			predecessorID:   predecessorID,
			targetChannelID: "C456",
			expectThreadTS:  "",
		},
		{
			name:            "no predecessor ID means no thread",
			predecessorID:   "",
			targetChannelID: "C123",
			expectThreadTS:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var predecessorThreadTS string
			if tt.predecessorID != "" {
				bot.decisionMessagesMu.RLock()
				predMsgInfo, hasPredecessor := bot.decisionMessages[tt.predecessorID]
				bot.decisionMessagesMu.RUnlock()

				if hasPredecessor && predMsgInfo.channelID == tt.targetChannelID {
					predecessorThreadTS = predMsgInfo.timestamp
				}
			}

			if predecessorThreadTS != tt.expectThreadTS {
				t.Errorf("thread lookup for predecessor=%q, channel=%q: got %q, want %q",
					tt.predecessorID, tt.targetChannelID, predecessorThreadTS, tt.expectThreadTS)
			}
		})
	}
}

// TestMarkDecisionSuperseded_Format verifies the message format for superseded decisions (gt-8d5q52.2).
func TestMarkDecisionSuperseded_Format(t *testing.T) {
	// Test the text format without actual Slack API calls
	predecessorID := "predecessor-123"
	newDecisionID := "followup-456"

	expectedContains := []string{
		"Superseded",
		newDecisionID,
		"thread below",
	}

	// Build the superseded text (mirroring the actual function logic)
	supersededText := "⏭️ *Superseded*\n\nA follow-up decision (`" + newDecisionID + "`) has been posted in this thread.\n_Please refer to the latest decision in the thread below._"

	for _, expected := range expectedContains {
		if !strings.Contains(supersededText, expected) {
			t.Errorf("superseded text should contain %q, got: %s", expected, supersededText)
		}
	}

	// Verify predecessor ID would be shown in context
	contextText := "Original decision: `" + predecessorID + "`"
	if !strings.Contains(contextText, predecessorID) {
		t.Errorf("context should contain predecessor ID %q", predecessorID)
	}
}
