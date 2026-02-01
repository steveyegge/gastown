// Package discord provides Discord integration configuration.
package discord

import (
	"os"
	"strconv"
	"strings"
)

// AutoResponseConfig holds configuration for auto-response behavior.
type AutoResponseConfig struct {
	// AutoRespond enables automatic responses to messages.
	// Controlled by DISCORD_AUTO_RESPOND env var (true/false).
	// Default: false
	AutoRespond bool

	// RespondChannels is the list of channel IDs to respond in.
	// If empty, responses are allowed in all channels.
	// Controlled by DISCORD_RESPOND_CHANNELS env var (comma-separated IDs).
	RespondChannels []string

	// SystemPrompt is a custom system prompt for the AI.
	// Controlled by DISCORD_SYSTEM_PROMPT env var.
	// Default: empty (use default prompt)
	SystemPrompt string

	// MaxTokens is the maximum number of tokens for responses.
	// Controlled by DISCORD_MAX_TOKENS env var.
	// Default: 1024
	MaxTokens int
}

// LoadAutoResponseConfig loads configuration from environment variables.
func LoadAutoResponseConfig() *AutoResponseConfig {
	cfg := &AutoResponseConfig{
		AutoRespond:     parseBool(os.Getenv("DISCORD_AUTO_RESPOND")),
		RespondChannels: parseChannelList(os.Getenv("DISCORD_RESPOND_CHANNELS")),
		SystemPrompt:    os.Getenv("DISCORD_SYSTEM_PROMPT"),
		MaxTokens:       parseMaxTokens(os.Getenv("DISCORD_MAX_TOKENS")),
	}
	return cfg
}

// IsChannelAllowed returns true if the channel is allowed for responses.
// If no channels are configured, all channels are allowed.
func (c *AutoResponseConfig) IsChannelAllowed(channelID string) bool {
	if len(c.RespondChannels) == 0 {
		return true
	}
	for _, id := range c.RespondChannels {
		if id == channelID {
			return true
		}
	}
	return false
}

// parseBool parses a boolean string value.
// Accepts: "true", "TRUE", "True", "1", "yes", "YES" as true.
// Everything else is false.
func parseBool(s string) bool {
	lower := strings.ToLower(s)
	return lower == "true" || lower == "1" || lower == "yes"
}

// parseChannelList parses a comma-separated list of channel IDs.
// Trims whitespace from each ID.
func parseChannelList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	channels := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			channels = append(channels, trimmed)
		}
	}
	return channels
}

// parseMaxTokens parses the max tokens value with default fallback.
func parseMaxTokens(s string) int {
	const defaultMaxTokens = 1024

	if s == "" {
		return defaultMaxTokens
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultMaxTokens
	}

	return n
}
