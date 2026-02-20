package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	beadsdk "github.com/steveyegge/beads"
	"github.com/steveyegge/gastown/internal/convoy"
	"github.com/steveyegge/gastown/internal/workspace"

	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:     "close [bead-id...]",
	GroupID: GroupWork,
	Short:   "Close one or more beads",
	Long: `Close one or more beads (wrapper for 'bd close').

This is a convenience command that passes through to 'bd close' with
all arguments and flags preserved.

When an issue is closed, any convoys tracking it are checked for
completion. If all tracked issues in a convoy are closed, the convoy
is auto-closed.

Examples:
  gt close gt-abc              # Close bead gt-abc
  gt close gt-abc gt-def       # Close multiple beads
  gt close --reason "Done"     # Close with reason
  gt close --comment "Done"    # Same as --reason (alias)
  gt close --force             # Force close pinned beads`,
	DisableFlagParsing: true, // Pass all flags through to bd close
	RunE:               runClose,
}

func init() {
	rootCmd.AddCommand(closeCmd)
}

func runClose(cmd *cobra.Command, args []string) error {
	// Handle --help since DisableFlagParsing bypasses Cobra's help handling
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	// Convert --comment to --reason (alias support)
	convertedArgs := make([]string, len(args))
	for i, arg := range args {
		if arg == "--comment" {
			convertedArgs[i] = "--reason"
		} else if strings.HasPrefix(arg, "--comment=") {
			convertedArgs[i] = "--reason=" + strings.TrimPrefix(arg, "--comment=")
		} else {
			convertedArgs[i] = arg
		}
	}

	// Build bd close command with all args passed through
	bdArgs := append([]string{"close"}, convertedArgs...)
	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdin = os.Stdin
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr
	if err := bdCmd.Run(); err != nil {
		return err
	}

	// After successful close, check convoy completion for each closed issue.
	// This is best-effort: failures are silently ignored since the daemon's
	// event polling and deacon patrol serve as backup mechanisms.
	beadIDs := extractBeadIDs(args)
	if len(beadIDs) > 0 {
		checkConvoyCompletion(beadIDs)
	}

	return nil
}

// extractBeadIDs extracts bead IDs from raw args, skipping flags and flag values.
// Since DisableFlagParsing is true, we get unparsed args and must identify flags manually.
func extractBeadIDs(args []string) []string {
	// Flags that consume a following argument (value flags without = form)
	valueFlags := map[string]bool{
		"--reason": true, "-r": true,
		"--session": true,
		"--actor": true,
		"--db": true,
		"--dolt-auto-commit": true,
		// Also handle the --comment alias (before conversion)
		"--comment": true,
	}

	var ids []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Check for --flag=value (consumed in one token)
			if strings.Contains(arg, "=") {
				continue
			}
			// Check if this flag takes a value argument
			if valueFlags[arg] {
				skipNext = true
			}
			continue
		}
		ids = append(ids, arg)
	}
	return ids
}

// checkConvoyCompletion checks if any closed issues are tracked by convoys
// and triggers convoy completion checks. This implements the ZFC principle:
// the closure event propagates at the source (bd close) rather than relying
// solely on daemon event polling.
//
// This is best-effort. If the workspace or hq store is unavailable, the
// daemon's event polling and deacon patrol serve as backup mechanisms.
func checkConvoyCompletion(beadIDs []string) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return
	}

	hqBeadsDir := filepath.Join(townRoot, ".beads")
	ctx := context.Background()

	store, err := beadsdk.Open(ctx, hqBeadsDir)
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()

	gtPath, err := os.Executable()
	if err != nil {
		gtPath, _ = exec.LookPath("gt")
		if gtPath == "" {
			return
		}
	}

	for _, beadID := range beadIDs {
		convoy.CheckConvoysForIssue(ctx, store, townRoot, beadID, "Close", nil, gtPath, nil)
	}
}
