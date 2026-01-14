package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	diagnoseFix     bool
	diagnoseVerbose bool
	diagnoseRig     string
)

var diagnoseCmd = &cobra.Command{
	Use:     "diagnose",
	GroupID: GroupDiag,
	Short:   "Run focused diagnostics on key system components",
	Long: `Run focused diagnostic checks on key Gas Town components.

This is a lighter-weight alternative to 'gt doctor' that focuses on
common issues that affect day-to-day operation:

Runtime checks:
  - runtime-state      Verify .runtime/*.json files are valid
  - namepool-health    Detect stale namepool entries
  - sync-parity        Check if clones are in sync with remote

With --rig flag:
  - sparse-checkout    Verify sparse checkout configuration
  - rig-structure      Verify rig directory structure
  - crew-state         Validate crew worker state

Use --fix to attempt automatic fixes:
  - Reset corrupted runtime state files
  - Prune stale namepool entries

For comprehensive diagnostics, use 'gt doctor' instead.

Examples:
  gt diagnose              # Quick health check
  gt diagnose --fix        # Auto-fix common issues
  gt diagnose --rig pai    # Check specific rig
  gt diagnose -v           # Verbose output`,
	RunE: runDiagnose,
}

func init() {
	diagnoseCmd.Flags().BoolVar(&diagnoseFix, "fix", false, "Attempt to automatically fix issues")
	diagnoseCmd.Flags().BoolVarP(&diagnoseVerbose, "verbose", "v", false, "Show detailed output")
	diagnoseCmd.Flags().StringVar(&diagnoseRig, "rig", "", "Check specific rig")
	rootCmd.AddCommand(diagnoseCmd)
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create check context
	ctx := &doctor.CheckContext{
		TownRoot: townRoot,
		RigName:  diagnoseRig,
		Verbose:  diagnoseVerbose,
	}

	// Create doctor and register focused checks
	d := doctor.NewDoctor()

	// Always run these core checks
	d.Register(doctor.NewRuntimeStateCheck())
	d.Register(doctor.NewNamepoolHealthCheck())
	d.Register(doctor.NewCloneDivergenceCheck())
	d.Register(doctor.NewDaemonCheck())

	// Rig-specific checks (lighter than full rig checks)
	if diagnoseRig != "" {
		d.Register(doctor.NewSparseCheckoutCheck())
		d.Register(doctor.NewCrewStateCheck())
	}

	// Run checks
	var report *doctor.Report
	if diagnoseFix {
		report = d.Fix(ctx)
	} else {
		report = d.Run(ctx)
	}

	// Print report
	report.Print(os.Stdout, diagnoseVerbose)

	// Exit with error code if there are errors
	if report.HasErrors() {
		return fmt.Errorf("diagnose found %d error(s)", report.Summary.Errors)
	}

	return nil
}
