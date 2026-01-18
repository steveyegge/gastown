package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
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
		t := tmux.NewTmux()
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
		polecatsStopped := stopAllPolecats(townRoot, rigs, downForce, downDryRun)
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

	// Phase 2a: Stop refineries
	agents := factory.Agents()

	for _, rigName := range rigs {
		id := agent.RefineryAddress(rigName)
		if downDryRun {
			if agents.Exists(id) {
				printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "would stop")
			}
			continue
		}
		err := agents.Stop(id, true)
		if err == agent.ErrNotRunning {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "not running")
		} else if err != nil {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
		}
	}

	// Phase 2b: Stop witnesses
	for _, rigName := range rigs {
		id := agent.WitnessAddress(rigName)
		if downDryRun {
			if agents.Exists(id) {
				printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "would stop")
			}
			continue
		}
		err := agents.Stop(id, true)
		if err == agent.ErrNotRunning {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "not running")
		} else if err != nil {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
		}
	}

	// Phase 3: Stop town-level agents (Mayor, Boot, Deacon)
	// Order matters: Boot (Deacon's watchdog) must stop before Deacon.
	// Reuse agents from rig-level operations (same underlying Agents interface)
	townAgents := []struct {
		name string
		id   agent.AgentID
	}{
		{"Mayor", agent.MayorAddress},
		{"Boot", agent.BootAddress},
		{"Deacon", agent.DeaconAddress},
	}
	for _, ta := range townAgents {
		if downDryRun {
			if agents.Exists(ta.id) {
				printDownStatus(ta.name, true, "would stop")
			}
			continue
		}
		wasRunning := agents.Exists(ta.id)
		err := agents.Stop(ta.id, !downForce) // graceful = !force
		if err != nil {
			printDownStatus(ta.name, false, err.Error())
			allOK = false
		} else if wasRunning {
			printDownStatus(ta.name, true, "stopped")
		} else {
			printDownStatus(ta.name, true, "not running")
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
		respawned := verifyShutdown(agents, townRoot)
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
			t := tmux.NewTmux()
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
func stopAllPolecats(townRoot string, rigNames []string, force bool, dryRun bool) int {
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

		polecatMgr := factory.New(townRoot).PolecatSessionManager(r, "")
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
func verifyShutdown(agents agent.Agents, townRoot string) []string {
	var respawned []string

	if count := beads.CountBdDaemons(); count > 0 {
		respawned = append(respawned, fmt.Sprintf("bd daemon (%d running)", count))
	}

	if count := beads.CountBdActivityProcesses(); count > 0 {
		respawned = append(respawned, fmt.Sprintf("bd activity (%d running)", count))
	}

	// Check if any agents respawned
	agentIDs, err := agents.List()
	if err == nil && len(agentIDs) > 0 {
		for _, id := range agentIDs {
			respawned = append(respawned, fmt.Sprintf("agent %s", id))
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
