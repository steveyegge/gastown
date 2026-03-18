package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/followup"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var followupCmd = &cobra.Command{
	Use:     "followup",
	GroupID: GroupWork,
	Short:   "Schedule a follow-up reminder (survives session restarts)",
	Long: `Schedule a persistent follow-up reminder for deferred work.

When you defer work ("I'll wait for feedback"), use this command to ensure
you actually come back to it. Followups survive session restarts and are
surfaced by the deacon patrol when overdue.

Examples:
  gt followup "Check if Rome has feedback on PR #42" --in 30m
  gt followup "Iterate on auth refactor after review" --in 2h --bead gt-abc
  gt followup list
  gt followup resolve <id>`,
	RunE:         runFollowupCreate,
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
}

var followupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending follow-up reminders",
	RunE:  runFollowupList,
}

var followupResolveCmd = &cobra.Command{
	Use:   "resolve <id>",
	Short: "Mark a follow-up as resolved",
	Args:  cobra.ExactArgs(1),
	RunE:  runFollowupResolve,
}

var (
	followupIn    string
	followupBead  string
)

func init() {
	followupCmd.Flags().StringVar(&followupIn, "in", "", "Time until followup fires (e.g. 30m, 2h). Default from config.")
	followupCmd.Flags().StringVar(&followupBead, "bead", "", "Optional bead ID to link to this followup")

	followupCmd.AddCommand(followupListCmd)
	followupCmd.AddCommand(followupResolveCmd)
	rootCmd.AddCommand(followupCmd)
}

func runFollowupCreate(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// No topic: show list instead
		return runFollowupList(cmd, args)
	}

	topic := args[0]

	townRoot, _, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agent := detectSender()
	if agent == "" || agent == "overseer" {
		return fmt.Errorf("cannot determine agent identity (GT_ROLE not set?)")
	}

	// Determine delay
	var delay time.Duration
	if followupIn != "" {
		delay, err = time.ParseDuration(followupIn)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", followupIn, err)
		}
	} else {
		cfg := config.LoadOperationalConfig(townRoot)
		delay = cfg.GetFollowupConfig().DefaultDelayD()
	}

	if delay <= 0 {
		return fmt.Errorf("followup delay must be positive (got %v)", delay)
	}

	dueAt := time.Now().Add(delay)
	f, err := followup.Create(townRoot, agent, topic, dueAt, followupBead)
	if err != nil {
		return fmt.Errorf("creating followup: %w", err)
	}

	fmt.Printf("%s Follow-up scheduled: %s\n", style.Bold.Render("✓"), f.ID)
	fmt.Printf("  Topic: %s\n", topic)
	fmt.Printf("  Due: %s (in %s)\n", dueAt.Format("15:04:05"), delay)
	if followupBead != "" {
		fmt.Printf("  Bead: %s\n", followupBead)
	}
	fmt.Printf("\n  Resolve when done: gt followup resolve %s\n", f.ID)
	return nil
}

func runFollowupList(cmd *cobra.Command, args []string) error {
	townRoot, _, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agent := detectSender()
	if agent == "" {
		return fmt.Errorf("cannot determine agent identity (GT_ROLE not set?)")
	}

	pending, err := followup.ListPending(townRoot, agent)
	if err != nil {
		return fmt.Errorf("listing followups: %w", err)
	}

	if len(pending) == 0 {
		fmt.Println("No pending follow-ups.")
		return nil
	}

	fmt.Printf("%s Pending follow-ups for %s:\n\n", style.Bold.Render("📋"), agent)
	for _, f := range pending {
		status := "pending"
		if f.IsOverdue() {
			status = style.Bold.Render("OVERDUE")
		} else {
			remaining := f.TimeUntilDue().Truncate(time.Second)
			status = fmt.Sprintf("due in %s", remaining)
		}
		fmt.Printf("  %s  %s  [%s]\n", f.ID, f.Topic, status)
		if f.BeadID != "" {
			fmt.Printf("       bead: %s\n", f.BeadID)
		}
	}
	fmt.Println()
	return nil
}

func runFollowupResolve(cmd *cobra.Command, args []string) error {
	id := args[0]

	townRoot, _, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	agent := detectSender()
	if agent == "" {
		return fmt.Errorf("cannot determine agent identity (GT_ROLE not set?)")
	}

	if err := followup.Resolve(townRoot, agent, id); err != nil {
		// Try with different agent identity formats if first attempt fails
		if strings.Contains(err.Error(), "no such file") {
			return fmt.Errorf("followup %s not found for %s", id, agent)
		}
		return fmt.Errorf("resolving followup: %w", err)
	}

	fmt.Printf("%s Follow-up %s resolved\n", style.Bold.Render("✓"), id)
	return nil
}

// checkPendingFollowups returns the count of pending followups for an agent.
// Used by the gt done guard.
func checkPendingFollowups(townRoot, agent string) int {
	pending, err := followup.ListPending(townRoot, agent)
	if err != nil {
		return 0
	}
	return len(pending)
}

// warnNoFollowupOnDefer prints a warning when an agent defers without
// setting a followup. Returns true if the warning was shown.
func warnNoFollowupOnDefer(townRoot, agent string) bool {
	pending := checkPendingFollowups(townRoot, agent)
	if pending > 0 {
		return false // already has followups, no warning needed
	}

	// Only warn for polecat/crew actors
	if !isAgentActor(agent) {
		return false
	}

	cliName := os.Getenv("GT_COMMAND")
	if cliName == "" {
		cliName = "gt"
	}

	fmt.Printf("\n%s Deferring without a follow-up reminder.\n", style.Bold.Render("⚠"))
	fmt.Printf("  Without a follow-up, this work may stall until someone manually prompts you.\n")
	fmt.Printf("  Schedule one now:\n\n")
	fmt.Printf("    %s followup \"<what to check back on>\" --in 30m\n\n", cliName)
	return true
}

// isAgentActor returns true if the actor looks like a polecat or crew member.
func isAgentActor(actor string) bool {
	return strings.Contains(actor, "polecat") || strings.Contains(actor, "crew")
}
