package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var reloadCmd = &cobra.Command{
	Use:     "reload",
	GroupID: GroupServices,
	Short:   "Reload all Gas Town services (restart with fresh binary)",
	Long: `Reload all Gas Town services by stopping and restarting them.

This is useful after installing a new version of gt/bd to ensure all
services pick up the new binary. It performs:

  1. Stop all rig agents (refineries, witnesses)
  2. Stop bd daemons (all workspaces)
  3. Restart bd daemons
  4. Start all rig agents

By default, the Mayor session is preserved (not restarted). Use --mayor
to also reload the Mayor (will kill your current session if you're in it).

Use --polecats to also stop and restart polecats with pinned work.`,
	RunE: runReload,
}

var (
	reloadQuiet    bool
	reloadMayor    bool
	reloadPolecats bool
	reloadForce    bool
)

// pidChange tracks before/after PIDs for a service
type pidChange struct {
	name     string
	oldPID   int
	newPID   int
	reloaded bool // true if PID actually changed
}

func init() {
	reloadCmd.Flags().BoolVarP(&reloadQuiet, "quiet", "q", false, "Only show errors")
	reloadCmd.Flags().BoolVar(&reloadMayor, "mayor", false, "Also reload Mayor session (kills current session if attached)")
	reloadCmd.Flags().BoolVarP(&reloadPolecats, "polecats", "p", false, "Also reload polecats with pinned work")
	reloadCmd.Flags().BoolVarP(&reloadForce, "force", "f", false, "Force kill without graceful shutdown")
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()
	if !t.IsAvailable() {
		return fmt.Errorf("tmux not available")
	}

	rigs := discoverRigs(townRoot)
	allOK := true

	// Track PID changes for summary
	var pidChanges []pidChange

	// Capture gt daemon PID before
	_, gtOldPID, _ := daemon.IsRunning(townRoot)

	// Capture bd daemon PIDs before
	bdWorkspaces := findBdWorkspaces(townRoot)
	bdOldPIDs := make(map[string]int)
	for _, ws := range bdWorkspaces {
		if pid := getBdDaemonPID(ws); pid > 0 {
			bdOldPIDs[ws] = pid
		}
	}

	fmt.Println("═══ Stopping services ═══")
	fmt.Println()

	// Phase 1: Stop polecats if requested
	if reloadPolecats {
		polecatsStopped := reloadStopPolecats(t, townRoot, rigs)
		if polecatsStopped > 0 {
			printReloadStatus("Polecats", true, fmt.Sprintf("%d stopped", polecatsStopped))
		} else {
			printReloadStatus("Polecats", true, "none running")
		}
	}

	// Phase 2 & 3: Stop all sessions in parallel (rig agents + town sessions)
	// This dramatically reduces reload time since each stop has a 2s grace period
	type stopResult struct {
		name    string
		ok      bool
		detail  string
		running bool
	}
	var stopResults []stopResult
	var stopMu sync.Mutex
	var stopWg sync.WaitGroup

	// Collect all sessions to stop
	var sessionsToStop []struct {
		name        string
		sessionName string
		isTown      bool
		townSession session.TownSession
	}

	// Rig agents
	for _, rigName := range rigs {
		sessionsToStop = append(sessionsToStop,
			struct {
				name        string
				sessionName string
				isTown      bool
				townSession session.TownSession
			}{fmt.Sprintf("Refinery (%s)", rigName), fmt.Sprintf("gt-%s-refinery", rigName), false, session.TownSession{}},
			struct {
				name        string
				sessionName string
				isTown      bool
				townSession session.TownSession
			}{fmt.Sprintf("Witness (%s)", rigName), fmt.Sprintf("gt-%s-witness", rigName), false, session.TownSession{}},
		)
	}

	// Town sessions (except Deacon - handled separately via KillSessionWithProcesses)
	for _, ts := range session.TownSessions() {
		if ts.Name == "Mayor" && !reloadMayor {
			continue
		}
		if ts.Name == "Deacon" {
			continue // Handled below via KillSessionWithProcesses
		}
		sessionsToStop = append(sessionsToStop, struct {
			name        string
			sessionName string
			isTown      bool
			townSession session.TownSession
		}{ts.Name, ts.SessionID, true, ts})
	}

	// Stop all in parallel
	for _, s := range sessionsToStop {
		stopWg.Add(1)
		go func(name, sessionName string, isTown bool, ts session.TownSession) {
			defer stopWg.Done()
			var wasRunning bool
			var err error

			if isTown {
				wasRunning, err = session.StopTownSession(t, ts, reloadForce)
			} else {
				wasRunning, err = reloadStopSession(t, sessionName)
			}

			stopMu.Lock()
			if err != nil {
				// "session not found" is not an error - session already stopped
				if strings.Contains(err.Error(), "session not found") {
					// Don't report, already stopped
				} else {
					stopResults = append(stopResults, stopResult{name, false, err.Error(), true})
				}
			} else if wasRunning {
				stopResults = append(stopResults, stopResult{name, true, "stopped", true})
			}
			stopMu.Unlock()
		}(s.name, s.sessionName, s.isTown, s.townSession)
	}
	stopWg.Wait()

	// Stop Deacon - use KillSessionWithProcesses to properly kill process tree
	// (avoids pkill -f pattern matching which can kill unrelated processes)
	deaconSession := session.DeaconSessionName()
	if running, _ := t.HasSession(deaconSession); running {
		_ = t.KillSessionWithProcesses(deaconSession)
		printReloadStatus("Deacon", true, "stopped")
	}

	// Print results
	for _, r := range stopResults {
		if !r.ok {
			allOK = false
		}
		printReloadStatus(r.name, r.ok, r.detail)
	}

	// Phase 4: Stop gt daemon
	running, pid, _ := daemon.IsRunning(townRoot)
	if running {
		if err := daemon.StopDaemon(townRoot); err != nil {
			printReloadStatus("Daemon", false, err.Error())
			allOK = false
		} else {
			printReloadStatus("Daemon", true, fmt.Sprintf("stopped (was PID %d)", pid))
		}
	}

	// Phase 5: Stop bd daemons via RPC (clean shutdown)
	for _, ws := range bdWorkspaces {
		beadsDir := filepath.Join(ws, ".beads")
		if err := beads.StopBdDaemonForWorkspace(beadsDir); err != nil {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), false, err.Error())
		} else {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), true, "stopped")
		}
	}

	fmt.Println()
	fmt.Println("═══ Starting services ═══")
	fmt.Println()

	// Phase 6: Kill any bd daemons that may have restarted between stop and start phases
	// (bd auto-starts daemons when running commands, so hooks/background tasks can restart them)
	if len(bdWorkspaces) > 0 {
		cmd := exec.Command("bd", "daemon", "killall", "--force")
		_ = cmd.Run() // Ignore errors - we just want to ensure they're dead
		time.Sleep(500 * time.Millisecond) // Give processes time to die
	}

	// Phase 7: Start bd daemons sequentially (parallel causes database contention)
	for _, ws := range bdWorkspaces {
		err := startBdDaemon(ws)
		if err != nil {
			allOK = false
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), false, err.Error())
		} else {
			printReloadStatus(fmt.Sprintf("bd daemon (%s)", shortPath(ws)), true, "started")
		}
	}

	// Phase 7: Start gt daemon
	if err := ensureDaemon(townRoot); err != nil {
		printReloadStatus("Daemon", false, err.Error())
		allOK = false
	} else {
		running, pid, _ := daemon.IsRunning(townRoot)
		if running {
			printReloadStatus("Daemon", true, fmt.Sprintf("PID %d", pid))
		}
	}

	// Phase 8: Start town-level sessions (Deacon, optionally Mayor)
	if reloadMayor {
		mayorMgr := mayor.NewManager(townRoot)
		if err := mayorMgr.Start(""); err != nil && err != mayor.ErrAlreadyRunning {
			printReloadStatus("Mayor", false, err.Error())
			allOK = false
		} else {
			printReloadStatus("Mayor", true, mayorMgr.SessionName())
		}
	}

	deaconMgr := deacon.NewManager(townRoot)
	if err := deaconMgr.Start(""); err != nil && err != deacon.ErrAlreadyRunning {
		printReloadStatus("Deacon", false, err.Error())
		allOK = false
	} else {
		printReloadStatus("Deacon", true, deaconMgr.SessionName())
	}

	// Phase 9: Start rig agents in parallel
	prefetchedRigs, rigErrors := prefetchRigs(rigs)
	witnessResults, refineryResults := startRigAgentsWithPrefetch(rigs, prefetchedRigs, rigErrors)

	for _, rigName := range rigs {
		if result, ok := witnessResults[rigName]; ok {
			printReloadStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}
	for _, rigName := range rigs {
		if result, ok := refineryResults[rigName]; ok {
			printReloadStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}

	// Phase 10: Start polecats with work if requested
	if reloadPolecats {
		for _, rigName := range rigs {
			polecatsStarted, polecatErrors := startPolecatsWithWork(townRoot, rigName)
			for _, name := range polecatsStarted {
				printReloadStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), true, "started")
			}
			for name, err := range polecatErrors {
				printReloadStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), false, err.Error())
				allOK = false
			}
		}
	}

	// Capture new PIDs and build summary
	_, gtNewPID, _ := daemon.IsRunning(townRoot)
	if gtOldPID > 0 || gtNewPID > 0 {
		pidChanges = append(pidChanges, pidChange{
			name:     "gt daemon",
			oldPID:   gtOldPID,
			newPID:   gtNewPID,
			reloaded: gtOldPID != gtNewPID && gtNewPID > 0,
		})
	}

	for _, ws := range bdWorkspaces {
		newPID := getBdDaemonPID(ws)
		oldPID := bdOldPIDs[ws]
		if oldPID > 0 || newPID > 0 {
			pidChanges = append(pidChanges, pidChange{
				name:     fmt.Sprintf("bd daemon (%s)", shortPath(ws)),
				oldPID:   oldPID,
				newPID:   newPID,
				reloaded: oldPID != newPID && newPID > 0,
			})
		}
	}

	// Summary
	fmt.Println()
	if allOK {
		fmt.Printf("%s All services reloaded\n", style.Bold.Render("✓"))
		_ = events.LogFeed(events.TypeBoot, "gt", events.BootPayload("reload", []string{"all"}))
	} else {
		fmt.Printf("%s Some services failed to reload\n", style.Bold.Render("✗"))
	}

	// PID change summary
	if len(pidChanges) > 0 {
		fmt.Println()
		fmt.Println("═══ PID Changes ═══")
		fmt.Println()
		allReloaded := true
		for _, pc := range pidChanges {
			if pc.reloaded {
				fmt.Printf("%s %s: %d → %d\n", style.SuccessPrefix, pc.name,
					pc.oldPID, pc.newPID)
			} else if pc.newPID == 0 {
				fmt.Printf("%s %s: %d → %s\n", style.ErrorPrefix, pc.name,
					pc.oldPID, style.Dim.Render("not running"))
				allReloaded = false
			} else if pc.oldPID == pc.newPID {
				fmt.Printf("%s %s: %d %s\n", style.WarningPrefix, pc.name,
					pc.oldPID, style.Dim.Render("(unchanged - not restarted!)"))
				allReloaded = false
			} else if pc.oldPID == 0 {
				fmt.Printf("%s %s: %s → %d\n", style.SuccessPrefix, pc.name,
					style.Dim.Render("not running"), pc.newPID)
			}
		}
		if !allReloaded {
			allOK = false
		}
	}

	if !allOK {
		return fmt.Errorf("not all services reloaded")
	}

	return nil
}

