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
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
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

	t := tmux.NewTmux()
	allOK := true

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

	// Get session names
	deaconSession := getDeaconSessionName()
	mayorSession := getMayorSessionName()

	// 2. Deacon (Claude agent)
	if err := ensureSession(t, deaconSession, townRoot, "deacon"); err != nil {
		printStatus("Deacon", false, err.Error())
		allOK = false
	} else {
		printStatus("Deacon", true, deaconSession)
	}

	// 3. Mayor (Claude agent)
	if err := ensureSession(t, mayorSession, townRoot, "mayor"); err != nil {
		printStatus("Mayor", false, err.Error())
		allOK = false
	} else {
		printStatus("Mayor", true, mayorSession)
	}

	// 4. Witnesses (one per rig)
	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-witness", rigName)
		rigPath := filepath.Join(townRoot, rigName)

		if err := ensureWitness(t, sessionName, rigPath, rigName); err != nil {
			printStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printStatus(fmt.Sprintf("Witness (%s)", rigName), true, sessionName)
		}
	}

	// 5. Refineries (one per rig)
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
				sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
				printStatus(fmt.Sprintf("Refinery (%s)", rigName), true, sessionName)
			} else {
				printStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
				allOK = false
			}
		} else {
			sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
			printStatus(fmt.Sprintf("Refinery (%s)", rigName), true, sessionName)
		}
	}

	// 6. Crew (if --restore)
	if upRestore {
		for _, rigName := range rigs {
			crewStarted, crewErrors := startCrewFromSettings(t, townRoot, rigName)
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
			polecatsStarted, polecatErrors := startPolecatsWithWork(t, townRoot, rigName)
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

// ensureSession starts a Claude session if not running.
func ensureSession(t *tmux.Tmux, sessionName, workDir, role string) error {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	// Create session
	if err := t.NewSession(sessionName, workDir); err != nil {
		return err
	}

	// Set environment (non-fatal: session works without these)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", role)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", role)

	// Apply theme based on role (non-fatal: theming failure doesn't affect operation)
	switch role {
	case "mayor":
		theme := tmux.MayorTheme()
		_ = t.ConfigureGasTownSession(sessionName, theme, "", "Mayor", "coordinator")
	case "deacon":
		theme := tmux.DeaconTheme()
		_ = t.ConfigureGasTownSession(sessionName, theme, "", "Deacon", "health-check")
	}

	// Launch Claude
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	var claudeCmd string
	runtimeCmd := config.GetRuntimeCommand("")
	if role == "deacon" {
		// Deacon uses respawn loop
		claudeCmd = `export GT_ROLE=deacon BD_ACTOR=deacon GIT_AUTHOR_NAME=deacon && while true; do echo "⛪ Starting Deacon session..."; ` + runtimeCmd + `; echo ""; echo "Deacon exited. Restarting in 2s... (Ctrl-C to stop)"; sleep 2; done`
	} else {
		claudeCmd = config.BuildAgentStartupCommand(role, role, "", "")
	}

	if err := t.SendKeysDelayed(sessionName, claudeCmd, 200); err != nil {
		return err
	}

	// Wait for Claude to start (non-fatal)
	// Note: Deacon respawn loop makes beacon tricky - Claude restarts multiple times
	// For non-respawn (mayor), inject beacon
	if role != "deacon" {
		if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
			// Non-fatal
		}

		// Accept bypass permissions warning dialog if it appears.
		_ = t.AcceptBypassPermissionsWarning(sessionName)

		time.Sleep(constants.ShutdownNotifyDelay)

		// Inject startup nudge for predecessor discovery via /resume
		_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
			Recipient: role,
			Sender:    "human",
			Topic:     "cold-start",
		}) // Non-fatal
	}

	return nil
}

