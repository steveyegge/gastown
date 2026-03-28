// Package cmd provides gt CLI commands.
//
// bd_passthrough.go adds bead verbs to `gt bd` (aka `gt bead`) that route
// cross-rig bead operations correctly. beads v0.62 removed built-in multi-rig
// routing from the standalone bd CLI, so agents must use `gt bd` instead of
// bare `bd` to ensure bead IDs resolve to the correct rig database.
//
// Each subcommand shells out to bd with the correct working directory and a
// clean BEADS_DIR environment. This is a bridge implementation — PR #3166
// will replace these shell-outs with direct Go module Storage API calls.
//
// Usage: gt bd create, gt bd update, gt bd list, gt bd dep, gt bd comments
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/workspace"
)

// ---------------------------------------------------------------------------
// gt bd create
// ---------------------------------------------------------------------------

var bdCreateCmd = &cobra.Command{
	Use:   "create [title] [flags]",
	Short: "Create a new bead",
	Long: `Create a new bead with cross-rig routing.

Routes to the correct rig database when --rig is specified.
All bd create flags are supported and passed through.

Examples:
  gt bd create "fix: broken login"                # Create in current context
  gt bd create "fix: broken login" --rig gastown  # Create in gastown rig
  gt bd create "my task" --type=bug -p 1          # Bug with P1 priority`,
	DisableFlagParsing: true,
	RunE:               runBdCreatePassthrough,
}

// ---------------------------------------------------------------------------
// gt bd update
// ---------------------------------------------------------------------------

var bdUpdateCmd = &cobra.Command{
	Use:   "update <bead-id> [flags]",
	Short: "Update a bead",
	Long: `Update a bead's fields with cross-rig routing.

Routes to the correct rig database based on the bead ID prefix.
All bd update flags are supported and passed through.

Examples:
  gt bd update gas-abc --status=in_progress
  gt bd update mc-xyz --assignee=me
  gt bd update gt-123 --description="new description"`,
	DisableFlagParsing: true,
	RunE:               runBdPassthrough("update"),
}

// ---------------------------------------------------------------------------
// gt bd list
// ---------------------------------------------------------------------------

var bdListCmd = &cobra.Command{
	Use:   "list [flags]",
	Short: "List beads",
	Long: `List beads with cross-rig routing.

Use --rig to target a specific rig's database. Without --rig, lists
beads in the current context (HQ from town root, rig from rig dir).

All bd list flags are supported and passed through.

Examples:
  gt bd list                           # List in current context
  gt bd list --status=open             # Open beads only
  gt bd list --rig gastown             # List gastown rig beads
  gt bd list --json                    # JSON output`,
	DisableFlagParsing: true,
	RunE:               runBdList,
}

// ---------------------------------------------------------------------------
// gt bd dep
// ---------------------------------------------------------------------------

var bdDepCmd = &cobra.Command{
	Use:   "dep [add|remove|list] <bead-id> [depends-on-id] [flags]",
	Short: "Manage bead dependencies",
	Long: `Manage bead dependencies with cross-rig routing.

Routes to the correct rig database based on the first bead ID prefix.
All bd dep subcommand flags are supported and passed through.

Examples:
  gt bd dep add gas-abc gas-def               # gas-abc depends on gas-def
  gt bd dep add gas-abc gas-def --type=tracks # Tracking relation
  gt bd dep remove gas-abc gas-def            # Remove dependency
  gt bd dep list gas-abc                      # List dependencies`,
	DisableFlagParsing: true,
	RunE:               runBdPassthrough("dep"),
}

// ---------------------------------------------------------------------------
// gt bd close
// ---------------------------------------------------------------------------

var bdCloseCmd = &cobra.Command{
	Use:   "close <bead-id> [flags]",
	Short: "Close one or more beads",
	Long: `Close beads with cross-rig routing.

Routes to the correct rig database based on the bead ID prefix.
All bd close flags are supported and passed through.

Examples:
  gt bd close gas-abc
  gt bd close gas-abc --reason "Done"
  gt bd close gas-abc gas-def         # Close multiple`,
	DisableFlagParsing: true,
	RunE:               runBdPassthrough("close"),
}

// ---------------------------------------------------------------------------
// gt bd ready
// ---------------------------------------------------------------------------

var bdReadyCmd = &cobra.Command{
	Use:   "ready [flags]",
	Short: "Show beads ready for work",
	Long: `List beads that are ready to work (no blockers).

Use --rig to target a specific rig's database.
All bd ready flags are supported and passed through.

Examples:
  gt bd ready                         # Ready beads in current context
  gt bd ready --rig gastown           # Ready beads in gastown`,
	DisableFlagParsing: true,
	RunE:               runBdReady,
}

// ---------------------------------------------------------------------------
// gt bd comments
// ---------------------------------------------------------------------------

var bdCommentsCmd = &cobra.Command{
	Use:   "comments [add] <bead-id> [text] [flags]",
	Short: "Manage bead comments",
	Long: `View or add comments on a bead with cross-rig routing.

Routes to the correct rig database based on the bead ID prefix.
All bd comments flags are supported and passed through.

Examples:
  gt bd comments gas-abc                      # List comments
  gt bd comments add gas-abc "Working on it"  # Add a comment
  gt bd comments add gas-abc -f notes.txt     # Add from file`,
	DisableFlagParsing: true,
	RunE:               runBdPassthrough("comments"),
}

