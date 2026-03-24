//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// execBdShow replaces the current process with 'bd show'.
// Resolves the correct rig directory from the bead's prefix via routes.jsonl
// so that rig-prefixed beads (e.g., myproject-abc) are found in their rig
// database rather than only the town-level hq database. (GH#2126)
func execBdShow(args []string) error {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return fmt.Errorf("bd not found in PATH: %w", err)
	}

	if beadID := extractBeadIDFromArgs(args); beadID != "" {
		if dir := resolveBeadDir(beadID); dir != "" && dir != "." {
			_ = os.Chdir(dir)
		}
	}

	env := stripEnvKey(os.Environ(), "BEADS_DIR")

	// Build args: bd show <all-args>
	// argv[0] must be the program name for exec
	fullArgs := append([]string{"bd", "show"}, args...)

	return syscall.Exec(bdPath, fullArgs, env)
}
