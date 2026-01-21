package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/swarm"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Swarm command flags
var (
	swarmEpic       string
	swarmTasks      []string
	swarmWorkers    []string
	swarmStart      bool
	swarmStatusJSON bool
	swarmListRig    string
	swarmListStatus string
	swarmListJSON   bool
	swarmTarget     string
)

var swarmCmd = &cobra.Command{
	Use:        "swarm",
	GroupID:    GroupWork,
	Short:      "[DEPRECATED] Use 'gt convoy' instead",
	Deprecated: "Use 'gt convoy' for work tracking. A 'swarm' is now just the ephemeral workers on a convoy.",
	RunE:       requireSubcommand,
	Long: `DEPRECATED: Use 'gt convoy' instead.

The term "swarm" now refers to the ephemeral set of workers on a convoy's issues,
not a persistent tracking unit. Use 'gt convoy' for creating and tracking batched work.

TERMINOLOGY:
  Convoy: Persistent tracking unit (what this command was trying to be)
  Swarm:  Ephemeral workers on a convoy (no separate tracking needed)

MIGRATION:
  gt swarm create  →  gt convoy create
  gt swarm status  →  gt convoy status
  gt swarm list    →  gt convoy list

See 'gt convoy --help' for the new workflow.`,
}

var swarmCreateCmd = &cobra.Command{
	Use:   "create <rig>",
	Short: "Create a new swarm",
	Long: `Create a new swarm in a rig.

Creates a swarm that coordinates multiple polecats working on tasks from
a beads epic. All workers branch from the same base commit.

Examples:
  gt swarm create greenplace --epic gp-abc --worker Toast --worker Nux
  gt swarm create greenplace --epic gp-abc --worker Toast --start`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmCreate,
}

var swarmStatusCmd = &cobra.Command{
	Use:   "status <swarm-id>",
	Short: "Show swarm status",
	Long: `Show detailed status for a swarm.

Displays swarm metadata, task progress, worker assignments, and integration
branch status.`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmStatus,
}

var swarmListCmd = &cobra.Command{
	Use:   "list [rig]",
	Short: "List swarms",
	Long: `List swarms, optionally filtered by rig or status.

Examples:
  gt swarm list
  gt swarm list greenplace
  gt swarm list --status=active
  gt swarm list greenplace --status=landed`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSwarmList,
}

var swarmLandCmd = &cobra.Command{
	Use:   "land <swarm-id>",
	Short: "Land a swarm to main",
	Long: `Manually trigger landing for a completed swarm.

Merges the integration branch to the target branch (usually main).
Normally this is done automatically by the Refinery.`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmLand,
}

var swarmCancelCmd = &cobra.Command{
	Use:   "cancel <swarm-id>",
	Short: "Cancel a swarm",
	Long: `Cancel an active swarm.

Marks the swarm as canceled and optionally cleans up branches.`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmCancel,
}

var swarmStartCmd = &cobra.Command{
	Use:   "start <swarm-id>",
	Short: "Start a created swarm",
	Long: `Start a swarm that was created without --start.

Transitions the swarm from 'created' to 'active' state.`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmStart,
}

var swarmDispatchCmd = &cobra.Command{
	Use:   "dispatch <epic-id>",
	Short: "Assign next ready task to a fresh polecat",
	Long: `Dispatch the next ready task from an epic to a new polecat.

Finds the first unassigned task in the epic's ready front and spawns a
fresh polecat to work on it. Self-cleaning model: polecats are always
fresh - there are no idle polecats to reuse.

Examples:
  gt swarm dispatch gt-abc         # Dispatch next task from epic gt-abc
  gt swarm dispatch gt-abc --rig greenplace  # Dispatch in specific rig`,
	Args: cobra.ExactArgs(1),
	RunE: runSwarmDispatch,
}

var swarmDispatchRig string

