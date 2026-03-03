package cmd

import (
	"github.com/spf13/cobra"
)

var grafanaCmd = &cobra.Command{
	Use:     "grafana",
	GroupID: GroupServices,
	Short:   "Manage Grafana dashboards",
	RunE:    requireSubcommand,
	Long: `Manage Grafana dashboards for the Gas Town observability stack.

Subcommands:
  export    Export dashboards from Grafana API to provisioning JSON files`,
}

func init() {
	rootCmd.AddCommand(grafanaCmd)
}
