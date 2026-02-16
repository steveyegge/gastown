package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var tapGuardCmd = &cobra.Command{
	Use:   "guard",
	Short: "Block forbidden operations (PreToolUse hook)",
	Long: `Block forbidden operations via Claude Code PreToolUse hooks.

Guard commands exit with code 2 to BLOCK tool execution when a policy
is violated. They're called before the tool runs, preventing the
forbidden operation entirely.

Available guards:
  pr-workflow      - Block PR creation and feature branches
  task-dispatch    - Block Task tool for Mayor (use gt sling instead)

Example hook configuration:
  {
    "PreToolUse": [{
      "matcher": "Bash(gh pr create*)",
      "hooks": [{"command": "gt tap guard pr-workflow"}]
    }]
  }`,
}

var tapGuardPRWorkflowCmd = &cobra.Command{
	Use:   "pr-workflow",
	Short: "Block PR creation and feature branches",
	Long: `Block PR workflow operations in Gas Town.

Gas Town workers push directly to main. PRs add friction that breaks
the autonomous execution model (GUPP principle).

This guard blocks:
  - gh pr create
  - git checkout -b (feature branches)
  - git switch -c (feature branches)

Exit codes:
  0 - Operation allowed (not in Gas Town agent context)
  2 - Operation BLOCKED (in agent context)

The guard only blocks when running as a Gas Town agent (crew, polecat,
witness, etc.). Humans running outside Gas Town can still use PRs.`,
	RunE: runTapGuardPRWorkflow,
}

var tapGuardTaskDispatchCmd = &cobra.Command{
	Use:   "task-dispatch",
	Short: "Block Task tool for Mayor (use gt sling instead)",
	Long: `Block the Claude Code Task tool when running as Mayor.

The Mayor should dispatch work via gt sling, not by spawning Claude Code
subagents. Subagents block the Mayor's context until they finish, breaking
GUPP (the propulsion principle) and preventing parallel polecat execution.

This guard blocks:
  - Task tool invocations (Claude Code subagent spawning)

Exit codes:
  0 - Operation allowed (not Mayor)
  2 - Operation BLOCKED (Mayor context detected)

The guard only blocks when GT_ROLE=mayor. Crew members and polecats
can still use the Task tool for parallel research.

See: https://github.com/steveyegge/gastown/issues/904`,
	RunE: runTapGuardTaskDispatch,
}

func init() {
	tapCmd.AddCommand(tapGuardCmd)
	tapGuardCmd.AddCommand(tapGuardPRWorkflowCmd)
	tapGuardCmd.AddCommand(tapGuardTaskDispatchCmd)
}

func runTapGuardPRWorkflow(cmd *cobra.Command, args []string) error {
	// Check if we're in a Gas Town agent context
	if !isGasTownAgentContext() {
		// Not in a Gas Town managed context - allow the operation
		return nil
	}

	// We're in a Gas Town context - block PR operations
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(os.Stderr, "║  ❌ PR WORKFLOW BLOCKED                                          ║")
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintln(os.Stderr, "║  Gas Town workers push directly to main. PRs are forbidden.     ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Instead of:  gh pr create / git checkout -b / git switch -c    ║")
	fmt.Fprintln(os.Stderr, "║  Do this:     git add . && git commit && git push origin main   ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Why? PRs add friction that breaks autonomous execution.        ║")
	fmt.Fprintln(os.Stderr, "║  See: ~/gt/docs/PRIMING.md (GUPP principle)                     ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")
	return NewSilentExit(2) // Exit 2 = BLOCK in Claude Code hooks
}

func runTapGuardTaskDispatch(cmd *cobra.Command, args []string) error {
	// Only block when running as Mayor
	if os.Getenv("GT_ROLE") != "mayor" {
		return nil
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(os.Stderr, "║  TASK TOOL BLOCKED -- Mayor cannot spawn subagents              ║")
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintln(os.Stderr, "║  Subagents block your context and prevent parallel execution.    ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Instead of:  Task tool (spawning a subagent)                   ║")
	fmt.Fprintln(os.Stderr, "║  Do this:     bd create \"...\" && gt sling <bead-id> <rig>       ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Why? Polecats execute in parallel without blocking the Mayor.  ║")
	fmt.Fprintln(os.Stderr, "║  See: https://github.com/steveyegge/gastown/issues/904          ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")
	return NewSilentExit(2) // Exit 2 = BLOCK in Claude Code hooks
}

// isGasTownAgentContext returns true if we're running as a Gas Town managed agent.
func isGasTownAgentContext() bool {
	// Check environment variables set by Gas Town session management
	envVars := []string{
		"GT_POLECAT",
		"GT_CREW",
		"GT_WITNESS",
		"GT_REFINERY",
		"GT_MAYOR",
		"GT_DEACON",
	}
	for _, env := range envVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	// Also check if we're in a crew or polecat worktree by path
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	agentPaths := []string{"/crew/", "/polecats/"}
	for _, path := range agentPaths {
		if strings.Contains(cwd, path) {
			return true
		}
	}

	return false
}
