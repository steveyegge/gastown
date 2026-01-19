package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/wisp"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// agentStartResult holds the result of starting an agent.
type agentStartResult struct {
	name   string // Display name like "Witness (gastown)"
	ok     bool   // Whether start succeeded
	detail string // Status detail (session name or error)
}

// maxConcurrentAgentStarts limits parallel agent startups to avoid resource exhaustion.
const maxConcurrentAgentStarts = 10

var upCmd = &cobra.Command{
	Use:     "up",
	GroupID: GroupServices,
	Short:   "Bring up all Gas Town services",
	Long: `Start all Gas Town long-lived services.

This is the idempotent "boot" command for Gas Town. It ensures all
infrastructure agents are running:

  • Daemon     - Go background process that pokes agents
  • Deacon     - Health orchestrator (monitors Mayor/Witnesses)
  • Mayor      - Global work coordinator
  • Witnesses  - Per-rig polecat managers
  • Refineries - Per-rig merge queue processors

Polecats are NOT started by this command - they are transient workers
spawned on demand by the Mayor or Witnesses.

Use --restore to also start:
  • Crew       - Per rig settings (settings/config.json crew.startup)
  • Polecats   - Those with pinned beads (work attached)

Running 'gt up' multiple times is safe - it only starts services that
aren't already running.`,
	RunE: runUp,
}

var (
	upQuiet   bool
	upRestore bool
)

func init() {
	upCmd.Flags().BoolVarP(&upQuiet, "quiet", "q", false, "Only show errors")
	upCmd.Flags().BoolVar(&upRestore, "restore", false, "Also restore crew (from settings) and polecats (from hooks)")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	allOK := true

	// Discover rigs early so we can prefetch while daemon/deacon/mayor start
	rigs := discoverRigs(townRoot)

	// Start daemon, deacon, mayor, and rig prefetch in parallel
	var daemonErr error
	var daemonPID int
	var deaconResult, mayorResult agentStartResult
	var prefetchedRigs map[string]*rig.Rig
	var rigErrors map[string]error

	var startupWg sync.WaitGroup
	startupWg.Add(4)

	// 1. Daemon (Go process)
	go func() {
		defer startupWg.Done()
		if err := ensureDaemon(townRoot); err != nil {
			daemonErr = err
		} else {
			running, pid, _ := daemon.IsRunning(townRoot)
			if running {
				daemonPID = pid
			}
		}
	}()

	// 2. Deacon
	go func() {
		defer startupWg.Done()
		deaconMgr := deacon.NewManager(townRoot)
		if err := deaconMgr.Start(""); err != nil {
			if err == deacon.ErrAlreadyRunning {
				deaconResult = agentStartResult{name: "Deacon", ok: true, detail: deaconMgr.SessionName()}
			} else {
				deaconResult = agentStartResult{name: "Deacon", ok: false, detail: err.Error()}
			}
		} else {
			deaconResult = agentStartResult{name: "Deacon", ok: true, detail: deaconMgr.SessionName()}
		}
	}()

	// 3. Mayor
	go func() {
		defer startupWg.Done()
		mayorMgr := mayor.NewManager(townRoot)
		if err := mayorMgr.Start(""); err != nil {
			if err == mayor.ErrAlreadyRunning {
				mayorResult = agentStartResult{name: "Mayor", ok: true, detail: mayorMgr.SessionName()}
			} else {
				mayorResult = agentStartResult{name: "Mayor", ok: false, detail: err.Error()}
			}
		} else {
			mayorResult = agentStartResult{name: "Mayor", ok: true, detail: mayorMgr.SessionName()}
		}
	}()

	// 4. Prefetch rig configs (overlaps with daemon/deacon/mayor startup)
	go func() {
		defer startupWg.Done()
		prefetchedRigs, rigErrors = prefetchRigs(rigs)
	}()

	startupWg.Wait()

	// Print daemon/deacon/mayor results
	if daemonErr != nil {
		printStatus("Daemon", false, daemonErr.Error())
		allOK = false
	} else if daemonPID > 0 {
		printStatus("Daemon", true, fmt.Sprintf("PID %d", daemonPID))
	}
	printStatus(deaconResult.name, deaconResult.ok, deaconResult.detail)
	if !deaconResult.ok {
		allOK = false
	}
	printStatus(mayorResult.name, mayorResult.ok, mayorResult.detail)
	if !mayorResult.ok {
		allOK = false
	}

	// 5 & 6. Witnesses and Refineries (using prefetched rigs)
	witnessResults, refineryResults := startRigAgentsWithPrefetch(rigs, prefetchedRigs, rigErrors)

	// Print results in order: all witnesses first, then all refineries
	for _, rigName := range rigs {
		if result, ok := witnessResults[rigName]; ok {
			printStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}
	for _, rigName := range rigs {
		if result, ok := refineryResults[rigName]; ok {
			printStatus(result.name, result.ok, result.detail)
			if !result.ok {
				allOK = false
			}
		}
	}

	// 7. Crew (if --restore)
	if upRestore {
		for _, rigName := range rigs {
			crewStarted, crewErrors := startCrewFromSettings(townRoot, rigName)
			for _, name := range crewStarted {
				printStatus(fmt.Sprintf("Crew (%s/%s)", rigName, name), true, fmt.Sprintf("gt-%s-crew-%s", rigName, name))
			}
			for name, err := range crewErrors {
				printStatus(fmt.Sprintf("Crew (%s/%s)", rigName, name), false, err.Error())
				allOK = false
			}
		}

		// 7. Polecats with pinned work (if --restore)
		for _, rigName := range rigs {
			polecatsStarted, polecatErrors := startPolecatsWithWork(townRoot, rigName)
			for _, name := range polecatsStarted {
				printStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), true, fmt.Sprintf("gt-%s-polecat-%s", rigName, name))
			}
			for name, err := range polecatErrors {
				printStatus(fmt.Sprintf("Polecat (%s/%s)", rigName, name), false, err.Error())
				allOK = false
			}
		}
	}

	fmt.Println()
	if allOK {
		fmt.Printf("%s All services running\n", style.Bold.Render("✓"))
		// Log boot event with started services
		startedServices := []string{"daemon", "deacon", "mayor"}
		for _, rigName := range rigs {
			startedServices = append(startedServices, fmt.Sprintf("%s/witness", rigName))
			startedServices = append(startedServices, fmt.Sprintf("%s/refinery", rigName))
		}
		_ = events.LogFeed(events.TypeBoot, "gt", events.BootPayload("town", startedServices))
	} else {
		fmt.Printf("%s Some services failed to start\n", style.Bold.Render("✗"))
		return fmt.Errorf("not all services started")
	}

	return nil
}