// ensureWitness starts a witness session for a rig.
func ensureWitness(t *tmux.Tmux, sessionName, rigPath, rigName string) error {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	// Create session in rig directory
	if err := t.NewSession(sessionName, rigPath); err != nil {
		return err
	}

	// Set environment (non-fatal: session works without these)
	bdActor := fmt.Sprintf("%s/witness", rigName)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "witness")
	_ = t.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Apply theme (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(rigName)
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Witness", rigName)

	// Launch Claude using runtime config
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	claudeCmd := config.BuildAgentStartupCommand("witness", bdActor, rigPath, "")
	if err := t.SendKeysDelayed(sessionName, claudeCmd, 200); err != nil {
		return err
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionName)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/witness", rigName)
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "patrol",
	}) // Non-fatal

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
func startCrewFromSettings(t *tmux.Tmux, townRoot, rigName string) ([]string, map[string]error) {
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

	// Start each crew member
	for _, crewName := range toStart {
		sessionName := fmt.Sprintf("gt-%s-crew-%s", rigName, crewName)

		running, err := t.HasSession(sessionName)
		if err != nil {
			errors[crewName] = err
			continue
		}
		if running {
			started = append(started, crewName)
			continue
		}

		// Start the crew member
		crewPath := filepath.Join(rigPath, "crew", crewName)
		if err := ensureCrewSession(t, sessionName, crewPath, rigName, crewName); err != nil {
			errors[crewName] = err
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

// ensureCrewSession starts a crew session.
func ensureCrewSession(t *tmux.Tmux, sessionName, crewPath, rigName, crewName string) error {
	// Create session in crew directory
	if err := t.NewSession(sessionName, crewPath); err != nil {
		return err
	}

	// Set environment
	bdActor := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "crew")
	_ = t.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionName, "GT_CREW", crewName)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Apply theme (use rig-based theme)
	theme := tmux.AssignTheme(rigName)
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Crew", crewName)

	// Launch Claude using runtime config
	// crewPath is like ~/gt/gastown/crew/max, so rig path is two dirs up
	rigPath := filepath.Dir(filepath.Dir(crewPath))
	claudeCmd := config.BuildCrewStartupCommand(rigName, crewName, rigPath, "")
	if err := t.SendKeysDelayed(sessionName, claudeCmd, 200); err != nil {
		return err
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionName)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     "cold-start",
	}) // Non-fatal

	return nil
}

// startPolecatsWithWork starts polecats that have pinned beads (work attached).
// Returns list of started polecat names and map of errors.
func startPolecatsWithWork(t *tmux.Tmux, townRoot, rigName string) ([]string, map[string]error) {
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

	for _, entry := range entries {
		if !entry.IsDir() {
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

		// This polecat has work - start it
		sessionName := fmt.Sprintf("gt-%s-polecat-%s", rigName, polecatName)

		running, err := t.HasSession(sessionName)
		if err != nil {
			errors[polecatName] = err
			continue
		}
		if running {
			started = append(started, polecatName)
			continue
		}

		// Start the polecat
		if err := ensurePolecatSession(t, sessionName, polecatPath, rigName, polecatName); err != nil {
			errors[polecatName] = err
		} else {
			started = append(started, polecatName)
		}
	}

	return started, errors
}

// ensurePolecatSession starts a polecat session.
func ensurePolecatSession(t *tmux.Tmux, sessionName, polecatPath, rigName, polecatName string) error {
	// Create session in polecat directory
	if err := t.NewSession(sessionName, polecatPath); err != nil {
		return err
	}

	// Set environment
	bdActor := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	_ = t.SetEnvironment(sessionName, "GT_ROLE", "polecat")
	_ = t.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = t.SetEnvironment(sessionName, "GT_POLECAT", polecatName)
	_ = t.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Apply theme (use rig-based theme)
	theme := tmux.AssignTheme(rigName)
	_ = t.ConfigureGasTownSession(sessionName, theme, "", "Polecat", polecatName)

	// Launch Claude using runtime config
	// polecatPath is like ~/gt/gastown/polecats/toast, so rig path is two dirs up
	rigPath := filepath.Dir(filepath.Dir(polecatPath))
	claudeCmd := config.BuildPolecatStartupCommand(rigName, polecatName, rigPath, "")
	if err := t.SendKeysDelayed(sessionName, claudeCmd, 200); err != nil {
		return err
	}

	// Wait for Claude to start (non-fatal)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionName)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	_ = session.StartupNudge(t, sessionName, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "witness",
		Topic:     "dispatch",
	}) // Non-fatal

	return nil
}
