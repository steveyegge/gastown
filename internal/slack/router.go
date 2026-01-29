// Package slack provides channel routing for per-agent Slack notifications.
package slack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config represents Slack integration configuration.
type Config struct {
	Type    string `json:"type"`    // "slack"
	Version int    `json:"version"` // schema version

	// Enabled controls whether Slack notifications are active.
	Enabled bool `json:"enabled"`

	// DefaultChannel is the fallback channel when no pattern matches.
	// Format: channel ID (e.g., "C0123456789") or name (e.g., "#decisions")
	DefaultChannel string `json:"default_channel"`

	// Channels maps agent patterns to Slack channel IDs.
	// Patterns support wildcards: "*" matches any single segment.
	// Examples:
	//   "gastown/polecats/*"  → all polecats in gastown
	//   "*/crew/*"            → all crew members across rigs
	//   "beads/*"             → all agents in beads rig
	Channels map[string]string `json:"channels"`

	// Overrides maps exact agent identities to dedicated channel IDs.
	// These take precedence over pattern matching. Created via "Break Out" button.
	// Example: "gastown/crew/slack_decisions" → "C0987654321"
	Overrides map[string]string `json:"overrides,omitempty"`

	// ChannelNames maps channel IDs to human-readable names for display.
	// Optional; used for logging and debugging.
	ChannelNames map[string]string `json:"channel_names,omitempty"`

	// WebhookURL is the default webhook for posting messages.
	// Individual channels may have their own webhooks in ChannelWebhooks.
	WebhookURL string `json:"webhook_url,omitempty"`

	// ChannelWebhooks maps channel IDs to their webhook URLs.
	// If a channel has no entry, uses WebhookURL.
	ChannelWebhooks map[string]string `json:"channel_webhooks,omitempty"`
}

// Router resolves Slack channels for agent identities.
type Router struct {
	config     *Config
	configPath string // Path to config file for saving overrides
	patterns   []compiledPattern
	mu         sync.RWMutex
}

// compiledPattern is a pre-processed pattern for faster matching.
type compiledPattern struct {
	original string   // Original pattern string
	segments []string // Pattern split by "/"
	channel  string   // Target channel ID
}

// RouteResult contains the resolved channel information.
type RouteResult struct {
	ChannelID   string // Slack channel ID (e.g., "C0123456789")
	ChannelName string // Human-readable name if available
	WebhookURL  string // Webhook URL for this channel
	MatchedBy   string // Pattern that matched (for debugging)
	IsDefault   bool   // True if using default channel
}

// NewRouter creates a new channel router from config.
func NewRouter(cfg *Config) *Router {
	r := &Router{
		config: cfg,
	}
	r.compilePatterns()
	return r
}

// LoadRouter loads router configuration from the standard location.
// Config file: ~/gt/settings/slack.json or $GT_ROOT/settings/slack.json
func LoadRouter() (*Router, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	return LoadRouterFromFile(configPath)
}

// LoadRouterFromFile loads router configuration from a specific file.
func LoadRouterFromFile(path string) (*Router, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	r := NewRouter(&cfg)
	r.configPath = path
	return r, nil
}

// findConfigPath locates the Slack config file.
func findConfigPath() (string, error) {
	// Check GT_ROOT environment variable first
	if gtRoot := os.Getenv("GT_ROOT"); gtRoot != "" {
		path := filepath.Join(gtRoot, "settings", "slack.json")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Check ~/gt/settings/slack.json
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	path := filepath.Join(home, "gt", "settings", "slack.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Check current town's settings (for multi-town setups)
	// Walk up from cwd looking for .beads directory indicating town root
	cwd, err := os.Getwd()
	if err == nil {
		for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			beadsDir := filepath.Join(dir, ".beads")
			if _, err := os.Stat(beadsDir); err == nil {
				path := filepath.Join(dir, "settings", "slack.json")
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
				break
			}
		}
	}

	return "", fmt.Errorf("slack config not found (checked $GT_ROOT/settings/slack.json, ~/gt/settings/slack.json)")
}

// compilePatterns pre-processes patterns for efficient matching.
func (r *Router) compilePatterns() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.patterns = make([]compiledPattern, 0, len(r.config.Channels))

	for pattern, channel := range r.config.Channels {
		r.patterns = append(r.patterns, compiledPattern{
			original: pattern,
			segments: strings.Split(pattern, "/"),
			channel:  channel,
		})
	}

	// Sort patterns by specificity (more segments = more specific = higher priority)
	// Also prioritize patterns without wildcards
	for i := 0; i < len(r.patterns); i++ {
		for j := i + 1; j < len(r.patterns); j++ {
			if patternLessThan(r.patterns[j], r.patterns[i]) {
				r.patterns[i], r.patterns[j] = r.patterns[j], r.patterns[i]
			}
		}
	}
}

// patternLessThan returns true if a should be matched before b (higher priority).
func patternLessThan(a, b compiledPattern) bool {
	// More segments = more specific = higher priority
	if len(a.segments) != len(b.segments) {
		return len(a.segments) > len(b.segments)
	}

	// Fewer wildcards = more specific = higher priority
	aWildcards := countWildcards(a.segments)
	bWildcards := countWildcards(b.segments)
	if aWildcards != bWildcards {
		return aWildcards < bWildcards
	}

	// Tie-breaker: alphabetical order for determinism
	return a.original < b.original
}

