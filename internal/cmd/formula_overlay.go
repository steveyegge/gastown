package cmd

import (
	"github.com/spf13/cobra"
)

var formulaOverlayCmd = &cobra.Command{
	Use:   "overlay",
	Short: "Manage formula overlays",
	Long: `Manage formula overlays — per-formula step overrides.

Overlays are TOML files that customize formula steps via replace, append,
or skip modes. They are applied at prime time when formula steps are displayed.

Subcommands:
  show    Display the active overlay for a formula
  edit    Open an overlay in $EDITOR (creates if needed)
  list    List all overlay files

File layout:
  Town-level: <townRoot>/formula-overlays/<formula>.toml
  Rig-level:  <townRoot>/<rig>/formula-overlays/<formula>.toml

Resolution: If a rig-level overlay exists, it takes full precedence
(town-level is not merged).

Examples:
  gt formula overlay show mol-polecat-work
  gt formula overlay edit mol-polecat-work --rig gastown
  gt formula overlay list`,
	RunE: requireSubcommand,
}

func init() {
	formulaCmd.AddCommand(formulaOverlayCmd)
}
