package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/session"
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

	t := tmux.NewTmux()
	sess, err := t.CurrentSessionName()
	if err != nil {
		return err
	}

	if err := t.SetEnv(session.SessionID(sess), "GT_ISSUE", issueID); err != nil {
		return fmt.Errorf("setting issue: %w", err)
	}

	fmt.Printf("Issue set to: %s\n", issueID)
	return nil
}

func runIssueClear(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()
	sess, err := t.CurrentSessionName()
	if err != nil {
		return err
	}

	// Set to empty string to clear
	if err := t.SetEnv(session.SessionID(sess), "GT_ISSUE", ""); err != nil {
		return fmt.Errorf("clearing issue: %w", err)
	}

	fmt.Println("Issue cleared")
	return nil
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()
	sess, err := t.CurrentSessionName()
	if err != nil {
		return err
	}

	issue, err := t.GetEnvironment(sess, "GT_ISSUE")
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