func printReloadStatus(name string, ok bool, detail string) {
	if reloadQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// reloadStopSession gracefully stops a tmux session.
func reloadStopSession(t *tmux.Tmux, sessionName string) (bool, error) {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	if !reloadForce {
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	return true, t.KillSessionWithProcesses(sessionName)
}

// reloadStopPolecats stops all polecat sessions across all rigs.
func reloadStopPolecats(t *tmux.Tmux, townRoot string, rigNames []string) int {
	stopped := 0

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
			if err := polecatMgr.Stop(info.Polecat, reloadForce); err == nil {
				stopped++
			}
		}
	}

	return stopped
}

// findBdWorkspaces finds all bd workspace directories that might have daemons.
func findBdWorkspaces(townRoot string) []string {
	workspaces := []string{}

	// Main town workspace
	if _, err := os.Stat(filepath.Join(townRoot, ".beads", "beads.db")); err == nil {
		workspaces = append(workspaces, townRoot)
	}

	// Rig workspaces
	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		rigPath := filepath.Join(townRoot, rigName)

		// Check for .beads directory with actual database
		beadsPath := filepath.Join(rigPath, ".beads")
		if _, err := os.Stat(beadsPath); err == nil {
			// Check if it's a redirect
			redirectPath := filepath.Join(beadsPath, "redirect")
			if _, err := os.Stat(redirectPath); err == nil {
				// It's a redirect - read target and resolve
				if content, err := os.ReadFile(redirectPath); err == nil {
					target := strings.TrimSpace(string(content))
					if !filepath.IsAbs(target) {
						target = filepath.Join(townRoot, target)
					}
					// Only add if the target actually exists and has a database
					if _, err := os.Stat(filepath.Join(target, "beads.db")); err == nil {
						workspaces = append(workspaces, target)
					}
				}
			} else if _, err := os.Stat(filepath.Join(beadsPath, "beads.db")); err == nil {
				// Not a redirect, has actual database
				workspaces = append(workspaces, rigPath)
			}
		}

		// Check mayor/rig subdirectory (common pattern)
		mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
		if _, err := os.Stat(filepath.Join(mayorRigPath, ".beads", "beads.db")); err == nil {
			workspaces = append(workspaces, mayorRigPath)
		}
	}

	// Deduplicate and verify paths exist, use absolute paths
	seen := make(map[string]bool)
	unique := []string{}
	for _, ws := range workspaces {
		absPath, err := filepath.Abs(ws)
		if err != nil {
			continue
		}
		// Verify the path actually exists
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		if !seen[absPath] {
			seen[absPath] = true
			unique = append(unique, absPath) // Use absolute path
		}
	}

	return unique
}

