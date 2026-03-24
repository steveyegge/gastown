package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rally"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	rallyReportKind        string
	rallyReportReason      string
	rallyReportImprovement string
	rallyReportTag         string
	rallyReportDryRun      bool
)

func init() {
	rallyCmd.AddCommand(rallyReportCmd)
	rallyCmd.AddCommand(rallyVerifyCmd)

	f := rallyReportCmd.Flags()
	f.StringVar(&rallyReportKind, "kind", "stale", "Report kind: stale, wrong, improve (default: stale)")
	f.StringVar(&rallyReportReason, "reason", "", "Why this entry is stale or wrong (required for stale/wrong)")
	f.StringVar(&rallyReportImprovement, "improve", "", "Suggested improvement text (required for kind=improve)")
	f.StringVar(&rallyReportTag, "tag", "", "Identify entry by tag instead of ID")
	f.BoolVar(&rallyReportDryRun, "dry-run", false, "Print what would be sent without sending")
}

var rallyReportCmd = &cobra.Command{
	Use:   "report <entry-id>",
	Short: "Report a knowledge entry as stale, wrong, or improvable",
	Long: `Signal to the Barkeep that a knowledge entry needs attention.

Kinds:
  stale    Entry was correct but is now outdated (default)
  wrong    Entry contains factual errors
  improve  Entry is correct but could be better

Examples:
  gt rally report gas-town-upgrade-sequence --reason "beads now via go install not brew"
  gt rally report gas-town-upgrade-sequence --kind wrong --reason "step 3 no longer applies"
  gt rally report gas-town-upgrade-sequence --kind improve --improve "add note about gt upgrade step"
  gt rally report --tag dolt --reason "port changed from 3307 to 3308"`,
	RunE:         runRallyReport,
	SilenceUsage: true,
}

var rallyVerifyCmd = &cobra.Command{
	Use:   "verify <entry-id>",
	Short: "Confirm a knowledge entry is still accurate",
	Long: `Signal to the Barkeep that you used an entry and it still works.

This updates last_verified on the entry, preventing it from being flagged
as potentially stale during knowledge hygiene checks.

Examples:
  gt rally verify gas-town-dolt-diagnostics
  gt rally verify --tag tmux`,
	RunE:         runRallyVerify,
	SilenceUsage: true,
}

func init() {
	rallyVerifyCmd.Flags().StringVar(&rallyReportTag, "tag", "", "Identify entry by tag instead of ID")
	rallyVerifyCmd.Flags().BoolVar(&rallyReportDryRun, "dry-run", false, "Print what would be sent without sending")
}

func runRallyReport(cmd *cobra.Command, args []string) error {
	entryID := ""
	if len(args) > 0 {
		entryID = strings.Join(args, "-")
	}

	r := &rally.Report{
		EntryID:     entryID,
		EntryTag:    rallyReportTag,
		Kind:        rally.ReportKind(rallyReportKind),
		Reason:      rallyReportReason,
		Improvement: rallyReportImprovement,
		ReportedBy:  detectSender(),
		ReportedAt:  rally.NowRFC3339(),
		ReportID:    rally.GenerateReportID(),
	}

	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid report: %w", err)
	}

	return sendReport(cmd, r)
}

func runRallyVerify(cmd *cobra.Command, args []string) error {
	entryID := ""
	if len(args) > 0 {
		entryID = strings.Join(args, "-")
	}

	r := &rally.Report{
		EntryID:    entryID,
		EntryTag:   rallyReportTag,
		Kind:       rally.ReportKindVerify,
		ReportedBy: detectSender(),
		ReportedAt: rally.NowRFC3339(),
		ReportID:   rally.GenerateReportID(),
	}

	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid verify: %w", err)
	}

	return sendReport(cmd, r)
}

func sendReport(cmd *cobra.Command, r *rally.Report) error {
	body, err := r.ToMailBody()
	if err != nil {
		return err
	}

	if rallyReportDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "-- dry run: would send to %s --\n\n", rallyNominationTarget)
		fmt.Fprintln(cmd.OutOrStdout(), body)
		return nil
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	idx, err := rally.LoadKnowledgeIndex(townRoot)
	if err != nil {
		style.PrintWarning("rally_tavern unavailable (%v) — report dropped", err)
		return nil
	}
	if idx == nil {
		style.PrintWarning("rally_tavern not found — report dropped")
		return nil
	}

	router := mail.NewRouter(townRoot)
	defer router.WaitPendingNotifications()

	msg := &mail.Message{
		From:     r.ReportedBy,
		To:       rallyNominationTarget,
		Subject:  r.SubjectLine(),
		Body:     body,
		Type:     mail.TypeTask,
		Priority: mail.PriorityNormal,
	}

	if err := router.Send(msg); err != nil {
		style.PrintWarning("could not send report to %s: %v — dropped", rallyNominationTarget, err)
		return nil
	}

	verb := string(r.Kind)
	target := r.EntryID
	if target == "" {
		target = "tag:" + r.EntryTag
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s Report sent to %s (%s: %s %s)\n",
		style.Bold.Render("✓"), rallyNominationTarget, r.ReportID, verb, target)

	return nil
}
