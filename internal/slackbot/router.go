// Package slackbot provides Slack integration for Gas Town.
// router.go implements channel routing based on agent identity patterns.

package slackbot

import (
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// Router resolves agent identities to Slack channel IDs.
// It matches agents against configured patterns in specificity order.
type Router struct {
	config *config.SlackConfig
	// patterns are pre-compiled and sorted by specificity (most specific first)
	patterns []compiledPattern
}

// compiledPattern represents a pre-processed channel routing pattern.
type compiledPattern struct {
	pattern   string   // original pattern string
	channel   string   // target channel ID
	segments  []string // split pattern segments
	wildcards int      // count of "*" segments
}

// NewRouter creates a Router from SlackConfig.
// Patterns are compiled and sorted by specificity for efficient matching.
func NewRouter(cfg *config.SlackConfig) *Router {
	if cfg == nil {
		cfg = config.NewSlackConfig()
	}

	r := &Router{config: cfg}
	r.compilePatterns()
	return r
}

// compilePatterns processes all channel patterns and sorts by specificity.
// Specificity order:
//  1. Exact matches (no wildcards)
//  2. Patterns with fewer wildcards
//  3. Patterns with more segments (longer paths)
func (r *Router) compilePatterns() {
	if r.config.Channels == nil {
		r.patterns = nil
		return
	}

	patterns := make([]compiledPattern, 0, len(r.config.Channels))

	for pattern, channel := range r.config.Channels {
		segments := strings.Split(pattern, "/")
		wildcards := 0
		for _, seg := range segments {
			if seg == "*" {
				wildcards++
			}
		}

		patterns = append(patterns, compiledPattern{
			pattern:   pattern,
			channel:   channel,
			segments:  segments,
			wildcards: wildcards,
		})
	}

	// Sort by specificity: fewer wildcards first, then more segments, then alphabetically
	sort.Slice(patterns, func(i, j int) bool {
		pi, pj := patterns[i], patterns[j]

		// Fewer wildcards = more specific
		if pi.wildcards != pj.wildcards {
			return pi.wildcards < pj.wildcards
		}

		// More segments = more specific
		if len(pi.segments) != len(pj.segments) {
			return len(pi.segments) > len(pj.segments)
		}

		// Alphabetical for deterministic ordering
		return pi.pattern < pj.pattern
	})

	r.patterns = patterns
}

// ResolveChannel returns the Slack channel ID for an agent identity.
// It tries patterns in specificity order and returns the first match.
// If no pattern matches, returns the default channel.
//
// Examples:
//
//	agent "gastown/polecats/furiosa" matches:
//	  - "gastown/polecats/furiosa" (exact)
//	  - "gastown/polecats/*" (wildcard)
//	  - "*/polecats/*" (multi-wildcard)
//	  - "gastown/*" (rig wildcard) - but only if segment count matches!
func (r *Router) ResolveChannel(agent string) string {
	if agent == "" {
		return r.config.DefaultChannel
	}

	// Try each pattern in specificity order
	for _, p := range r.patterns {
		if r.matches(agent, p) {
			return p.channel
		}
	}

	return r.config.DefaultChannel
}

// matches checks if an agent identity matches a compiled pattern.
// Both must have the same number of segments, and each segment must match
// exactly or the pattern segment must be "*".
func (r *Router) matches(agent string, p compiledPattern) bool {
	agentParts := strings.Split(agent, "/")

	// Segment count must match exactly
	if len(agentParts) != len(p.segments) {
		return false
	}

	for i, seg := range p.segments {
		if seg != "*" && seg != agentParts[i] {
			return false
		}
	}

	return true
}

// ChannelName returns the human-readable name for a channel ID.
// Returns the channel ID if no name is configured.
func (r *Router) ChannelName(channelID string) string {
	if r.config.ChannelNames != nil {
		if name, ok := r.config.ChannelNames[channelID]; ok {
			return name
		}
	}
	return channelID
}

// IsEnabled returns whether Slack routing is enabled.
func (r *Router) IsEnabled() bool {
	return r.config != nil && r.config.Enabled
}
