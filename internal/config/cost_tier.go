package config

import (
	"fmt"
	"strings"
)

// CostTier represents a predefined cost optimization tier for model selection.
type CostTier string

const (
	// TierStandard uses opus for all roles (default, highest quality).
	TierStandard CostTier = "standard"
	// TierEconomy uses sonnet/haiku for patrol roles, keeps opus for workers.
	TierEconomy CostTier = "economy"
	// TierBudget uses haiku/sonnet for patrols, sonnet for workers.
	TierBudget CostTier = "budget"
)

// ValidCostTiers returns all valid tier names.
func ValidCostTiers() []string {
	return []string{string(TierStandard), string(TierEconomy), string(TierBudget)}
}

// IsValidTier checks if a string is a valid cost tier name.
func IsValidTier(tier string) bool {
	switch CostTier(tier) {
	case TierStandard, TierEconomy, TierBudget:
		return true
	default:
		return false
	}
}

// CostTierRoleAgents returns the role_agents mapping for a given tier.
// Standard tier returns an empty map (all roles use default/opus).
// Returns nil if the tier is invalid.
func CostTierRoleAgents(tier CostTier) map[string]string {
	switch tier {
	case TierStandard:
		return map[string]string{}
	case TierEconomy:
		return map[string]string{
			"mayor":    "claude-sonnet",
			"deacon":   "claude-haiku",
			"witness":  "claude-sonnet",
			"refinery": "claude-sonnet",
			// polecat and crew omitted → use default (opus)
		}
	case TierBudget:
		return map[string]string{
			"mayor":    "claude-sonnet",
			"deacon":   "claude-haiku",
			"witness":  "claude-haiku",
			"refinery": "claude-haiku",
			"polecat":  "claude-sonnet",
			"crew":     "claude-sonnet",
		}
	default:
		return nil
	}
}

// CostTierAgents returns the custom agent definitions needed for a given tier.
// These define the claude-sonnet and claude-haiku agent presets.
// Standard tier returns an empty map (no custom agents needed).
func CostTierAgents(tier CostTier) map[string]*RuntimeConfig {
	switch tier {
	case TierStandard:
		return map[string]*RuntimeConfig{}
	case TierEconomy, TierBudget:
		return map[string]*RuntimeConfig{
			"claude-sonnet": claudeSonnetPreset(),
			"claude-haiku":  claudeHaikuPreset(),
		}
	default:
		return nil
	}
}

// claudeSonnetPreset returns a RuntimeConfig for Claude Sonnet.
func claudeSonnetPreset() *RuntimeConfig {
	return &RuntimeConfig{
		Command: "claude",
		Args:    []string{"--dangerously-skip-permissions", "--model", "sonnet"},
	}
}

// claudeHaikuPreset returns a RuntimeConfig for Claude Haiku.
func claudeHaikuPreset() *RuntimeConfig {
	return &RuntimeConfig{
		Command: "claude",
		Args:    []string{"--dangerously-skip-permissions", "--model", "haiku"},
	}
}

// ApplyCostTier writes the tier's agent and role_agents configuration to town settings.
// For standard tier, it clears role_agents and tier-specific agents.
func ApplyCostTier(settings *TownSettings, tier CostTier) error {
	roleAgents := CostTierRoleAgents(tier)
	if roleAgents == nil {
		return fmt.Errorf("invalid cost tier: %q (valid: %s)", tier, strings.Join(ValidCostTiers(), ", "))
	}

	agents := CostTierAgents(tier)

	// Set role agents
	settings.RoleAgents = roleAgents

	// Ensure agents map exists
	if settings.Agents == nil {
		settings.Agents = make(map[string]*RuntimeConfig)
	}

	// For standard tier, remove tier-specific agent presets if they exist
	if tier == TierStandard {
		delete(settings.Agents, "claude-sonnet")
		delete(settings.Agents, "claude-haiku")
	} else {
		// Add/update tier-specific agent presets
		for name, rc := range agents {
			settings.Agents[name] = rc
		}
	}

	// Track the tier for display purposes
	settings.CostTier = string(tier)

	return nil
}

// GetCurrentTier infers the current cost tier from the settings' RoleAgents.
// Returns the tier name if it matches a known tier exactly, or empty string for custom configs.
func GetCurrentTier(settings *TownSettings) string {
	// Check informational field first for quick path
	if settings.CostTier != "" && IsValidTier(settings.CostTier) {
		// Verify it still matches the actual config
		expected := CostTierRoleAgents(CostTier(settings.CostTier))
		if roleAgentsMatch(settings.RoleAgents, expected) {
			return settings.CostTier
		}
	}

	// Infer from RoleAgents by checking each tier
	for _, tierName := range ValidCostTiers() {
		tier := CostTier(tierName)
		expected := CostTierRoleAgents(tier)
		if roleAgentsMatch(settings.RoleAgents, expected) {
			return tierName
		}
	}

	return "" // Custom configuration
}

// roleAgentsMatch checks if two role_agents maps are equivalent.
// nil and empty maps are treated as equal.
func roleAgentsMatch(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TierDescription returns a human-readable description of the tier's model assignments.
func TierDescription(tier CostTier) string {
	switch tier {
	case TierStandard:
		return "All roles use Opus (highest quality)"
	case TierEconomy:
		return "Patrol roles use Sonnet/Haiku, workers use Opus"
	case TierBudget:
		return "Patrol roles use Haiku, workers use Sonnet"
	default:
		return "Unknown tier"
	}
}

// FormatTierRoleTable returns a formatted string showing role→model assignments for a tier.
func FormatTierRoleTable(tier CostTier) string {
	roleAgents := CostTierRoleAgents(tier)
	if roleAgents == nil {
		return ""
	}

	roles := []string{"mayor", "deacon", "witness", "refinery", "polecat", "crew"}
	var lines []string
	for _, role := range roles {
		agent := roleAgents[role]
		if agent == "" {
			agent = "(default/opus)"
		}
		lines = append(lines, fmt.Sprintf("  %-10s %s", role+":", agent))
	}
	return strings.Join(lines, "\n")
}
