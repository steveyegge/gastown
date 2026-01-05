package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/auditor"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/refinery"
)

var verifyCmd = &cobra.Command{
	Use:     "verify",
	Short:   "Verification commands",
	Long:    `Commands for independent code verification using alternate AI models.`,
	GroupID: GroupWork,
	RunE:    requireSubcommand,
}

var verifyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show verification queue status",
	Long: `Shows pending verifications and the current verification configuration.

This command displays:
- Beads pending verification
- The active verification runtime (Codex, OpenCode, or Claude)
- Verification configuration settings`,
	Run: runVerifyStatus,
}

var verifyRunCmd = &cobra.Command{
	Use:   "run <bead-id>",
	Short: "Manually trigger verification for a bead",
	Long: `Triggers independent verification for a specific bead.

The verification uses an alternate AI runtime (preferring Codex > OpenCode > Claude)
to review the work associated with the bead.

Possible verdicts:
  PASS        - Work passed verification, ready for merge
  FAIL        - Work has issues that need to be addressed
  NEEDS_HUMAN - Unable to determine, requires human review

Examples:
  gt verify run gt-abc123           # Verify a specific bead
  gt verify run gt-xyz789 --timeout 10m  # With custom timeout`,
	Args: cobra.ExactArgs(1),
	RunE: runVerifyRun,
}

var verifyConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show verification runtime configuration",
	Long: `Displays the current verification runtime configuration.

Shows:
- The runtime that will be used for the auditor role
- All available runtimes and their status
- Fallback order for verification`,
	Run: runVerifyConfig,
}

var verifyMRCmd = &cobra.Command{
	Use:   "mr <mr-id>",
	Short: "Verify a merge request before merge",
	Long: `Performs verification on a merge request.

This is typically called by the Refinery before merging, but can be
triggered manually for testing or re-verification.

Examples:
  gt verify mr gt-mr-abc123
  gt verify mr gt-mr-abc123 --workdir /path/to/rig`,
	Args: cobra.ExactArgs(1),
	RunE: runVerifyMR,
}

// Command flags
var (
	verifyTimeout time.Duration
	verifyWorkdir string
)

func init() {
	// Add subcommands
	verifyCmd.AddCommand(verifyStatusCmd)
	verifyCmd.AddCommand(verifyRunCmd)
	verifyCmd.AddCommand(verifyConfigCmd)
	verifyCmd.AddCommand(verifyMRCmd)

	// Flags for verify run
	verifyRunCmd.Flags().DurationVar(&verifyTimeout, "timeout", 5*time.Minute,
		"Timeout for verification")

	// Flags for verify mr
	verifyMRCmd.Flags().StringVar(&verifyWorkdir, "workdir", "",
		"Working directory for verification (defaults to current directory)")
	verifyMRCmd.Flags().DurationVar(&verifyTimeout, "timeout", 5*time.Minute,
		"Timeout for verification")

	// Add to root
	rootCmd.AddCommand(verifyCmd)
}

