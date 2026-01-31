// Package mail provides address resolution for beads-native messaging.
// This module implements the resolution order:
// 1. Contains '/' → agent address or pattern
// 2. Starts with '@' → special pattern (@town, @crew, @rig/X, @role/X)
// 3. Otherwise → lookup by name: group → queue → channel
// 4. If conflict, require prefix (group:X, queue:X, channel:X)
package mail

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// RecipientType indicates the type of resolved recipient.
type RecipientType string

const (
	RecipientAgent   RecipientType = "agent"   // Direct to agent(s)
	RecipientQueue   RecipientType = "queue"   // Single message, workers claim
	RecipientChannel RecipientType = "channel" // Broadcast, retained
)

// Recipient represents a resolved message recipient.
type Recipient struct {
	Address      string        // The resolved address (e.g., "gastown/crew/max")
	Type         RecipientType // Type of recipient (agent, queue, channel)
	OriginalName string        // Original name before resolution (for queues/channels)
}

// Resolver handles address resolution for beads-native messaging.
type Resolver struct {
	beads    *beads.Beads
	townRoot string
}

// NewResolver creates a new address resolver.
func NewResolver(b *beads.Beads, townRoot string) *Resolver {
	return &Resolver{
		beads:    b,
		townRoot: townRoot,
	}
}

// Resolve resolves an address to a list of recipients.
// Resolution order:
// 1. Contains '/' → agent address or pattern (direct delivery)
// 2. Starts with '@' → special pattern (@town, @crew, etc.)
// 3. Starts with explicit prefix → use that type (group:, queue:, channel:)
// 4. Otherwise → lookup by name: group → queue → channel
func (r *Resolver) Resolve(address string) ([]Recipient, error) {
	// 1. Explicit prefix takes precedence
	if strings.HasPrefix(address, "group:") {
		name := strings.TrimPrefix(address, "group:")
		return r.resolveBeadsGroup(name)
	}
	if strings.HasPrefix(address, "queue:") {
		name := strings.TrimPrefix(address, "queue:")
		return r.resolveQueue(name)
	}
	if strings.HasPrefix(address, "channel:") {
		name := strings.TrimPrefix(address, "channel:")
		return r.resolveChannel(name)
	}

	// Legacy prefixes (list:, announce:) - pass through
	if strings.HasPrefix(address, "list:") || strings.HasPrefix(address, "announce:") {
		// These are handled by existing router logic
		return []Recipient{{Address: address, Type: RecipientAgent}}, nil
	}

	// 2. Contains '/' → agent address or pattern
	if strings.Contains(address, "/") {
		return r.resolveAgentAddress(address)
	}

	// 3. Starts with '@' → special pattern
	if strings.HasPrefix(address, "@") {
		return r.resolveAtPattern(address)
	}

	// 4. Name lookup: group → queue → channel
	return r.resolveByName(address)
}

// resolveAgentAddress handles addresses containing '/'.
// These are either direct addresses or patterns.
func (r *Resolver) resolveAgentAddress(address string) ([]Recipient, error) {
	// Check for wildcard patterns
	if strings.Contains(address, "*") {
		return r.resolvePattern(address)
	}

	// Direct address - single recipient
	return []Recipient{{
		Address: address,
		Type:    RecipientAgent,
	}}, nil
}

// resolvePattern expands a wildcard pattern to matching agents.
// Patterns like "*/witness" or "gastown/*" are expanded.
func (r *Resolver) resolvePattern(pattern string) ([]Recipient, error) {
	if r.beads == nil {
		return nil, fmt.Errorf("beads not available for pattern resolution")
	}

	// Get all agent beads
	agents, err := r.beads.ListAgentBeads()
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	var recipients []Recipient
	for id := range agents {
		// Convert bead ID to address and check match
		addr := agentBeadIDToAddress(id)
		if addr != "" && matchPattern(pattern, addr) {
			recipients = append(recipients, Recipient{
				Address: addr,
				Type:    RecipientAgent,
			})
		}
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("no agents match pattern: %s", pattern)
	}

	return recipients, nil
}

