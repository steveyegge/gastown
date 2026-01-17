package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:     "close [bead-id...]",
	GroupID: GroupWork,
	Short:   "Close one or more beads",
	Long: `Close one or more beads (wrapper for 'bd close').

This is a convenience command that passes through to 'bd close' with
all arguments and flags preserved.

Examples:
  gt close gt-abc              # Close bead gt-abc
  gt close gt-abc gt-def       # Close multiple beads
  gt close --reason "Done"     # Close with reason
  gt close --force             # Force close pinned beads`,
	DisableFlagParsing: true, // Pass all flags through to bd close
	RunE:               runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)
}

func runClose(cmd *cobra.Command, args []string) error {
	// Build bd close command with all args passed through
	bdArgs := append([]string{"close"}, args...)
	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdin = os.Stdin
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr
	return bdCmd.Run()
}
