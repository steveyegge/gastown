package cmd

import (
	"github.com/spf13/cobra"
)

var (
	circuitJSON bool
)

var circuitCmd = &cobra.Command{
	Use:     "circuit",
	GroupID: GroupDiag,
	Short:   "Pipeline circuit breaker (per-rig failure tracking)",
	Long: `Manage per-rig pipeline circuit breakers.

The circuit breaker tracks consecutive failures in witness and refinery stages.
When failures exceed the threshold (default: 3), the circuit OPENS:
  - gt sling refuses to dispatch to the rig
  - gt scheduler run skips the rig
  - An escalation is sent to mayor

States:
  CLOSED    Normal operation, dispatch allowed.
  OPEN      Failures exceeded threshold, dispatch blocked.
  HALF_OPEN After 5min timeout, one test dispatch is allowed.
            If it succeeds → CLOSED. If it fails → OPEN.`,
}

var circuitStatusCmd = &cobra.Command{
	Use:   "status <rig>",
	Short: "Show circuit breaker state for a rig",
	Args:  cobra.ExactArgs(1),
	RunE:  runCircuitStatus,
}

var circuitResetCmd = &cobra.Command{
	Use:   "reset <rig>",
	Short: "Force circuit breaker to CLOSED (mayor override)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCircuitReset,
}

func init() {
	circuitStatusCmd.Flags().BoolVar(&circuitJSON, "json", false, "Output as JSON")

	circuitCmd.AddCommand(circuitStatusCmd)
	circuitCmd.AddCommand(circuitResetCmd)

	rootCmd.AddCommand(circuitCmd)
}
