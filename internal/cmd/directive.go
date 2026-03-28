package cmd

import (
	"github.com/spf13/cobra"
)

var directiveCmd = &cobra.Command{
	Use:     "directive",
	Aliases: []string{"directives"},
	GroupID: GroupConfig,
	Short:   "Manage role directives",
	Long: `Manage operator-provided role directives.

Directives are markdown files that customize agent behavior per role.
They are injected at prime time and override formula defaults where
they conflict.

Subcommands:
  show    Display the active directive for a role
  edit    Open a directive in $EDITOR (creates if needed)
  list    List all directive files

File layout:
  Town-level: <townRoot>/directives/<role>.md
  Rig-level:  <townRoot>/<rig>/directives/<role>.md

Resolution: Town and rig directives are concatenated (town first, rig last).
Rig-level content gets the last word.

Examples:
  gt directive show polecat             # Show active polecat directive
  gt directive show witness --rig sky   # Show witness directive for sky rig
  gt directive edit crew                # Edit crew directive (rig-level)
  gt directive list                     # List all directive files`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(directiveCmd)
}
