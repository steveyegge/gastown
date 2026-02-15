package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doctor"
	"github.com/steveyegge/gastown/internal/ui"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	doctorFix             bool
	doctorVerbose         bool
	doctorRig             string
	doctorRestartSessions bool
	doctorSlow            string
	doctorDryRun          bool
)

var doctorCmd = &cobra.Command{
	Use:     "doctor [check-name | category]...",
	GroupID: GroupDiag,
	Short:   "Run health checks on the workspace",
	Args:              cobra.ArbitraryArgs,
	RunE:              runDoctor,
	ValidArgsFunction: completeDoctorArgs,
}

func init() {
	doctorCmd.Long = buildDoctorLong()
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Attempt to automatically fix issues")
	doctorCmd.Flags().BoolVarP(&doctorVerbose, "verbose", "v", false, "Show detailed output")
	doctorCmd.PersistentFlags().StringVar(&doctorRig, "rig", "", "Check specific rig only")
	doctorCmd.Flags().BoolVar(&doctorRestartSessions, "restart-sessions", false, "Restart patrol sessions when fixing stale settings (use with --fix)")
	doctorCmd.Flags().StringVar(&doctorSlow, "slow", "", "Highlight slow checks (optional threshold, default 1s)")
	// Allow --slow without a value (uses default 1s)
	doctorCmd.Flags().Lookup("slow").NoOptDefVal = "1s"
	doctorCmd.Flags().BoolVar(&doctorDryRun, "dry-run", false, "Preview fixes without applying (use with --fix)")
	rootCmd.AddCommand(doctorCmd)
}

// buildDoctorLong generates the Long help text from registered checks,
// so it stays in sync as checks are added, removed, or recategorized.
func buildDoctorLong() string {
	baseChecks, rigChecks := allDoctorChecks()

	// Group by category
	byCategory := make(map[string][]doctor.Check)
	for _, c := range baseChecks {
		cat := c.Category()
		if cat == "" {
			cat = "Other"
		}
		byCategory[cat] = append(byCategory[cat], c)
	}

	var b strings.Builder
	b.WriteString("Run diagnostic checks on the Gas Town workspace.\n\n")
	b.WriteString("Run all checks (default), specific checks by name, or all checks in a category.\n")

	// Print checks grouped by category
	for _, category := range doctor.CategoryOrder {
		checks, exists := byCategory[category]
		if !exists || len(checks) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", category)
		for _, c := range checks {
			fix := "  "
			if c.CanFix() {
				fix = "🔧"
			}
			fmt.Fprintf(&b, "  %-27s %s %s\n", c.Name(), fix, c.Description())
		}
	}

	// Rig checks (separate section)
	if len(rigChecks) > 0 {
		fmt.Fprintf(&b, "\nRig (requires --rig):\n")
		for _, c := range rigChecks {
			fix := "  "
			if c.CanFix() {
				fix = "🔧"
			}
			fmt.Fprintf(&b, "  %-27s %s %s\n", c.Name(), fix, c.Description())
		}
	}

	b.WriteString("\nChecks marked 🔧 can be fixed automatically with gt doctor <check> --fix\n")
	b.WriteString("\nExamples:\n")
	b.WriteString("  gt doctor                                  # Run all checks\n")
	b.WriteString("  gt doctor orphan-sessions                  # Run one check\n")
	b.WriteString("  gt doctor orphan-sessions wisp-gc          # Run multiple checks\n")
	b.WriteString("  gt doctor cleanup                          # Run all Cleanup checks\n")
	b.WriteString("  gt doctor orphan-sessions --fix            # Run and fix one check\n")
	b.WriteString("  gt doctor orphan-sessions --fix --dry-run  # Preview fixes\n")
	b.WriteString("  gt doctor list                             # Show available checks")

	return b.String()
}

