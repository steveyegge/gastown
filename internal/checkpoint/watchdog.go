// Package checkpoint provides session checkpointing for crash recovery.
// The watchdog runs as a background process alongside a polecat session,
// periodically auto-committing uncommitted work as WIP checkpoints.
package checkpoint

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WIPCommitPrefix is the prefix used for automatic WIP checkpoint commits.
// This prefix is used to identify and squash WIP commits during gt done.
const WIPCommitPrefix = "WIP: checkpoint (auto)"

// DefaultWatchdogInterval is the default interval between WIP checkpoint commits.
const DefaultWatchdogInterval = 10 * time.Minute

// WatchdogConfig configures the WIP checkpoint watchdog.
type WatchdogConfig struct {
	// WorkDir is the git working directory to monitor.
	WorkDir string

	// Interval is the time between checkpoint checks.
	Interval time.Duration

	// ExcludePatterns are gitignore-style patterns to exclude from auto-commits.
	// Runtime artifacts (.claude/, .beads/, .runtime/, __pycache__/) are always excluded.
	ExcludePatterns []string
}

// runtimeDirs are directories that should never be auto-committed.
// These are toolchain-managed and normally gitignored.
var runtimeDirs = []string{
	".claude/",
	".beads/",
	".runtime/",
	"__pycache__/",
	".polecat-checkpoint.json",
}

// RunWatchdog runs the WIP checkpoint watchdog loop. It periodically checks for
// uncommitted changes and auto-commits them as WIP checkpoints. The loop exits
// when the context is cancelled.
//
// Returns nil on clean shutdown (context cancelled), error on unexpected failure.
func RunWatchdog(ctx context.Context, cfg WatchdogConfig) error {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultWatchdogInterval
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := doCheckpoint(cfg.WorkDir); err != nil {
				// Log but don't exit — transient git errors shouldn't kill the watchdog.
				fmt.Printf("checkpoint watchdog: %v\n", err)
			}
		}
	}
}

// doCheckpoint checks for uncommitted changes and creates a WIP commit if any exist.
func doCheckpoint(workDir string) error {
	// Check if there are any uncommitted changes (excluding runtime dirs).
	changes, err := hasSignificantChanges(workDir)
	if err != nil {
		return fmt.Errorf("checking changes: %w", err)
	}
	if !changes {
		return nil
	}

	// Stage all changes (git add -A).
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("staging changes: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Unstage runtime directories that shouldn't be committed.
	for _, dir := range runtimeDirs {
		cmd = exec.Command("git", "reset", "HEAD", "--", dir)
		cmd.Dir = workDir
		_ = cmd.Run() // Ignore errors — dir may not exist or not be staged
	}

	// Check if anything is still staged after excluding runtime dirs.
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = workDir
	if err := cmd.Run(); err == nil {
		// Exit status 0 means no staged changes — nothing to commit.
		return nil
	}

	// Create the WIP checkpoint commit.
	msg := fmt.Sprintf("%s\n\nAuto-committed by gt checkpoint watchdog at %s",
		WIPCommitPrefix, time.Now().UTC().Format(time.RFC3339))
	cmd = exec.Command("git", "commit", "-m", msg)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing checkpoint: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// hasSignificantChanges checks if the working tree has uncommitted changes
// that aren't just runtime artifacts.
func hasSignificantChanges(workDir string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		file := strings.TrimSpace(line[3:])
		if file == "" {
			continue
		}

		// Skip runtime directories.
		isRuntime := false
		for _, dir := range runtimeDirs {
			if strings.HasPrefix(file, dir) || file == strings.TrimSuffix(dir, "/") {
				isRuntime = true
				break
			}
		}
		if !isRuntime {
			return true, nil
		}
	}

	return false, nil
}
