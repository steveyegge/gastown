package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	doctorFix             bool
	doctorVerbose         bool
	doctorRig             string
	doctorRestartSessions bool
)

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	GroupID: GroupDiag,
	Short:   "Run health checks on the workspace",
	Long: `Run diagnostic checks on the Gas Town workspace.

Doctor checks for common configuration issues, missing files,
and other problems that could affect workspace operation.

Workspace checks:
  - town-config-exists       Check mayor/town.json exists
  - town-config-valid        Check mayor/town.json is valid
  - rigs-registry-exists     Check mayor/rigs.json exists (fixable)
  - rigs-registry-valid      Check registered rigs exist (fixable)
  - mayor-exists             Check mayor/ directory structure

Town root protection:
  - town-git                 Verify town root is under version control
  - town-root-branch         Verify town root is on main branch (fixable)
  - pre-checkout-hook        Verify pre-checkout hook prevents branch switches (fixable)

Infrastructure checks:
  - stale-binary             Check if gt binary is up to date with repo
  - daemon                   Check if daemon is running (fixable)
  - repo-fingerprint         Check database has valid repo fingerprint (fixable)
  - boot-health              Check Boot watchdog health (vet mode)

Cleanup checks (fixable):
  - orphan-sessions          Detect orphaned tmux sessions
  - orphan-processes         Detect orphaned Claude processes
  - wisp-gc                  Detect and clean abandoned wisps (>1h)

Clone divergence checks:
  - persistent-role-branches Detect crew/witness/refinery not on main
  - clone-divergence         Detect clones significantly behind origin/main

Crew workspace checks:
  - crew-state               Validate crew worker state.json files (fixable)
  - crew-worktrees           Detect stale cross-rig worktrees (fixable)

Rig checks (with --rig flag):
  - rig-is-git-repo          Verify rig is a valid git repository
  - git-exclude-configured   Check .git/info/exclude has Gas Town dirs (fixable)
  - witness-exists           Verify witness/ structure exists (fixable)
  - refinery-exists          Verify refinery/ structure exists (fixable)
  - mayor-clone-exists       Verify mayor/rig/ clone exists (fixable)
  - polecat-clones-valid     Verify polecat directories are valid clones
  - beads-config-valid       Verify beads configuration (fixable)

Routing checks (fixable):
  - routes-config            Check beads routing configuration
  - prefix-mismatch          Detect rigs.json vs routes.jsonl prefix mismatches (fixable)

Session hook checks:
  - session-hooks            Check settings.json use session-start.sh
  - claude-settings          Check Claude settings.json match templates (fixable)

Patrol checks:
  - patrol-molecules-exist   Verify patrol molecules exist
  - patrol-hooks-wired       Verify daemon triggers patrols
  - patrol-not-stuck         Detect stale wisps (>1h)
  - patrol-plugins-accessible Verify plugin directories
  - patrol-roles-have-prompts Verify role prompts exist

Use --fix to attempt automatic fixes for issues that support it.
Use --rig to check a specific rig instead of the entire workspace.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Attempt to automatically fix issues")
	doctorCmd.Flags().BoolVarP(&doctorVerbose, "verbose", "v", false, "Show detailed output")
	doctorCmd.Flags().StringVar(&doctorRig, "rig", "", "Check specific rig only")
	doctorCmd.Flags().BoolVar(&doctorRestartSessions, "restart-sessions", false, "Restart patrol sessions when fixing stale settings (use with --fix)")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create check context
	ctx := &doctor.CheckContext{
		TownRoot:        townRoot,
		RigName:         doctorRig,
		Verbose:         doctorVerbose,
		RestartSessions: doctorRestartSessions,
	}

	// Create doctor and register checks
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
	d.Register(doctor.NewBdDaemonCheck())
	d.Register(doctor.NewPrefixConflictCheck())
	d.Register(doctor.NewPrefixMismatchCheck())
	d.Register(doctor.NewRoutesCheck())
	d.Register(doctor.NewRigRoutesJSONLCheck())
	d.Register(doctor.NewOrphanSessionCheck())
	d.Register(doctor.NewOrphanProcessCheck())
	d.Register(doctor.NewWispGCCheck())
	d.Register(doctor.NewBranchCheck())
	d.Register(doctor.NewBeadsSyncOrphanCheck())
	d.Register(doctor.NewCloneDivergenceCheck())
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
	d.Register(doctor.NewRigBeadsCheck())
	d.Register(doctor.NewRoleBeadsCheck())

	// NOTE: StaleAttachmentsCheck removed - staleness detection belongs in Deacon molecule

	// Config architecture checks
	d.Register(doctor.NewSettingsCheck())
	d.Register(doctor.NewSessionHookCheck())
	d.Register(doctor.NewRuntimeGitignoreCheck())
	d.Register(doctor.NewLegacyGastownCheck())
	d.Register(doctor.NewClaudeSettingsCheck())

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

	// Rig-specific checks (only when --rig is specified)
	if doctorRig != "" {
		d.RegisterAll(doctor.RigChecks()...)
	}

	// Run checks
	var report *doctor.Report
	if doctorFix {
		report = d.Fix(ctx)
	} else {
		report = d.Run(ctx)
	}

	// Print report
	report.Print(os.Stdout, doctorVerbose)

	// Exit with error code if there are errors
	if report.HasErrors() {
		return fmt.Errorf("doctor found %d error(s)", report.Summary.Errors)
	}

	return nil
}
