package config

// DefaultAgentTierConfig returns a default 4-tier agent configuration.
//
// Tiers (lowest to highest capability):
//   - small:     Lightweight tasks — haiku-class agents
//   - medium:    Standard feature work — sonnet-class agents
//   - large:     Cross-cutting work — opus-class agents
//   - reasoning: Deep analysis and hard algorithms — reasoning-class agents
//
// All tiers use "priority" selection by default.
// The "reasoning" tier has Fallback=false — it is the highest tier and cannot
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
				Agents:      []string{"claude-haiku"},
				Selection:   "priority",
				Fallback:    true,
			},
			"medium": {
				Description: "Standard feature work, multi-file changes, bug fixes, merge queue processing",
				Agents:      []string{"claude-sonnet"},
				Selection:   "priority",
				Fallback:    true,
			},
			"large": {
				Description: "Cross-cutting refactors, new subsystem integration, strategic coordination",
				Agents:      []string{"claude-opus"},
				Selection:   "priority",
				Fallback:    true,
			},
			"reasoning": {
				Description: "Deep debugging, architecture decisions, tricky algorithms, security analysis",
				Agents:      []string{"claude-reasoning"},
				Selection:   "priority",
				Fallback:    false,
			},
		},
		TierOrder: []string{"small", "medium", "large", "reasoning"},
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
