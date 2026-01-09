package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	startAll               bool
	startAgentOverride     string
	startCrewRig           string
	startCrewAccount       string
	startCrewAgentOverride string
	shutdownGraceful       bool
	shutdownWait           int
	shutdownAll            bool
	shutdownForce          bool
	shutdownYes            bool
	shutdownPolecatsOnly   bool
	shutdownNuclear        bool
)

var startCmd = &cobra.Command{
	Use:     "start [path]",
	GroupID: GroupServices,
	Short:   "Start Gas Town or a crew workspace",
	Long: `Start Gas Town by launching the Deacon and Mayor.

The Deacon is the health-check orchestrator that monitors Mayor and Witnesses.
The Mayor is the global coordinator that dispatches work.

By default, other agents (Witnesses, Refineries) are started lazily as needed.
Use --all to start Witnesses and Refineries for all registered rigs immediately.

Crew shortcut:
  If a path like "rig/crew/name" is provided, starts that crew workspace.
  This is equivalent to 'gt start crew rig/name'.

To stop Gas Town, use 'gt shutdown'.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStart,
}

var shutdownCmd = &cobra.Command{
	Use:     "shutdown",
	GroupID: GroupServices,
	Short:   "Shutdown Gas Town",
	Long: `Shutdown Gas Town by stopping agents and cleaning up polecats.

By default, preserves crew sessions (your persistent workspaces).
Prompts for confirmation before stopping.

After killing sessions, polecats are cleaned up:
  - Worktrees are removed
  - Polecat branches are deleted
  - Polecats with uncommitted work are SKIPPED (protected)

Shutdown levels (progressively more aggressive):
  (default)       - Stop infrastructure (Mayor, Deacon, Witnesses, Refineries, Polecats)
  --all           - Also stop crew sessions
  --polecats-only - Only stop polecats (leaves everything else running)

Use --force or --yes to skip confirmation prompt.
Use --graceful to allow agents time to save state before killing.
Use --nuclear to force cleanup even if polecats have uncommitted work (DANGER).`,
	RunE: runShutdown,
}

var startCrewCmd = &cobra.Command{
	Use:   "crew <name>",
	Short: "Start a crew workspace (creates if needed)",
	Long: `Start a crew workspace, creating it if it doesn't exist.

This is a convenience command that combines 'gt crew add' and 'gt crew at --detached'.
The crew session starts in the background with Claude running and ready.

The name can include the rig in slash format (e.g., greenplace/joe).
If not specified, the rig is inferred from the current directory.

Examples:
  gt start crew joe                    # Start joe in current rig
  gt start crew greenplace/joe            # Start joe in gastown rig
  gt start crew joe --rig beads        # Start joe in beads rig`,
	Args: cobra.ExactArgs(1),
	RunE: runStartCrew,
}

