package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

const (
	shutdownLockFile    = "daemon/shutdown.lock"
	shutdownLockTimeout = 5 * time.Second
)

var downCmd = &cobra.Command{
	Use:     "down",
	GroupID: GroupServices,
	Short:   "Stop all Gas Town services",
	Long: `Stop all Gas Town long-lived services.

This gracefully shuts down all infrastructure agents:

  • Refineries - Per-rig work processors
  • Witnesses  - Per-rig polecat managers
  • Mayor      - Global work coordinator
  • Boot       - Deacon's watchdog
  • Deacon     - Health orchestrator
  • Daemon     - Go background process

With --all, also stops resurrection layer (bd daemon/activity) and verifies
shutdown. Polecats are NOT stopped - use 'gt swarm stop' for that.

Flags:
  --all      Stop bd daemons/activity, verify complete shutdown
  --nuke     Kill entire tmux server (DESTRUCTIVE!)
  --dry-run  Preview what would be stopped
  --force    Skip graceful shutdown, use SIGKILL`,
	RunE: runDown,
}

var (
	downQuiet  bool
	downForce  bool
	downAll    bool
	downNuke   bool
	downDryRun bool
)

func init() {
	downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
	downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
	downCmd.Flags().BoolVarP(&downAll, "all", "a", false, "Stop bd daemons/activity and verify shutdown")
	downCmd.Flags().BoolVar(&downNuke, "nuke", false, "Kill entire tmux server (DESTRUCTIVE - kills non-GT sessions!)")
	downCmd.Flags().BoolVar(&downDryRun, "dry-run", false, "Preview what would be stopped without taking action")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()
	if !t.IsAvailable() {
		return fmt.Errorf("tmux not available (is tmux installed and on PATH?)")
	}

	// Phase 0: Acquire shutdown lock (skip for dry-run)
	if !downDryRun {
		lock, err := acquireShutdownLock(townRoot)
		if err != nil {
			return fmt.Errorf("cannot proceed: %w", err)
		}
		defer lock.Unlock()
	}
	allOK := true

	if downDryRun {
		fmt.Println("═══ DRY RUN: Preview of shutdown actions ═══")
		fmt.Println()
	}

	// Phase 1: Stop bd resurrection layer (--all only)
	if downAll {
		daemonsKilled, activityKilled, err := beads.StopAllBdProcesses(downDryRun, downForce)
		if err != nil {
			printDownStatus("bd processes", false, err.Error())
			allOK = false
		} else {
			if downDryRun {
				if daemonsKilled > 0 || activityKilled > 0 {
					printDownStatus("bd daemon", true, fmt.Sprintf("%d would stop", daemonsKilled))
					printDownStatus("bd activity", true, fmt.Sprintf("%d would stop", activityKilled))
				} else {
					printDownStatus("bd processes", true, "none running")
				}
			} else {
				if daemonsKilled > 0 {
					printDownStatus("bd daemon", true, fmt.Sprintf("%d stopped", daemonsKilled))
				}
				if activityKilled > 0 {
					printDownStatus("bd activity", true, fmt.Sprintf("%d stopped", activityKilled))
				}
				if daemonsKilled == 0 && activityKilled == 0 {
					printDownStatus("bd processes", true, "none running")
				}
			}
		}
	}

	rigs := discoverRigs(townRoot)

	// Phase 2a: Stop refineries
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
		if downDryRun {
			if running, _ := t.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "would stop")
			}
			continue
		}
		wasRunning, err := stopSession(t, sessionName)
		if err != nil {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
			allOK = false
		} else if wasRunning {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
		} else {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "not running")
		}
	}

	// Phase 2b: Stop witnesses
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-witness", rigName)
		if downDryRun {
			if running, _ := t.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "would stop")
			}
			continue
		}
		wasRunning, err := stopSession(t, sessionName)
		if err != nil {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else if wasRunning {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
		} else {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "not running")
		}
	}

	// Phase 3: Stop town-level sessions (Mayor, Boot, Deacon)
	for _, ts := range session.TownSessions() {
		if downDryRun {
			if running, _ := t.HasSession(ts.SessionID); running {
				printDownStatus(ts.Name, true, "would stop")
			}
			continue
		}
		stopped, err := session.StopTownSession(t, ts, downForce)
		if err != nil {
			printDownStatus(ts.Name, false, err.Error())
			allOK = false
		} else if stopped {
			printDownStatus(ts.Name, true, "stopped")
		} else {
			printDownStatus(ts.Name, true, "not running")
		}
	}

	// Phase 4: Stop Daemon
	running, pid, daemonErr := daemon.IsRunning(townRoot)
	if daemonErr != nil {
		printDownStatus("Daemon", false, fmt.Sprintf("status check failed: %v", daemonErr))
		allOK = false
	} else if downDryRun {
		if running {
			printDownStatus("Daemon", true, fmt.Sprintf("would stop (PID %d)", pid))
		}
	} else {
		if running {
			if err := daemon.StopDaemon(townRoot); err != nil {
				printDownStatus("Daemon", false, err.Error())
				allOK = false
			} else {
				printDownStatus("Daemon", true, fmt.Sprintf("stopped (was PID %d)", pid))
			}
		} else {
			printDownStatus("Daemon", true, "not running")
		}
	}

	// Phase 5: Verification (--all only)
	if downAll && !downDryRun {
		time.Sleep(500 * time.Millisecond)
		respawned := verifyShutdown(t, townRoot)
		if len(respawned) > 0 {
			fmt.Println()
			fmt.Printf("%s Warning: Some processes may have respawned:\n", style.Bold.Render("⚠"))
			for _, r := range respawned {
				fmt.Printf("  • %s\n", r)
			}
			fmt.Println()
			fmt.Printf("This may indicate systemd/launchd is managing bd.\n")
			fmt.Printf("Check with:\n")
			fmt.Printf("  %s\n", style.Dim.Render("systemctl status bd-daemon  # Linux"))
			fmt.Printf("  %s\n", style.Dim.Render("launchctl list | grep bd    # macOS"))
			allOK = false
		}
	}

	// Phase 6: Nuke tmux server (--nuke only, DESTRUCTIVE)
	if downNuke {
		if downDryRun {
			printDownStatus("Tmux server", true, "would kill (DESTRUCTIVE)")
		} else if os.Getenv("GT_NUKE_ACKNOWLEDGED") == "" {
			// Require explicit acknowledgement for destructive operation
			fmt.Println()
			fmt.Printf("%s The --nuke flag kills ALL tmux sessions, not just Gas Town.\n",
				style.Bold.Render("⚠ BLOCKED:"))
			fmt.Printf("This includes vim sessions, running builds, SSH connections, etc.\n")
			fmt.Println()
			fmt.Printf("To proceed, run with: %s\n", style.Bold.Render("GT_NUKE_ACKNOWLEDGED=1 gt down --nuke"))
			allOK = false
		} else {
			if err := t.KillServer(); err != nil {
				printDownStatus("Tmux server", false, err.Error())
				allOK = false
			} else {
				printDownStatus("Tmux server", true, "killed (all tmux sessions destroyed)")
			}
		}
	}

	// Summary
	fmt.Println()
	if downDryRun {
		fmt.Println("═══ DRY RUN COMPLETE (no changes made) ═══")
		return nil
	}

	if allOK {
		fmt.Printf("%s All services stopped\n", style.Bold.Render("✓"))
		stoppedServices := []string{"daemon", "deacon", "boot", "mayor"}
		for _, rigName := range rigs {
			stoppedServices = append(stoppedServices, fmt.Sprintf("%s/refinery", rigName))
			stoppedServices = append(stoppedServices, fmt.Sprintf("%s/witness", rigName))
		}
		if downAll {
			stoppedServices = append(stoppedServices, "bd-processes")
		}
		if downNuke {
			stoppedServices = append(stoppedServices, "tmux-server")
		}
		_ = events.LogFeed(events.TypeHalt, "gt", events.HaltPayload(stoppedServices))
	} else {
		fmt.Printf("%s Some services failed to stop\n", style.Bold.Render("✗"))
		return fmt.Errorf("not all services stopped")
	}

	return nil
}

