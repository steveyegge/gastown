package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var tapGuardMolPatrolCmd = &cobra.Command{
	Use:   "mol-patrol",
	Short: "Block mol patrol in agent contexts",
	Long: `Block mol patrol operations that could disrupt running agents.

'gt mol patrol' terminates stale molecules. Running it from within an
agent session risks killing sibling agents or even the caller's own
molecule.

This guard blocks:
  - gt mol patrol (when called from within a Gas Town agent)

Exit codes:
  0 - Operation allowed (not in Gas Town agent context, or Mayor)
  2 - Operation BLOCKED (in agent context)

Mol patrol should only be run by the Mayor or by humans from outside
the Gas Town agent tree.`,
	RunE: runTapGuardMolPatrol,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardMolPatrolCmd)
}

func runTapGuardMolPatrol(cmd *cobra.Command, args []string) error {
	if !isGasTownAgentContext() {
		return nil
	}

	// Allow Mayor to run patrol (it coordinates agents)
	if os.Getenv("GT_MAYOR") != "" {
		return nil
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(os.Stderr, "║  ❌ MOL PATROL BLOCKED                                           ║")
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintln(os.Stderr, "║  Running 'gt mol patrol' from an agent can kill sibling agents  ║")
	fmt.Fprintln(os.Stderr, "║  or even your own molecule.                                     ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Only the Mayor or human operators should run mol patrol.       ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  If you need to check molecule status, use:                     ║")
	fmt.Fprintln(os.Stderr, "║    gt mol status    (safe, read-only)                           ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")
	return NewSilentExit(2)
}
