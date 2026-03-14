package cmd

import (
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:     "agent",
	GroupID: GroupAgents,
	Short:   "Agent runtime operations",
	Long: `Agent runtime operations.

Commands:
  gt agent tier list              List all agent tiers with availability
  gt agent tier list --available  List only tiers with at least one available agent`,
	RunE: requireSubcommand,
}

func init() {
	agentCmd.AddCommand(agentTierCmd)
	rootCmd.AddCommand(agentCmd)
}
