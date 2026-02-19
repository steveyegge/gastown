package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(observeCmd)
}

var observeCmd = &cobra.Command{
	Use:     "observe",
	GroupID: GroupDiag,
	Short:   "Manage observability sources",
	Long: `Manage runtime observability sources (logs, metrics, traces).

Observability sources are configured in town settings and can tail
log files in real-time, applying redaction and severity filtering.

Use subcommands to list, add, remove, check status, or tail sources.`,
	RunE: requireSubcommand,
}
