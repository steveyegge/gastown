package cmd

import (
	"github.com/spf13/cobra"
)

// Decision command flags
var (
	decisionPrompt               string   // Primary flag (--prompt)
	decisionContext              string
	decisionAnalysis             string   // Detailed analysis of the situation
	decisionTradeoffs            string   // General tradeoffs discussion
	decisionRecommendRationale   string   // Why the recommended option is suggested
	decisionOptions              []string
	decisionOptionPros           []string // Pros for options (repeatable: "1:Pro text" format)
	decisionOptionCons           []string // Cons for options (repeatable: "1:Con text" format)
	decisionRecommend            int
	decisionBlocks               string   // Primary flag (--blocks)
	decisionParent               string   // Parent bead relationship
	decisionUrgency              string
	decisionJSON                 bool
	decisionListJSON             bool
	decisionListAll              bool
	decisionChoice               int
	decisionRationale            string
	decisionAwaitTimeout         string   // For await command
)

var decisionCmd = &cobra.Command{
	Use:     "decision",
	GroupID: GroupComm,
	Short:   "Request and resolve decisions from humans",
	Long: `Create and manage decision requests for human input.

The decision system provides a lightweight way for agents to request
structured decisions from humans. Unlike escalations (for problems),
decisions are for choosing between valid options.

This is the high-level agent workflow layer. For low-level primitives
with full control, use 'bd decision' commands.

DECISION WORKFLOW:
  1. Agent needs human input on approach
  2. Runs: gt decision request --prompt "..." --option "A: desc" --option "B: desc"
  3. Decision bead is created, human notified
  4. Human reviews and resolves: gt decision resolve <id> --choice 1
  5. Agent proceeds with chosen option

URGENCY LEVELS:
  high    Blocking work, needs quick response
  medium  Standard decision request (default)
  low     Can wait, informational

Examples:
  gt decision request --prompt "Auth approach?" --option "JWT: Stateless" --option "Session: Traditional"
  gt decision request --prompt "DB choice?" --option "Postgres" --option "MySQL" --blocks gt-work-xyz
  gt decision list                                # Show pending decisions
  gt decision show hq-dec-abc                     # Show decision details
  gt decision resolve hq-dec-abc --choice 1 --rationale "JWT fits our architecture"
  gt decision await hq-dec-abc                    # Block until resolved`,
}

