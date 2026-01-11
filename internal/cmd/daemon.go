package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/ui"
	"github.com/steveyegge/gastown/internal/workspace"
)

var daemonCmd = &cobra.Command{
	Use:     "daemon",
	GroupID: GroupServices,
	Short:   "Manage the Gas Town daemon",
	RunE:    requireSubcommand,
	Long: `Manage the Gas Town background daemon.

The daemon is a simple Go process that:
- Pokes agents periodically (heartbeat)
- Processes lifecycle requests (cycle, restart, shutdown)
- Restarts sessions when agents request cycling

The daemon is a "dumb scheduler" - all intelligence is in agents.`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	Long: `Start the Gas Town daemon in the background.

The daemon will run until stopped with 'gt daemon stop'.`,
	RunE: runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon",
	Long:  `Stop the running Gas Town daemon.`,
	RunE:  runDaemonStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Long:  `Show the current status of the Gas Town daemon.`,
	RunE:  runDaemonStatus,
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	Long:  `View the daemon log file.`,
	RunE:  runDaemonLogs,
}

var daemonRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon in foreground (internal)",
	Hidden: true,
	RunE:   runDaemonRun,
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon",
	Long:  `Stop and start the daemon. Useful after upgrading gt.`,
	RunE:  runDaemonRestart,
}

var (
	daemonLogLines  int
	daemonLogFollow bool
)

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonRunCmd)

	daemonLogsCmd.Flags().IntVarP(&daemonLogLines, "lines", "n", 50, "Number of lines to show")
	daemonLogsCmd.Flags().BoolVarP(&daemonLogFollow, "follow", "f", false, "Follow log output")

	rootCmd.AddCommand(daemonCmd)
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Check if already running
	running, pid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}
	if running {
		return fmt.Errorf("daemon already running (PID %d)", pid)
	}

	// Start daemon in background
	// We use 'gt daemon run' as the actual daemon process
	gtPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	daemonCmd := exec.Command(gtPath, "daemon", "run")
	daemonCmd.Dir = townRoot

	// Detach from terminal
	daemonCmd.Stdin = nil
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil

	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Wait a moment for the daemon to initialize and acquire the lock
	time.Sleep(200 * time.Millisecond)

	// Verify it started
	running, pid, err = daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}
	if !running {
		return fmt.Errorf("daemon failed to start (check logs with 'gt daemon logs')")
	}

	// Check if our spawned process is the one that won the race.
	// If another concurrent start won, our process would have exited after
	// failing to acquire the lock, and the PID file would have a different PID.
	if pid != daemonCmd.Process.Pid {
		// Another daemon won the race - that's fine, report it
		fmt.Printf("%s Daemon already running (PID %d)\n", ui.RenderWarnIcon(), pid)
		return nil
	}

	fmt.Printf("%s Daemon started (PID %d, v%s)\n", ui.RenderPassIcon(), pid, Version)
	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	running, pid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}
	if !running {
		return fmt.Errorf("daemon is not running")
	}

	if err := daemon.StopDaemon(townRoot); err != nil {
		return fmt.Errorf("stopping daemon: %w", err)
	}

	fmt.Printf("%s Daemon stopped (was PID %d)\n", ui.RenderPassIcon(), pid)
	return nil
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	running, pid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}

	if running {
		// Load state for more details
		state, err := daemon.LoadState(townRoot)

		// Header line with semantic styling
		fmt.Printf("%s Daemon running (PID %d, v%s)\n",
			ui.RenderPassIcon(), pid, Version)
		fmt.Println()

		// Details with aligned labels
		fmt.Printf("  Workspace:  %s\n", ui.ShortenPath(townRoot))

		if err == nil && !state.StartedAt.IsZero() {
			fmt.Printf("  Started:    %s (%s)\n",
				state.StartedAt.Format("2006-01-02 15:04:05"),
				ui.RelativeTime(state.StartedAt))

			if !state.LastHeartbeat.IsZero() {
				fmt.Printf("  Heartbeat:  #%d (%s)\n",
					state.HeartbeatCount,
					ui.RelativeTime(state.LastHeartbeat))
			}
		}

		// Log file location (shortened path)
		logFile := filepath.Join(townRoot, "daemon", "daemon.log")
		fmt.Printf("  Log:        %s\n", ui.ShortenPath(logFile))

		// Check if binary is newer than process (version mismatch warning)
		if err == nil && !state.StartedAt.IsZero() {
			if binaryModTime, err := getBinaryModTime(); err == nil {
				if binaryModTime.After(state.StartedAt) {
					fmt.Println()
					fmt.Printf("  %s Binary updated since daemon start\n", ui.RenderWarnIcon())
					fmt.Printf("    Run: %s\n", ui.RenderMuted("gt daemon restart"))
				}
			}
		}
	} else {
		fmt.Printf("%s Daemon not running\n", ui.RenderMuted("○"))
		fmt.Println()
		fmt.Printf("  Workspace:  %s\n", ui.ShortenPath(townRoot))
		fmt.Println()
		fmt.Printf("  Start with: %s\n", ui.RenderMuted("gt daemon start"))
	}

	return nil
}

// getBinaryModTime returns the modification time of the current executable
func getBinaryModTime() (time.Time, error) {
	exePath, err := os.Executable()
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func runDaemonLogs(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	logFile := filepath.Join(townRoot, "daemon", "daemon.log")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", logFile)
	}

	if daemonLogFollow {
		// Use tail -f for following
		tailCmd := exec.Command("tail", "-f", logFile)
		tailCmd.Stdout = os.Stdout
		tailCmd.Stderr = os.Stderr
		return tailCmd.Run()
	}

	// Use tail -n for last N lines
	tailCmd := exec.Command("tail", "-n", fmt.Sprintf("%d", daemonLogLines), logFile)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr
	return tailCmd.Run()
}

func runDaemonRun(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := daemon.DefaultConfig(townRoot)
	d, err := daemon.New(config)
	if err != nil {
		return fmt.Errorf("creating daemon: %w", err)
	}

	return d.Run()
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Check if running and stop if so
	running, pid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}

	if running {
		fmt.Printf("Stopping daemon (PID %d)...\n", pid)
		if err := daemon.StopDaemon(townRoot); err != nil {
			return fmt.Errorf("stopping daemon: %w", err)
		}
		// Brief pause to ensure clean shutdown
		time.Sleep(200 * time.Millisecond)
	}

	// Start the daemon
	fmt.Println("Starting daemon...")
	gtPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	daemonProc := exec.Command(gtPath, "daemon", "run")
	daemonProc.Dir = townRoot
	daemonProc.Stdin = nil
	daemonProc.Stdout = nil
	daemonProc.Stderr = nil

	if err := daemonProc.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Wait for it to initialize
	time.Sleep(200 * time.Millisecond)

	// Verify it started
	running, newPid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking daemon status: %w", err)
	}
	if !running {
		return fmt.Errorf("daemon failed to start (check logs with 'gt daemon logs')")
	}

	if pid > 0 {
		fmt.Printf("%s Daemon restarted (PID %d → %d, v%s)\n",
			ui.RenderPassIcon(), pid, newPid, Version)
	} else {
		fmt.Printf("%s Daemon started (PID %d, v%s)\n",
			ui.RenderPassIcon(), newPid, Version)
	}
	return nil
}