func init() {
	startCmd.Flags().BoolVarP(&startAll, "all", "a", false,
		"Also start Witnesses and Refineries for all rigs")
	startCmd.Flags().StringVar(&startAgentOverride, "agent", "", "Agent alias to run Mayor/Deacon with (overrides town default)")

	startCrewCmd.Flags().StringVar(&startCrewRig, "rig", "", "Rig to use")
	startCrewCmd.Flags().StringVar(&startCrewAccount, "account", "", "Claude Code account handle to use")
	startCrewCmd.Flags().StringVar(&startCrewAgentOverride, "agent", "", "Agent alias to run crew worker with (overrides rig/town default)")
	startCmd.AddCommand(startCrewCmd)

	shutdownCmd.Flags().BoolVarP(&shutdownGraceful, "graceful", "g", false,
		"Send ESC to agents and wait for them to handoff before killing")
	shutdownCmd.Flags().IntVarP(&shutdownWait, "wait", "w", 30,
		"Seconds to wait for graceful shutdown (default 30)")
	shutdownCmd.Flags().BoolVarP(&shutdownAll, "all", "a", false,
		"Also stop crew sessions (by default, crew is preserved)")
	shutdownCmd.Flags().BoolVarP(&shutdownForce, "force", "f", false,
		"Skip confirmation prompt (alias for --yes)")
	shutdownCmd.Flags().BoolVarP(&shutdownYes, "yes", "y", false,
		"Skip confirmation prompt")
	shutdownCmd.Flags().BoolVar(&shutdownPolecatsOnly, "polecats-only", false,
		"Only stop polecats (minimal shutdown)")
	shutdownCmd.Flags().BoolVar(&shutdownNuclear, "nuclear", false,
		"Force cleanup even if polecats have uncommitted work (DANGER: may lose work)")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(shutdownCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check if arg looks like a crew path (rig/crew/name)
	if len(args) == 1 && strings.Contains(args[0], "/crew/") {
		// Parse rig/crew/name format
		parts := strings.SplitN(args[0], "/crew/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			// Route to crew start with rig/name format
			crewArg := parts[0] + "/" + parts[1]
			return runStartCrew(cmd, []string{crewArg})
		}
	}

	// Verify we're in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	if err := config.EnsureDaemonPatrolConfig(townRoot); err != nil {
		fmt.Printf("  %s Could not ensure daemon config: %v\n", style.Dim.Render("○"), err)
	}

	t := tmux.NewTmux()

	fmt.Printf("Starting Gas Town from %s\n\n", style.Dim.Render(townRoot))

	// Start core agents (Mayor and Deacon)
	if err := startCoreAgents(townRoot, startAgentOverride); err != nil {
		return err
	}

	// If --all, start witnesses and refineries for all rigs
	if startAll {
		fmt.Println()
		fmt.Println("Starting rig agents...")
		startRigAgents(t, townRoot)
	}

	// Auto-start configured crew for each rig
	fmt.Println()
	fmt.Println("Starting configured crew...")
	startConfiguredCrew(t, townRoot)

	fmt.Println()
	fmt.Printf("%s Gas Town is running\n", style.Bold.Render("✓"))
	fmt.Println()
	fmt.Printf("  Attach to Mayor:  %s\n", style.Dim.Render("gt mayor attach"))
	fmt.Printf("  Attach to Deacon: %s\n", style.Dim.Render("gt deacon attach"))
	fmt.Printf("  Check status:     %s\n", style.Dim.Render("gt status"))

	return nil
}

// startCoreAgents starts Mayor and Deacon sessions using the Manager pattern.
func startCoreAgents(townRoot string, agentOverride string) error {
	// Start Mayor first (so Deacon sees it as up)
	mayorMgr := mayor.NewManager(townRoot)
	if err := mayorMgr.Start(agentOverride); err != nil {
		if err == mayor.ErrAlreadyRunning {
			fmt.Printf("  %s Mayor already running\n", style.Dim.Render("○"))
		} else {
			return fmt.Errorf("starting Mayor: %w", err)
		}
	} else {
		fmt.Printf("  %s Mayor started\n", style.Bold.Render("✓"))
	}

	// Start Deacon (health monitor)
	deaconMgr := deacon.NewManager(townRoot)
	if err := deaconMgr.Start(agentOverride); err != nil {
		if err == deacon.ErrAlreadyRunning {
			fmt.Printf("  %s Deacon already running\n", style.Dim.Render("○"))
		} else {
			return fmt.Errorf("starting Deacon: %w", err)
		}
	} else {
		fmt.Printf("  %s Deacon started\n", style.Bold.Render("✓"))
	}

	return nil
}

// startRigAgents starts witness and refinery for all rigs.
// Called when --all flag is passed to gt start.
func startRigAgents(t *tmux.Tmux, townRoot string) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		return
	}

	for _, r := range rigs {
		// Start Witness
		witnessSession := fmt.Sprintf("gt-%s-witness", r.Name)
		witnessRunning, _ := t.HasSession(witnessSession)
		if witnessRunning {
			fmt.Printf("  %s %s witness already running\n", style.Dim.Render("○"), r.Name)
		} else {
			witMgr := witness.NewManager(r)
			if err := witMgr.Start(false, ""); err != nil {
				if err == witness.ErrAlreadyRunning {
					fmt.Printf("  %s %s witness already running\n", style.Dim.Render("○"), r.Name)
				} else {
					fmt.Printf("  %s %s witness failed: %v\n", style.Dim.Render("○"), r.Name, err)
				}
			} else {
				fmt.Printf("  %s %s witness started\n", style.Bold.Render("✓"), r.Name)
			}
		}

		// Start Refinery
		refineryMgr := refinery.NewManager(r)
		if err := refineryMgr.Start(false); err != nil {
			if errors.Is(err, refinery.ErrAlreadyRunning) {
				fmt.Printf("  %s %s refinery already running\n", style.Dim.Render("○"), r.Name)
			} else {
				fmt.Printf("  %s %s refinery failed: %v\n", style.Dim.Render("○"), r.Name, err)
			}
		} else {
			fmt.Printf("  %s %s refinery started\n", style.Bold.Render("✓"), r.Name)
		}
	}
}

