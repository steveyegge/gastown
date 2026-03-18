package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/checkpoint"
)

var checkpointWatchdogCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "Run WIP checkpoint watchdog (background daemon)",
	Long: `Run a background watchdog that periodically auto-commits uncommitted work
as WIP checkpoint commits. This protects against session death (crash, context
limit, API error) by ensuring uncommitted work survives on the branch.

WIP checkpoint commits use the message prefix "WIP: checkpoint (auto)" and are
automatically squashed by gt done before pushing to the merge queue.

This command is typically started automatically by the session manager when
spawning a polecat. It can also be run manually for debugging.

The watchdog exits when:
- SIGTERM or SIGINT is received
- The --session's tmux session dies (if specified)`,
	RunE: runCheckpointWatchdog,
}

var (
	watchdogWorkDir  string
	watchdogInterval string
	watchdogSession  string
)

func init() {
	checkpointWatchdogCmd.Flags().StringVar(&watchdogWorkDir, "work-dir", "",
		"Git working directory to monitor (default: current directory)")
	checkpointWatchdogCmd.Flags().StringVar(&watchdogInterval, "interval", "10m",
		"Interval between checkpoint checks (e.g., 5m, 10m, 15m)")
	checkpointWatchdogCmd.Flags().StringVar(&watchdogSession, "session", "",
		"Tmux session to monitor — watchdog exits when session dies")

	checkpointCmd.AddCommand(checkpointWatchdogCmd)
}

func runCheckpointWatchdog(cmd *cobra.Command, args []string) error {
	workDir := watchdogWorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}

	interval, err := time.ParseDuration(watchdogInterval)
	if err != nil {
		return fmt.Errorf("parsing interval %q: %w", watchdogInterval, err)
	}

	// Set up context with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	// If a session is specified, monitor it and exit when it dies.
	if watchdogSession != "" {
		go monitorSession(ctx, cancel, watchdogSession)
	}

	cfg := checkpoint.WatchdogConfig{
		WorkDir:  workDir,
		Interval: interval,
	}

	fmt.Printf("checkpoint watchdog: started (interval=%s, workDir=%s)\n", interval, workDir)
	return checkpoint.RunWatchdog(ctx, cfg)
}

// monitorSession polls for the tmux session and cancels the context when it dies.
func monitorSession(ctx context.Context, cancel context.CancelFunc, sessionID string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if tmux session still exists.
			if err := checkTmuxSession(sessionID); err != nil {
				fmt.Printf("checkpoint watchdog: session %s gone, shutting down\n", sessionID)
				cancel()
				return
			}
		}
	}
}

// checkTmuxSession returns nil if the tmux session exists, error otherwise.
func checkTmuxSession(sessionID string) error {
	cmd := exec.Command("tmux", "has-session", "-t", sessionID)
	return cmd.Run()
}
