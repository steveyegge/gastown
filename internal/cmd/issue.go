package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/tmux"
)

var issueCmd = &cobra.Command{
	Use:     "issue",
	GroupID: GroupConfig,
	Short:   "Manage current issue for status line display",
}

var issueSetCmd = &cobra.Command{
	Use:   "set <issue-id>",
	Short: "Set the current issue (shown in tmux status line)",
	Args:  cobra.ExactArgs(1),
	RunE:  runIssueSet,
}

var issueClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the current issue from status line",
	RunE:  runIssueClear,
}

var issueShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current issue",
	RunE:  runIssueShow,
}

func init() {
	rootCmd.AddCommand(issueCmd)
	issueCmd.AddCommand(issueSetCmd)
	issueCmd.AddCommand(issueClearCmd)
	issueCmd.AddCommand(issueShowCmd)
}

func runIssueSet(cmd *cobra.Command, args []string) error {
	issueID := args[0]

	// Get current tmux session
	session := os.Getenv("TMUX_PANE")
	if session == "" {
		// Try to detect from GT env vars
		session = detectCurrentSession()
		if session == "" {
			return fmt.Errorf("not in a tmux session")
		}
	}

	t := tmux.NewTmux()
	if err := t.SetEnvironment(session, "GT_ISSUE", issueID); err != nil {
		return fmt.Errorf("setting issue: %w", err)
	}

	fmt.Printf("Issue set to: %s\n", issueID)
	return nil
}

func runIssueClear(cmd *cobra.Command, args []string) error {
	session := os.Getenv("TMUX_PANE")
	if session == "" {
		session = detectCurrentSession()
		if session == "" {
			return fmt.Errorf("not in a tmux session")
		}
	}

	t := tmux.NewTmux()
	// Set to empty string to clear
	if err := t.SetEnvironment(session, "GT_ISSUE", ""); err != nil {
		return fmt.Errorf("clearing issue: %w", err)
	}

	fmt.Println("Issue cleared")
	return nil
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	session := os.Getenv("TMUX_PANE")
	if session == "" {
		session = detectCurrentSession()
		if session == "" {
			return fmt.Errorf("not in a tmux session")
		}
	}

	t := tmux.NewTmux()
	issue, err := t.GetEnvironment(session, "GT_ISSUE")
	if err != nil {
		return fmt.Errorf("getting issue: %w", err)
	}

	if issue == "" {
		fmt.Println("No issue set")
	} else {
		fmt.Printf("Current issue: %s\n", issue)
	}
	return nil
}

// detectCurrentSession tries to find the tmux session name from env.
func detectCurrentSession() string {
	// Try to build session name from GT env vars
	rig := os.Getenv("GT_RIG")
	polecat := os.Getenv("GT_POLECAT")
	crew := os.Getenv("GT_CREW")

	if rig != "" {
		if polecat != "" {
			return fmt.Sprintf("gt-%s-%s", rig, polecat)
		}
		if crew != "" {
			return fmt.Sprintf("gt-%s-crew-%s", rig, crew)
		}
	}

	// Check if we're mayor
	if os.Getenv("GT_ROLE") == "mayor" {
		return getMayorSessionName()
	}

	return ""
}
