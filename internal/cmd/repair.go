package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
	"github.com/steveyegge/gastown/internal/workspace"
)

var repairCmd = &cobra.Command{
	Use:     "repair",
	GroupID: GroupDiag,
	Short:   "Repair bootstrap, routing, and configuration issues",
	Long: `Repair common database identity mismatches and configuration issues.

This is a focused version of 'gt doctor --fix' that targets the most common
failure mode: metadata.json pointing to the wrong Dolt database after a crash,
rig addition, or bd init conflict.

What it repairs:
  - metadata.json dolt_database pointing to wrong database
  - Missing config.json for registered rigs
  - Prefix mismatches between config.json and rigs.json
  - Missing Dolt databases
  - Missing rig identity beads
  - Stale Dolt port in metadata.json

For a full diagnostic, use 'gt doctor' instead.
For a full diagnostic with auto-fix, use 'gt doctor --fix'.`,
	RunE: runRepair,
}

var repairBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Repair the narrow HQ bootstrap contract",
	Long: `Repair the minimal HQ/bootstrap state needed for first-login control-plane bring-up.

This targets canonical town markers, town-level beads configuration, routing,
tmux town-root environment, and Dolt metadata needed for HQ control-plane
commands to function from the canonical town root.`,
	RunE: runRepairBootstrap,
}

func init() {
	rootCmd.AddCommand(repairCmd)
	repairCmd.AddCommand(repairBootstrapCmd)
}

func runRepairChecks(townRoot, rigName, title string, checks ...doctor.Check) error {
	ctx := &doctor.CheckContext{
		TownRoot: townRoot,
		RigName:  rigName,
		Verbose:  true,
	}

	d := doctor.NewDoctor()
	d.RegisterAll(checks...)

	if title != "" {
		fmt.Println(title)
		fmt.Println()
	}

	report := d.FixStreaming(ctx, os.Stdout, 0)
	if report.HasErrors() {
		return fmt.Errorf("repair left %d blocking issue(s)", report.Summary.Errors)
	}
	return nil
}

func runRepair(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	return runRepairChecks(
		townRoot,
		"",
		"Repairing rig identity and convergence...",
		doctor.NewRigConfigSyncCheck(),
		doctor.NewRoutesCheck(),
		doctor.NewDatabasePrefixCheck(),
		doctor.NewStaleBeadsRedirectCheck(),
		doctor.NewRigBeadsCheck(),
		doctor.NewAgentBeadsCheck(),
		doctor.NewStaleDoltPortCheck(),
	)
}

func runRepairBootstrap(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	return runRepairChecks(
		townRoot,
		"",
		"Repairing HQ bootstrap state...",
		doctor.NewTownConfigExistsCheck(),
		doctor.NewTownConfigValidCheck(),
		doctor.NewRigsRegistryExistsCheck(),
		doctor.NewRigsRegistryValidCheck(),
		doctor.NewMayorExistsCheck(),
		doctor.NewTownBeadsConfigCheck(),
		doctor.NewRoutesCheck(),
		doctor.NewTmuxGlobalEnvCheck(),
		doctor.NewStaleDoltPortCheck(),
		doctor.NewDatabasePrefixCheck(),
	)
}
