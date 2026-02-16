package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(signalCmd)
	signalCmd.AddCommand(signalStopCmd)
}

var signalCmd = &cobra.Command{
	Use:     "signal",
	GroupID: GroupAgents,
	Short:   "Claude Code hook signal handlers",
	Long: `Signal handlers for Claude Code hooks.

These commands are designed to be called by Claude Code's hooks system,
not directly by users. They output JSON that Claude Code interprets.

Subcommands:
  stop   Called by the Stop hook at turn boundaries. Checks for queued
         work/messages and either blocks (injects work) or allows (agent
         goes idle).

Example hook configuration (.claude/settings.json):
  {
    "hooks": {
      "Stop": [{
        "hooks": [{
          "type": "command",
          "command": "gt signal stop"
        }]
      }]
    }
  }`,
}
