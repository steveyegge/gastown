package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
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
	Short:   "Shutdown Gas Town with cleanup",
	Long: `Shutdown Gas Town by stopping agents and cleaning up polecats.

This is the "done for the day" command - it stops everything AND removes
polecat worktrees/branches. For a reversible pause, use 'gt down' instead.

Comparison:
  gt down      - Pause (stop processes, keep worktrees) - reversible
  gt shutdown  - Done (stop + cleanup worktrees) - permanent cleanup

After killing sessions, polecats are cleaned up:
  - Worktrees are removed
  - Polecat branches are deleted
  - Polecats with uncommitted work are SKIPPED (protected)

Shutdown levels (progressively more aggressive):
  (default)       - Stop infrastructure + polecats + cleanup
  --all           - Also stop crew sessions
  --polecats-only - Only stop polecats (leaves infrastructure running)

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

	fmt.Printf("Starting Gas Town from %s\n\n", style.Dim.Render(townRoot))
	fmt.Println("Starting all agents in parallel...")
	fmt.Println()

	// Start all agent groups in parallel for maximum speed
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects stdout
	var coreErr error

	// Start core agents (Mayor and Deacon) in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := startCoreAgentsParallel(townRoot, startAgentOverride, &mu); err != nil {
			mu.Lock()
			coreErr = err
			mu.Unlock()
		}
	}()

	// Start rig agents (witnesses, refineries) if --all
	if startAll {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startRigAgentsParallel(townRoot, &mu)
		}()
	}

	// Start configured crew
	wg.Add(1)
	go func() {
		defer wg.Done()
		startConfiguredCrewParallel(townRoot, &mu)
	}()

	wg.Wait()

	if coreErr != nil {
		return coreErr
	}

	fmt.Println()
	fmt.Printf("%s Gas Town is running\n", style.Bold.Render("✓"))
	fmt.Println()
	fmt.Printf("  Attach to Mayor:  %s\n", style.Dim.Render("gt mayor attach"))
	fmt.Printf("  Attach to Deacon: %s\n", style.Dim.Render("gt deacon attach"))
	fmt.Printf("  Check status:     %s\n", style.Dim.Render("gt status"))

	return nil
}

// startCoreAgents starts Mayor and Deacon sessions.
func startCoreAgents(townRoot string, agentOverride string) error {
	// Start Mayor first (so Deacon sees it as up) - agent resolved automatically, with optional override
	if _, err := factory.Start(townRoot, agent.MayorAddress, factory.WithAgent(agentOverride)); err != nil {
		if err == agent.ErrAlreadyRunning {
			fmt.Printf("  %s Mayor already running\n", style.Dim.Render("○"))
		} else {
			return fmt.Errorf("starting Mayor: %w", err)
		}
	} else {
		fmt.Printf("  %s Mayor started\n", style.Bold.Render("✓"))
	}

	// Start Deacon (health monitor) - agent resolved automatically, with optional override
	if _, err := factory.Start(townRoot, agent.DeaconAddress, factory.WithAgent(agentOverride)); err != nil {
		if err == agent.ErrAlreadyRunning {
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
func startRigAgents(townRoot string) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		return
	}

	for _, r := range rigs {
		// Start Witness (agent resolved automatically)
		witnessID := agent.WitnessAddress(r.Name)
		if _, err := factory.Start(townRoot, witnessID); err != nil {
			if err == agent.ErrAlreadyRunning {
				fmt.Printf("  %s %s witness already running\n", style.Dim.Render("○"), r.Name)
			} else {
				fmt.Printf("  %s %s witness failed: %v\n", style.Dim.Render("○"), r.Name, err)
			}
		} else {
			fmt.Printf("  %s %s witness started\n", style.Bold.Render("✓"), r.Name)
		}

		// Start Refinery (agent resolved automatically)
		refineryID := agent.RefineryAddress(r.Name)
		if _, err := factory.Start(townRoot, refineryID); err != nil {
			if errors.Is(err, agent.ErrAlreadyRunning) {
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
// Uses crew.Manager.Start() which handles zombie detection automatically.
func startConfiguredCrew(townRoot string) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		return
	}

	startedAny := false
	for _, r := range rigs {
		crewToStart := getCrewToStart(r, townRoot)
		for _, crewName := range crewToStart {
			err := startCrewMember(r.Name, crewName, townRoot)
			if err != nil {
				if errors.Is(err, crew.ErrSessionRunning) {
					fmt.Printf("  %s %s/%s already running\n", style.Dim.Render("○"), r.Name, crewName)
				} else {
					fmt.Printf("  %s %s/%s failed: %v\n", style.Dim.Render("○"), r.Name, crewName, err)
				}
			} else {
				fmt.Printf("  %s %s/%s started\n", style.Bold.Render("✓"), r.Name, crewName)
				startedAny = true
			}
		}
	}

	if !startedAny {
		fmt.Printf("  %s No crew configured or all already running\n", style.Dim.Render("○"))
	}
}

// startCoreAgentsParallel starts Mayor and Deacon sessions in parallel using factory.Start().
// The mutex is used to synchronize output with other parallel startup operations.
func startCoreAgentsParallel(townRoot string, agentOverride string, mu *sync.Mutex) error {
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	// Start Mayor in goroutine (agent resolved automatically, with optional override)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := factory.Start(townRoot, agent.MayorAddress, factory.WithAgent(agentOverride)); err != nil {
			if errors.Is(err, agent.ErrAlreadyRunning) {
				mu.Lock()
				fmt.Printf("  %s Mayor already running\n", style.Dim.Render("○"))
				mu.Unlock()
			} else {
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("starting Mayor: %w", err)
				}
				errMu.Unlock()
				mu.Lock()
				fmt.Printf("  %s Mayor failed: %v\n", style.Dim.Render("○"), err)
				mu.Unlock()
			}
		} else {
			mu.Lock()
			fmt.Printf("  %s Mayor started\n", style.Bold.Render("✓"))
			mu.Unlock()
		}
	}()

	// Start Deacon in goroutine (agent resolved automatically, with optional override)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := factory.Start(townRoot, agent.DeaconAddress, factory.WithAgent(agentOverride)); err != nil {
			if errors.Is(err, agent.ErrAlreadyRunning) {
				mu.Lock()
				fmt.Printf("  %s Deacon already running\n", style.Dim.Render("○"))
				mu.Unlock()
			} else {
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("starting Deacon: %w", err)
				}
				errMu.Unlock()
				mu.Lock()
				fmt.Printf("  %s Deacon failed: %v\n", style.Dim.Render("○"), err)
				mu.Unlock()
			}
		} else {
			mu.Lock()
			fmt.Printf("  %s Deacon started\n", style.Bold.Render("✓"))
			mu.Unlock()
		}
	}()

	wg.Wait()
	return firstErr
}

// startRigAgentsParallel starts witness and refinery for all rigs in parallel.
// Uses the provided mutex for synchronized output with other parallel operations.
func startRigAgentsParallel(townRoot string, mu *sync.Mutex) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		mu.Lock()
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		mu.Unlock()
		return
	}

	var wg sync.WaitGroup

	for _, r := range rigs {
		wg.Add(2) // Witness + Refinery

		// Start Witness in goroutine
		go func(r *rig.Rig) {
			defer wg.Done()
			msg := startWitnessForRig(townRoot, r)
			mu.Lock()
			fmt.Print(msg)
			mu.Unlock()
		}(r)

		// Start Refinery in goroutine
		go func(r *rig.Rig) {
			defer wg.Done()
			msg := startRefineryForRig(townRoot, r)
			mu.Lock()
			fmt.Print(msg)
			mu.Unlock()
		}(r)
	}

	wg.Wait()
}

// startWitnessForRig starts the witness for a single rig and returns a status message.
func startWitnessForRig(townRoot string, r *rig.Rig) string {
	witnessID := agent.WitnessAddress(r.Name)
	if _, err := factory.Start(townRoot, witnessID); err != nil {
		if errors.Is(err, agent.ErrAlreadyRunning) {
			return fmt.Sprintf("  %s %s witness already running\n", style.Dim.Render("○"), r.Name)
		}
		return fmt.Sprintf("  %s %s witness failed: %v\n", style.Dim.Render("○"), r.Name, err)
	}
	return fmt.Sprintf("  %s %s witness started\n", style.Bold.Render("✓"), r.Name)
}

// startRefineryForRig starts the refinery for a single rig and returns a status message.
func startRefineryForRig(townRoot string, r *rig.Rig) string {
	refineryID := agent.RefineryAddress(r.Name)
	if _, err := factory.Start(townRoot, refineryID); err != nil {
		if errors.Is(err, agent.ErrAlreadyRunning) {
			return fmt.Sprintf("  %s %s refinery already running\n", style.Dim.Render("○"), r.Name)
		}
		return fmt.Sprintf("  %s %s refinery failed: %v\n", style.Dim.Render("○"), r.Name, err)
	}
	return fmt.Sprintf("  %s %s refinery started\n", style.Bold.Render("✓"), r.Name)
}

// startConfiguredCrewParallel starts crew members configured in rig settings in parallel.
// Uses the provided mutex for synchronized output with other parallel operations.
func startConfiguredCrewParallel(townRoot string, mu *sync.Mutex) {
	rigs, err := discoverAllRigs(townRoot)
	if err != nil {
		mu.Lock()
		fmt.Printf("  %s Could not discover rigs: %v\n", style.Dim.Render("○"), err)
		mu.Unlock()
		return
	}

	var wg sync.WaitGroup
	startedAny := false
	var startedMu sync.Mutex // Protects startedAny

	for _, r := range rigs {
		crewToStart := getCrewToStart(r, townRoot)
		for _, crewName := range crewToStart {
			wg.Add(1)
			go func(r *rig.Rig, crewName string) {
				defer wg.Done()
				err := startCrewMember(r.Name, crewName, townRoot)
				if err != nil {
					mu.Lock()
					if errors.Is(err, crew.ErrSessionRunning) {
						fmt.Printf("  %s %s/%s already running\n", style.Dim.Render("○"), r.Name, crewName)
					} else {
						fmt.Printf("  %s %s/%s failed: %v\n", style.Dim.Render("○"), r.Name, crewName, err)
					}
					mu.Unlock()
				} else {
					mu.Lock()
					fmt.Printf("  %s %s/%s started\n", style.Bold.Render("✓"), r.Name, crewName)
					mu.Unlock()
					startedMu.Lock()
					startedAny = true
					startedMu.Unlock()
				}
			}(r, crewName)
		}
	}

	wg.Wait()

	if !startedAny {
		mu.Lock()
		fmt.Printf("  %s No crew configured or all already running\n", style.Dim.Render("○"))
		mu.Unlock()
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
	// Find workspace root for polecat cleanup
	townRoot, _ := workspace.FindFromCwd()

	// Create agents interface - use nil config to include all sessions (not just healthy ones)
	agents := agent.Default()

	// List all agents
	agentList, err := agents.List()
	if err != nil {
		return fmt.Errorf("listing agents: %w", err)
	}

	// Categorize agents by role
	toStop, preserved := categorizeAgents(agentList)

	if len(toStop) == 0 {
		fmt.Printf("%s Gas Town was not running\n", style.Dim.Render("○"))
		return nil
	}

	// Show what will happen
	fmt.Println("Agents to stop:")
	for _, id := range toStop {
		fmt.Printf("  %s %s\n", style.Bold.Render("→"), id)
	}
	if len(preserved) > 0 && !shutdownAll {
		fmt.Println()
		fmt.Println("Agents preserved (crew):")
		for _, id := range preserved {
			fmt.Printf("  %s %s\n", style.Dim.Render("○"), id)
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

	return runAgentShutdown(agents, toStop, townRoot, shutdownGraceful)
}

// categorizeAgents splits agents into those to stop and those to preserve.
// AgentID format: "mayor", "deacon", "rig/witness", "rig/refinery", "rig/crew/name", "rig/polecat/name"
func categorizeAgents(agents []agent.AgentID) (toStop, preserved []agent.AgentID) {
	for _, id := range agents {
		// Determine role from AgentID struct
		isCrew := id.Role == "crew"
		isPolecat := id.Role == "polecat"

		// Decide based on flags
		if shutdownPolecatsOnly {
			// Only stop polecats
			if isPolecat {
				toStop = append(toStop, id)
			} else {
				preserved = append(preserved, id)
			}
		} else if shutdownAll {
			// Stop everything including crew
			toStop = append(toStop, id)
		} else {
			// Default: preserve crew
			if isCrew {
				preserved = append(preserved, id)
			} else {
				toStop = append(toStop, id)
			}
		}
	}
	return
}

// runAgentShutdown stops agents using the Agents interface.
// Stops in order: deacon first (so it doesn't restart others), then others, then mayor last.
func runAgentShutdown(agents agent.Agents, toStop []agent.AgentID, townRoot string, graceful bool) error {
	if graceful {
		fmt.Printf("Graceful shutdown of Gas Town (waiting up to %ds)...\n\n", shutdownWait)
		fmt.Printf("Waiting %ds for agents to complete work...\n", shutdownWait)
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
		fmt.Println()
	}

	fmt.Println("Stopping agents...")
	stopped := stopAgentsInOrder(agents, toStop, graceful)

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
	fmt.Printf("%s Gas Town shutdown complete (%d agents stopped)\n", style.Bold.Render("✓"), stopped)
	return nil
}

// stopAgentsInOrder stops agents in the correct order:
// 1. Deacon first (so it doesn't restart others)
// 2. Everything except Mayor
// 3. Mayor last
func stopAgentsInOrder(agents agent.Agents, toStop []agent.AgentID, graceful bool) int {
	stopped := 0

	// Helper to check if agent is in our list
	inList := func(id agent.AgentID) bool {
		for _, item := range toStop {
			if item == id {
				return true
			}
		}
		return false
	}

	// 1. Stop Deacon first
	deaconID := agent.DeaconAddress
	if inList(deaconID) {
		if err := agents.Stop(deaconID, graceful); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), deaconID)
			stopped++
		}
	}

	// 2. Stop others (except Mayor)
	mayorID := agent.MayorAddress
	for _, id := range toStop {
		if id == deaconID || id == mayorID {
			continue
		}
		if err := agents.Stop(id, graceful); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), id)
			stopped++
		}
	}

	// 3. Stop Mayor last
	if inList(mayorID) {
		if err := agents.Stop(mayorID, graceful); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), mayorID)
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
		polecatMgr := polecat.NewManager(nil, r, polecatGit) // nil agents: just listing, not allocating

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

	// Validate rig exists
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	if _, err := rigMgr.GetRig(rigName); err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Use factory.Start() for crew (agent resolved automatically, with optional override)
	crewID := agent.CrewAddress(rigName, name)
	sessionName := fmt.Sprintf("gt-%s-c-%s", rigName, name)
	if _, err = factory.Start(townRoot, crewID, factory.WithAgent(startCrewAgentOverride)); err != nil {
		if errors.Is(err, agent.ErrAlreadyRunning) {
			fmt.Printf("%s Session already running: %s\n", style.Dim.Render("○"), sessionName)
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
func getCrewToStart(r *rig.Rig, townRoot string) []string {
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
		agentName, _ := config.ResolveRoleAgentName("crew", townRoot, r.Path)
		crewMgr := factory.New(townRoot).CrewManager(r, agentName)
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
// Returns crew.ErrSessionRunning if the crew member is already running.
func startCrewMember(rigName, crewName, townRoot string) error {
	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Validate rig exists
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	if _, err := rigMgr.GetRig(rigName); err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Use factory.Start() for crew (agent resolved automatically)
	crewID := agent.CrewAddress(rigName, crewName)

	// Zombie sessions are detected and restarted automatically by factory.Start()
	_, err = factory.Start(townRoot, crewID)
	return err
}