// ---------------------------------------------------------------------------
// Registration — add to existing beadCmd (gt bead / gt bd)
// ---------------------------------------------------------------------------

func init() {
	beadCmd.AddCommand(bdCreateCmd)
	beadCmd.AddCommand(bdUpdateCmd)
	beadCmd.AddCommand(bdListCmd)
	beadCmd.AddCommand(bdCloseCmd)
	beadCmd.AddCommand(bdReadyCmd)
	beadCmd.AddCommand(bdDepCmd)
	beadCmd.AddCommand(bdCommentsCmd)
}

// ---------------------------------------------------------------------------
// Passthrough execution
// ---------------------------------------------------------------------------

// runBdPassthrough returns a RunE function that passes all args to the given
// bd subcommand with cross-rig routing. The first non-flag argument that looks
// like a bead ID is used for prefix-based routing via resolveBeadDir().
func runBdPassthrough(subcommand string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
			return err
		}

		bdArgs := append([]string{subcommand}, args...)
		bdCmd := exec.Command("bd", bdArgs...)
		bdCmd.Stdin = os.Stdin
		bdCmd.Stdout = os.Stdout
		bdCmd.Stderr = os.Stderr

		// Route to the correct rig database if any arg looks like a bead ID.
		if beadID := firstBeadIDArg(args); beadID != "" {
			if dir := resolveBeadDir(beadID); dir != "" && dir != "." {
				bdCmd.Dir = dir
				bdCmd.Env = filterEnvKey(os.Environ(), "BEADS_DIR")
			}
		}

		return bdCmd.Run()
	}
}

// runBdCreatePassthrough handles gt bd create, which supports --rig for
// targeting a specific rig's database.
func runBdCreatePassthrough(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	// Extract --rig flag (gt-specific, not passed to bd).
	rigName, filteredArgs := extractRigFlag(args)

	bdArgs := append([]string{"create"}, filteredArgs...)
	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdin = os.Stdin
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr

	if rigName != "" {
		if dir := resolveRigDir(rigName); dir != "" {
			bdCmd.Dir = dir
			bdCmd.Env = filterEnvKey(os.Environ(), "BEADS_DIR")
		} else {
			return fmt.Errorf("unknown rig %q — check gt rig list", rigName)
		}
	}

	return bdCmd.Run()
}

// runBdList handles gt bd list, which supports --rig for cross-rig listing.
func runBdList(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	// Extract --rig flag (gt-specific, not passed to bd).
	rigName, filteredArgs := extractRigFlag(args)

	bdArgs := append([]string{"list"}, filteredArgs...)
	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdin = os.Stdin
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr

	if rigName != "" {
		if dir := resolveRigDir(rigName); dir != "" {
			bdCmd.Dir = dir
			bdCmd.Env = filterEnvKey(os.Environ(), "BEADS_DIR")
		} else {
			return fmt.Errorf("unknown rig %q — check gt rig list", rigName)
		}
	}

	return bdCmd.Run()
}

// runBdReady handles gt bd ready, which supports --rig for cross-rig queries.
func runBdReady(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	rigName, filteredArgs := extractRigFlag(args)

	bdArgs := append([]string{"ready"}, filteredArgs...)
	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdin = os.Stdin
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr

	if rigName != "" {
		if dir := resolveRigDir(rigName); dir != "" {
			bdCmd.Dir = dir
			bdCmd.Env = filterEnvKey(os.Environ(), "BEADS_DIR")
		} else {
			return fmt.Errorf("unknown rig %q — check gt rig list", rigName)
		}
	}

	return bdCmd.Run()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractRigFlag removes --rig / --rig=value from args and returns the rig name.
func extractRigFlag(args []string) (string, []string) {
	var rigName string
	var filtered []string
	skipNext := false
	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--rig" && i+1 < len(args) {
			rigName = args[i+1]
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--rig=") {
			rigName = strings.TrimPrefix(arg, "--rig=")
			continue
		}
		filtered = append(filtered, arg)
	}
	return rigName, filtered
}

// resolveRigDir resolves a rig name to the directory containing its .beads
// database. Checks candidates in order, returning the first that has a .beads
// subdirectory so bd can discover the database.
func resolveRigDir(rigName string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}
	candidates := []string{
		townRoot + "/" + rigName + "/mayor/rig",
		townRoot + "/" + rigName,
	}
	for _, dir := range candidates {
		beadsDir := dir + "/.beads"
		if fi, err := os.Stat(beadsDir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}

// firstBeadIDArg returns the first argument that looks like a bead ID
// (contains a hyphen, starts with lowercase letters). Skips flags and
// bd subcommands (add, remove, list).
func firstBeadIDArg(args []string) string {
	subcommands := map[string]bool{
		"add": true, "remove": true, "list": true,
	}
	// Flags known to consume a following value argument.
	valueFlags := map[string]bool{
		"--status": true, "--assignee": true, "--type": true,
		"--priority": true, "-p": true, "--description": true, "-d": true,
		"--title": true, "--reason": true, "-r": true, "--author": true,
		"--file": true, "-f": true, "--parent": true, "--rig": true,
		"--repo": true, "--direction": true, "--limit": true, "-n": true,
		"--label": true, "-l": true, "--due": true, "--actor": true,
		"--db": true, "--dolt-auto-commit": true, "--id": true,
	}
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(arg, "=") && valueFlags[arg] {
				skipNext = true
			}
			continue
		}
		if subcommands[arg] {
			continue
		}
		if isBeadID(arg) {
			return arg
		}
	}
	return ""
}