func runVerifyStatus(cmd *cobra.Command, args []string) {
	// Get current working directory for beads
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	// Show verification configuration
	registry := agent.NewRuntimeRegistry()
	auditorRuntime := registry.GetForRole("auditor")

	fmt.Println("Verification Configuration")
	fmt.Println("==========================")
	fmt.Println()

	fmt.Println("Verification is MANDATORY - all work must be LLM reviewed")
	fmt.Println()

	if auditorRuntime != nil {
		fmt.Printf("Active runtime: %s\n", auditorRuntime.Name())
		if auditorRuntime.Name() != "claude" {
			fmt.Println("Mode: Independent verification (different model)")
		} else {
			fmt.Println("Mode: Same-model verification (Claude reviewing Claude)")
		}
	} else {
		fmt.Println("Active runtime: NONE - verification will fail!")
		fmt.Println("ERROR: Install claude, codex, or opencode CLI")
	}
	fmt.Println()

	// List available runtimes
	fmt.Println("Available runtimes:")
	runtimes := []struct {
		name   string
		status string
	}{
		{"claude", statusIcon(registry.HasRuntime("claude"))},
		{"codex", statusIcon(registry.HasRuntime("codex"))},
		{"opencode", statusIcon(registry.HasRuntime("opencode"))},
	}

	for _, rt := range runtimes {
		fmt.Printf("  %s %s\n", rt.status, rt.name)
	}
	fmt.Println()

	// Show pending verifications (beads with pending verification status)
	db := beads.New(cwd)
	issues, err := db.List(beads.ListOptions{
		Status:   "open",
		Priority: -1,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list beads: %v\n", err)
		return
	}

	// Filter for issues that might need verification
	// (This is a simplified check - real implementation would check verification labels)
	var pending int
	for _, issue := range issues {
		// Check for verification-related labels
		for _, label := range issue.Labels {
			if label == "needs-verification" || label == "pending-verification" {
				pending++
				fmt.Printf("  %s: %s\n", issue.ID, issue.Title)
				break
			}
		}
	}

	if pending == 0 {
		fmt.Println("No beads pending verification.")
	} else {
		fmt.Printf("\nPending verifications: %d\n", pending)
	}
}

func runVerifyRun(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Create registry and auditor
	registry := agent.NewRuntimeRegistry()
	db := beads.New(cwd)

	aud, err := auditor.New(registry, db)
	if err != nil {
		return fmt.Errorf("creating auditor: %w\n\nNo verification runtime is available.\nInstall one of: codex, opencode, or claude", err)
	}

	fmt.Printf("Verifying bead %s...\n", beadID)
	fmt.Printf("Using runtime: %s\n\n", aud.RuntimeName())

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	// Run verification
	result, err := aud.Verify(ctx, beadID, cwd)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	// Display results
	printVerificationResult(result)

	// Exit with error code if failed
	if result.IsFail() {
		return &SilentExitError{Code: 1}
	}

	return nil
}

func runVerifyConfig(cmd *cobra.Command, args []string) {
	registry := agent.NewRuntimeRegistry()
	auditorRuntime := registry.GetForRole("auditor")

	fmt.Println("Verification Runtime Configuration")
	fmt.Println("===================================")
	fmt.Println()

	if auditorRuntime != nil {
		fmt.Printf("Auditor runtime: %s\n", auditorRuntime.Name())
	} else {
		fmt.Println("Auditor runtime: none (no runtimes available)")
	}
	fmt.Println()

	fmt.Println("Runtime availability:")
	runtimeChecks := []struct {
		name string
		rt   agent.Runtime
	}{
		{"claude", &agent.ClaudeRuntime{}},
		{"codex", &agent.CodexRuntime{}},
		{"opencode", &agent.OpenCodeRuntime{}},
	}

	for _, check := range runtimeChecks {
		status := "not installed"
		if check.rt.Available() {
			status = "available"
		}
		fmt.Printf("  %-10s %s\n", check.name+":", status)
	}

	fmt.Println()
	fmt.Println("Fallback order for auditor role:")
	fmt.Println("  1. codex (preferred)")
	fmt.Println("  2. opencode")
	fmt.Println("  3. claude (same model fallback)")

	fmt.Println()
	config := auditor.DefaultVerificationConfig()
	fmt.Println("Verification settings:")
	fmt.Println("  Mandatory:           YES (cannot be disabled)")
	fmt.Printf("  Required confidence: %.0f%%\n", config.RequiredConfidence*100)
	fmt.Printf("  Timeout:             %ds\n", config.TimeoutSeconds)
	fmt.Printf("  Require independent: %v\n", config.RequireIndependent)

	fmt.Println()
	fmt.Println("Verification ensures:")
	fmt.Println("  - Code meets requirements")
	fmt.Println("  - No bugs or security issues")
	fmt.Println("  - Tests are adequate")
	fmt.Println("  - Code quality standards met")
}

func runVerifyMR(cmd *cobra.Command, args []string) error {
	mrID := args[0]

	workdir := verifyWorkdir
	if workdir == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	// Create verification gate
	gate, err := refinery.NewVerificationGate(workdir)
	if err != nil {
		return fmt.Errorf("creating verification gate: %w\n\nNo verification runtime is available", err)
	}

	fmt.Printf("Verifying MR %s...\n", mrID)
	fmt.Printf("Using runtime: %s\n\n", gate.RuntimeName())

	// Create a mock MR for verification
	// In real usage, this would be fetched from the merge queue
	mr := &refinery.MergeRequest{
		ID:           mrID,
		Branch:       "polecat/unknown",
		TargetBranch: "main",
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	// Run verification
	info, err := gate.VerifyMR(ctx, mr, workdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: verification error: %v\n", err)
	}

	// Display results
	printVerificationInfo(info)

	// Exit with error code based on status
	if gate.ShouldSlingBack(info) {
		return &SilentExitError{Code: 1}
	}
	if gate.ShouldEscalate(info) {
		return &SilentExitError{Code: 2}
	}

	return nil
}

func printVerificationResult(result *auditor.VerificationResult) {
	// Verdict with color hint
	switch result.Verdict {
	case auditor.VerdictPass:
		fmt.Printf("Verdict: PASS\n")
	case auditor.VerdictFail:
		fmt.Printf("Verdict: FAIL\n")
	case auditor.VerdictNeedsHuman:
		fmt.Printf("Verdict: NEEDS_HUMAN\n")
	}

	fmt.Printf("Confidence: %.0f%%\n", result.Confidence*100)
	fmt.Printf("Reviewed by: %s\n", result.ReviewedBy)

	if result.IsIndependent {
		fmt.Println("Verification type: Independent (different model)")
	} else {
		fmt.Println("Verification type: Same-model review")
	}

	fmt.Printf("Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Println()

	if len(result.Issues) > 0 {
		fmt.Println("Issues:")
		for _, issue := range result.Issues {
			fmt.Printf("  - %s\n", issue)
		}
		fmt.Println()
	}

	if len(result.Suggestions) > 0 {
		fmt.Println("Suggestions:")
		for _, suggestion := range result.Suggestions {
			fmt.Printf("  - %s\n", suggestion)
		}
		fmt.Println()
	}
}

func printVerificationInfo(info *refinery.VerificationInfo) {
	if info == nil {
		fmt.Println("Status: unknown (no verification info)")
		return
	}

	fmt.Printf("Status: %s\n", info.Status)

	if info.ReviewedBy != "" {
		fmt.Printf("Reviewed by: %s\n", info.ReviewedBy)
	}

	if info.IsIndependent {
		fmt.Println("Verification type: Independent (different model)")
	} else if info.ReviewedBy != "" {
		fmt.Println("Verification type: Same-model review")
	}

	if info.Confidence > 0 {
		fmt.Printf("Confidence: %.0f%%\n", info.Confidence*100)
	}

	if info.VerifiedAt != nil {
		fmt.Printf("Verified at: %s\n", info.VerifiedAt.Format(time.RFC3339))
	}

	fmt.Println()

	if len(info.Issues) > 0 {
		fmt.Println("Issues:")
		for _, issue := range info.Issues {
			fmt.Printf("  - %s\n", issue)
		}
		fmt.Println()
	}

	if len(info.Suggestions) > 0 {
		fmt.Println("Suggestions:")
		for _, suggestion := range info.Suggestions {
			fmt.Printf("  - %s\n", suggestion)
		}
		fmt.Println()
	}
}

func statusIcon(available bool) string {
	if available {
		return "[x]"
	}
	return "[ ]"
}