// startConfiguredCrew starts crew members configured in rig settings.
func startConfiguredCrew(t *tmux.Tmux, townRoot string) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		return
	}

	startedAny := false
	for _, r := range rigs {
		crewToStart := getCrewToStart(r)
		for _, crewName := range crewToStart {
			sessionID := crewSessionName(r.Name, crewName)
			if running, _ := t.HasSession(sessionID); running {
				// Session exists - check if Claude is still running
				agentCfg := config.ResolveAgentConfig(townRoot, r.Path)
				if !t.IsAgentRunning(sessionID, config.ExpectedPaneCommands(agentCfg)...) {
					// Claude has exited, restart it
					fmt.Printf("  %s %s/%s session exists, restarting Claude...\n", style.Dim.Render("○"), r.Name, crewName)
					// Build startup beacon for predecessor discovery via /resume
					address := fmt.Sprintf("%s/crew/%s", r.Name, crewName)
					beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
						Recipient: address,
						Sender:    "human",
						Topic:     "restart",
					})
					claudeCmd := config.BuildCrewStartupCommand(r.Name, crewName, r.Path, beacon)
					if err := t.SendKeys(sessionID, claudeCmd); err != nil {
						fmt.Printf("  %s %s/%s restart failed: %v\n", style.Dim.Render("○"), r.Name, crewName, err)
					} else {
						fmt.Printf("  %s %s/%s Claude restarted\n", style.Bold.Render("✓"), r.Name, crewName)
						startedAny = true
					}
				} else {
					fmt.Printf("  %s %s/%s already running\n", style.Dim.Render("○"), r.Name, crewName)
				}
			} else {
				if err := startCrewMember(r.Name, crewName, townRoot); err != nil {
					fmt.Printf("  %s %s/%s failed: %v\n", style.Dim.Render("○"), r.Name, crewName, err)
				} else {
					fmt.Printf("  %s %s/%s started\n", style.Bold.Render("✓"), r.Name, crewName)
					startedAny = true
				}
			}
		}
	}

	if !startedAny {
		fmt.Printf("  %s No crew configured or all already running\n", style.Dim.Render("○"))
	}
}

// discoverAllRigs finds all rigs in the workspace.
func discoverAllRigs(townRoot string) ([]*rig.Rig, error) {
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	return rigMgr.DiscoverRigs()
}

func runShutdown(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	// Find workspace root for polecat cleanup
	townRoot, _ := workspace.FindFromCwd()

	// Collect sessions to show what will be stopped
	sessions, err := t.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Get session names for categorization
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()
	toStop, preserved := categorizeSessions(sessions, mayorSession, deaconSession)

	if len(toStop) == 0 {
		fmt.Printf("%s Gas Town was not running\n", style.Dim.Render("○"))
		return nil
	}

	// Show what will happen
	fmt.Println("Sessions to stop:")
	for _, sess := range toStop {
		fmt.Printf("  %s %s\n", style.Bold.Render("→"), sess)
	}
	if len(preserved) > 0 && !shutdownAll {
		fmt.Println()
		fmt.Println("Sessions preserved (crew):")
		for _, sess := range preserved {
			fmt.Printf("  %s %s\n", style.Dim.Render("○"), sess)
		}
	}
	fmt.Println()

	// Confirmation prompt
	if !shutdownYes && !shutdownForce {
		fmt.Printf("Proceed with shutdown? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Shutdown canceled.")
			return nil
		}
	}

	if shutdownGraceful {
		return runGracefulShutdown(t, toStop, townRoot)
	}
	return runImmediateShutdown(t, toStop, townRoot)
}