func printStatus(name string, ok bool, detail string) {
	if upQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// ensureDaemon starts the daemon if not running.
func ensureDaemon(townRoot string) error {
	running, _, err := daemon.IsRunning(townRoot)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	// Start daemon
	gtPath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(gtPath, "daemon", "run")
	cmd.Dir = townRoot
	// Detach from parent I/O for background daemon (uses its own logging)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for daemon to initialize
	time.Sleep(300 * time.Millisecond)

	// Verify it started
	running, _, err = daemon.IsRunning(townRoot)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("daemon failed to start")
	}

	return nil
}

// rigPrefetchResult holds the result of loading a single rig config.
type rigPrefetchResult struct {
	index int
	rig   *rig.Rig
	err   error
}

// prefetchRigs loads all rig configs in parallel for faster agent startup.
// Returns a map of rig name to loaded Rig, and any errors encountered.
func prefetchRigs(rigNames []string) (map[string]*rig.Rig, map[string]error) {
	n := len(rigNames)
	if n == 0 {
		return make(map[string]*rig.Rig), make(map[string]error)
	}

	// Use channel to collect results without locking
	results := make(chan rigPrefetchResult, n)

	for i, name := range rigNames {
		go func(idx int, rigName string) {
			_, r, err := getRig(rigName)
			results <- rigPrefetchResult{index: idx, rig: r, err: err}
		}(i, name)
	}

	// Collect results - pre-allocate maps with capacity
	rigs := make(map[string]*rig.Rig, n)
	errors := make(map[string]error)

	for i := 0; i < n; i++ {
		res := <-results
		name := rigNames[res.index]
		if res.err != nil {
			errors[name] = res.err
		} else {
			rigs[name] = res.rig
		}
	}

	return rigs, errors
}

// agentTask represents a unit of work for the agent worker pool.
type agentTask struct {
	rigName   string
	rigObj    *rig.Rig
	isWitness bool // true for witness, false for refinery
}

// agentResultMsg carries result back from worker to collector.
type agentResultMsg struct {
	rigName   string
	isWitness bool
	result    agentStartResult
}

// startRigAgentsParallel starts all Witnesses and Refineries concurrently.
// Discovers and prefetches rigs internally. For use when rigs aren't pre-loaded.
func startRigAgentsParallel(rigNames []string) (witnessResults, refineryResults map[string]agentStartResult) {
	prefetchedRigs, rigErrors := prefetchRigs(rigNames)
	return startRigAgentsWithPrefetch(rigNames, prefetchedRigs, rigErrors)
}

