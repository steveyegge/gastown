package config

// DefaultAgentTierConfig returns a default 3-tier agent configuration.
//
// Tiers (lowest to highest capability):
//   - small:  Lightweight tasks — monitoring, health checks, routine dispatch
//   - medium: Standard feature work — bug fixes, multi-file changes, merge processing
//   - large:  Cross-cutting work — refactors, new subsystems, strategic coordination
//
// All tiers default to the built-in "claude" preset, which resolves to the
// platform's current flagship model. Operators can customize tiers to use
// different agents or model-specific presets (e.g., custom "claude-haiku"
// entries in settings/agents.json).
//
// All tiers use "priority" selection by default.
// The "large" tier has Fallback=false — it is the highest tier and cannot
// be escalated further.
//
// Default role mappings:
//   - mayor, crew → large
//   - polecat, refinery → medium
//   - witness, deacon, dogs → small
func DefaultAgentTierConfig() *AgentTierConfig {
	return &AgentTierConfig{
		Tiers: map[string]*AgentTier{
			"small": {
				Description: "Lightweight monitoring and patrol tasks: zombie detection, health checks, routine dispatch",
				Agents:      []string{"claude"},
				Selection:   "priority",
				Fallback:    true,
			},
			"medium": {
				Description: "Standard feature work, multi-file changes, bug fixes, merge queue processing",
				Agents:      []string{"claude"},
				Selection:   "priority",
				Fallback:    true,
			},
			"large": {
				Description: "Cross-cutting refactors, new subsystem integration, strategic coordination",
				Agents:      []string{"claude"},
				Selection:   "priority",
				Fallback:    false,
			},
		},
		TierOrder: []string{"small", "medium", "large"},
		RoleDefaults: map[string]string{
			"mayor":    "large",
			"crew":     "large",
			"polecat":  "medium",
			"refinery": "medium",
			"witness":  "small",
			"deacon":   "small",
			"dogs":     "small",
		},
	}
}