func init() {
	// Create flags
	swarmCreateCmd.Flags().StringVar(&swarmEpic, "epic", "", "Beads epic ID for this swarm (required)")
	swarmCreateCmd.Flags().StringSliceVar(&swarmWorkers, "worker", nil, "Polecat names to assign (repeatable)")
	swarmCreateCmd.Flags().BoolVar(&swarmStart, "start", false, "Start swarm immediately after creation")
	swarmCreateCmd.Flags().StringVar(&swarmTarget, "target", "main", "Target branch for landing")
	_ = swarmCreateCmd.MarkFlagRequired("epic") // cobra flags: error only at runtime if missing

	// Status flags
	swarmStatusCmd.Flags().BoolVar(&swarmStatusJSON, "json", false, "Output as JSON")

	// List flags
	swarmListCmd.Flags().StringVar(&swarmListStatus, "status", "", "Filter by status (active, landed, canceled, failed)")
	swarmListCmd.Flags().BoolVar(&swarmListJSON, "json", false, "Output as JSON")

	// Dispatch flags
	swarmDispatchCmd.Flags().StringVar(&swarmDispatchRig, "rig", "", "Rig to dispatch in (auto-detected from epic if not specified)")

	// Add subcommands
	swarmCmd.AddCommand(swarmCreateCmd)
	swarmCmd.AddCommand(swarmStartCmd)
	swarmCmd.AddCommand(swarmStatusCmd)
	swarmCmd.AddCommand(swarmListCmd)
	swarmCmd.AddCommand(swarmLandCmd)
	swarmCmd.AddCommand(swarmCancelCmd)
	swarmCmd.AddCommand(swarmDispatchCmd)

	rootCmd.AddCommand(swarmCmd)
}

// getSwarmRig gets a rig by name.
func getSwarmRig(rigName string) (*rig.Rig, string, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, "", fmt.Errorf("rig '%s' not found", rigName)
	}

	return r, townRoot, nil
}

// getAllRigs returns all discovered rigs.
func getAllRigs() ([]*rig.Rig, string, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	rigs, err := rigMgr.DiscoverRigs()
	if err != nil {
		return nil, "", err
	}

	return rigs, townRoot, nil
}

