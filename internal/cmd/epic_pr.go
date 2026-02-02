package cmd

import (
	"github.com/spf13/cobra"
)

var epicPRCmd = &cobra.Command{
	Use:   "pr",
	Short: "Manage upstream PRs for an epic",
	Long: `Manage the lifecycle of upstream PRs created for an epic.

SUBCOMMANDS:

  status    Show status of all upstream PRs
  check     Check for conflicts, CI failures
  sync      Rebase branches on upstream
  respond   Address review feedback on a PR
  resolve   Manual conflict resolution workflow

WORKFLOW:

After submitting PRs with 'gt epic submit', use these commands to:
1. Monitor PR status and reviews
2. Keep branches up-to-date with upstream
3. Address review feedback
4. Resolve conflicts when they arise

EXAMPLES:

  gt epic pr status gt-epic-abc12       # Show all PR status
  gt epic pr check gt-epic-abc12        # Check for issues
  gt epic pr sync gt-epic-abc12         # Rebase on upstream
  gt epic pr respond 102                # Address review on PR #102`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return requireSubcommand(cmd, args)
	},
}

func init() {
	epicPRCmd.AddCommand(epicPRStatusCmd)
	epicPRCmd.AddCommand(epicPRCheckCmd)
	epicPRCmd.AddCommand(epicPRSyncCmd)
	epicPRCmd.AddCommand(epicPRRespondCmd)
	epicPRCmd.AddCommand(epicPRResolveCmd)
}
