package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/advice"
	"github.com/steveyegge/gastown/internal/style"
)

var adviceCmd = &cobra.Command{
	Use:   "advice",
	Short: "Manage advice hooks",
	Long:  `Commands for working with advice hooks.`,
}

var adviceRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute advice hooks for a trigger",
	Long: `Execute all advice hooks subscribed by this agent for a specific trigger.

Triggers:
  session-end      Run at session end (Claude Code Stop hook)
  before-commit    Run before committing (gt done)
  before-push      Run before pushing (gt done)
  before-handoff   Run before handoff (gt handoff)

Examples:
  gt advice run --trigger session-end
  gt advice run --trigger before-commit

This command is typically called by Claude Code hooks, not directly by users.`,
	RunE: runAdviceRun,
}

var (
	adviceRunTrigger string
	adviceRunQuiet   bool
)

func init() {
	adviceRunCmd.Flags().StringVar(&adviceRunTrigger, "trigger", "", "Hook trigger (session-end, before-commit, before-push, before-handoff)")
	adviceRunCmd.Flags().BoolVarP(&adviceRunQuiet, "quiet", "q", false, "Suppress output except for errors")
	_ = adviceRunCmd.MarkFlagRequired("trigger")

	adviceCmd.AddCommand(adviceRunCmd)
	rootCmd.AddCommand(adviceCmd)
}

func runAdviceRun(cmd *cobra.Command, args []string) error {
	// Validate trigger
	if !advice.IsValidTrigger(adviceRunTrigger) {
		return fmt.Errorf("invalid trigger: %s (valid: %v)", adviceRunTrigger, advice.ValidTriggers)
	}

	// Get agent ID from environment
	agentID := os.Getenv("BD_ACTOR")
	if agentID == "" {
		// Try to detect from role info
		if roleInfo, err := GetRole(); err == nil {
			agentID = buildAgentID(roleInfo)
		}
	}
	if agentID == "" {
		// Non-fatal: no agent context means no hooks to run
		if !adviceRunQuiet {
			fmt.Println("No agent context (BD_ACTOR not set), skipping hooks")
		}
		return nil
	}

	// Get working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Query and run hooks
	results, blockErr := advice.RunHooksForTrigger(cwd, agentID, adviceRunTrigger)

	// Report results
	if len(results) > 0 && !adviceRunQuiet {
		for _, result := range results {
			if result.Success {
				fmt.Printf("%s Hook %s completed (%v)\n",
					style.Bold.Render("✓"), result.Hook.Title, result.Duration)
			} else if result.TimedOut {
				style.PrintWarning("hook %s timed out after %v", result.Hook.Title, result.Duration)
			} else if result.Error != nil {
				style.PrintWarning("hook %s error: %v", result.Hook.Title, result.Error)
			} else {
				// Command failed (non-zero exit)
				if result.Hook.OnFailure == advice.OnFailureBlock {
					fmt.Printf("%s Hook %s BLOCKED (exit %d)\n",
						style.Bold.Render("✗"), result.Hook.Title, result.ExitCode)
				} else {
					style.PrintWarning("hook %s failed (exit %d): %s",
						result.Hook.Title, result.ExitCode, advice.TruncateOutput(result.Output, 100))
				}
			}
		}
	}

	// Return blocking error if any
	return blockErr
}
