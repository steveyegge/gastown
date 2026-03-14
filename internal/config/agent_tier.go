package config

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// AgentTier defines a capability tier that maps to an ordered list of agent presets.
// Tiers abstract away specific agent choices, letting operators configure capability
// levels independently of role assignments.
type AgentTier struct {
	// Description is a human-readable description of what tasks this tier handles.
	// The Phase 3 router agent uses this to select the appropriate tier.
	Description string `json:"description"`

	// Agents is the ordered list of agent preset names in this tier.
	// For "priority" selection, the first available agent wins.
	// For "round-robin" selection, agents are cycled in order.
	// Agents are string references to preset names (built-in or custom) — not embedded configs.
	Agents []string `json:"agents"`

	// Selection is the agent selection strategy within this tier.
	// Valid values: "priority" (default), "round-robin".
	// "priority": first non-excluded agent in list wins.
	// "round-robin": cycle through agents in list order, skipping excluded ones.
	Selection string `json:"selection"`

	// Fallback controls whether this tier can be used as an automatic fallback
	// when all agents in a lower tier are excluded via AGENT_FAILURE mail.
	// Set to false for the highest capability tier to prevent escalation beyond it.
	// Default: true.
	Fallback bool `json:"fallback"`
}

// AgentTierConfig holds the full agent tier configuration.
// Stored in settings/config.json under "agent_tiers".
type AgentTierConfig struct {
	// Tiers maps tier names to their configuration.
	// Tier names are arbitrary strings — users can define custom tier names.
	// Built-in defaults use: "small", "medium", "large", "reasoning".
	Tiers map[string]*AgentTier `json:"tiers"`

	// TierOrder defines the capability ordering of tiers from lowest to highest.
	// Used for automatic fallback: when all agents in a tier are excluded,
	// Go routing code moves up one level in TierOrder.
	// Every tier name in Tiers must appear in TierOrder exactly once.
	TierOrder []string `json:"tier_order"`

	// RoleDefaults maps role names to default tier names.
	// Applied at dispatch time when no explicit --agent or --tier override is set
	// and no rig/town-level role_agents entry exists.
	// Keys are role names (e.g., "mayor", "polecat", "witness").
	// Values are tier names (must be keys in Tiers).
	RoleDefaults map[string]string `json:"role_defaults,omitempty"`

	// rrCounters holds per-tier round-robin counters (not persisted).
	// Keys are tier names (string), values are *atomic.Uint64.
	// Resets on process restart, which is intentional.
	rrCounters sync.Map
}

// TierSummary is a compact tier descriptor for the Phase 3 router agent.
// The router sees only names and descriptions — not agent lists, exclusion state,
// or selection strategies.
type TierSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HasTier reports whether a tier with the given name exists in this config.
func (c *AgentTierConfig) HasTier(name string) bool {
	if c == nil || c.Tiers == nil {
		return false
	}
	_, ok := c.Tiers[name]
	return ok
}

// TierNames returns the tier names in TierOrder sequence.
// If TierOrder is empty, returns the keys of Tiers in unspecified order.
func (c *AgentTierConfig) TierNames() []string {
	if c == nil {
		return nil
	}
	if len(c.TierOrder) > 0 {
		result := make([]string, len(c.TierOrder))
		copy(result, c.TierOrder)
		return result
	}
	names := make([]string, 0, len(c.Tiers))
	for name := range c.Tiers {
		names = append(names, name)
	}
	return names
}

// BuildTierSummaries returns tier name+description pairs in TierOrder sequence.
// Used by the Phase 3 router agent to select a tier based on task complexity.
// The router sees only descriptions — not agent lists, selection strategies, or exclusions.
func (c *AgentTierConfig) BuildTierSummaries() []TierSummary {
	if c == nil || len(c.Tiers) == 0 {
		return nil
	}
	names := c.TierNames()
	summaries := make([]TierSummary, 0, len(names))
	for _, name := range names {
		tier, ok := c.Tiers[name]
		if !ok || tier == nil {
			continue
		}
		summaries = append(summaries, TierSummary{
			Name:        name,
			Description: tier.Description,
		})
	}
	return summaries
}

