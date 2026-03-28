package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	showCmd.GroupID = GroupWork
	rootCmd.AddCommand(showCmd)
}

var showCmd = &cobra.Command{
	Use:   "show <bead-id> [flags]",
	Short: "Show details of a bead",
	Long: `Displays the full details of a bead by ID.

Delegates to 'bd show' - all bd show flags are supported.
Works with any bead prefix (gt-, bd-, hq-, etc.) and routes
to the correct beads database automatically.

Examples:
  gt show gt-abc123          # Show a gastown issue
  gt show hq-xyz789          # Show a town-level bead (convoy, mail, etc.)
  gt show bd-def456          # Show a beads issue
  gt show gt-abc123 --json   # Output as JSON
  gt show gt-abc123 -v       # Verbose output`,
	DisableFlagParsing: true, // Pass all flags through to bd show
	RunE:               runShow,
}

func runShow(cmd *cobra.Command, args []string) error {
	// Handle --help since DisableFlagParsing bypasses Cobra's help handling
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("bead ID required\n\nUsage: gt show <bead-id> [flags]")
	}

	return execBdShow(args)
}

// execBdShow replaces the current process with 'bd show'.
// Resolves the correct rig directory from the bead's prefix via routes.jsonl
// so that rig-prefixed beads (e.g., myproject-abc) are found in their rig
// database rather than only the town-level hq database. (GH#2126)
func execBdShow(args []string) error {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return fmt.Errorf("bd not found in PATH: %w", err)
	}

	// Resolve the rig directory for the bead's prefix so bd runs from the
	// correct working directory. Without this, bd may query the wrong database
	// when inherited BEADS_DIR is set or when bd's routing doesn't handle
	// cross-rig lookups from the town root.
	if beadID := extractBeadIDFromArgs(args); beadID != "" {
		if dir := resolveBeadDir(beadID); dir != "" && dir != "." {
			_ = os.Chdir(dir)
		}
	}

	// Strip BEADS_DIR from the environment so bd discovers the database from
	// its working directory rather than using an inherited value that may point
	// to the wrong (e.g., town-level) database.
	env := stripEnvKey(os.Environ(), "BEADS_DIR")

	// Build args: bd show <all-args>
	// argv[0] must be the program name for exec
	fullArgs := append([]string{"bd", "show"}, args...)

	return syscall.Exec(bdPath, fullArgs, env)
}

// extractBeadIDFromArgs returns the first non-flag argument, which is the bead ID.
// Returns empty string if no non-flag argument is found.
func extractBeadIDFromArgs(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

// stripEnvKey removes all entries matching the given key from an environment slice.
func stripEnvKey(env []string, key string) []string { //nolint:unparam // key is always BEADS_DIR today but the function is intentionally generic
	prefix := key + "="
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}
