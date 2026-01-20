package cmd

import (
	"github.com/spf13/cobra"
)

var tapCmd = &cobra.Command{
	Use:   "tap",
	Short: "Claude Code hook handlers",
	Long: `Hook handlers for Claude Code PreToolUse and PostToolUse events.

These commands are called by Claude Code hooks to implement policies,
auditing, and input transformation. They tap into the tool execution
flow to guard, audit, inject, or check.

Subcommands:
  guard   - Block forbidden operations (PreToolUse, exit 2)
  audit   - Log/record tool executions (PostToolUse) [planned]
  inject  - Modify tool inputs (PreToolUse, updatedInput) [planned]
  check   - Validate after execution (PostToolUse) [planned]

Hook configuration in .claude/settings.json:
  {
    "PreToolUse": [{
      "matcher": "Bash(gh pr create*)",
      "hooks": [{"command": "gt tap guard pr-workflow"}]
    }]
  }

See ~/gt/docs/HOOKS.md for full documentation.`,
}

func init() {
	rootCmd.AddCommand(tapCmd)
}
