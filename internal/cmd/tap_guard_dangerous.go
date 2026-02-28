package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var tapGuardDangerousCmd = &cobra.Command{
	Use:   "dangerous-command",
	Short: "Block dangerous commands (rm -rf, force push, etc.)",
	Long: `Block dangerous commands via Claude Code PreToolUse hooks.

This guard blocks operations that could cause irreversible damage:
  - rm -rf with absolute paths (e.g., rm -rf /path)
  - git push --force / git push -f
  - git reset --hard
  - git clean -f / git clean -fd

The guard reads the tool input from stdin (Claude Code hook protocol)
and exits with code 2 to block dangerous operations.

Exit codes:
  0 - Operation allowed
  2 - Operation BLOCKED`,
	RunE: runTapGuardDangerous,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardDangerousCmd)
}

// dangerousPattern defines a pattern to match and its human-readable reason.
type dangerousPattern struct {
	contains []string // all substrings must be present
	reason   string
}

var dangerousPatterns = []dangerousPattern{
	{
		contains: []string{"rm", "-rf", "/"},
		reason:   "rm -rf with absolute path can destroy system files",
	},
	{
		contains: []string{"git", "push", "--force"},
		reason:   "Force push rewrites remote history and can destroy others' work",
	},
	{
		contains: []string{"git", "push", "-f"},
		reason:   "Force push rewrites remote history and can destroy others' work",
	},
	{
		contains: []string{"git", "reset", "--hard"},
		reason:   "Hard reset discards all uncommitted changes irreversibly",
	},
	{
		contains: []string{"git", "clean", "-f"},
		reason:   "git clean -f deletes untracked files irreversibly",
	},
}

func runTapGuardDangerous(cmd *cobra.Command, args []string) error {
	// Read hook input from stdin (Claude Code protocol)
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Can't read stdin — allow operation (fail open for non-hook usage)
		return nil
	}

	// Extract the command from the hook input
	command := extractCommand(input)
	if command == "" {
		// No command found — allow operation
		return nil
	}

	// Check against dangerous patterns
	for _, pattern := range dangerousPatterns {
		if matchesDangerous(command, pattern) {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
			fmt.Fprintln(os.Stderr, "║  ❌ DANGEROUS COMMAND BLOCKED                                    ║")
			fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
			fmt.Fprintf(os.Stderr, "║  Command: %-53s ║\n", truncateStr(command, 53))
			fmt.Fprintf(os.Stderr, "║  Reason:  %-53s ║\n", truncateStr(pattern.reason, 53))
			fmt.Fprintln(os.Stderr, "║                                                                  ║")
			fmt.Fprintln(os.Stderr, "║  If this is intentional, ask the user to run it manually.        ║")
			fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
			fmt.Fprintln(os.Stderr, "")
			return NewSilentExit(2) // Exit 2 = BLOCK
		}
	}

	// Not dangerous — allow
	return nil
}

// extractCommand extracts the bash command from Claude Code hook input JSON.
// The input format is: {"tool_name": "Bash", "tool_input": {"command": "..."}}
func extractCommand(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	var hookInput struct {
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}

	if err := json.Unmarshal(input, &hookInput); err != nil {
		return ""
	}

	return hookInput.ToolInput.Command
}

// matchesDangerous checks if a command matches a dangerous pattern.
// All substrings in the pattern must be present in the command.
func matchesDangerous(command string, pattern dangerousPattern) bool {
	lower := strings.ToLower(command)
	for _, substr := range pattern.contains {
		if !strings.Contains(lower, strings.ToLower(substr)) {
			return false
		}
	}
	return true
}