func runSwarmCreate(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	r, townRoot, err := getSwarmRig(rigName)
	if err != nil {
		return err
	}

	// Use beads to create the swarm molecule
	// First check if the epic already exists (it may be pre-created)
	// Use BeadsPath() to ensure we read from git-synced beads location
	beadsPath := r.BeadsPath()
	checkCmd := exec.Command("bd", "show", swarmEpic, "--json")
	checkCmd.Dir = beadsPath
	if err := checkCmd.Run(); err != nil {
		// Epic doesn't exist, create it as a swarm molecule
		createArgs := []string{
			"create",
			"--type=epic",
			"--mol-type=swarm",
			"--title", swarmEpic,
			"--silent",
		}
		createCmd := exec.Command("bd", createArgs...)
		createCmd.Dir = beadsPath
		var stdout bytes.Buffer
		createCmd.Stdout = &stdout
		if err := createCmd.Run(); err != nil {
			return fmt.Errorf("creating swarm epic: %w", err)
		}
	}

	// Get current git commit as base
	baseCommit := "unknown"
	gitCmd := exec.Command("git", "rev-parse", "HEAD")
	gitCmd.Dir = r.Path
	if out, err := gitCmd.Output(); err == nil {
		baseCommit = strings.TrimSpace(string(out))
	}

	integration := fmt.Sprintf("swarm/%s", swarmEpic)

	// Output
	fmt.Printf("%s Created swarm %s\n\n", style.Bold.Render("✓"), swarmEpic)
	fmt.Printf("  Epic:        %s\n", swarmEpic)
	fmt.Printf("  Rig:         %s\n", rigName)
	fmt.Printf("  Base commit: %s\n", truncate(baseCommit, 8))
	fmt.Printf("  Integration: %s\n", integration)
	fmt.Printf("  Target:      %s\n", swarmTarget)
	fmt.Printf("  Workers:     %s\n", strings.Join(swarmWorkers, ", "))

	// If workers specified, assign them to tasks
	if len(swarmWorkers) > 0 {
		fmt.Printf("\nNote: Worker assignment to tasks is handled during swarm start\n")
	}

	// Start if requested
	if swarmStart {
		// Get swarm status to find ready tasks
		statusCmd := exec.Command("bd", "swarm", "status", swarmEpic, "--json")
		statusCmd.Dir = beadsPath
		var statusOut bytes.Buffer
		statusCmd.Stdout = &statusOut
		if err := statusCmd.Run(); err != nil {
			return fmt.Errorf("getting swarm status: %w", err)
		}

		// Parse status to dispatch workers
		var status struct {
			Ready []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"ready"`
		}
		if err := json.Unmarshal(statusOut.Bytes(), &status); err == nil && len(status.Ready) > 0 {
			fmt.Printf("\nReady front has %d tasks available\n", len(status.Ready))
			if len(swarmWorkers) > 0 {
				// Spawn workers for ready tasks
				fmt.Printf("Spawning workers...\n")
				_ = spawnSwarmWorkersFromBeads(r, townRoot, swarmEpic, swarmWorkers, status.Ready)
			}
		}
	} else {
		fmt.Printf("\n  %s\n", style.Dim.Render("Use --start or 'gt swarm start' to activate"))
	}

	return nil
}

func runSwarmStart(cmd *cobra.Command, args []string) error {
	swarmID := args[0]

	// Find the swarm's rig
	rigs, townRoot, err := getAllRigs()
	if err != nil {
		return err
	}

	var foundRig *rig.Rig
	for _, r := range rigs {
		// Check if swarm exists in this rig by querying beads
		// Use BeadsPath() to ensure we read from git-synced location
		checkCmd := exec.Command("bd", "show", swarmID, "--json")
		checkCmd.Dir = r.BeadsPath()
		if err := checkCmd.Run(); err == nil {
			foundRig = r
			break
		}
	}

	if foundRig == nil {
		return fmt.Errorf("swarm '%s' not found", swarmID)
	}

	// Get swarm status from beads
	statusCmd := exec.Command("bd", "swarm", "status", swarmID, "--json")
	statusCmd.Dir = foundRig.BeadsPath()
	var stdout bytes.Buffer
	statusCmd.Stdout = &stdout

	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("getting swarm status: %w", err)
	}

	var status struct {
		EpicID string `json:"epic_id"`
		Ready  []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"ready"`
		Active []struct {
			ID       string `json:"id"`
			Assignee string `json:"assignee"`
		} `json:"active"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return fmt.Errorf("parsing swarm status: %w", err)
	}

	if len(status.Active) > 0 {
		fmt.Printf("Swarm already has %d active tasks\n", len(status.Active))
	}

	if len(status.Ready) == 0 {
		fmt.Println("No ready tasks to dispatch")
		return nil
	}

	fmt.Printf("%s Swarm %s starting with %d ready tasks\n", style.Bold.Render("✓"), swarmID, len(status.Ready))

	// If workers were specified in create, use them; otherwise prompt user
	if len(swarmWorkers) > 0 {
		fmt.Printf("\nSpawning workers...\n")
		_ = spawnSwarmWorkersFromBeads(foundRig, townRoot, swarmID, swarmWorkers, status.Ready)
	} else {
		fmt.Printf("\nReady tasks:\n")
		for _, task := range status.Ready {
			fmt.Printf("  ○ %s: %s\n", task.ID, task.Title)
		}
		fmt.Printf("\nUse 'gt sling <task-id> <rig>/<worker>' to assign tasks\n")
	}

	return nil
}

func runSwarmDispatch(cmd *cobra.Command, args []string) error {
	epicID := args[0]

	// Find the epic's rig by trying to show it in each rig
	rigs, townRoot, err := getAllRigs()
	if err != nil {
		return err
	}

	var foundRig *rig.Rig
	for _, r := range rigs {
		// If --rig specified, only check that rig
		if swarmDispatchRig != "" && r.Name != swarmDispatchRig {
			continue
		}
		// Use BeadsPath() to ensure we read from git-synced location
		checkCmd := exec.Command("bd", "show", epicID, "--json")
		checkCmd.Dir = r.BeadsPath()
		if err := checkCmd.Run(); err == nil {
			foundRig = r
			break
		}
	}

	if foundRig == nil {
		if swarmDispatchRig != "" {
			return fmt.Errorf("epic '%s' not found in rig '%s'", epicID, swarmDispatchRig)
		}
		return fmt.Errorf("epic '%s' not found in any rig", epicID)
	}

	// Get swarm/epic status to find ready tasks
	statusCmd := exec.Command("bd", "swarm", "status", epicID, "--json")
	statusCmd.Dir = foundRig.BeadsPath()
	var stdout bytes.Buffer
	statusCmd.Stdout = &stdout

	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("getting epic status: %w", err)
	}

	var status struct {
		Ready []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Assignee string `json:"assignee"`
		} `json:"ready"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return fmt.Errorf("parsing epic status: %w", err)
	}

	// Filter to unassigned ready tasks
	var unassigned []struct {
		ID    string
		Title string
	}
	for _, task := range status.Ready {
		if task.Assignee == "" {
			unassigned = append(unassigned, struct {
				ID    string
				Title string
			}{task.ID, task.Title})
		}
	}

	if len(unassigned) == 0 {
		fmt.Println("No unassigned ready tasks to dispatch")
		return nil
	}

	// Self-cleaning model: Always spawn fresh polecats for work.
	// There are no "idle" polecats - polecats self-nuke when done.
	// Just sling to the rig and let gt sling spawn a fresh polecat.
	task := unassigned[0]

	fmt.Printf("Dispatching %s to fresh polecat in %s...\n", task.ID, foundRig.Name)

	// Use gt sling to spawn a fresh polecat and assign the task
	slingCmd := exec.Command("gt", "sling", task.ID, foundRig.Name)
	slingCmd.Dir = townRoot
	slingCmd.Stdout = os.Stdout
	slingCmd.Stderr = os.Stderr

	if err := slingCmd.Run(); err != nil {
		return fmt.Errorf("slinging task: %w", err)
	}

	fmt.Printf("%s Dispatched %s: %s → fresh polecat\n", style.Bold.Render("✓"), task.ID, task.Title)

	// Show remaining tasks
	if len(unassigned) > 1 {
		fmt.Printf("\n%d more ready tasks available\n", len(unassigned)-1)
	}

	return nil
}

// spawnSwarmWorkersFromBeads spawns sessions for swarm workers using beads task list.
func spawnSwarmWorkersFromBeads(r *rig.Rig, townRoot string, swarmID string, workers []string, tasks []struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}) error { //nolint:unparam // error return kept for future use
	t := tmux.NewTmux()
	polecatSessMgr := polecat.NewSessionManager(t, r)
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit, t)

	// Pair workers with tasks (round-robin if more tasks than workers)
	workerIdx := 0
	for _, task := range tasks {
		if workerIdx >= len(workers) {
			break // No more workers
		}

		worker := workers[workerIdx]
		workerIdx++

		// Use gt sling to assign task to worker (this updates beads)
		slingCmd := exec.Command("gt", "sling", task.ID, fmt.Sprintf("%s/%s", r.Name, worker))
		slingCmd.Dir = townRoot
		if err := slingCmd.Run(); err != nil {
			style.PrintWarning("  couldn't sling %s to %s: %v", task.ID, worker, err)

			// Fallback: update polecat state directly
			if err := polecatMgr.AssignIssue(worker, task.ID); err != nil {
				style.PrintWarning("  couldn't assign %s to %s: %v", task.ID, worker, err)
				continue
			}
		}

		// Check if already running
		running, _ := polecatSessMgr.IsRunning(worker)
		if running {
			fmt.Printf("  %s already running, injecting task...\n", worker)
		} else {
			fmt.Printf("  Starting %s...\n", worker)
			if err := polecatSessMgr.Start(worker, polecat.SessionStartOptions{}); err != nil {
				style.PrintWarning("  couldn't start %s: %v", worker, err)
				continue
			}
			// Wait for Claude to initialize
			time.Sleep(5 * time.Second)
		}

		// Inject work assignment
		context := fmt.Sprintf("[SWARM] You are part of swarm %s.\n\nAssigned task: %s\nTitle: %s\n\nWork on this task. When complete, commit and signal DONE.",
			swarmID, task.ID, task.Title)
		if err := polecatSessMgr.Inject(worker, context); err != nil {
			style.PrintWarning("  couldn't inject to %s: %v", worker, err)
		} else {
			fmt.Printf("  %s → %s ✓\n", worker, task.ID)
		}
	}

	return nil
}