// resolveAtPattern handles @-prefixed patterns.
// These include @town, @crew, @rig/X, @role/X, @overseer.
func (r *Resolver) resolveAtPattern(address string) ([]Recipient, error) {
	// First check if this is a beads-native group (if beads available)
	if r.beads != nil {
		groupName := strings.TrimPrefix(address, "@")
		issue, fields, err := r.beads.LookupGroupByName(groupName)
		if err != nil {
			return nil, err
		}
		if issue != nil && fields != nil {
			// Found a beads-native group - expand its members
			return r.expandGroupMembers(fields)
		}
	}

	// Fall back to built-in patterns (handled by existing router)
	// Return as-is for router to handle
	return []Recipient{{Address: address, Type: RecipientAgent}}, nil
}

// resolveByName looks up a name as group → queue → channel.
// Returns error if name conflicts exist without explicit prefix.
func (r *Resolver) resolveByName(name string) ([]Recipient, error) {
	var foundGroup, foundQueue, foundChannel bool
	var groupFields *beads.GroupFields

	// Check for beads-native group
	if r.beads != nil {
		_, fields, err := r.beads.LookupGroupByName(name)
		if err != nil {
			return nil, err
		}
		if fields != nil {
			foundGroup = true
			groupFields = fields
		}
	}

	// Check for beads-native queue
	if r.beads != nil {
		_, queueFields, err := r.beads.LookupQueueByName(name)
		if err != nil {
			return nil, err
		}
		if queueFields != nil {
			foundQueue = true
		}
	}

	// Check for beads-native channel
	if r.beads != nil {
		_, channelFields, err := r.beads.LookupChannelByName(name)
		if err != nil {
			return nil, err
		}
		if channelFields != nil {
			foundChannel = true
		}
	}

	// Check for queue/channel in config (legacy)
	if r.townRoot != "" {
		cfg, err := config.LoadMessagingConfig(config.MessagingConfigPath(r.townRoot))
		if err == nil && cfg != nil {
			if _, ok := cfg.Queues[name]; ok {
				foundQueue = true
			}
			if _, ok := cfg.Announces[name]; ok {
				foundChannel = true
			}
		}
	}

	// Count conflicts
	conflictCount := 0
	if foundGroup {
		conflictCount++
	}
	if foundQueue {
		conflictCount++
	}
	if foundChannel {
		conflictCount++
	}

	if conflictCount == 0 {
		return nil, fmt.Errorf("unknown address: %s (not a group, queue, or channel)", name)
	}

	if conflictCount > 1 {
		var types []string
		if foundGroup {
			types = append(types, "group:"+name)
		}
		if foundQueue {
			types = append(types, "queue:"+name)
		}
		if foundChannel {
			types = append(types, "channel:"+name)
		}
		return nil, fmt.Errorf("ambiguous address %q: matches multiple types. Use explicit prefix: %s",
			name, strings.Join(types, ", "))
	}

	// Single match - resolve it
	if foundGroup {
		return r.expandGroupMembers(groupFields)
	}
	if foundQueue {
		return r.resolveQueue(name)
	}
	return r.resolveChannel(name)
}

// resolveBeadsGroup resolves a beads-native group by name.
func (r *Resolver) resolveBeadsGroup(name string) ([]Recipient, error) {
	if r.beads == nil {
		return nil, fmt.Errorf("beads not available")
	}

	_, fields, err := r.beads.LookupGroupByName(name)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("group not found: %s", name)
	}

	return r.expandGroupMembers(fields)
}

// expandGroupMembers expands a group's members to recipients.
// Handles nested groups and patterns recursively.
func (r *Resolver) expandGroupMembers(fields *beads.GroupFields) ([]Recipient, error) {
	return r.expandGroupMembersWithVisited(fields, make(map[string]bool))
}

