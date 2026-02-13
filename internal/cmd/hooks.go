package cmd

import (
	"github.com/spf13/cobra"
)

var hooksCmd = &cobra.Command{
	Use:     "hooks",
	GroupID: GroupConfig,
	Short:   "Centralized hook management for Gas Town",
	Long: `Manage Claude Code hooks across the Gas Town workspace.

Provides centralized hook configuration with a base config and
per-role/per-rig overrides. Changes are propagated to all workers
via the sync command.

Subcommands:
  base       Edit the shared base hook config
  override   Edit overrides for a role or rig
  sync       Regenerate all .claude/settings.local.json files
  diff       Show what sync would change
  list       Show all managed settings.local.json locations
  scan       Scan workspace for existing hooks
  registry   List hooks from the registry
  install    Install a hook from the registry

Config structure:
  Base:      ~/.gt/hooks-base.json
  Overrides: ~/.gt/hooks-overrides/<target>.json

Merge strategy: base → role → rig+role (more specific wins)

Examples:
  gt hooks sync           # Regenerate all settings.local.json files
  gt hooks diff           # Preview what sync would change
  gt hooks base           # Edit the shared base config
  gt hooks override crew  # Edit overrides for all crew workers
  gt hooks list           # Show managed locations and sync status`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(hooksCmd)
}
