package cmd

import (
	"github.com/spf13/cobra"
)

// Escalate command flags
var (
	escalateSeverity    string
	escalateReason      string
	escalateRelatedBead string
	escalateJSON        bool
	escalateListJSON    bool
	escalateListAll     bool
	escalateStaleJSON   bool
	escalateDryRun      bool
	escalateCloseReason string
)

var escalateCmd = &cobra.Command{
	Use:     "escalate [description]",
	GroupID: GroupComm,
	Short:   "Escalation system for critical issues",
	RunE:    runEscalate,
	Long: `Create and manage escalations for critical issues.

The escalation system provides severity-based routing for issues that need
human or mayor attention. Escalations are tracked as beads with gt:escalation label.

SEVERITY LEVELS:
  critical  (P0) Immediate attention required
  high      (P1) Urgent, needs attention soon
  normal    (P2) Standard escalation (default)
  low       (P3) Informational, can wait

WORKFLOW:
  1. Agent encounters blocking issue
  2. Runs: gt escalate "Description" --severity high --reason "details"
  3. Escalation is routed based on config/escalation.json
  4. Recipient acknowledges with: gt escalate ack <id>
  5. After resolution: gt escalate close <id> --reason "fixed"

CONFIGURATION:
  Routing is configured in ~/gt/config/escalation.json:
  - severity_routes: Map severity to notification targets
  - external_channels: Optional email/SMS for critical issues
  - stale_threshold: When unacked escalations are flagged

Examples:
  gt escalate "Build failing" --severity critical --reason "CI blocked"
  gt escalate "Need API credentials" --severity high
  gt escalate "Code review requested" --reason "PR #123 ready"
  gt escalate list                          # Show open escalations
  gt escalate ack hq-abc123                 # Acknowledge
  gt escalate close hq-abc123 --reason "Fixed in commit abc"
  gt escalate stale                         # Show unacked escalations`,
}

var escalateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List open escalations",
	Long: `List all open escalations.

Shows escalations that haven't been closed yet. Use --all to include
closed escalations.

Examples:
  gt escalate list              # Open escalations only
  gt escalate list --all        # Include closed
  gt escalate list --json       # JSON output`,
	RunE: runEscalateList,
}

var escalateAckCmd = &cobra.Command{
	Use:   "ack <escalation-id>",
	Short: "Acknowledge an escalation",
	Long: `Acknowledge an escalation to indicate you're working on it.

Adds an "acked" label and records who acknowledged and when.
This stops the stale escalation warnings.

Examples:
  gt escalate ack hq-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runEscalateAck,
}

var escalateCloseCmd = &cobra.Command{
	Use:   "close <escalation-id>",
	Short: "Close a resolved escalation",
	Long: `Close an escalation after the issue is resolved.

Records who closed it and the resolution reason.

Examples:
  gt escalate close hq-abc123 --reason "Fixed in commit abc"
  gt escalate close hq-abc123 --reason "Not reproducible"`,
	Args: cobra.ExactArgs(1),
	RunE: runEscalateClose,
}

var escalateStaleCmd = &cobra.Command{
	Use:   "stale",
	Short: "Show stale unacknowledged escalations",
	Long: `Show escalations that haven't been acknowledged within the threshold.

The threshold is configured in config/escalation.json (default: 1 hour).
Useful for patrol agents to detect escalations that need attention.

Examples:
  gt escalate stale           # Show stale escalations
  gt escalate stale --json    # JSON output`,
	RunE: runEscalateStale,
}

var escalateShowCmd = &cobra.Command{
	Use:   "show <escalation-id>",
	Short: "Show details of an escalation",
	Long: `Display detailed information about an escalation.

Examples:
  gt escalate show hq-abc123
  gt escalate show hq-abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runEscalateShow,
}

func init() {
	// Main escalate command flags
	escalateCmd.Flags().StringVarP(&escalateSeverity, "severity", "s", "normal", "Severity level: critical, high, normal, low")
	escalateCmd.Flags().StringVarP(&escalateReason, "reason", "r", "", "Detailed reason for escalation")
	escalateCmd.Flags().StringVar(&escalateRelatedBead, "related", "", "Related bead ID (task, bug, etc.)")
	escalateCmd.Flags().BoolVar(&escalateJSON, "json", false, "Output as JSON")
	escalateCmd.Flags().BoolVarP(&escalateDryRun, "dry-run", "n", false, "Show what would be done without executing")

	// List subcommand flags
	escalateListCmd.Flags().BoolVar(&escalateListJSON, "json", false, "Output as JSON")
	escalateListCmd.Flags().BoolVar(&escalateListAll, "all", false, "Include closed escalations")

	// Close subcommand flags
	escalateCloseCmd.Flags().StringVar(&escalateCloseReason, "reason", "", "Resolution reason")
	_ = escalateCloseCmd.MarkFlagRequired("reason")

	// Stale subcommand flags
	escalateStaleCmd.Flags().BoolVar(&escalateStaleJSON, "json", false, "Output as JSON")

	// Show subcommand flags
	escalateShowCmd.Flags().BoolVar(&escalateJSON, "json", false, "Output as JSON")

	// Add subcommands
	escalateCmd.AddCommand(escalateListCmd)
	escalateCmd.AddCommand(escalateAckCmd)
	escalateCmd.AddCommand(escalateCloseCmd)
	escalateCmd.AddCommand(escalateStaleCmd)
	escalateCmd.AddCommand(escalateShowCmd)

	rootCmd.AddCommand(escalateCmd)
}