// countWildcards counts "*" segments in a pattern.
func countWildcards(segments []string) int {
	count := 0
	for _, s := range segments {
		if s == "*" {
			count++
		}
	}
	return count
}

// Resolve finds the appropriate Slack channel for an agent identity.
// Agent format: "rig/role/name" (e.g., "gastown/polecats/furiosa")
// Priority: 1. Exact override, 2. Pattern match, 3. Default channel
func (r *Router) Resolve(agent string) *RouteResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check overrides first (exact agent match, highest priority)
	if r.config.Overrides != nil {
		if channelID, ok := r.config.Overrides[agent]; ok {
			return &RouteResult{
				ChannelID:   channelID,
				ChannelName: r.config.ChannelNames[channelID],
				WebhookURL:  r.getWebhookForChannel(channelID),
				MatchedBy:   "(override)",
				IsDefault:   false,
			}
		}
	}

	agentSegments := strings.Split(agent, "/")

	// Try each pattern in priority order
	for _, p := range r.patterns {
		if matchPattern(p.segments, agentSegments) {
			return &RouteResult{
				ChannelID:   p.channel,
				ChannelName: r.config.ChannelNames[p.channel],
				WebhookURL:  r.getWebhookForChannel(p.channel),
				MatchedBy:   p.original,
				IsDefault:   false,
			}
		}
	}

	// Fall back to default channel
	return &RouteResult{
		ChannelID:   r.config.DefaultChannel,
		ChannelName: r.config.ChannelNames[r.config.DefaultChannel],
		WebhookURL:  r.getWebhookForChannel(r.config.DefaultChannel),
		MatchedBy:   "(default)",
		IsDefault:   true,
	}
}

// matchPattern checks if an agent matches a pattern.
// Pattern segments can be literal strings or "*" for wildcard.
func matchPattern(pattern, agent []string) bool {
	if len(pattern) != len(agent) {
		return false
	}

	for i, p := range pattern {
		if p != "*" && p != agent[i] {
			return false
		}
	}

	return true
}

// getWebhookForChannel returns the webhook URL for a channel.
func (r *Router) getWebhookForChannel(channelID string) string {
	if r.config.ChannelWebhooks != nil {
		if webhook, ok := r.config.ChannelWebhooks[channelID]; ok {
			return webhook
		}
	}
	return r.config.WebhookURL
}

// IsEnabled returns whether Slack notifications are enabled.
func (r *Router) IsEnabled() bool {
	return r.config.Enabled
}

// GetConfig returns the underlying configuration (for debugging).
func (r *Router) GetConfig() *Config {
	return r.config
}

// ResolveAll resolves channels for multiple agents, returning unique channels.
// Useful for notifications that should go to multiple channels.
func (r *Router) ResolveAll(agents []string) []*RouteResult {
	seen := make(map[string]bool)
	var results []*RouteResult

	for _, agent := range agents {
		result := r.Resolve(agent)
		if !seen[result.ChannelID] {
			seen[result.ChannelID] = true
			results = append(results, result)
		}
	}

	return results
}

// HasOverride returns true if the agent has a dedicated channel override.
func (r *Router) HasOverride(agent string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config.Overrides == nil {
		return false
	}
	_, ok := r.config.Overrides[agent]
	return ok
}

// GetOverride returns the override channel ID for an agent, or empty string if none.
func (r *Router) GetOverride(agent string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config.Overrides == nil {
		return ""
	}
	return r.config.Overrides[agent]
}

// AddOverride sets a dedicated channel override for an agent.
// The override takes precedence over pattern matching.
func (r *Router) AddOverride(agent, channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.Overrides == nil {
		r.config.Overrides = make(map[string]string)
	}
	r.config.Overrides[agent] = channelID

	// Also add to channel names if we know the name
	if r.config.ChannelNames == nil {
		r.config.ChannelNames = make(map[string]string)
	}
}

// AddOverrideWithName sets a dedicated channel override and records the channel name.
func (r *Router) AddOverrideWithName(agent, channelID, channelName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.Overrides == nil {
		r.config.Overrides = make(map[string]string)
	}
	r.config.Overrides[agent] = channelID

	if r.config.ChannelNames == nil {
		r.config.ChannelNames = make(map[string]string)
	}
	r.config.ChannelNames[channelID] = channelName
}

// RemoveOverride removes a dedicated channel override for an agent.
// Returns the previous channel ID if one existed, empty string otherwise.
func (r *Router) RemoveOverride(agent string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.Overrides == nil {
		return ""
	}
	prev := r.config.Overrides[agent]
	delete(r.config.Overrides, agent)
	return prev
}

// Save persists the current configuration to the config file.
// Returns an error if no config path is known (e.g., router created with NewRouter directly).
// Uses atomic write (temp file + rename) to prevent data corruption.
func (r *Router) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.configPath == "" {
		return fmt.Errorf("no config path: router was not loaded from file")
	}

	data, err := json.MarshalIndent(r.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := r.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, r.configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}
