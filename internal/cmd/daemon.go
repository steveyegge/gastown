package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/style"
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

var (
	daemonLogLines int
	daemonLogFollow bool
)

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
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
		fmt.Printf("%s Daemon already running (PID %d)\n", style.Bold.Render("●"), pid)
		return nil
	}

	fmt.Printf("%s Daemon started (PID %d)\n", style.Bold.Render("✓"), pid)
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

	fmt.Printf("%s Daemon stopped (was PID %d)\n", style.Bold.Render("✓"), pid)
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
		fmt.Printf("%s Daemon is %s (PID %d)\n",
			style.Bold.Render("●"),
			style.Bold.Render("running"),
			pid)

		// Load state for more details
		state, err := daemon.LoadState(townRoot)
		if err == nil && !state.StartedAt.IsZero() {
			fmt.Printf("  Started: %s\n", state.StartedAt.Format("2006-01-02 15:04:05"))
			if !state.LastHeartbeat.IsZero() {
				fmt.Printf("  Last heartbeat: %s (#%d)\n",
					state.LastHeartbeat.Format("15:04:05"),
					state.HeartbeatCount)
			}

			// Check if binary is newer than process
			if binaryModTime, err := getBinaryModTime(); err == nil {
				fmt.Printf("  Binary: %s\n", binaryModTime.Format("2006-01-02 15:04:05"))
				if binaryModTime.After(state.StartedAt) {
					fmt.Printf("  %s Binary is newer than process - consider '%s'\n",
						style.Bold.Render("⚠"),
						style.Dim.Render("gt daemon stop && gt daemon start"))
				}
			}
		}
	} else {
		fmt.Printf("%s Daemon is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt daemon start"))
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