func printDownStatus(name string, ok bool, detail string) {
	if downQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// stopSession gracefully stops a tmux session.
// Returns (wasRunning, error) - wasRunning is true if session existed and was stopped.
func stopSession(t *tmux.Tmux, sessionName string) (bool, error) {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil // Already stopped
	}

	// Try graceful shutdown first (Ctrl-C, best-effort interrupt)
	if !downForce {
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Kill the session
	return true, t.KillSession(sessionName)
}

// acquireShutdownLock prevents concurrent shutdowns.
// Returns the lock (caller must defer Unlock()) or error if lock held.
func acquireShutdownLock(townRoot string) (*flock.Flock, error) {
	lockPath := filepath.Join(townRoot, shutdownLockFile)

	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lock := flock.New(lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownLockTimeout)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("lock acquisition failed: %w", err)
	}

	if !locked {
		return nil, fmt.Errorf("another shutdown is in progress (lock held: %s)", lockPath)
	}

	return lock, nil
}

// verifyShutdown checks for respawned processes after shutdown.
// Returns list of things that are still running or respawned.
func verifyShutdown(t *tmux.Tmux, townRoot string) []string {
	var respawned []string

	if count := beads.CountBdDaemons(); count > 0 {
		respawned = append(respawned, fmt.Sprintf("bd daemon (%d running)", count))
	}

	if count := beads.CountBdActivityProcesses(); count > 0 {
		respawned = append(respawned, fmt.Sprintf("bd activity (%d running)", count))
	}

	sessions, err := t.ListSessions()
	if err == nil {
		for _, sess := range sessions {
			if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
				respawned = append(respawned, fmt.Sprintf("tmux session %s", sess))
			}
		}
	}

	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	if pidData, err := os.ReadFile(pidFile); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err == nil {
			if isProcessRunning(pid) {
				respawned = append(respawned, fmt.Sprintf("gt daemon (PID %d)", pid))
			}
		}
	}

	return respawned
}

// isProcessRunning checks if a process with the given PID exists.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false // Invalid PID
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means process exists but we don't have permission to signal it
	if err == syscall.EPERM {
		return true
	}
	return false
}