// Validate checks the AgentTierConfig for structural consistency.
// It verifies:
//   - Every name in TierOrder references a key in Tiers
//   - No duplicate names in TierOrder
//   - Every role in RoleDefaults references a tier in Tiers
//   - Each tier's Selection value is "priority" or "round-robin" (or empty, meaning priority)
//
// Note: Agent names are not validated here because they may reference custom agents
// defined in settings/agents.json, which is not available at this level.
func (c *AgentTierConfig) Validate() error {
	if c == nil {
		return nil
	}

	// Validate TierOrder references existing tiers, no duplicates
	seen := make(map[string]bool, len(c.TierOrder))
	for i, name := range c.TierOrder {
		if _, ok := c.Tiers[name]; !ok {
			return fmt.Errorf("tier_order[%d]: %q is not defined in tiers", i, name)
		}
		if seen[name] {
			return fmt.Errorf("tier_order: duplicate tier name %q", name)
		}
		seen[name] = true
	}

	// Validate per-tier fields
	for name, tier := range c.Tiers {
		if tier == nil {
			return fmt.Errorf("tier %q: nil tier config", name)
		}
		sel := tier.Selection
		if sel != "" && sel != "priority" && sel != "round-robin" {
			return fmt.Errorf("tier %q: invalid selection %q (must be \"priority\" or \"round-robin\")", name, sel)
		}
	}

	// Validate RoleDefaults reference existing tiers
	var invalidRoles []string
	for role, tierName := range c.RoleDefaults {
		if !c.HasTier(tierName) {
			invalidRoles = append(invalidRoles, fmt.Sprintf("%s→%s", role, tierName))
		}
	}
	if len(invalidRoles) > 0 {
		return fmt.Errorf("role_defaults references undefined tiers: %s", strings.Join(invalidRoles, ", "))
	}

	return nil
}

// ResolveTierForRole returns the default tier name for the given role.
// Returns "" if no mapping exists.
func (c *AgentTierConfig) ResolveTierForRole(role string) string {
	if c == nil || c.RoleDefaults == nil {
		return ""
	}
	return c.RoleDefaults[role]
}

// UpOneTier returns the next tier above tierName in TierOrder.
// Returns "" if tierName is not in TierOrder or is already the highest tier.
func (c *AgentTierConfig) UpOneTier(tierName string) string {
	for i, name := range c.TierOrder {
		if name == tierName {
			if i+1 < len(c.TierOrder) {
				return c.TierOrder[i+1]
			}
			return ""
		}
	}
	return ""
}

// rrCounter returns the atomic round-robin counter for the given tier,
// creating it on first access.
func (c *AgentTierConfig) rrCounter(tierName string) *atomic.Uint64 {
	v, _ := c.rrCounters.LoadOrStore(tierName, new(atomic.Uint64))
	return v.(*atomic.Uint64)
}

// ResolveTierToRuntimeConfig resolves a tier name to a RuntimeConfig.
//
// Resolution steps:
//  1. Look up tier by name — returns error if not found.
//  2. Filter agents: skip any in excludedAgents.
//  3. Select agent based on tier's Selection strategy:
//     - "priority" (default): first available agent in list order.
//     - "round-robin": next agent in cycle via per-tier atomic counter.
//  4. Resolve selected agent name to RuntimeConfig via GetAgentPresetByName + RuntimeConfigFromPreset.
//  5. If no agents remain and Fallback is true: recurse via UpOneTier.
//  6. If no agents remain and Fallback is false (or no higher tier): return error.
func (c *AgentTierConfig) ResolveTierToRuntimeConfig(tierName string, excludedAgents map[string]bool) (*RuntimeConfig, error) {
	if c == nil || c.Tiers == nil {
		return nil, fmt.Errorf("agent tier config is nil")
	}

	tier, ok := c.Tiers[tierName]
	if !ok {
		return nil, fmt.Errorf("tier %q not found", tierName)
	}
	if tier == nil {
		return nil, fmt.Errorf("tier %q has nil config", tierName)
	}

	// Filter to available (non-excluded) agents.
	available := make([]string, 0, len(tier.Agents))
	for _, agent := range tier.Agents {
		if !excludedAgents[agent] {
			available = append(available, agent)
		}
	}

	if len(available) == 0 {
		if !tier.Fallback {
			return nil, fmt.Errorf("no agents available in tier %q and fallback is disabled", tierName)
		}
		next := c.UpOneTier(tierName)
		if next == "" {
			return nil, fmt.Errorf("no agents available in tier %q and no higher tier exists", tierName)
		}
		return c.ResolveTierToRuntimeConfig(next, excludedAgents)
	}

	// Select agent.
	var selected string
	switch tier.Selection {
	case "round-robin":
		counter := c.rrCounter(tierName)
		idx := int(counter.Add(1)-1) % len(available)
		selected = available[idx]
	default: // "priority" or ""
		selected = available[0]
	}

	preset := GetAgentPresetByName(selected)
	if preset == nil {
		return nil, fmt.Errorf("agent preset %q not found", selected)
	}

	rc := RuntimeConfigFromPreset(preset.Name)
	rc.ResolvedAgent = selected
	return rc, nil
}
