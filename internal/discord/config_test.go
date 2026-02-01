package discord

import (
	"os"
	"testing"
)

func TestAutoRespondDisabledByDefault(t *testing.T) {
	// Clear any existing env vars
	os.Unsetenv("DISCORD_AUTO_RESPOND")

	cfg := LoadAutoResponseConfig()

	if cfg.AutoRespond {
		t.Error("expected AutoRespond to be false by default")
	}
}

func TestAutoRespondEnabled(t *testing.T) {
	os.Setenv("DISCORD_AUTO_RESPOND", "true")
	defer os.Unsetenv("DISCORD_AUTO_RESPOND")

	cfg := LoadAutoResponseConfig()

	if !cfg.AutoRespond {
		t.Error("expected AutoRespond to be true when DISCORD_AUTO_RESPOND=true")
	}
}

func TestAutoRespondEnabledVariants(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("DISCORD_AUTO_RESPOND", tt.value)
			defer os.Unsetenv("DISCORD_AUTO_RESPOND")

			cfg := LoadAutoResponseConfig()

			if cfg.AutoRespond != tt.expected {
				t.Errorf("DISCORD_AUTO_RESPOND=%q: got AutoRespond=%v, want %v",
					tt.value, cfg.AutoRespond, tt.expected)
			}
		})
	}
}

func TestChannelAllowlistFiltering(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		channelID      string
		expectAllowed  bool
	}{
		{
			name:          "empty means all channels allowed",
			envValue:      "",
			channelID:     "123456789",
			expectAllowed: true,
		},
		{
			name:          "channel in allowlist",
			envValue:      "123456789,987654321",
			channelID:     "123456789",
			expectAllowed: true,
		},
		{
			name:          "channel not in allowlist",
			envValue:      "123456789,987654321",
			channelID:     "111111111",
			expectAllowed: false,
		},
		{
			name:          "whitespace trimmed",
			envValue:      " 123456789 , 987654321 ",
			channelID:     "123456789",
			expectAllowed: true,
		},
		{
			name:          "single channel allowlist",
			envValue:      "123456789",
			channelID:     "123456789",
			expectAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DISCORD_RESPOND_CHANNELS", tt.envValue)
			defer os.Unsetenv("DISCORD_RESPOND_CHANNELS")

			cfg := LoadAutoResponseConfig()
			allowed := cfg.IsChannelAllowed(tt.channelID)

			if allowed != tt.expectAllowed {
				t.Errorf("IsChannelAllowed(%q) with DISCORD_RESPOND_CHANNELS=%q: got %v, want %v",
					tt.channelID, tt.envValue, allowed, tt.expectAllowed)
			}
		})
	}
}

func TestCustomSystemPromptUsed(t *testing.T) {
	customPrompt := "You are a helpful Discord bot assistant."
	os.Setenv("DISCORD_SYSTEM_PROMPT", customPrompt)
	defer os.Unsetenv("DISCORD_SYSTEM_PROMPT")

	cfg := LoadAutoResponseConfig()

	if cfg.SystemPrompt != customPrompt {
		t.Errorf("expected SystemPrompt=%q, got %q", customPrompt, cfg.SystemPrompt)
	}
}

func TestDefaultSystemPrompt(t *testing.T) {
	os.Unsetenv("DISCORD_SYSTEM_PROMPT")

	cfg := LoadAutoResponseConfig()

	if cfg.SystemPrompt != "" {
		t.Errorf("expected empty default SystemPrompt, got %q", cfg.SystemPrompt)
	}
}

func TestMaxTokensDefault(t *testing.T) {
	os.Unsetenv("DISCORD_MAX_TOKENS")

	cfg := LoadAutoResponseConfig()

	if cfg.MaxTokens != 1024 {
		t.Errorf("expected default MaxTokens=1024, got %d", cfg.MaxTokens)
	}
}

func TestMaxTokensCustom(t *testing.T) {
	os.Setenv("DISCORD_MAX_TOKENS", "2048")
	defer os.Unsetenv("DISCORD_MAX_TOKENS")

	cfg := LoadAutoResponseConfig()

	if cfg.MaxTokens != 2048 {
		t.Errorf("expected MaxTokens=2048, got %d", cfg.MaxTokens)
	}
}

func TestMaxTokensInvalid(t *testing.T) {
	os.Setenv("DISCORD_MAX_TOKENS", "not-a-number")
	defer os.Unsetenv("DISCORD_MAX_TOKENS")

	cfg := LoadAutoResponseConfig()

	if cfg.MaxTokens != 1024 {
		t.Errorf("expected default MaxTokens=1024 for invalid input, got %d", cfg.MaxTokens)
	}
}

func TestMaxTokensNegative(t *testing.T) {
	os.Setenv("DISCORD_MAX_TOKENS", "-100")
	defer os.Unsetenv("DISCORD_MAX_TOKENS")

	cfg := LoadAutoResponseConfig()

	if cfg.MaxTokens != 1024 {
		t.Errorf("expected default MaxTokens=1024 for negative input, got %d", cfg.MaxTokens)
	}
}