// categorizeSessions splits sessions into those to stop and those to preserve.
// mayorSession and deaconSession are the dynamic session names for the current town.
func categorizeSessions(sessions []string, mayorSession, deaconSession string) (toStop, preserved []string) {
	for _, sess := range sessions {
		// Gas Town sessions use gt- (rig-level) or hq- (town-level) prefix
		if !strings.HasPrefix(sess, "gt-") && !strings.HasPrefix(sess, "hq-") {
			continue // Not a Gas Town session
		}

		// Check if it's a crew session (pattern: gt-<rig>-crew-<name>)
		isCrew := strings.Contains(sess, "-crew-")

		// Check if it's a polecat session (pattern: gt-<rig>-<name> where name is not crew/witness/refinery)
		isPolecat := false
		if !isCrew && sess != mayorSession && sess != deaconSession {
			parts := strings.Split(sess, "-")
			if len(parts) >= 3 {
				role := parts[2]
				if role != "witness" && role != "refinery" && role != "crew" {
					isPolecat = true
				}
			}
		}

		// Decide based on flags
		if shutdownPolecatsOnly {
			// Only stop polecats
			if isPolecat {
				toStop = append(toStop, sess)
			} else {
				preserved = append(preserved, sess)
			}
		} else if shutdownAll {
			// Stop everything including crew
			toStop = append(toStop, sess)
		} else {
			// Default: preserve crew
			if isCrew {
				preserved = append(preserved, sess)
			} else {
				toStop = append(toStop, sess)
			}
		}
	}
	return
}

func runGracefulShutdown(t *tmux.Tmux, gtSessions []string, townRoot string) error {
	fmt.Printf("Graceful shutdown of Gas Town (waiting up to %ds)...\n\n", shutdownWait)

	// Phase 1: Send ESC to all agents to interrupt them
	fmt.Printf("Phase 1: Sending ESC to %d agent(s)...\n", len(gtSessions))
	for _, sess := range gtSessions {
		fmt.Printf("  %s Interrupting %s\n", style.Bold.Render("→"), sess)
		_ = t.SendKeysRaw(sess, "Escape") // best-effort interrupt
	}

	// Phase 2: Send shutdown message asking agents to handoff
	fmt.Printf("\nPhase 2: Requesting handoff from agents...\n")
	shutdownMsg := "[SHUTDOWN] Gas Town is shutting down. Please save your state and update your handoff bead, then type /exit or wait to be terminated."
	for _, sess := range gtSessions {
		// Small delay then send the message
		time.Sleep(constants.ShutdownNotifyDelay)
		_ = t.SendKeys(sess, shutdownMsg) // best-effort notification
	}

	// Phase 3: Wait for agents to complete handoff
	fmt.Printf("\nPhase 3: Waiting %ds for agents to complete handoff...\n", shutdownWait)
	fmt.Printf("  %s\n", style.Dim.Render("(Press Ctrl-C to force immediate shutdown)"))

	// Wait with countdown
	for remaining := shutdownWait; remaining > 0; remaining -= 5 {
		if remaining < shutdownWait {
			fmt.Printf("  %s %ds remaining...\n", style.Dim.Render("⏳"), remaining)
		}
		sleepTime := 5
		if remaining < 5 {
			sleepTime = remaining
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	// Phase 4: Kill sessions in correct order
	fmt.Printf("\nPhase 4: Terminating sessions...\n")
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()
	stopped := killSessionsInOrder(t, gtSessions, mayorSession, deaconSession)

	// Phase 5: Cleanup polecat worktrees and branches
	fmt.Printf("\nPhase 5: Cleaning up polecats...\n")
	if townRoot != "" {
		cleanupPolecats(townRoot)
	}

	// Phase 6: Stop the daemon
	fmt.Printf("\nPhase 6: Stopping daemon...\n")
	if townRoot != "" {
		stopDaemonIfRunning(townRoot)
	}

	fmt.Println()
	fmt.Printf("%s Graceful shutdown complete (%d sessions stopped)\n", style.Bold.Render("✓"), stopped)
	return nil
}

func runImmediateShutdown(t *tmux.Tmux, gtSessions []string, townRoot string) error {
	fmt.Println("Shutting down Gas Town...")

	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()
	stopped := killSessionsInOrder(t, gtSessions, mayorSession, deaconSession)

	// Cleanup polecat worktrees and branches
	if townRoot != "" {
		fmt.Println()
		fmt.Println("Cleaning up polecats...")
		cleanupPolecats(townRoot)
	}

	// Stop the daemon
	if townRoot != "" {
		fmt.Println()
		fmt.Println("Stopping daemon...")
		stopDaemonIfRunning(townRoot)
	}

	fmt.Println()
	fmt.Printf("%s Gas Town shutdown complete (%d sessions stopped)\n", style.Bold.Render("✓"), stopped)

	return nil
}

// killSessionsInOrder stops sessions in the correct order:
// 1. Deacon first (so it doesn't restart others)
// 2. Everything except Mayor
// 3. Mayor last
// mayorSession and deaconSession are the dynamic session names for the current town.
func killSessionsInOrder(t *tmux.Tmux, sessions []string, mayorSession, deaconSession string) int {
	stopped := 0

	// Helper to check if session is in our list
	inList := func(sess string) bool {
		for _, s := range sessions {
			if s == sess {
				return true
			}
		}
		return false
	}

	// 1. Stop Deacon first
	if inList(deaconSession) {
		if err := t.KillSession(deaconSession); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), deaconSession)
			stopped++
		}
	}

	// 2. Stop others (except Mayor)
	for _, sess := range sessions {
		if sess == deaconSession || sess == mayorSession {
			continue
		}
		if err := t.KillSession(sess); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), sess)
			stopped++
		}
	}

	// 3. Stop Mayor last
	if inList(mayorSession) {
		if err := t.KillSession(mayorSession); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), mayorSession)
			stopped++
		}
	}

	return stopped
}

