package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var agentTierCmd = &cobra.Command{
	Use:   "tier",
	Short: "Agent tier operations",
	Long:  `Agent tier operations.`,
	RunE:  requireSubcommand,
}

var agentTierListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent tiers with availability",
	Long: `List all agent tiers with current agent availability.

In Phase 1, availability is based purely on config (all configured agents
are shown as available). The exclusion cache integration comes in Phase 2.

Examples:
  gt agent tier list              # Show all tiers
  gt agent tier list --available  # Show only tiers with available agents`,
	RunE: runAgentTierList,
}

var agentTierListAvailable bool

func init() {
	agentTierListCmd.Flags().BoolVar(&agentTierListAvailable, "available", false, "Show only tiers with at least one available agent")
	agentTierCmd.AddCommand(agentTierListCmd)
}

func runAgentTierList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	tierCfg := townSettings.AgentTiers
	if tierCfg == nil {
		tierCfg = config.DefaultAgentTierConfig()
	}

	tierNames := tierCfg.TierNames()

	// Phase 1: all tiers are available (no exclusion cache yet).
	totalDefined := len(tierNames)
	totalAvailable := totalDefined

	if agentTierListAvailable {
		// Filter to tiers with at least one agent (all tiers in Phase 1).
		var filtered []string
		for _, name := range tierNames {
			tier := tierCfg.Tiers[name]
			if tier != nil && len(tier.Agents) > 0 {
				filtered = append(filtered, name)
			}
		}
		tierNames = filtered
	}

	fmt.Printf("Agent Tiers (%d defined, %d available)\n", totalDefined, totalAvailable)

	for _, name := range tierNames {
		tier := tierCfg.Tiers[name]
		if tier == nil {
			continue
		}

		fmt.Println()
		fmt.Printf("  %-10s %s\n", style.Bold.Render(name), style.Dim.Render(`"`+tier.Description+`"`))

		// Format agent list with availability markers.
		// Phase 1: all agents show ✓
		agentParts := make([]string, 0, len(tier.Agents))
		for _, agent := range tier.Agents {
			agentParts = append(agentParts, agent+" (✓)")
		}
		if len(agentParts) > 0 {
			fmt.Printf("             Agents: %s\n", strings.Join(agentParts, ", "))
		} else {
			fmt.Printf("             Agents: (none)\n")
		}

		sel := tier.Selection
		if sel == "" {
			sel = "priority"
		}
		fmt.Printf("             Selection: %s\n", sel)
	}

	return nil
}