func runSwarmStatus(cmd *cobra.Command, args []string) error {
	swarmID := args[0]

	// Find the swarm's rig by trying to show it in each rig
	rigs, _, err := getAllRigs()
	if err != nil {
		return err
	}
	if len(rigs) == 0 {
		return fmt.Errorf("no rigs found")
	}

	// Find which rig has this swarm
	var foundRig *rig.Rig
	for _, r := range rigs {
		// Use BeadsPath() to ensure we read from git-synced location
		checkCmd := exec.Command("bd", "show", swarmID, "--json")
		checkCmd.Dir = r.BeadsPath()
		if err := checkCmd.Run(); err == nil {
			foundRig = r
			break
		}
	}

	if foundRig == nil {
		return fmt.Errorf("swarm '%s' not found in any rig", swarmID)
	}

	// Use bd swarm status to get swarm info from beads
	bdArgs := []string{"swarm", "status", swarmID}
	if swarmStatusJSON {
		bdArgs = append(bdArgs, "--json")
	}

	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Dir = foundRig.BeadsPath()
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr

	return bdCmd.Run()
}

func runSwarmList(cmd *cobra.Command, args []string) error {
	rigs, _, err := getAllRigs()
	if err != nil {
		return err
	}

	// Filter by rig if specified
	if len(args) > 0 {
		rigName := args[0]
		var filtered []*rig.Rig
		for _, r := range rigs {
			if r.Name == rigName {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("rig '%s' not found", rigName)
		}
		rigs = filtered
	}

	if len(rigs) == 0 {
		fmt.Println("No rigs found.")
		return nil
	}

	// Use bd list --mol-type=swarm to find swarm molecules
	bdArgs := []string{"list", "--mol-type=swarm", "--type=epic"}
	if swarmListJSON {
		bdArgs = append(bdArgs, "--json")
	}

	// Collect swarms from all rigs
	type swarmListEntry struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Rig    string `json:"rig"`
	}
	var allSwarms []swarmListEntry

	for _, r := range rigs {
		bdCmd := exec.Command("bd", bdArgs...)
		bdCmd.Dir = r.BeadsPath() // Use BeadsPath() for git-synced beads
		var stdout bytes.Buffer
		bdCmd.Stdout = &stdout

		if err := bdCmd.Run(); err != nil {
			continue
		}

		if swarmListJSON {
			// Parse JSON output
			var issues []struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &issues); err == nil {
				for _, issue := range issues {
					allSwarms = append(allSwarms, swarmListEntry{
						ID:     issue.ID,
						Title:  issue.Title,
						Status: issue.Status,
						Rig:    r.Name,
					})
				}
			}
		} else {
			// Parse line output - each line is an issue
			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				// Filter by status if specified
				if swarmListStatus != "" && !strings.Contains(strings.ToLower(line), swarmListStatus) {
					continue
				}
				allSwarms = append(allSwarms, swarmListEntry{
					ID:  line,
					Rig: r.Name,
				})
			}
		}
	}

	// JSON output
	if swarmListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(allSwarms)
	}

	// Human-readable output
	if len(allSwarms) == 0 {
		fmt.Println("No swarms found.")
		fmt.Println("Create a swarm with: gt swarm create <rig> --epic <epic-id>")
		return nil
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Swarms"))
	for _, entry := range allSwarms {
		fmt.Printf("  %s [%s]\n", entry.ID, entry.Rig)
	}
	fmt.Printf("\nUse 'gt swarm status <id>' for detailed status.\n")

	return nil
}

