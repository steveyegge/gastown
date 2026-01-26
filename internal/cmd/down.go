package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
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
	Long: `Stop Gas Town services (reversible pause).

Shutdown levels (progressively more aggressive):
  gt down                    Stop infrastructure (default)
  gt down --polecats         Also stop all polecat sessions
  gt down --all              Also stop bd daemons/activity
  gt down --nuke             Also kill the tmux server (DESTRUCTIVE)

Infrastructure agents stopped:
  • Refineries - Per-rig work processors
  • Witnesses  - Per-rig polecat managers
  • Mayor      - Global work coordinator
  • Boot       - Deacon's watchdog
  • Deacon     - Health orchestrator
  • Daemon     - Go background process

This is a "pause" operation - use 'gt start' to bring everything back up.
For permanent cleanup (removing worktrees), use 'gt shutdown' instead.

Use cases:
  • Taking a break (stop token consumption)
  • Clean shutdown before system maintenance
  • Resetting the town to a clean state`,
	RunE: runDown,
}

var (
	downQuiet    bool
	downForce    bool
	downAll      bool
	downNuke     bool
	downDryRun   bool
	downPolecats bool
)

func init() {
	downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
	downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
	downCmd.Flags().BoolVarP(&downPolecats, "polecats", "p", false, "Also stop all polecat sessions")
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
		defer func() { _ = lock.Unlock() }()

		// Prevent tmux server from exiting when all sessions are killed.
		// By default, tmux exits when there are no sessions (exit-empty on).
		// This ensures the server stays running for subsequent `gt up`.
		// Ignore errors - if there's no server, nothing to configure.
		_ = t.SetExitEmpty(false)
	}
	allOK := true

	if downDryRun {
		fmt.Println("═══ DRY RUN: Preview of shutdown actions ═══")
		fmt.Println()
	}

	rigs := discoverRigs(townRoot)

	// Phase 0.5: Stop polecats if --polecats
	if downPolecats {
		if downDryRun {
			fmt.Println("Would stop polecats...")
		} else {
			fmt.Println("Stopping polecats...")
		}
		polecatsStopped := stopAllPolecats(t, townRoot, rigs, downForce, downDryRun)
		if downDryRun {
			if polecatsStopped > 0 {
				printDownStatus("Polecats", true, fmt.Sprintf("%d would stop", polecatsStopped))
			} else {
				printDownStatus("Polecats", true, "none running")
			}
		} else {
			if polecatsStopped > 0 {
				printDownStatus("Polecats", true, fmt.Sprintf("%d stopped", polecatsStopped))
			} else {
				printDownStatus("Polecats", true, "none running")
			}
		}
		fmt.Println()
	}

	// Phase 1: Stop refineries
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

	// Phase 2: Stop witnesses
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
		if downPolecats {
			stoppedServices = append(stoppedServices, "polecats")
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

// stopAllPolecats stops all polecat sessions across all rigs.
// Returns the number of polecats stopped (or would be stopped in dry-run).
func stopAllPolecats(t *tmux.Tmux, townRoot string, rigNames []string, force bool, dryRun bool) int {
	stopped := 0

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	for _, rigName := range rigNames {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			continue
		}

		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err != nil {
			continue
		}

		for _, info := range infos {
			if dryRun {
				stopped++
				fmt.Printf("  %s [%s] %s would stop\n", style.Dim.Render("○"), rigName, info.Polecat)
				continue
			}
			err := polecatMgr.Stop(info.Polecat, force)
			if err == nil {
				stopped++
				fmt.Printf("  %s [%s] %s stopped\n", style.SuccessPrefix, rigName, info.Polecat)
			} else {
				fmt.Printf("  %s [%s] %s: %s\n", style.ErrorPrefix, rigName, info.Polecat, err.Error())
			}
		}
	}

	return stopped
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

	// Kill the session (with explicit process termination to prevent orphans)
	return true, t.KillSessionWithProcesses(sessionName)
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

	// Check for orphaned Claude/node processes
	// These can be left behind if tmux sessions were killed but child processes didn't terminate
	if pids := findOrphanedClaudeProcesses(townRoot); len(pids) > 0 {
		respawned = append(respawned, fmt.Sprintf("orphaned Claude processes (PIDs: %v)", pids))
	}

	return respawned
}

// findOrphanedClaudeProcesses finds Claude/node processes that are running in the
// town directory but aren't associated with any active tmux session.
// This can happen when tmux sessions are killed but child processes don't terminate.
func findOrphanedClaudeProcesses(townRoot string) []int {
	// Use pgrep to find all claude/node processes
	cmd := exec.Command("pgrep", "-l", "node")
	output, err := cmd.Output()
	if err != nil {
		return nil // pgrep found no processes or failed
	}

	var orphaned []int
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "PID command"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pidStr := parts[0]
		var pid int
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
			continue
		}

		// Check if this process is running in the town directory
		if isProcessInTown(pid, townRoot) {
			orphaned = append(orphaned, pid)
		}
	}

	return orphaned
}

// isProcessInTown checks if a process is running in the given town directory.
// Uses ps to check the process's working directory.
func isProcessInTown(pid int, townRoot string) bool {
	// Use ps to get the process's working directory
	cmd := exec.Command("ps", "-o", "command=", "-p", fmt.Sprintf("%d", pid))
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check if the command line includes the town path
	command := string(output)
	return strings.Contains(command, townRoot)
}