// stopBdDaemon stops the bd daemon in a workspace and waits for it to die.
func stopBdDaemon(workspace string) error {
	// Get current PID before stopping
	pidFile := filepath.Join(workspace, ".beads", "daemon.pid")
	oldPIDData, _ := os.ReadFile(pidFile)
	oldPID, _ := strconv.Atoi(strings.TrimSpace(string(oldPIDData)))

	if oldPID == 0 {
		return fmt.Errorf("not running")
	}

	// Check if process is actually running
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", oldPID)); err != nil {
		return fmt.Errorf("not running")
	}

	// Use RPC shutdown for graceful cleanup (flushes DB, closes connections)
	cmd := exec.Command("bd", "daemons", "stop", workspace)
	_ = cmd.Run() // Ignore error - we'll verify via /proc

	// Wait for process to actually die (up to 3 seconds)
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", oldPID)); err != nil {
			return nil // Process is dead
		}
		time.Sleep(100 * time.Millisecond)
	}

	// RPC didn't kill it in time - escalate to SIGTERM
	proc, _ := os.FindProcess(oldPID)
	if proc != nil {
		_ = proc.Signal(os.Interrupt)
	}

	// Wait another 2 seconds
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", oldPID)); err != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Still alive - SIGKILL
	if proc != nil {
		_ = proc.Kill()
	}
	time.Sleep(200 * time.Millisecond)

	if _, err := os.Stat(fmt.Sprintf("/proc/%d", oldPID)); err != nil {
		return nil
	}

	return fmt.Errorf("process %d did not die after kill", oldPID)
}

// startBdDaemon starts the bd daemon in a workspace.
// During reload, "already running" is an error since we expect fresh starts.
func startBdDaemon(workspace string) error {
	cmd := exec.Command("bd", "daemon", "start")
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
	}
	// Give daemon time to initialize
	time.Sleep(200 * time.Millisecond)
	return nil
}

// shortPath returns a shortened path for display.
func shortPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// getBdDaemonPID returns the PID of the bd daemon for a workspace, or 0 if not running.
func getBdDaemonPID(workspace string) int {
	pidFile := filepath.Join(workspace, ".beads", "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	// Verify process is actually running by checking /proc
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return 0
	}
	return pid
}