// cleanupPolecats removes polecat worktrees and branches for all rigs.
// It refuses to clean up polecats with uncommitted work unless --nuclear is set.
func cleanupPolecats(townRoot string) {
	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		fmt.Printf("  %s Could not load rigs config: %v\n", style.Dim.Render("○"), err)
		return
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	// Discover all rigs
	rigs, err := rigMgr.DiscoverRigs()
	if err != nil {
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		return
	}

	totalCleaned := 0
	totalSkipped := 0
	var uncommittedPolecats []string

	for _, r := range rigs {
		polecatGit := git.NewGit(r.Path)
		polecatMgr := polecat.NewManager(r, polecatGit)

		polecats, err := polecatMgr.List()
		if err != nil {
			continue
		}

		for _, p := range polecats {
			// Check for uncommitted work
			pGit := git.NewGit(p.ClonePath)
			status, err := pGit.CheckUncommittedWork()
			if err != nil {
				// Can't check, be safe and skip unless nuclear
				if !shutdownNuclear {
					fmt.Printf("  %s %s/%s: could not check status, skipping\n",
						style.Dim.Render("○"), r.Name, p.Name)
					totalSkipped++
					continue
				}
			} else if !status.Clean() {
				// Has uncommitted work
				if !shutdownNuclear {
					uncommittedPolecats = append(uncommittedPolecats,
						fmt.Sprintf("%s/%s (%s)", r.Name, p.Name, status.String()))
					totalSkipped++
					continue
				}
				// Nuclear mode: warn but proceed
				fmt.Printf("  %s %s/%s: NUCLEAR - removing despite %s\n",
					style.Bold.Render("⚠"), r.Name, p.Name, status.String())
			}

			// Clean: remove worktree and branch
			if err := polecatMgr.RemoveWithOptions(p.Name, true, shutdownNuclear); err != nil {
				fmt.Printf("  %s %s/%s: cleanup failed: %v\n",
					style.Dim.Render("○"), r.Name, p.Name, err)
				totalSkipped++
				continue
			}

			// Delete the polecat branch from mayor's clone
			branchName := fmt.Sprintf("polecat/%s", p.Name)
			mayorPath := filepath.Join(r.Path, "mayor", "rig")
			mayorGit := git.NewGit(mayorPath)
			_ = mayorGit.DeleteBranch(branchName, true) // Ignore errors

			fmt.Printf("  %s %s/%s: cleaned up\n", style.Bold.Render("✓"), r.Name, p.Name)
			totalCleaned++
		}
	}

	// Summary
	if len(uncommittedPolecats) > 0 {
		fmt.Println()
		fmt.Printf("  %s Polecats with uncommitted work (use --nuclear to force):\n",
			style.Bold.Render("⚠"))
		for _, pc := range uncommittedPolecats {
			fmt.Printf("    • %s\n", pc)
		}
	}

	if totalCleaned > 0 || totalSkipped > 0 {
		fmt.Printf("  Cleaned: %d, Skipped: %d\n", totalCleaned, totalSkipped)
	} else {
		fmt.Printf("  %s No polecats to clean up\n", style.Dim.Render("○"))
	}
}

