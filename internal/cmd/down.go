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
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/registry"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/terminal"
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

	backend := terminal.NewCoopBackend(terminal.CoopConfig{})

	// Phase 0: Acquire shutdown lock (skip for dry-run)
	if !downDryRun {
		lock, err := acquireShutdownLock(townRoot)
		if err != nil {
			return fmt.Errorf("cannot proceed: %w", err)
		}
		defer func() { _ = lock.Unlock() }()
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

	// Phase 1: Stop refineries
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
		if downDryRun {
			if running, _ := backend.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "would stop")
			}
			continue
		}
		wasRunning, err := stopSession(sessionName)
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
			if running, _ := backend.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "would stop")
			}
			continue
		}
		wasRunning, err := stopSession(sessionName)
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
			if running, _ := backend.HasSession(ts.SessionID); running {
				printDownStatus(ts.Name, true, "would stop")
			}
			continue
		}
		stopped, err := session.StopTownSession(nil, ts, downForce, backend)
		if err != nil {
			printDownStatus(ts.Name, false, err.Error())
			allOK = false
		} else if stopped {
			printDownStatus(ts.Name, true, "stopped")
		} else {
			printDownStatus(ts.Name, true, "not running")
		}
	}

	// Phase 3.5: Stop K8s agents via coop backend
	k8sStopped, k8sErrors := stopK8sAgents(townRoot, downDryRun)
	for _, label := range k8sStopped {
		if downDryRun {
			printDownStatus(label, true, "would stop")
		} else {
			printDownStatus(label, true, "stopped")
		}
	}
	for _, label := range k8sErrors {
		printDownStatus(label, false, "failed to stop")
		allOK = false
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
		respawned := verifyShutdown(townRoot)
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
		// --nuke was a tmux-only operation (kill tmux server). In K8s, this is not applicable.
		printDownStatus("Nuke", false, "not available in K8s (no tmux server)")
		allOK = false
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
	rigsConfig, err := loadRigsConfigBeadsFirst(townRoot)
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

		polecatMgr := polecat.NewSessionManager(r)
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

// stopSession gracefully stops a session (local tmux or remote).
// Returns (wasRunning, error) - wasRunning is true if session existed and was stopped.
func stopSession(sessionName string) (bool, error) {
	backend, sessionKey := resolveBackendForSession(sessionName)
	running, err := backend.HasSession(sessionKey)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil // Already stopped
	}

	// Try graceful shutdown first (Ctrl-C, best-effort interrupt)
	if !downForce {
		_ = backend.SendKeys(sessionKey, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Kill the session via Backend (delegates to KillSessionWithProcesses for tmux)
	return true, backend.KillSession(sessionKey)
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
func verifyShutdown(townRoot string) []string {
	var respawned []string

	for _, sess := range discoverSessionNames(townRoot) {
		if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
			respawned = append(respawned, fmt.Sprintf("agent session %s", sess))
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

// stopK8sAgents discovers and stops K8s agents via their coop backend.
// Checks both town-level and rig-level agent beads for execution_target:k8s.
// Returns lists of successfully stopped agent labels and error labels.
func stopK8sAgents(townRoot string, dryRun bool) (stopped []string, errors []string) {
	// Collect all agent beads from town and rig levels
	allAgents := make(map[string]*beads.Issue)

	// Town-level agent beads
	townBeadsPath := beads.GetTownBeadsPath(townRoot)
	if agents, err := beads.New(townBeadsPath).ListAgentBeads(); err == nil {
		for id, issue := range agents {
			allAgents[id] = issue
		}
	}

	// Rig-level agent beads
	for _, rigName := range discoverRigs(townRoot) {
		rigBeadsPath := filepath.Join(townRoot, rigName, "mayor", "rig")
		if agents, err := beads.New(rigBeadsPath).ListAgentBeads(); err == nil {
			for id, issue := range agents {
				allAgents[id] = issue
			}
		}
	}

	// Use SessionRegistry to discover K8s sessions with backend resolution
	lister := &mapAgentLister{agents: allAgents}
	reg := registry.New(lister, nil)
	ctx := context.Background()
	sessions, err := reg.DiscoverAll(ctx, registry.DiscoverOpts{})
	if err != nil {
		return stopped, errors
	}

	for _, s := range sessions {
		if s.Target != "k8s" {
			continue
		}

		label := fmt.Sprintf("K8s agent (%s)", s.ID)
		if dryRun {
			stopped = append(stopped, label)
			continue
		}

		backend := terminal.ResolveBackend(s.ID)
		if err := backend.KillSession("claude"); err != nil {
			errors = append(errors, label)
		} else {
			stopped = append(stopped, label)
		}
	}

	return stopped, errors
}

