package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

var upCmd = &cobra.Command{
	Use:     "up",
	GroupID: GroupServices,
	Short:   "Bring up all Gas Town services",
	Long: `Start all Gas Town long-lived services.

This is the idempotent "boot" command for Gas Town. It ensures all
infrastructure agents are running:

  • Daemon     - Go background process that pokes agents
  • Boot       - Watchdog that triages Deacon health (the dog)
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

	// 0. Sync beads databases from JSONL (ensures db is up-to-date after git pulls)
	// This must happen before any beads-dependent operations (witnesses, polecats)
	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		rigPath := filepath.Join(townRoot, rigName)
		b := beads.New(rigPath)
		if b.IsBeadsRepo() {
			if err := b.SyncImportOnly(); err != nil {
				// Log but don't fail - some rigs may not have beads configured
				if !upQuiet {
					fmt.Printf("%s Beads sync (%s): %s\n", style.WarningPrefix, rigName, err.Error())
				}
			}
		}
	}

	// 1. Daemon (Go process)
	if err := ensureDaemon(townRoot); err != nil {
		printStatus("Daemon", false, err.Error())
		allOK = false
	} else {
		running, pid, _ := daemon.IsRunning(townRoot)
		if running {
			printStatus("Daemon", true, fmt.Sprintf("PID %d", pid))
		}
	}

	// 2. Boot (the dog - watchdog that triages Deacon health)
	bootMgr := boot.New(townRoot)
	if bootMgr.IsRunning() {
		printStatus("Boot", true, boot.SessionName)
	} else {
		if err := bootMgr.Spawn(); err != nil {
			printStatus("Boot", false, err.Error())
			allOK = false
		} else {
			printStatus("Boot", true, boot.SessionName)
		}
	}

	// 3. Deacon (Claude agent)
	deaconMgr := deacon.NewManager(townRoot)
	if err := deaconMgr.Start(""); err != nil {
		if err == deacon.ErrAlreadyRunning {
			printStatus("Deacon", true, deaconMgr.SessionName())
		} else {
			printStatus("Deacon", false, err.Error())
			allOK = false
		}
	} else {
		printStatus("Deacon", true, deaconMgr.SessionName())
	}

	// 4. Mayor (Claude agent)
	mayorMgr := mayor.NewManager(townRoot)
	if err := mayorMgr.Start(""); err != nil {
		if err == mayor.ErrAlreadyRunning {
			printStatus("Mayor", true, mayorMgr.SessionName())
		} else {
			printStatus("Mayor", false, err.Error())
			allOK = false
		}
	} else {
		printStatus("Mayor", true, mayorMgr.SessionName())
	}

	// 5. Witnesses (one per rig)
	for _, rigName := range rigs {
		_, r, err := getRig(rigName)
		if err != nil {
			printStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
			continue
		}

		mgr := witness.NewManager(r)
		if err := mgr.Start(false, "", nil); err != nil {
			if err == witness.ErrAlreadyRunning {
				printStatus(fmt.Sprintf("Witness (%s)", rigName), true, mgr.SessionName())
			} else {
				printStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
				allOK = false
			}
		} else {
			printStatus(fmt.Sprintf("Witness (%s)", rigName), true, mgr.SessionName())
		}
	}

	// 6. Refineries (one per rig)
	for _, rigName := range rigs {
		_, r, err := getRig(rigName)
		if err != nil {
			printStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
			allOK = false
			continue
		}

		mgr := refinery.NewManager(r)
		if err := mgr.Start(false); err != nil {
			if err == refinery.ErrAlreadyRunning {
				printStatus(fmt.Sprintf("Refinery (%s)", rigName), true, mgr.SessionName())
			} else {
				printStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
				allOK = false
			}
		} else {
			printStatus(fmt.Sprintf("Refinery (%s)", rigName), true, mgr.SessionName())
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

		// 8. Polecats with pinned work (if --restore)
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
		startedServices := []string{"daemon", "boot", "deacon", "mayor"}
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