// completeDoctorArgs provides tab completion for check names and category names.
func completeDoctorArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	baseChecks, rigChecks := allDoctorChecks()
	var completions []string
	seen := make(map[string]bool)

	for _, c := range baseChecks {
		if !seen[c.Name()] {
			completions = append(completions, c.Name())
			seen[c.Name()] = true
		}
	}
	if doctorRig != "" {
		for _, c := range rigChecks {
			if !seen[c.Name()] {
				completions = append(completions, c.Name())
				seen[c.Name()] = true
			}
		}
	}

	for _, cat := range doctor.CategoryOrder {
		lower := strings.ToLower(cat)
		if !seen[lower] {
			completions = append(completions, lower)
			seen[lower] = true
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
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
	// All database queries go through bd CLI
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

	// NOTE: StaleAttachmentsCheck removed - staleness detection belongs in Deacon molecule

	// Config architecture checks
	d.Register(doctor.NewSettingsCheck())
	d.Register(doctor.NewSessionHookCheck())
	d.Register(doctor.NewRuntimeGitignoreCheck())
	d.Register(doctor.NewLegacyGastownCheck())
	d.Register(doctor.NewClaudeSettingsCheck())
	d.Register(doctor.NewDeprecatedMergeQueueKeysCheck())
	d.Register(doctor.NewLandWorktreeGitignoreCheck())
	d.Register(doctor.NewHooksPathAllRigsCheck())

	// Sparse checkout migration (runs across all rigs, not just --rig mode)
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
	d.Register(doctor.NewDoltOrphanedDatabaseCheck())

	// Worktree gitdir validity (runs across all rigs, or specific rig with --rig)
	d.Register(doctor.NewWorktreeGitdirCheck())

	// Rig-specific checks (only when --rig is specified)
	if doctorRig != "" {
		d.RegisterAll(doctor.RigChecks()...)
	}

	allChecks := d.Checks()

	// Handle reserved word "all"
	for _, arg := range args {
		if strings.EqualFold(arg, "all") {
			return fmt.Errorf("unknown check %q. Run \"gt doctor\" with no arguments to run all checks", "all")
		}
	}

	// Filter checks if args provided
	isTargeted := len(args) > 0
	var checks []doctor.Check
	var categoryName string

	if isTargeted {
		result := doctor.FilterChecks(allChecks, args)

		if len(result.Unmatched) > 0 {
			return formatUnmatchedError(allChecks, result.Unmatched)
		}

		checks = result.Matched

		// Detect category mode: single arg that matched multiple checks
		if len(args) == 1 && len(checks) > 1 {
			for _, cat := range doctor.CategoryOrder {
				if strings.EqualFold(cat, args[0]) {
					categoryName = cat
					break
				}
			}
		}
	} else {
		checks = allChecks
	}

	isSingleCheck := isTargeted && len(checks) == 1

	// Create filtered doctor with only selected checks
	filtered := doctor.NewDoctor()
	filtered.RegisterAll(checks...)

	// Parse slow threshold (0 = disabled)
	var slowThreshold time.Duration
	if doctorSlow != "" {
		slowThreshold, err = time.ParseDuration(doctorSlow)
		if err != nil {
			return fmt.Errorf("invalid --slow duration %q: %w", doctorSlow, err)
		}
	}

	// Detect TTY for output formatting
	isTTY := ui.IsTerminal()

	// Print header
	fmt.Println()
	if isTargeted {
		header := "Running: "
		if isSingleCheck {
			header += checks[0].Name()
		} else if categoryName != "" {
			header += fmt.Sprintf("%s (%d checks)", categoryName, len(checks))
		} else {
			header += fmt.Sprintf("%d checks", len(checks))
		}
		if doctorFix && doctorDryRun {
			header += " (dry run)"
		}
		fmt.Println(header)
	}

	// Execute checks
	var report *doctor.Report
	if doctorFix && !doctorDryRun {
		report = filtered.FixStreaming(ctx, os.Stdout, slowThreshold, isTTY)
	} else {
		// Normal run and dry-run both use RunStreaming (dry-run skips Fix)
		report = filtered.RunStreaming(ctx, os.Stdout, slowThreshold, isTTY)
	}

	// Dry-run: show "Would fix:" hints for fixable checks with issues
	if doctorFix && doctorDryRun {
		printDryRunHints(report, checks, isTTY)
	}

	// Single check + --fix on non-fixable: add hint
	if isSingleCheck && doctorFix && !doctorDryRun && !checks[0].CanFix() {
		for _, r := range report.Checks {
			if r.Status != doctor.StatusOK {
				if isTTY {
					fmt.Fprintf(os.Stdout, "     %s%s\n",
						ui.MutedStyle.Render(ui.TreeLast),
						ui.RenderMuted("This check does not support auto-fix."))
				} else {
					fmt.Fprintln(os.Stdout, "     This check does not support auto-fix.")
				}
			}
		}
	}

	// Print summary (skip for single check runs)
	if !isSingleCheck {
		report.PrintSummaryOnly(os.Stdout, doctorVerbose, slowThreshold)
	}

	// Dry-run always exits 0 (no action taken)
	if doctorFix && doctorDryRun {
		return nil
	}

	// Exit with error code if there are errors
	if report.HasErrors() {
		return fmt.Errorf("doctor found %d error(s)", report.Summary.Errors)
	}

	return nil
}

// formatUnmatchedError builds an error message for unknown check names with suggestions.
func formatUnmatchedError(allChecks []doctor.Check, unmatched []string) error {
	var b strings.Builder

	if len(unmatched) == 1 {
		name := unmatched[0]
		fmt.Fprintf(&b, "unknown check %q", name)

		suggestions := doctor.SuggestCheck(allChecks, name)
		if len(suggestions) == 1 {
			fmt.Fprintf(&b, "\n\n  Did you mean: %s?", suggestions[0])
		} else if len(suggestions) > 1 {
			fmt.Fprintf(&b, "\n\n  Did you mean one of: %s?", strings.Join(suggestions, ", "))
		}
	} else {
		quoted := make([]string, len(unmatched))
		for i, name := range unmatched {
			quoted[i] = fmt.Sprintf("%q", name)
		}
		fmt.Fprintf(&b, "unknown checks %s", strings.Join(quoted, ", "))
	}

	b.WriteString("\n\n  Run \"gt doctor list\" to see all available checks.")
	if doctorRig == "" {
		b.WriteString("\n  Some checks require --rig. Run \"gt doctor list --rig <name>\".")
	}

	return fmt.Errorf("%s", b.String())
}

// printDryRunHints shows "Would fix:" lines for fixable checks that have issues.
func printDryRunHints(report *doctor.Report, checks []doctor.Check, isTTY bool) {
	fixable := make(map[string]bool)
	for _, c := range checks {
		if c.CanFix() {
			fixable[c.Name()] = true
		}
	}

	for _, result := range report.Checks {
		if result.Status != doctor.StatusOK && fixable[result.Name] {
			hint := fmt.Sprintf("Would fix: %s", result.Name)
			if result.Message != "" {
				hint = fmt.Sprintf("Would fix: %s", result.Message)
			}
			if isTTY {
				fmt.Fprintf(os.Stdout, "     %s%s\n",
					ui.MutedStyle.Render(ui.TreeLast),
					ui.RenderMuted(hint))
			} else {
				fmt.Fprintf(os.Stdout, "     %s\n", hint)
			}
		}
	}
}
