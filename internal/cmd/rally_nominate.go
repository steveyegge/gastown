package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rally"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	rallyNominateCategory    string
	rallyNominateTitle       string
	rallyNominateSummary     string
	rallyNominateDetails     string
	rallyNominateTags        []string
	rallyNominateCodebase    string
	rallyNominateGotchas     []string
	rallyNominateExamples    []string
	rallyNominateProblem     string
	rallyNominateSolution    string
	rallyNominateContext     string
	rallyNominateLesson      string
	rallyNominateIssue       string
	rallyNominateStdin       bool
	rallyNominateDryRun      bool
)

// rallyNominationTarget is the address of the rally_tavern Barkeep (Mayor).
const rallyNominationTarget = "rally_tavern/mayor"

func init() {
	rallyCmd.AddCommand(rallyNominateCmd)

	f := rallyNominateCmd.Flags()
	f.StringVar(&rallyNominateCategory, "category", "", "Knowledge category: practice, solution, or learned (required)")
	f.StringVar(&rallyNominateTitle, "title", "", "Short title (required without --stdin)")
	f.StringVar(&rallyNominateSummary, "summary", "", "One-sentence summary (required without --stdin)")
	f.StringVar(&rallyNominateDetails, "details", "", "Full details (multiline)")
	f.StringSliceVar(&rallyNominateTags, "tags", nil, "Tags (comma-separated)")
	f.StringVar(&rallyNominateCodebase, "codebase-type", "general", "Codebase type (e.g. go-cobra, general)")
	f.StringSliceVar(&rallyNominateGotchas, "gotcha", nil, "Gotcha (practice only, repeatable)")
	f.StringSliceVar(&rallyNominateExamples, "example", nil, "Example (practice only, repeatable)")
	f.StringVar(&rallyNominateProblem, "problem", "", "Problem description (solution only)")
	f.StringVar(&rallyNominateSolution, "solution", "", "Solution text (solution only)")
	f.StringVar(&rallyNominateContext, "context", "", "Context of discovery (learned only)")
	f.StringVar(&rallyNominateLesson, "lesson", "", "Lesson text (learned only)")
	f.StringVar(&rallyNominateIssue, "issue", "", "Source issue ID (for audit trail)")
	f.BoolVar(&rallyNominateStdin, "stdin", false, "Read full nomination YAML from stdin")
	f.BoolVar(&rallyNominateDryRun, "dry-run", false, "Print what would be sent without sending")
}

var rallyNominateCmd = &cobra.Command{
	Use:   "nominate",
	Short: "Nominate a knowledge contribution to rally_tavern",
	Long: `Nominate a knowledge contribution to the rally_tavern knowledge base.

The nomination is routed to franklin (rally_tavern's knowledge curator) for review.
If accepted, the contribution is written to the rally_tavern knowledge directory
and becomes searchable via 'gt rally search' and 'gt rally lookup'.

Nominations are non-blocking: gt done proceeds whether or not rally_tavern is available.

Examples:
  gt rally nominate \
    --category practice \
    --title "Enable tmux mouse support" \
    --summary "Add 'setw -g mouse on' to ~/.tmux.conf" \
    --tags tmux,developer-tools,terminal

  gt rally nominate --category solution --stdin <<'YAML'
  title: "Swift 6 NSMutableArray observer box"
  summary: "Box observer tokens to avoid Swift 6 Sendable mutation warning"
  problem: "NSMutableArray observer tokens cause Sendable errors in Swift 6"
  solution: "Wrap in a Box<T> class to isolate mutation"
  tags: [swift, swift6, sendable]
  YAML

  gt rally nominate --category practice --title "..." --dry-run`,
	RunE:         runRallyNominate,
	SilenceUsage: true,
}

func runRallyNominate(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	var nom *rally.Nomination

	if rallyNominateStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		nom = &rally.Nomination{}
		if err := yaml.Unmarshal(data, nom); err != nil {
			return fmt.Errorf("parsing nomination YAML from stdin: %w", err)
		}
		// Allow --category override even with --stdin
		if rallyNominateCategory != "" {
			nom.Category = rallyNominateCategory
		}
	} else {
		nom = &rally.Nomination{
			Category:     rallyNominateCategory,
			Title:        rallyNominateTitle,
			Summary:      rallyNominateSummary,
			Details:      rallyNominateDetails,
			Tags:         rallyNominateTags,
			CodebaseType: rallyNominateCodebase,
			Gotchas:      rallyNominateGotchas,
			Examples:     rallyNominateExamples,
			Problem:      rallyNominateProblem,
			Solution:     rallyNominateSolution,
			Context:      rallyNominateContext,
			Lesson:       rallyNominateLesson,
			SourceIssue:  rallyNominateIssue,
		}
	}

	// Auto-populate provenance
	nom.NominatedBy = detectSender()
	nom.NominatedAt = time.Now().UTC().Format(time.RFC3339)
	nom.NominationID = rally.GenerateNominationID()

	if err := nom.Validate(); err != nil {
		return fmt.Errorf("invalid nomination: %w", err)
	}

	body, err := nom.ToMailBody()
	if err != nil {
		return err
	}

	if rallyNominateDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "-- dry run: would send to %s --\n\n", rallyNominationTarget)
		fmt.Fprintln(cmd.OutOrStdout(), body)
		return nil
	}

	// Check if rally_tavern is present; degrade gracefully if absent.
	idx, err := rally.LoadKnowledgeIndex(townRoot)
	if err != nil {
		style.PrintWarning("rally_tavern unavailable (%v) — nomination dropped", err)
		return nil
	}
	if idx == nil {
		style.PrintWarning("rally_tavern not found at %s/rally_tavern/ — nomination dropped", townRoot)
		return nil
	}

	router := mail.NewRouter(townRoot)
	defer router.WaitPendingNotifications()

	msg := &mail.Message{
		From:     nom.NominatedBy,
		To:       rallyNominationTarget,
		Subject:  fmt.Sprintf("RALLY_NOMINATION: %s [%s]", nom.Title, nom.Category),
		Body:     body,
		Type:     mail.TypeTask,
		Priority: mail.PriorityNormal,
	}

	if err := router.Send(msg); err != nil {
		style.PrintWarning("could not send nomination to %s: %v — nomination dropped", rallyNominationTarget, err)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s Nomination sent to %s (%s)\n",
		style.Bold.Render("✓"), rallyNominationTarget, nom.NominationID)

	if strings.TrimSpace(nom.SourceIssue) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Source issue: %s\n", nom.SourceIssue)
	}

	return nil
}