// expandGroupMembersWithVisited expands group members with cycle detection.
func (r *Resolver) expandGroupMembersWithVisited(fields *beads.GroupFields, visited map[string]bool) ([]Recipient, error) {
	if fields == nil {
		return nil, nil
	}

	// Mark this group as visited for cycle detection
	if fields.Name != "" {
		if visited[fields.Name] {
			// Cycle detected - skip silently (as per design: "silent skip with warning")
			return nil, nil
		}
		visited[fields.Name] = true
	}

	seen := make(map[string]bool)
	var recipients []Recipient

	for _, member := range fields.Members {
		// Recursively resolve each member
		resolved, err := r.resolveMemberWithVisited(member, visited)
		if err != nil {
			// Log warning but continue with other members
			continue
		}

		for _, rec := range resolved {
			// Deduplicate
			if !seen[rec.Address] {
				seen[rec.Address] = true
				recipients = append(recipients, rec)
			}
		}
	}

	return recipients, nil
}

// resolveMemberWithVisited resolves a single group member with cycle detection.
func (r *Resolver) resolveMemberWithVisited(member string, visited map[string]bool) ([]Recipient, error) {
	// Check if this is a nested group reference
	if r.beads != nil && !strings.Contains(member, "/") && !strings.HasPrefix(member, "@") {
		_, fields, err := r.beads.LookupGroupByName(member)
		if err == nil && fields != nil {
			return r.expandGroupMembersWithVisited(fields, visited)
		}
	}

	// Otherwise resolve normally
	return r.Resolve(member)
}

// resolveQueue returns a queue recipient.
func (r *Resolver) resolveQueue(name string) ([]Recipient, error) {
	return []Recipient{{
		Address:      "queue:" + name,
		Type:         RecipientQueue,
		OriginalName: name,
	}}, nil
}

// resolveChannel returns a channel recipient.
func (r *Resolver) resolveChannel(name string) ([]Recipient, error) {
	return []Recipient{{
		Address:      "channel:" + name,
		Type:         RecipientChannel,
		OriginalName: name,
	}}, nil
}

// agentBeadIDToAddress converts an agent bead ID to a mail address.
// Handles both hq- (town agents) and gt- (rig agents) prefixes:
//   - hq-mayor → mayor/
//   - hq-deacon → deacon/
//   - gt-gastown-crew-max → gastown/crew/max
//   - hq-gastown-crew-max → gastown/crew/max
func agentBeadIDToAddress(id string) string {
	var rest string

	// Handle both hq- (town agents) and gt- (rig agents) prefixes
	if strings.HasPrefix(id, "hq-") {
		rest = strings.TrimPrefix(id, "hq-")
	} else if strings.HasPrefix(id, "gt-") {
		rest = strings.TrimPrefix(id, "gt-")
	} else {
		return ""
	}

	parts := strings.Split(rest, "-")

	switch len(parts) {
	case 1:
		// Town-level: gt-mayor → mayor/
		return parts[0] + "/"
	case 2:
		// Rig singleton: gt-gastown-witness → gastown/witness
		return parts[0] + "/" + parts[1]
	default:
		// Rig named agent: gt-gastown-crew-max → gastown/crew/max
		if len(parts) >= 3 {
			name := strings.Join(parts[2:], "-")
			return parts[0] + "/" + parts[1] + "/" + name
		}
		return ""
	}
}

// matchPattern checks if an address matches a wildcard pattern.
// '*' matches any single path segment (no slashes).
func matchPattern(pattern, address string) bool {
	patternParts := strings.Split(pattern, "/")
	addressParts := strings.Split(address, "/")

	if len(patternParts) != len(addressParts) {
		return false
	}

	for i, p := range patternParts {
		if p == "*" {
			continue // Wildcard matches anything
		}
		if p != addressParts[i] {
			return false
		}
	}

	return true
}