var decisionRequestCmd = &cobra.Command{
	Use:   "request",
	Short: "Request a decision from human",
	Long: `Create a new decision request for human input.

Agents use this when they need to choose between valid approaches and
want human guidance. The decision is tracked as a bead and blocks
dependent work until resolved.

FLAGS:
  --prompt                The decision to be made (required)
  --option                An option in "Label: Description" format (repeatable, 2-4 required)
  --context               Brief background information
  --analysis              Detailed analysis of the situation (paragraph-length encouraged)
  --tradeoffs             General tradeoffs discussion
  --recommend             Mark option N as recommended (1-indexed)
  --recommend-rationale   Why the recommended option is suggested
  --pro                   Pro for an option in "N:Pro text" format (repeatable)
  --con                   Con for an option in "N:Con text" format (repeatable)
  --blocks                Bead ID that's blocked by this decision
  --parent                Parent bead for hierarchy
  --urgency               Priority level: high, medium, low (default: medium)

Examples:
  # Simple decision
  gt decision request \
    --prompt "Which authentication method?" \
    --option "JWT tokens: Stateless, scalable, good for SPAs" \
    --option "Session cookies: Simpler, traditional approach" \
    --recommend 1 \
    --blocks gt-work-xyz

  # Rich decision with analysis and tradeoffs
  gt decision request \
    --prompt "Which database for the new service?" \
    --context "Building a new microservice that needs persistent storage" \
    --analysis "The service handles ~1000 requests/day with complex queries..." \
    --tradeoffs "Speed vs flexibility is the key consideration here" \
    --option "PostgreSQL: Full-featured relational database" \
    --option "SQLite: Lightweight, embedded, zero-config" \
    --pro "1:Excellent for complex queries and joins" \
    --pro "1:Strong ecosystem and tooling" \
    --con "1:Requires separate server process" \
    --pro "2:No external dependencies" \
    --con "2:Limited concurrent write performance" \
    --recommend 1 \
    --recommend-rationale "PostgreSQL better fits our scaling needs"`,
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

var decisionAwaitCmd = &cobra.Command{
	Use:   "await <decision-id>",
	Short: "Wait for a decision to be resolved",
	Long: `Block until the specified decision is resolved.

Useful for scripts and automation that need to wait for human input
before proceeding. Returns the chosen option when resolved.

FLAGS:
  --timeout     Maximum time to wait (e.g., "5m", "1h", default: no timeout)

Examples:
  gt decision await hq-dec-abc123
  gt decision await hq-dec-abc123 --timeout 5m
  gt decision await hq-dec-abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDecisionAwait,
}

var decisionRemindCmd = &cobra.Command{
	Use:   "remind",
	Short: "Remind to offer a decision at session end",
	Long: `Check for session work and remind to offer a decision.

Designed for Claude Code Stop hooks. When work is detected (uncommitted
changes, in-progress beads, etc.), outputs a reminder to offer a decision
about next steps before the session ends.

FLAGS:
  --inject     Output as <system-reminder> for Claude Code hooks
  --nudge      Send reminder as nudge to current agent's session

Exit codes (normal mode):
  0 - Work detected, reminder printed
  1 - No work detected

Exit codes (--inject/--nudge mode):
  0 - Always (hooks should never block)

Examples:
  # For Claude Code Stop hook (nudges self if work detected)
  gt decision remind --nudge

  # For UserPromptSubmit hook (injects context)
  gt decision remind --inject

  # Human-readable check
  gt decision remind`,
	RunE: runDecisionRemind,
}

var decisionWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Interactive TUI for monitoring and responding to decisions",
	Long: `Launch an interactive terminal UI for decision management.

The watch TUI provides:
  - Real-time view of all pending decisions
  - Keyboard navigation and quick selection
  - Option selection with number keys (1-4)
  - Rationale input before confirming
  - Automatic refresh every 5 seconds

KEYBOARD SHORTCUTS:
  j/k or ↑/↓    Navigate between decisions
  1-4           Select option by number
  r             Add rationale before confirming
  Enter         Confirm selection
  t             Enter custom text response
  R             Refresh immediately
  !             Filter to high urgency only
  a             Show all urgencies
  ?             Toggle help
  q             Quit

Examples:
  gt decision watch                    # Launch interactive TUI
  gt decision watch --urgent-only      # Show only high urgency
  gt decision watch --notify           # Enable desktop notifications`,
	RunE: runDecisionWatch,
}

var decisionTurnClearCmd = &cobra.Command{
	Use:    "turn-clear",
	Short:  "Clear turn marker (for hooks)",
	Hidden: true,
	RunE:   runDecisionTurnClear,
}

var decisionTurnMarkCmd = &cobra.Command{
	Use:    "turn-mark",
	Short:  "Mark decision offered (for hooks)",
	Hidden: true,
	RunE:   runDecisionTurnMark,
}

var decisionTurnCheckCmd = &cobra.Command{
	Use:    "turn-check",
	Short:  "Check turn marker (for hooks)",
	Hidden: true,
	RunE:   runDecisionTurnCheck,
}

// Watch-specific flags
var decisionWatchUrgentOnly bool
var decisionWatchNotify bool

// Turn-check flags
var decisionTurnCheckSoft bool

// Dashboard-specific flags
var decisionDashboardJSON bool

// Remind-specific flags
var decisionRemindInject bool
var decisionRemindNudge bool

func init() {
	// Request subcommand flags
	// Primary flags
	decisionRequestCmd.Flags().StringVarP(&decisionPrompt, "prompt", "p", "", "The decision to be made (required)")
	decisionRequestCmd.Flags().StringVarP(&decisionContext, "context", "c", "", "Brief background information")
	decisionRequestCmd.Flags().StringVarP(&decisionAnalysis, "analysis", "a", "", "Detailed analysis of the situation")
	decisionRequestCmd.Flags().StringVar(&decisionTradeoffs, "tradeoffs", "", "General tradeoffs discussion")
	decisionRequestCmd.Flags().StringArrayVarP(&decisionOptions, "option", "o", nil, "Option in 'Label: Description' format (repeatable)")
	decisionRequestCmd.Flags().StringArrayVar(&decisionOptionPros, "pro", nil, "Pro for option in 'N:Pro text' format (repeatable)")
	decisionRequestCmd.Flags().StringArrayVar(&decisionOptionCons, "con", nil, "Con for option in 'N:Con text' format (repeatable)")
	decisionRequestCmd.Flags().IntVarP(&decisionRecommend, "recommend", "r", 0, "Mark option N as recommended (1-indexed)")
	decisionRequestCmd.Flags().StringVar(&decisionRecommendRationale, "recommend-rationale", "", "Why the recommended option is suggested")
	decisionRequestCmd.Flags().StringVar(&decisionBlocks, "blocks", "", "Bead ID that this decision blocks")
	decisionRequestCmd.Flags().StringVar(&decisionParent, "parent", "", "Parent bead for hierarchy")
	decisionRequestCmd.Flags().StringVarP(&decisionUrgency, "urgency", "u", "medium", "Urgency level: high, medium, low")
	decisionRequestCmd.Flags().BoolVar(&decisionJSON, "json", false, "Output as JSON")

	// Aliases for backward compatibility
	decisionRequestCmd.Flags().StringVarP(&decisionPrompt, "question", "q", "", "Alias for --prompt (deprecated)")
	decisionRequestCmd.Flags().StringVarP(&decisionBlocks, "blocker", "b", "", "Alias for --blocks (deprecated)")
	_ = decisionRequestCmd.Flags().MarkHidden("question")
	_ = decisionRequestCmd.Flags().MarkHidden("blocker")

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

	// Await subcommand flags
	decisionAwaitCmd.Flags().StringVar(&decisionAwaitTimeout, "timeout", "", "Maximum time to wait (e.g., '5m', '1h')")
	decisionAwaitCmd.Flags().BoolVar(&decisionJSON, "json", false, "Output as JSON")

	// Remind subcommand flags
	decisionRemindCmd.Flags().BoolVar(&decisionRemindInject, "inject", false, "Output as <system-reminder> for Claude Code hooks")
	decisionRemindCmd.Flags().BoolVar(&decisionRemindNudge, "nudge", false, "Send reminder as nudge to current agent's session")

	// Watch subcommand flags
	decisionWatchCmd.Flags().BoolVar(&decisionWatchUrgentOnly, "urgent-only", false, "Show only high urgency decisions")
	decisionWatchCmd.Flags().BoolVar(&decisionWatchNotify, "notify", false, "Enable desktop notifications for new decisions")

	// Turn-check flags
	decisionTurnCheckCmd.Flags().BoolVar(&decisionTurnCheckSoft, "soft", false, "Soft mode: don't block, just allow")

	// Add subcommands
	decisionCmd.AddCommand(decisionRequestCmd)
	decisionCmd.AddCommand(decisionListCmd)
	decisionCmd.AddCommand(decisionShowCmd)
	decisionCmd.AddCommand(decisionResolveCmd)
	decisionCmd.AddCommand(decisionDashboardCmd)
	decisionCmd.AddCommand(decisionAwaitCmd)
	decisionCmd.AddCommand(decisionRemindCmd)
	decisionCmd.AddCommand(decisionWatchCmd)
	decisionCmd.AddCommand(decisionTurnClearCmd)
	decisionCmd.AddCommand(decisionTurnMarkCmd)
	decisionCmd.AddCommand(decisionTurnCheckCmd)

	rootCmd.AddCommand(decisionCmd)
}