func runSwarmLand(cmd *cobra.Command, args []string) error {
	swarmID := args[0]

	// Find the swarm's rig
	rigs, townRoot, err := getAllRigs()
	if err != nil {
		return err
	}

	var foundRig *rig.Rig
	for _, r := range rigs {
		// Use BeadsPath() for git-synced beads
		checkCmd := exec.Command("bd", "show", swarmID, "--json")
		checkCmd.Dir = r.BeadsPath()
		if err := checkCmd.Run(); err == nil {
			foundRig = r
			break
		}
	}

	if foundRig == nil {
		return fmt.Errorf("swarm '%s' not found", swarmID)
	}

	// Check swarm status - all children should be closed
	statusCmd := exec.Command("bd", "swarm", "status", swarmID, "--json")
	statusCmd.Dir = foundRig.BeadsPath()
	var stdout bytes.Buffer
	statusCmd.Stdout = &stdout

	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("getting swarm status: %w", err)
	}

	var status struct {
		Ready       []struct{ ID string } `json:"ready"`
		Active      []struct{ ID string } `json:"active"`
		Blocked     []struct{ ID string } `json:"blocked"`
		Completed   []struct{ ID string } `json:"completed"`
		TotalIssues int                   `json:"total_issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return fmt.Errorf("parsing swarm status: %w", err)
	}

	// Check if all tasks are complete
	if len(status.Ready) > 0 || len(status.Active) > 0 || len(status.Blocked) > 0 {
		return fmt.Errorf("swarm has incomplete tasks: %d ready, %d active, %d blocked",
			len(status.Ready), len(status.Active), len(status.Blocked))
	}

	fmt.Printf("Landing swarm %s to main...\n", swarmID)

	// Use swarm manager for the actual landing (git operations)
	mgr := swarm.NewManager(foundRig)
	sw, err := mgr.LoadSwarm(swarmID)
	if err != nil {
		return fmt.Errorf("loading swarm from beads: %w", err)
	}

	// Execute full landing protocol
	config := swarm.LandingConfig{
		TownRoot: townRoot,
	}
	result, err := mgr.ExecuteLanding(swarmID, config)
	if err != nil {
		return fmt.Errorf("landing protocol: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("landing failed: %s", result.Error)
	}

	// Close the swarm epic in beads
	closeArgs := []string{"close", swarmID, "--reason", "Swarm landed to main"}
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		closeArgs = append(closeArgs, "--session="+sessionID)
	}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = foundRig.BeadsPath()
	if err := closeCmd.Run(); err != nil {
		style.PrintWarning("couldn't close swarm epic in beads: %v", err)
	}

	fmt.Printf("%s Swarm %s landed to main\n", style.Bold.Render("✓"), sw.ID)
	fmt.Printf("  Sessions stopped: %d\n", result.SessionsStopped)
	fmt.Printf("  Branches cleaned: %d\n", result.BranchesCleaned)
	return nil
}

func runSwarmCancel(cmd *cobra.Command, args []string) error {
	swarmID := args[0]

	// Find the swarm's rig
	rigs, _, err := getAllRigs()
	if err != nil {
		return err
	}

	var foundRig *rig.Rig
	for _, r := range rigs {
		// Use BeadsPath() for git-synced beads
		checkCmd := exec.Command("bd", "show", swarmID, "--json")
		checkCmd.Dir = r.BeadsPath()
		if err := checkCmd.Run(); err == nil {
			foundRig = r
			break
		}
	}

	if foundRig == nil {
		return fmt.Errorf("swarm '%s' not found", swarmID)
	}

	// Check if swarm is already closed
	checkCmd := exec.Command("bd", "show", swarmID, "--json")
	checkCmd.Dir = foundRig.BeadsPath()
	var stdout bytes.Buffer
	checkCmd.Stdout = &stdout
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("checking swarm status: %w", err)
	}

	var issue struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issue); err == nil {
		if issue.Status == "closed" {
			return fmt.Errorf("swarm already closed")
		}
	}

	// Close the swarm epic in beads with canceled reason
	closeArgs := []string{"close", swarmID, "--reason", "Swarm canceled"}
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		closeArgs = append(closeArgs, "--session="+sessionID)
	}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = foundRig.BeadsPath()
	if err := closeCmd.Run(); err != nil {
		return fmt.Errorf("closing swarm: %w", err)
	}

	fmt.Printf("%s Swarm %s canceled\n", style.Bold.Render("✓"), swarmID)
	return nil
}

// Helper functions

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
