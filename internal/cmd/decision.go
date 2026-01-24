package cmd

import (
	"github.com/spf13/cobra"
)

// Decision command flags
var (
	decisionQuestion    string
	decisionContext     string
	decisionOptions     []string
	decisionRecommend   int
	decisionBlocker     string
	decisionUrgency     string
	decisionJSON        bool
	decisionListJSON    bool
	decisionListAll     bool
	decisionChoice      int
	decisionRationale   string
)

var decisionCmd = &cobra.Command{
	Use:     "decision",
	GroupID: GroupComm,
	Short:   "Request and resolve decisions from humans",
	Long: `Create and manage decision requests for human input.

The decision system provides a lightweight way for agents to request
structured decisions from humans. Unlike escalations (for problems),
decisions are for choosing between valid options.

DECISION WORKFLOW:
  1. Agent needs human input on approach
  2. Runs: gt decision request --question "..." --option "A: desc" --option "B: desc"
  3. Decision bead is created, human notified
  4. Human reviews and resolves: gt decision resolve <id> --choice 1
  5. Agent proceeds with chosen option

URGENCY LEVELS:
  high    Blocking work, needs quick response
  medium  Standard decision request (default)
  low     Can wait, informational

Examples:
  gt decision request --question "Auth approach?" --option "JWT: Stateless" --option "Session: Traditional"
  gt decision request --question "DB choice?" --option "Postgres" --option "MySQL" --blocker gt-work-xyz
  gt decision list                                # Show pending decisions
  gt decision show hq-dec-abc                     # Show decision details
  gt decision resolve hq-dec-abc --choice 1 --rationale "JWT fits our architecture"`,
}

var decisionRequestCmd = &cobra.Command{
	Use:   "request",
	Short: "Request a decision from human",
	Long: `Create a new decision request for human input.

Agents use this when they need to choose between valid approaches and
want human guidance. The decision is tracked as a bead and blocks
dependent work until resolved.

FLAGS:
  --question    The decision to be made (required)
  --option      An option in "Label: Description" format (repeatable, 2-4 required)
  --context     Background information or analysis
  --recommend   Mark option N as recommended (1-indexed)
  --blocker     Bead ID that's blocked by this decision
  --urgency     Priority level: high, medium, low (default: medium)

Examples:
  gt decision request \
    --question "Which authentication method?" \
    --option "JWT tokens: Stateless, scalable, good for SPAs" \
    --option "Session cookies: Simpler, traditional approach" \
    --recommend 1 \
    --blocker gt-work-xyz`,
	RunE: runDecisionRequest,
}

var decisionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending decisions",
	Long: `List all pending decisions awaiting resolution.

Shows decisions that haven't been resolved yet. Use --all to include
resolved decisions.

Examples:
  gt decision list              # Pending only
  gt decision list --all        # Include resolved
  gt decision list --json       # JSON output`,
	RunE: runDecisionList,
}

var decisionShowCmd = &cobra.Command{
	Use:   "show <decision-id>",
	Short: "Show decision details",
	Long: `Display detailed information about a decision.

Shows the question, options, context, and current status.

Examples:
  gt decision show hq-dec-abc123
  gt decision show hq-dec-abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDecisionShow,
}

var decisionResolveCmd = &cobra.Command{
	Use:   "resolve <decision-id>",
	Short: "Resolve a pending decision",
	Long: `Resolve a decision by selecting an option.

Records the chosen option, rationale, and who decided. Automatically
unblocks any work that was waiting on this decision.

FLAGS:
  --choice      The option number to select (1-indexed, required)
  --rationale   Explanation for why this option was chosen

Examples:
  gt decision resolve hq-dec-abc123 --choice 1 --rationale "JWT fits our scaling goals"
  gt decision resolve hq-dec-abc123 --choice 2`,
	Args: cobra.ExactArgs(1),
	RunE: runDecisionResolve,
}

var decisionDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show decision dashboard summary",
	Long: `Display a summary dashboard of decision status.

Shows pending decisions grouped by urgency, recently resolved decisions,
and stale decisions that may need attention.

Examples:
  gt decision dashboard
  gt decision dashboard --json`,
	RunE: runDecisionDashboard,
}

// Dashboard-specific flags
var decisionDashboardJSON bool

func init() {
	// Request subcommand flags
	decisionRequestCmd.Flags().StringVarP(&decisionQuestion, "question", "q", "", "The decision to be made (required)")
	decisionRequestCmd.Flags().StringVarP(&decisionContext, "context", "c", "", "Background information or analysis")
	decisionRequestCmd.Flags().StringArrayVarP(&decisionOptions, "option", "o", nil, "Option in 'Label: Description' format (repeatable)")
	decisionRequestCmd.Flags().IntVarP(&decisionRecommend, "recommend", "r", 0, "Mark option N as recommended (1-indexed)")
	decisionRequestCmd.Flags().StringVarP(&decisionBlocker, "blocker", "b", "", "Bead ID blocked by this decision")
	decisionRequestCmd.Flags().StringVarP(&decisionUrgency, "urgency", "u", "medium", "Urgency level: high, medium, low")
	decisionRequestCmd.Flags().BoolVar(&decisionJSON, "json", false, "Output as JSON")
	_ = decisionRequestCmd.MarkFlagRequired("question")

	// List subcommand flags
	decisionListCmd.Flags().BoolVar(&decisionListJSON, "json", false, "Output as JSON")
	decisionListCmd.Flags().BoolVar(&decisionListAll, "all", false, "Include resolved decisions")

	// Show subcommand flags
	decisionShowCmd.Flags().BoolVar(&decisionJSON, "json", false, "Output as JSON")

	// Resolve subcommand flags
	decisionResolveCmd.Flags().IntVar(&decisionChoice, "choice", 0, "Option number to select (1-indexed)")
	decisionResolveCmd.Flags().StringVar(&decisionRationale, "rationale", "", "Explanation for the choice")
	decisionResolveCmd.Flags().BoolVar(&decisionJSON, "json", false, "Output as JSON")
	_ = decisionResolveCmd.MarkFlagRequired("choice")

	// Dashboard subcommand flags
	decisionDashboardCmd.Flags().BoolVar(&decisionDashboardJSON, "json", false, "Output as JSON")

	// Add subcommands
	decisionCmd.AddCommand(decisionRequestCmd)
	decisionCmd.AddCommand(decisionListCmd)
	decisionCmd.AddCommand(decisionShowCmd)
	decisionCmd.AddCommand(decisionResolveCmd)
	decisionCmd.AddCommand(decisionDashboardCmd)

	rootCmd.AddCommand(decisionCmd)
}
