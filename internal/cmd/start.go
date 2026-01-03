package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	startAll             bool
	startCrewRig         string
	startCrewAccount     string
	shutdownGraceful     bool
	shutdownWait         int
	shutdownAll          bool
	shutdownForce        bool
	shutdownYes          bool
	shutdownPolecatsOnly bool
	shutdownNuclear      bool
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

	startCrewCmd.Flags().StringVar(&startCrewRig, "rig", "", "Rig to use")
	startCrewCmd.Flags().StringVar(&startCrewAccount, "account", "", "Claude Code account handle to use")
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

	t := tmux.NewTmux()

	fmt.Printf("Starting Gas Town from %s\n\n", style.Dim.Render(townRoot))

	// Start core agents (Mayor and Deacon)
	if err := startCoreAgents(t); err != nil {
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

// startCoreAgents starts Mayor and Deacon sessions.
func startCoreAgents(t *tmux.Tmux) error {
	// Get session names
	mayorSession, err := getMayorSessionName()
	if err != nil {
		return fmt.Errorf("getting Mayor session name: %w", err)
	}
	deaconSession, err := getDeaconSessionName()
	if err != nil {
		return fmt.Errorf("getting Deacon session name: %w", err)
	}

	// Start Mayor first (so Deacon sees it as up)
	mayorRunning, _ := t.HasSession(mayorSession)
	if mayorRunning {
		fmt.Printf("  %s Mayor already running\n", style.Dim.Render("○"))
	} else {
		fmt.Printf("  %s Starting Mayor...\n", style.Bold.Render("→"))
		if err := startMayorSession(t, mayorSession); err != nil {
			return fmt.Errorf("starting Mayor: %w", err)
		}
		fmt.Printf("  %s Mayor started\n", style.Bold.Render("✓"))
	}

	// Start Deacon (health monitor)
	deaconRunning, _ := t.HasSession(deaconSession)
	if deaconRunning {
		fmt.Printf("  %s Deacon already running\n", style.Dim.Render("○"))
	} else {
		fmt.Printf("  %s Starting Deacon...\n", style.Bold.Render("→"))
		if err := startDeaconSession(t, deaconSession); err != nil {
			return fmt.Errorf("starting Deacon: %w", err)
		}
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
			created, err := ensureWitnessSession(r.Name, r)
			if err != nil {
				fmt.Printf("  %s %s witness failed: %v\n", style.Dim.Render("○"), r.Name, err)
			} else if created {
				fmt.Printf("  %s %s witness started\n", style.Bold.Render("✓"), r.Name)
			}
		}

		// Start Refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", r.Name)
		refineryRunning, _ := t.HasSession(refinerySession)
		if refineryRunning {
			fmt.Printf("  %s %s refinery already running\n", style.Dim.Render("○"), r.Name)
		} else {
			created, err := ensureRefinerySession(r.Name, r)
			if err != nil {
				fmt.Printf("  %s %s refinery failed: %v\n", style.Dim.Render("○"), r.Name, err)
			} else if created {
				fmt.Printf("  %s %s refinery started\n", style.Bold.Render("✓"), r.Name)
			}
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
				fmt.Printf("  %s %s/%s already running\n", style.Dim.Render("○"), r.Name, crewName)
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

// ensureRefinerySession creates a refinery tmux session if it doesn't exist.
// Returns true if a new session was created, false if it already existed.
func ensureRefinerySession(rigName string, r *rig.Rig) (bool, error) {
	t := tmux.NewTmux()
	sessionName := fmt.Sprintf("gt-%s-refinery", rigName)

	// Check if session already exists
	running, err := t.HasSession(sessionName)
	if err != nil {
		return false, fmt.Errorf("checking session: %w", err)
	}

	if running {
		return false, nil
	}

	// Working directory is the refinery's rig clone
	refineryRigDir := filepath.Join(r.Path, "refinery", "rig")
	if _, err := os.Stat(refineryRigDir); os.IsNotExist(err) {
		// Fall back to rig path if refinery/rig doesn't exist
		refineryRigDir = r.Path
	}

	// Ensure Claude settings exist (autonomous role needs mail in SessionStart)
	if err := claude.EnsureSettingsForRole(refineryRigDir, "refinery"); err != nil {
		return false, fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Create new tmux session
	if err := t.NewSession(sessionName, refineryRigDir); err != nil {
		return false, fmt.Errorf("creating session: %w", err)
	}

	// Set environment
	bdActor := fmt.Sprintf("%s/refinery", rigName)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "refinery")
	_ = t.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Set beads environment
	beadsDir := filepath.Join(r.Path, "mayor", "rig", ".beads")
	_ = t.SetEnvironment(sessionName, "BEADS_DIR", beadsDir)
	_ = t.SetEnvironment(sessionName, "BEADS_NO_DAEMON", "1")
	_ = t.SetEnvironment(sessionName, "BEADS_AGENT_NAME", fmt.Sprintf("%s/refinery", rigName))

	// Apply Gas Town theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(rigName)
	_ = t.ConfigureGasTownSession(sessionName, theme, rigName, "refinery", "refinery")

	// Launch Claude directly (no respawn loop - daemon handles restart)
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	if err := t.SendKeys(sessionName, config.BuildAgentStartupCommand("refinery", bdActor, "", "")); err != nil {
		return false, fmt.Errorf("sending command: %w", err)
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}
	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/refinery", rigName)
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "patrol",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous patrol execution.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionName, session.PropulsionNudgeForRole("refinery", refineryRigDir)) // Non-fatal

	return true, nil
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
	mayorSession, _ := getMayorSessionName()
	deaconSession, _ := getDeaconSessionName()
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
		if !strings.HasPrefix(sess, "gt-") {
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
	mayorSession, _ := getMayorSessionName()
	deaconSession, _ := getDeaconSessionName()
	stopped := killSessionsInOrder(t, gtSessions, mayorSession, deaconSession)

	// Phase 5: Cleanup polecat worktrees and branches
	fmt.Printf("\nPhase 5: Cleaning up polecats...\n")
	if townRoot != "" {
		cleanupPolecats(townRoot)
	}

	fmt.Println()
	fmt.Printf("%s Graceful shutdown complete (%d sessions stopped)\n", style.Bold.Render("✓"), stopped)
	return nil
}

func runImmediateShutdown(t *tmux.Tmux, gtSessions []string, townRoot string) error {
	fmt.Println("Shutting down Gas Town...")

	mayorSession, _ := getMayorSessionName()
	deaconSession, _ := getDeaconSessionName()
	stopped := killSessionsInOrder(t, gtSessions, mayorSession, deaconSession)

	// Cleanup polecat worktrees and branches
	if townRoot != "" {
		fmt.Println()
		fmt.Println("Cleaning up polecats...")
		cleanupPolecats(townRoot)
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

	// Check if crew exists, create if not
	worker, err := crewMgr.Get(name)
	if err == crew.ErrCrewNotFound {
		fmt.Printf("Creating crew workspace %s in %s...\n", name, rigName)
		worker, err = crewMgr.Add(name, false) // No feature branch for crew
		if err != nil {
			return fmt.Errorf("creating crew workspace: %w", err)
		}
		fmt.Printf("%s Created crew workspace: %s/%s\n",
			style.Bold.Render("✓"), rigName, name)
	} else if err != nil {
		return fmt.Errorf("getting crew worker: %w", err)
	} else {
		fmt.Printf("Crew workspace %s/%s exists\n", rigName, name)
	}

	// Ensure crew workspace is on main branch
	ensureMainBranch(worker.ClonePath, fmt.Sprintf("Crew workspace %s/%s", rigName, name))

	// Resolve account for Claude config
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, startCrewAccount)
	if err != nil {
		return fmt.Errorf("resolving account: %w", err)
	}
	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Check if session exists
	t := tmux.NewTmux()
	sessionID := crewSessionName(rigName, name)
	hasSession, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if hasSession {
		// Session exists - check if Claude is still running
		if !t.IsClaudeRunning(sessionID) {
			// Claude has exited, restart it
			fmt.Printf("Session exists, restarting Claude...\n")
			claudeCmd := config.BuildCrewStartupCommand(rigName, name, r.Path, "")
			if err := t.SendKeys(sessionID, claudeCmd); err != nil {
				return fmt.Errorf("restarting claude: %w", err)
			}
			// Wait for Claude to start, then prime
			shells := constants.SupportedShells
			if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
				style.PrintWarning("Timeout waiting for Claude to start: %v", err)
			}
			time.Sleep(constants.ShutdownNotifyDelay)
			if err := t.NudgeSession(sessionID, "gt prime"); err != nil {
				style.PrintWarning("Could not send prime command: %v", err)
			}
		} else {
			fmt.Printf("%s Session already running: %s\n", style.Dim.Render("○"), sessionID)
		}
	} else {
		// Create new session
		if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
			return fmt.Errorf("creating session: %w", err)
		}

		// Set environment (non-fatal: session works without these)
		_ = t.SetEnvironment(sessionID, "GT_RIG", rigName)
		_ = t.SetEnvironment(sessionID, "GT_CREW", name)

		// Set CLAUDE_CONFIG_DIR for account selection (non-fatal)
		if claudeConfigDir != "" {
			_ = t.SetEnvironment(sessionID, "CLAUDE_CONFIG_DIR", claudeConfigDir)
		}

		// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
		// Note: ConfigureGasTownSession includes cycle bindings
		theme := getThemeForRig(rigName)
		_ = t.ConfigureGasTownSession(sessionID, theme, rigName, name, "crew")

		// Wait for shell to be ready after session creation
		if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
			return fmt.Errorf("waiting for shell: %w", err)
		}

		// Start claude with skip permissions and proper env vars for seance
		claudeCmd := config.BuildCrewStartupCommand(rigName, name, r.Path, "")
		if err := t.SendKeys(sessionID, claudeCmd); err != nil {
			return fmt.Errorf("starting claude: %w", err)
		}

		// Wait for Claude to start
		shells := constants.SupportedShells
		if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
			style.PrintWarning("Timeout waiting for Claude to start: %v", err)
		}

		// Give Claude time to initialize after process starts
		time.Sleep(constants.ShutdownNotifyDelay)

		// Inject startup nudge for predecessor discovery via /resume
		address := fmt.Sprintf("%s/crew/%s", rigName, name)
		_ = session.StartupNudge(t, sessionID, session.StartupNudgeConfig{
			Recipient: address,
			Sender:    "human",
			Topic:     "cold-start",
		}) // Non-fatal: session works without nudge

		// Send gt prime to initialize context
		if err := t.NudgeSession(sessionID, "gt prime"); err != nil {
			style.PrintWarning("Could not send prime command: %v", err)
		}

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

	// Create crew manager
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	// Check if crew exists, create if not
	worker, err := crewMgr.Get(crewName)
	if err == crew.ErrCrewNotFound {
		worker, err = crewMgr.Add(crewName, false)
		if err != nil {
			return fmt.Errorf("creating crew workspace: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("getting crew worker: %w", err)
	}

	// Ensure crew workspace is on main branch
	ensureMainBranch(worker.ClonePath, fmt.Sprintf("Crew workspace %s/%s", rigName, crewName))

	// Create tmux session
	t := tmux.NewTmux()
	sessionID := crewSessionName(rigName, crewName)

	if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	_ = t.SetEnvironment(sessionID, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionID, "GT_CREW", crewName)

	// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
	theme := getThemeForRig(rigName)
	_ = t.ConfigureGasTownSession(sessionID, theme, rigName, crewName, "crew")

	// Set up C-b n/p keybindings for crew session cycling (non-fatal)
	_ = t.SetCrewCycleBindings(sessionID)

	// Wait for shell to be ready
	if err := t.WaitForShellReady(sessionID, constants.ShellReadyTimeout); err != nil {
		return fmt.Errorf("waiting for shell: %w", err)
	}

	// Start claude with proper env vars for seance
	claudeCmd := config.BuildCrewStartupCommand(rigName, crewName, r.Path, "")
	if err := t.SendKeys(sessionID, claudeCmd); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	// Wait for Claude to start
	shells := constants.SupportedShells
	if err := t.WaitForCommand(sessionID, shells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal: Claude might still be starting
	}

	// Give Claude time to initialize
	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	_ = session.StartupNudge(t, sessionID, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     "cold-start",
	}) // Non-fatal

	// Send gt prime to initialize context (non-fatal: session works without priming)
	_ = t.NudgeSession(sessionID, "gt prime")

	return nil
}