// startRigAgentsWithPrefetch starts all Witnesses and Refineries using pre-loaded rig configs.
// Uses a worker pool with fixed goroutine count to limit concurrency and reduce overhead.
func startRigAgentsWithPrefetch(rigNames []string, prefetchedRigs map[string]*rig.Rig, rigErrors map[string]error) (witnessResults, refineryResults map[string]agentStartResult) {
	n := len(rigNames)
	witnessResults = make(map[string]agentStartResult, n)
	refineryResults = make(map[string]agentStartResult, n)

	if n == 0 {
		return
	}

	// Record errors for rigs that failed to load
	for rigName, err := range rigErrors {
		errDetail := err.Error()
		witnessResults[rigName] = agentStartResult{
			name:   "Witness (" + rigName + ")",
			ok:     false,
			detail: errDetail,
		}
		refineryResults[rigName] = agentStartResult{
			name:   "Refinery (" + rigName + ")",
			ok:     false,
			detail: errDetail,
		}
	}

	numTasks := len(prefetchedRigs) * 2 // witness + refinery per rig
	if numTasks == 0 {
		return
	}

	// Task channel and result channel
	tasks := make(chan agentTask, numTasks)
	results := make(chan agentResultMsg, numTasks)

	// Start fixed worker pool (bounded by maxConcurrentAgentStarts)
	numWorkers := maxConcurrentAgentStarts
	if numTasks < numWorkers {
		numWorkers = numTasks
	}

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				var result agentStartResult
				if task.isWitness {
					result = upStartWitness(task.rigName, task.rigObj)
				} else {
					result = upStartRefinery(task.rigName, task.rigObj)
				}
				results <- agentResultMsg{
					rigName:   task.rigName,
					isWitness: task.isWitness,
					result:    result,
				}
			}
		}()
	}

	// Enqueue all tasks
	for rigName, r := range prefetchedRigs {
		tasks <- agentTask{rigName: rigName, rigObj: r, isWitness: true}
		tasks <- agentTask{rigName: rigName, rigObj: r, isWitness: false}
	}
	close(tasks)

	// Close results channel when workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results - no locking needed, single goroutine collects
	for msg := range results {
		if msg.isWitness {
			witnessResults[msg.rigName] = msg.result
		} else {
			refineryResults[msg.rigName] = msg.result
		}
	}

	return
}

// upStartWitness starts a witness for the given rig and returns a result struct.
// Respects parked/docked status - skips starting if rig is not operational.
func upStartWitness(rigName string, r *rig.Rig) agentStartResult {
	name := "Witness (" + rigName + ")"

	// Check if rig is parked or docked
	townRoot := filepath.Dir(r.Path)
	cfg := wisp.NewConfig(townRoot, rigName)
	status := cfg.GetString("status")
	if status == "parked" || status == "docked" {
		return agentStartResult{name: name, ok: true, detail: fmt.Sprintf("skipped (rig %s)", status)}
	}

	mgr := witness.NewManager(r)
	if err := mgr.Start(false, "", nil); err != nil {
		if err == witness.ErrAlreadyRunning {
			return agentStartResult{name: name, ok: true, detail: mgr.SessionName()}
		}
		return agentStartResult{name: name, ok: false, detail: err.Error()}
	}
	return agentStartResult{name: name, ok: true, detail: mgr.SessionName()}
}

// upStartRefinery starts a refinery for the given rig and returns a result struct.
// Respects parked/docked status - skips starting if rig is not operational.
func upStartRefinery(rigName string, r *rig.Rig) agentStartResult {
	name := "Refinery (" + rigName + ")"

	// Check if rig is parked or docked
	townRoot := filepath.Dir(r.Path)
	cfg := wisp.NewConfig(townRoot, rigName)
	status := cfg.GetString("status")
	if status == "parked" || status == "docked" {
		return agentStartResult{name: name, ok: true, detail: fmt.Sprintf("skipped (rig %s)", status)}
	}

	mgr := refinery.NewManager(r)
	if err := mgr.Start(false, ""); err != nil {
		if err == refinery.ErrAlreadyRunning {
			return agentStartResult{name: name, ok: true, detail: mgr.SessionName()}
		}
		return agentStartResult{name: name, ok: false, detail: err.Error()}
	}
	return agentStartResult{name: name, ok: true, detail: mgr.SessionName()}
}

