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
	Use:               "doctor [category] [check]",
	GroupID:           GroupDiag,
	Short:             "Run health checks on the workspace",
	Args:              cobra.MaximumNArgs(2),
	RunE:              runDoctor,
	ValidArgsFunction: completeDoctorArgs,
}

func init() {
	doctorCmd.Long = buildDoctorLong()
	doctorCmd.SetHelpFunc(doctorHelp)
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
	b.WriteString("Run all checks (default), all checks in a category, or a specific check.\n")

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

	b.WriteString("\nChecks marked 🔧 can be fixed automatically with gt doctor <category> <check> --fix\n")
	b.WriteString("\nExamples:\n")
	b.WriteString("  gt doctor                                    # Run all checks\n")
	b.WriteString("  gt doctor --fix                              # Run all + fix\n")
	b.WriteString("  gt doctor <category>                         # Run all checks in a category\n")
	b.WriteString("  gt doctor <category> <check>                 # Run one check\n")
	b.WriteString("  gt doctor <category> <check> --fix           # Fix one check\n")
	b.WriteString("  gt doctor <category> --fix --dry-run         # Preview fixes for category\n")
	b.WriteString("  gt doctor list                               # Show available checks")

	return b.String()
}

// doctorHelp is a custom help function that shows category-filtered help
// when a category name is passed as a positional arg (e.g. "gt doctor core -h").
func doctorHelp(cmd *cobra.Command, args []string) {
	// Cobra passes raw args; scan for a category name among them
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		category := doctor.ResolveCategory(arg)
		if category != "" {
			// Temporarily replace Long with category-specific text, let cobra render
			originalLong := cmd.Long
			cmd.Long = buildCategoryHelp(category)
			cmd.SetHelpFunc(nil)
			cmd.Help()
			cmd.Long = originalLong
			cmd.SetHelpFunc(doctorHelp)
			return
		}
	}

	// Fall back to default help
	cmd.SetHelpFunc(nil) // clear to avoid recursion
	cmd.Help()
	cmd.SetHelpFunc(doctorHelp) // restore
}

// buildCategoryHelp generates help text for a specific category.
func buildCategoryHelp(category string) string {
	baseChecks, rigChecks := allDoctorChecks()

	// Combine if rig is specified
	allChecks := baseChecks
	if doctorRig != "" {
		allChecks = append(allChecks, rigChecks...)
	}

	checks := doctor.ChecksInCategory(allChecks, category)

	var b strings.Builder
	fmt.Fprintf(&b, "Run all %s checks, or a specific check in this category.\n", category)
	fmt.Fprintf(&b, "\n%s (%d checks):\n", category, len(checks))
	for _, c := range checks {
		fix := "  "
		if c.CanFix() {
			fix = "🔧"
		}
		fmt.Fprintf(&b, "  %-27s %s %s\n", c.Name(), fix, c.Description())
	}

	fmt.Fprintf(&b, "\nChecks marked 🔧 can be fixed automatically with gt doctor %s <check> --fix\n", strings.ToLower(category))
	fmt.Fprintf(&b, "\nExamples:\n")
	fmt.Fprintf(&b, "  gt doctor %-37s # Run all %s checks\n", strings.ToLower(category), category)
	if len(checks) > 0 {
		example := checks[0].Name()
		fmt.Fprintf(&b, "  gt doctor %s %-*s # Run one check\n",
			strings.ToLower(category), 37-len(strings.ToLower(category))-1, example)
		fmt.Fprintf(&b, "  gt doctor %s %s --fix\n", strings.ToLower(category), example)
	}

	return b.String()
}

// completeDoctorArgs provides position-aware tab completion.
// First arg: category names. Second arg: check names within that category.
func completeDoctorArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// First positional arg: complete with category names
		var completions []string
		for _, cat := range doctor.CategoryOrder {
			completions = append(completions, strings.ToLower(cat))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp

	case 1:
		// Second positional arg: complete with check names in the selected category
		category := doctor.ResolveCategory(args[0])
		if category == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		baseChecks, rigChecks := allDoctorChecks()
		allChecks := baseChecks
		if doctorRig != "" {
			allChecks = append(allChecks, rigChecks...)
		}

		var completions []string
		for _, c := range doctor.ChecksInCategory(allChecks, category) {
			completions = append(completions, c.Name())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp

	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
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
			return fmt.Errorf("unknown category %q. Run \"gt doctor\" with no arguments to run all checks", "all")
		}
	}

	// Parse category-first args
	isTargeted := len(args) > 0
	var checks []doctor.Check
	var categoryName string

	if isTargeted {
		var checkName string
		if len(args) > 1 {
			checkName = args[1]
		}

		result := doctor.FilterByCategory(allChecks, args[0], checkName)
		if result.Error != nil {
			return formatCategoryError(allChecks, result)
		}

		checks = result.Matched
		categoryName = result.CategoryName
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

// formatCategoryError builds context-aware error messages for category-first filtering.
func formatCategoryError(allChecks []doctor.Check, result *doctor.FilterCategoryResult) error {
	var b strings.Builder

	switch result.ErrorKind {
	case doctor.FilterErrorUnknownCategory:
		fmt.Fprintf(&b, "unknown category %q", result.CategoryInput)

		// Check if the input matches a check name (user used old flat syntax)
		normalized := doctor.NormalizeName(result.CategoryInput)
		for _, check := range allChecks {
			if doctor.NormalizeName(check.Name()) == normalized {
				fmt.Fprintf(&b, "\n\n  %q is a check in category %q. Use:\n    gt doctor %s %s",
					result.CategoryInput, check.Category(),
					strings.ToLower(check.Category()), check.Name())
				break
			}
		}

		// Suggest similar categories
		suggestions := doctor.SuggestCategory(result.CategoryInput)
		if len(suggestions) == 1 {
			fmt.Fprintf(&b, "\n\n  Did you mean: %s?", strings.ToLower(suggestions[0]))
		} else if len(suggestions) > 1 {
			lower := make([]string, len(suggestions))
			for i, s := range suggestions {
				lower[i] = strings.ToLower(s)
			}
			fmt.Fprintf(&b, "\n\n  Did you mean one of: %s?", strings.Join(lower, ", "))
		}

		// List valid categories
		b.WriteString("\n\n  Available categories:")
		for _, cat := range doctor.CategoryOrder {
			fmt.Fprintf(&b, "\n    %s", strings.ToLower(cat))
		}

	case doctor.FilterErrorUnknownCheck:
		fmt.Fprintf(&b, "unknown check %q in category %q", result.CheckInput, result.CategoryName)

		// Suggest similar check names within the category
		categoryChecks := doctor.ChecksInCategory(allChecks, result.CategoryName)
		suggestions := doctor.SuggestCheck(categoryChecks, result.CheckInput)
		if len(suggestions) == 1 {
			fmt.Fprintf(&b, "\n\n  Did you mean: %s?", suggestions[0])
		} else if len(suggestions) > 1 {
			fmt.Fprintf(&b, "\n\n  Did you mean one of: %s?", strings.Join(suggestions, ", "))
		}

		// List valid checks in this category
		fmt.Fprintf(&b, "\n\n  Checks in %s:", result.CategoryName)
		for _, c := range categoryChecks {
			fmt.Fprintf(&b, "\n    %s", c.Name())
		}
	}

	b.WriteString("\n\n  Run \"gt doctor list\" to see all available checks.")
	if doctorRig == "" {
		b.WriteString("\n  Some checks require --rig. Run \"gt doctor list --rig <name>\".")
	}
	b.WriteString("\n") // blank line before cobra's Usage block

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