// stopDaemonIfRunning stops the daemon if it is running.
// This prevents the daemon from restarting agents after shutdown.
func stopDaemonIfRunning(townRoot string) {
	running, _, _ := daemon.IsRunning(townRoot)
	if running {
		if err := daemon.StopDaemon(townRoot); err != nil {
			fmt.Printf("  %s Daemon: %s\n", style.Dim.Render("○"), err.Error())
		} else {
			fmt.Printf("  %s Daemon stopped\n", style.Bold.Render("✓"))
		}
	} else {
		fmt.Printf("  %s Daemon not running\n", style.Dim.Render("○"))
	}
}

// runStartCrew starts a crew workspace, creating it if it doesn't exist.
// This combines the functionality of 'gt crew add' and 'gt crew at --detached'.
func runStartCrew(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Parse rig/name format (e.g., "greenplace/joe" -> rig=gastown, name=joe)
	rigName := startCrewRig
	if parsedRig, crewName, ok := parseRigSlashName(name); ok {
		if rigName == "" {
			rigName = parsedRig
		}
		name = crewName
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// If rig still not specified, try to infer from cwd
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag or rig/name format): %w", err)
		}
	}

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Get rig
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Create crew manager
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	// Resolve account for Claude config
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, startCrewAccount)
	if err != nil {
		return fmt.Errorf("resolving account: %w", err)
	}
	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Use manager's Start() method - handles workspace creation, settings, and session
	err = crewMgr.Start(name, crew.StartOptions{
		Account:         startCrewAccount,
		ClaudeConfigDir: claudeConfigDir,
		AgentOverride:   startCrewAgentOverride,
	})
	if err != nil {
		if errors.Is(err, crew.ErrSessionRunning) {
			fmt.Printf("%s Session already running: %s\n", style.Dim.Render("○"), crewMgr.SessionName(name))
		} else {
			return err
		}
	} else {
		fmt.Printf("%s Started crew workspace: %s/%s\n",
			style.Bold.Render("✓"), rigName, name)
	}

	fmt.Printf("Attach with: %s\n", style.Dim.Render(fmt.Sprintf("gt crew at %s", name)))
	return nil
}

// getCrewToStart reads rig settings and parses the crew.startup field.
// Returns a list of crew names to start.
func getCrewToStart(r *rig.Rig) []string {
	// Load rig settings
	settingsPath := filepath.Join(r.Path, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return nil
	}

	if settings.Crew == nil || settings.Crew.Startup == "" || settings.Crew.Startup == "none" {
		return nil
	}

	startup := settings.Crew.Startup

	// Handle "all" - list all existing crew
	if startup == "all" {
		crewGit := git.NewGit(r.Path)
		crewMgr := crew.NewManager(r, crewGit)
		workers, err := crewMgr.List()
		if err != nil {
			return nil
		}
		var names []string
		for _, w := range workers {
			names = append(names, w.Name)
		}
		return names
	}

	// Parse names: "max", "max and joe", "max, joe", "max, joe, emma"
	// Replace "and" with comma for uniform parsing
	startup = strings.ReplaceAll(startup, " and ", ", ")
	parts := strings.Split(startup, ",")

	var names []string
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			names = append(names, name)
		}
	}

	return names
}

// startCrewMember starts a single crew member, creating if needed.
// This is a simplified version of runStartCrew that doesn't print output.
func startCrewMember(rigName, crewName, townRoot string) error {
	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Get rig
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Create crew manager and use Start() method
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	// Start handles workspace creation, settings, and session all in one
	err = crewMgr.Start(crewName, crew.StartOptions{})
	if err != nil && !errors.Is(err, crew.ErrSessionRunning) {
		return err
	}

	return nil
}