// discoverRigs finds all rigs in the town.
func discoverRigs(townRoot string) []string {
	var rigs []string

	// Try rigs.json first
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	if rigsConfig, err := config.LoadRigsConfig(rigsConfigPath); err == nil {
		for name := range rigsConfig.Rigs {
			rigs = append(rigs, name)
		}
		return rigs
	}

	// Fallback: scan directory for rig-like directories
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return rigs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip known non-rig directories
		if name == "mayor" || name == "daemon" || name == "deacon" ||
			name == ".git" || name == "docs" || name[0] == '.' {
			continue
		}

		dirPath := filepath.Join(townRoot, name)

		// Check for .beads directory (indicates a rig)
		beadsPath := filepath.Join(dirPath, ".beads")
		if _, err := os.Stat(beadsPath); err == nil {
			rigs = append(rigs, name)
			continue
		}

		// Check for polecats directory (indicates a rig)
		polecatsPath := filepath.Join(dirPath, "polecats")
		if _, err := os.Stat(polecatsPath); err == nil {
			rigs = append(rigs, name)
		}
	}

	return rigs
}

// startCrewFromSettings starts crew members based on rig settings.
// Returns list of started crew names and map of errors.
func startCrewFromSettings(townRoot, rigName string) ([]string, map[string]error) {
	started := []string{}
	errors := map[string]error{}

	rigPath := filepath.Join(townRoot, rigName)

	// Load rig settings
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		// No settings file or error - skip crew startup
		return started, errors
	}

	if settings.Crew == nil || settings.Crew.Startup == "" {
		// No crew startup preference
		return started, errors
	}

	// Get available crew members using helper
	crewMgr, _, err := getCrewManager(rigName)
	if err != nil {
		return started, errors
	}

	crewWorkers, err := crewMgr.List()
	if err != nil {
		return started, errors
	}

	if len(crewWorkers) == 0 {
		return started, errors
	}

	// Extract crew names
	crewNames := make([]string, len(crewWorkers))
	for i, w := range crewWorkers {
		crewNames[i] = w.Name
	}

	// Parse startup preference and determine which crew to start
	toStart := parseCrewStartupPreference(settings.Crew.Startup, crewNames)

	// Start each crew member using Manager
	for _, crewName := range toStart {
		if err := crewMgr.Start(crewName, crew.StartOptions{}); err != nil {
			if err == crew.ErrSessionRunning {
				started = append(started, crewName)
			} else {
				errors[crewName] = err
			}
		} else {
			started = append(started, crewName)
		}
	}

	return started, errors
}

// parseCrewStartupPreference parses the natural language crew startup preference.
// Examples: "max", "joe and max", "all", "none", "pick one"
func parseCrewStartupPreference(pref string, available []string) []string {
	pref = strings.ToLower(strings.TrimSpace(pref))

	// Special keywords
	switch pref {
	case "none", "":
		return []string{}
	case "all":
		return available
	case "pick one", "any", "any one":
		if len(available) > 0 {
			return []string{available[0]}
		}
		return []string{}
	}

	// Parse comma/and-separated list
	// "joe and max" -> ["joe", "max"]
	// "joe, max" -> ["joe", "max"]
	// "max" -> ["max"]
	pref = strings.ReplaceAll(pref, " and ", ",")
	pref = strings.ReplaceAll(pref, ", but not ", ",-")
	pref = strings.ReplaceAll(pref, " but not ", ",-")

	parts := strings.Split(pref, ",")

	include := []string{}
	exclude := map[string]bool{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, "-") {
			// Exclusion
			exclude[strings.TrimPrefix(part, "-")] = true
		} else {
			include = append(include, part)
		}
	}

	// Filter to only available crew members
	result := []string{}
	for _, name := range include {
		if exclude[name] {
			continue
		}
		// Check if this crew exists
		for _, avail := range available {
			if avail == name {
				result = append(result, name)
				break
			}
		}
	}

	return result
}

// startPolecatsWithWork starts polecats that have pinned beads (work attached).
// Returns list of started polecat names and map of errors.
func startPolecatsWithWork(townRoot, rigName string) ([]string, map[string]error) {
	started := []string{}
	errors := map[string]error{}

	rigPath := filepath.Join(townRoot, rigName)
	polecatsDir := filepath.Join(rigPath, "polecats")

	// List polecat directories
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		// No polecats directory
		return started, errors
	}

	// Get polecat session manager
	_, r, err := getRig(rigName)
	if err != nil {
		return started, errors
	}
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatName := entry.Name()
		polecatPath := filepath.Join(polecatsDir, polecatName)

		// Check if this polecat has a pinned bead (work attached)
		agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
		b := beads.New(polecatPath)
		pinnedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusPinned,
			Assignee: agentID,
			Priority: -1,
		})
		if err != nil || len(pinnedBeads) == 0 {
			// No pinned beads - skip
			continue
		}

		// This polecat has work - start it using SessionManager
		if err := polecatMgr.Start(polecatName, polecat.SessionStartOptions{}); err != nil {
			if err == polecat.ErrSessionRunning {
				started = append(started, polecatName)
			} else {
				errors[polecatName] = err
			}
		} else {
			started = append(started, polecatName)
		}
	}

	return started, errors
}
