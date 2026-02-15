package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
)

var doctorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available doctor checks",
	Long: `Show all available doctor checks grouped by category.

Each check shows its name, description, and whether it supports auto-fix (🔧).
Rig checks require the --rig flag and are shown in a separate section unless
--rig is provided.`,
	RunE: runDoctorList,
}

func init() {
	doctorCmd.AddCommand(doctorListCmd)
}

// allDoctorChecks returns all registered checks (base + rig).
// This mirrors the registration in runDoctor but always includes rig checks.
func allDoctorChecks() (base []doctor.Check, rig []doctor.Check) {
	d := doctor.NewDoctor()

	// Register workspace-level checks first (fundamental)
	d.RegisterAll(doctor.WorkspaceChecks()...)

	d.Register(doctor.NewGlobalStateCheck())

	// Register built-in checks
	d.Register(doctor.NewStaleBinaryCheck())
	d.Register(doctor.NewTownGitCheck())
	d.Register(doctor.NewTownRootBranchCheck())
	d.Register(doctor.NewPreCheckoutHookCheck())
	d.Register(doctor.NewDaemonCheck())
	d.Register(doctor.NewRepoFingerprintCheck())
	d.Register(doctor.NewBootHealthCheck())
	d.Register(doctor.NewBeadsDatabaseCheck())
	d.Register(doctor.NewCustomTypesCheck())
	d.Register(doctor.NewRoleLabelCheck())
	d.Register(doctor.NewFormulaCheck())
	d.Register(doctor.NewPrefixConflictCheck())
	d.Register(doctor.NewRigNameMismatchCheck())
	d.Register(doctor.NewPrefixMismatchCheck())
	d.Register(doctor.NewDatabasePrefixCheck())
	d.Register(doctor.NewRoutesCheck())
	d.Register(doctor.NewRigRoutesJSONLCheck())
	d.Register(doctor.NewRoutingModeCheck())
	d.Register(doctor.NewOrphanSessionCheck())
	d.Register(doctor.NewZombieSessionCheck())
	d.Register(doctor.NewOrphanProcessCheck())
	d.Register(doctor.NewWispGCCheck())
	d.Register(doctor.NewCheckMisclassifiedWisps())
	d.Register(doctor.NewStaleBeadsRedirectCheck())
	d.Register(doctor.NewBranchCheck())
	d.Register(doctor.NewBeadsSyncOrphanCheck())
	d.Register(doctor.NewBeadsSyncWorktreeCheck())
	d.Register(doctor.NewCloneDivergenceCheck())
	d.Register(doctor.NewDefaultBranchAllRigsCheck())
	d.Register(doctor.NewIdentityCollisionCheck())
	d.Register(doctor.NewLinkedPaneCheck())
	d.Register(doctor.NewThemeCheck())
	d.Register(doctor.NewCrashReportCheck())
	d.Register(doctor.NewEnvVarsCheck())

	// Patrol system checks
	d.Register(doctor.NewPatrolMoleculesExistCheck())
	d.Register(doctor.NewPatrolHooksWiredCheck())
	d.Register(doctor.NewPatrolNotStuckCheck())
	d.Register(doctor.NewPatrolPluginsAccessibleCheck())
	d.Register(doctor.NewPatrolRolesHavePromptsCheck())
	d.Register(doctor.NewAgentBeadsCheck())
	d.Register(doctor.NewStaleAgentBeadsCheck())
	d.Register(doctor.NewRigBeadsCheck())
	d.Register(doctor.NewRoleBeadsCheck())

	// Config architecture checks
	d.Register(doctor.NewSettingsCheck())
	d.Register(doctor.NewSessionHookCheck())
	d.Register(doctor.NewRuntimeGitignoreCheck())
	d.Register(doctor.NewLegacyGastownCheck())
	d.Register(doctor.NewClaudeSettingsCheck())
	d.Register(doctor.NewDeprecatedMergeQueueKeysCheck())
	d.Register(doctor.NewLandWorktreeGitignoreCheck())
	d.Register(doctor.NewHooksPathAllRigsCheck())

	// Sparse checkout migration
	d.Register(doctor.NewSparseCheckoutCheck())

	// Priming subsystem check
	d.Register(doctor.NewPrimingCheck())

	// Crew workspace checks
	d.Register(doctor.NewCrewStateCheck())
	d.Register(doctor.NewCrewWorktreeCheck())
	d.Register(doctor.NewCommandsCheck())

	// Lifecycle hygiene checks
	d.Register(doctor.NewLifecycleHygieneCheck())

	// Hook attachment checks
	d.Register(doctor.NewHookAttachmentValidCheck())
	d.Register(doctor.NewHookSingletonCheck())
	d.Register(doctor.NewOrphanedAttachmentsCheck())
	d.Register(doctor.NewDoltHooksCheck())

	// Hooks sync check
	d.Register(doctor.NewHooksSyncCheck())

	// Dolt health checks
	d.Register(doctor.NewDoltMetadataCheck())
	d.Register(doctor.NewDoltServerReachableCheck())

	base = d.Checks()
	rig = doctor.RigChecks()
	return base, rig
}

func runDoctorList(cmd *cobra.Command, args []string) error {
	w := os.Stdout
	baseChecks, rigChecks := allDoctorChecks()

	// If --rig is provided, include rig checks inline with other categories
	includeRigInline := doctorRig != ""

	// Group base checks by category
	checksByCategory := make(map[string][]doctor.Check)
	for _, c := range baseChecks {
		cat := c.Category()
		if cat == "" {
			cat = "Other"
		}
		checksByCategory[cat] = append(checksByCategory[cat], c)
	}

	// If --rig, add rig checks into their category (CategoryRig)
	if includeRigInline {
		for _, c := range rigChecks {
			cat := c.Category()
			if cat == "" {
				cat = "Other"
			}
			checksByCategory[cat] = append(checksByCategory[cat], c)
		}
	}

	// Print checks grouped by category in defined order
	fmt.Fprintln(w)
	for _, category := range doctor.CategoryOrder {
		checks, exists := checksByCategory[category]
		if !exists || len(checks) == 0 {
			continue
		}

		// Skip rig category when not inline (handled separately below)
		if category == doctor.CategoryRig && !includeRigInline {
			continue
		}

		printCategoryChecks(w, category, len(checks), checks)
	}

	// Print "Other" category if present
	if others, exists := checksByCategory["Other"]; exists && len(others) > 0 {
		printCategoryChecks(w, "Other", len(others), others)
	}

	// If --rig not provided, show rig checks in a separate section
	if !includeRigInline && len(rigChecks) > 0 {
		fmt.Fprintf(w, "%s (requires --rig)\n", doctor.CategoryRig)
		for _, c := range rigChecks {
			fixIcon := "  "
			if c.CanFix() {
				fixIcon = "🔧"
			}
			fmt.Fprintf(w, "  %-25s %s %s\n", c.Name(), fixIcon, c.Description())
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Checks marked 🔧 can be fixed automatically with gt doctor <category> <check> --fix")

	return nil
}

func printCategoryChecks(w *os.File, category string, count int, checks []doctor.Check) {
	fmt.Fprintf(w, "%s (%d checks)\n", category, count)
	for _, c := range checks {
		fixIcon := "  "
		if c.CanFix() {
			fixIcon = "🔧"
		}
		fmt.Fprintf(w, "  %-25s %s %s\n", c.Name(), fixIcon, c.Description())
	}
	fmt.Fprintln(w)
}
