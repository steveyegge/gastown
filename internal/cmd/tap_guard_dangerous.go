package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var tapGuardDangerousCmd = &cobra.Command{
	Use:   "dangerous-command",
	Short: "Block destructive shell commands",
	Long: `Block dangerous shell commands that could destroy data or state.

This guard reads the Claude Code hook input from stdin and checks the
command against a blocklist of destructive patterns.

Blocked patterns:
  - rm -rf /           (filesystem destruction)
  - git reset --hard   (discard uncommitted changes)
  - git clean -f       (delete untracked files)
  - git push --force   (overwrite remote history)
  - drop table/database (database destruction)

Exit codes:
  0 - Command allowed
  2 - Command BLOCKED`,
	RunE: runTapGuardDangerous,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardDangerousCmd)
}

// dangerousPattern defines a command pattern to block.
type dangerousPattern struct {
	fragments []string // all fragments must appear in the command
	reason    string
}

var dangerousPatterns = []dangerousPattern{
	{[]string{"rm", "-rf", "/"}, "filesystem destruction (rm -rf /)"},
	{[]string{"git", "reset", "--hard"}, "discards uncommitted changes"},
	{[]string{"git", "clean", "-f"}, "deletes untracked files permanently"},
	{[]string{"git", "push", "--force"}, "overwrites remote history"},
	{[]string{"git", "push", "-f"}, "overwrites remote history"},
	{[]string{"drop", "table"}, "database table destruction"},
	{[]string{"drop", "database"}, "database destruction"},
	{[]string{"truncate", "table"}, "database table truncation"},
}

// preToolUseInput represents the JSON input from Claude Code PreToolUse hooks.
type preToolUseInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

func runTapGuardDangerous(cmd *cobra.Command, args []string) error {
	if !isGasTownAgentContext() {
		return nil
	}

	// Read hook input from stdin
	var input preToolUseInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// Can't parse input — allow (fail open for robustness)
		return nil
	}

	command := strings.ToLower(input.ToolInput.Command)

	for _, pattern := range dangerousPatterns {
		if matchesAll(command, pattern.fragments) {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
			fmt.Fprintln(os.Stderr, "║  ❌ DANGEROUS COMMAND BLOCKED                                    ║")
			fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
			fmt.Fprintf(os.Stderr, "║  Reason: %-54s║\n", pattern.reason)
			fmt.Fprintln(os.Stderr, "║                                                                  ║")
			if len(command) <= 55 {
				fmt.Fprintf(os.Stderr, "║  Command: %-53s║\n", input.ToolInput.Command)
			} else {
				fmt.Fprintf(os.Stderr, "║  Command: %-53s║\n", input.ToolInput.Command[:52]+"…")
			}
			fmt.Fprintln(os.Stderr, "║                                                                  ║")
			fmt.Fprintln(os.Stderr, "║  If this operation is truly needed, ask the human operator.     ║")
			fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
			fmt.Fprintln(os.Stderr, "")
			return NewSilentExit(2)
		}
	}

	return nil
}

// matchesAll returns true if all fragments appear in the command string.
func matchesAll(command string, fragments []string) bool {
	for _, f := range fragments {
		if !strings.Contains(command, strings.ToLower(f)) {
			return false
		}
	}
	return true
}
