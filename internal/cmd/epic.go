package cmd

import (
	"github.com/spf13/cobra"
)

var epicCmd = &cobra.Command{
	Use:     "epic",
	GroupID: GroupWork,
	Short:   "Manage epics for upstream contribution workflow",
	Long: `Manage epics - the primary unit for planning and tracking upstream contributions.

An epic represents a substantial feature or change that will be contributed upstream.
The epic workflow provides:
- Structured planning with CONTRIBUTING.md awareness
- Subtask decomposition from plans
- Integration branch management
- Dependency-aware stacked PR submission

WORKFLOW OVERVIEW:

  gt epic start <rig> "Title"    # Create epic, discover CONTRIBUTING.md, plan
  gt epic ready [id]             # Parse plan → subtasks, create integration branch
  gt epic sling [id]             # Dispatch subtasks to workers
  gt epic status [id]            # Show progress
  gt epic review [id]            # Review completed work
  gt epic submit [id]            # Create stacked upstream PRs

PR LIFECYCLE:

  gt epic pr status [id]         # Show upstream PR status
  gt epic pr check [id]          # Check for conflicts, CI failures
  gt epic pr sync [id]           # Rebase branches on upstream
  gt epic pr respond <pr>        # Address review feedback
  gt epic pr resolve <pr>        # Manual conflict resolution

EPIC STATES:

  drafting     → Planning phase, editing plan
  ready        → Plan finalized, subtasks created
  in_progress  → Subtasks being worked
  review       → All MRs merged to integration branch
  submitted    → Upstream PRs created
  landed       → All PRs merged upstream
  closed       → Abandoned or completed

PLAN FORMAT:

The epic plan uses the molecule step format:

  ## Overview
  Brief feature description

  ## Step: implement-api
  Implement the core API changes
  Tier: opus

  ## Step: add-tests
  Write comprehensive tests
  Needs: implement-api
  Tier: sonnet

Each step becomes a subtask bead. Dependencies (Needs:) are wired automatically.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return requireSubcommand(cmd, args)
	},
}

func init() {
	// Add subcommands
	epicCmd.AddCommand(epicStartCmd)
	epicCmd.AddCommand(epicPlanCmd)
	epicCmd.AddCommand(epicReadyCmd)
	epicCmd.AddCommand(epicSlingCmd)
	epicCmd.AddCommand(epicStatusCmd)
	epicCmd.AddCommand(epicReviewCmd)
	epicCmd.AddCommand(epicSubmitCmd)
	epicCmd.AddCommand(epicListCmd)
	epicCmd.AddCommand(epicCloseCmd)
	epicCmd.AddCommand(epicPRCmd)

	rootCmd.AddCommand(epicCmd)
}
