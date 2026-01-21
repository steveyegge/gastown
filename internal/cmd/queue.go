package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/queue"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var queueCmd = &cobra.Command{
	Use:     "queue",
	GroupID: GroupWork,
	Short:   "Manage the work queue",
	Long: `Manage the work queue for dispatching beads to polecats.

The queue provides a staging area for work before it's dispatched to polecats.
This decouples work submission from execution, allowing batch operations and
capacity-based scheduling.

Running 'gt queue' with no subcommand shows queue status.

Subcommands:
  status  Show queue status (pending + running counts)
  add     Add a bead to the queue
  list    List queued beads
  run     Dispatch queued beads to polecats
  clear   Remove all beads from the queue`,
	RunE: runQueueStatus, // Bare command shows status
}

var queueStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show queue status",
	Long: `Show queue status including pending and running counts.

This is the same as running 'gt queue' with no subcommand.`,
	Args: cobra.NoArgs,
	RunE: runQueueStatus,
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queued beads",
	Long: `List all beads currently in the queue.

Shows beads grouped by their target rig.

Flags:
  --pending   Show only pending (not yet dispatched) beads
  --running   Show only running (dispatched) beads
  --json      Output in JSON format`,
	Args: cobra.NoArgs,
	RunE: runQueueList,
}

var queueRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Dispatch queued beads to polecats",
	Long: `Dispatch queued beads to polecats.

Each queued bead gets its own polecat. The --queue-max-polecats flag limits
the total number of polecats that can be running at once (checks current
count and spawns up to the limit). The --spawn-batch-size flag controls how
many polecats spawn concurrently.

Examples:
  gt queue run                           # Dispatch all queued beads
  gt queue run --queue-max-polecats 10   # Only spawn up to 10 total polecats
  gt queue run --spawn-batch-size 3      # Spawn 3 at a time
  gt queue run --dry-run                 # Show what would be dispatched`,
	Args: cobra.NoArgs,
	RunE: runQueueRun,
}

var queueClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the queue",
	Long: `Remove all beads from the queue.

This removes the "queued" label from all queued beads without dispatching them.`,
	Args: cobra.NoArgs,
	RunE: runQueueClear,
}

var (
	queueRunDryRun         bool
	queueRunMaxPolecats    int
	queueRunSpawnBatchSize int
	queueRunAgent          string
	queueRunAccount        string
	queueRunForce          bool
	queueListJSON          bool
	queueListPending       bool
	queueListRunning       bool
)

func init() {
	queueRunCmd.Flags().BoolVarP(&queueRunDryRun, "dry-run", "n", false, "Show what would be dispatched")
	queueRunCmd.Flags().IntVar(&queueRunMaxPolecats, "queue-max-polecats", 0, "Max total polecats running (0 = use config or unlimited)")
	queueRunCmd.Flags().IntVar(&queueRunSpawnBatchSize, "spawn-batch-size", 0, "Number of polecats to spawn concurrently (0 = use config default)")
	queueRunCmd.Flags().StringVar(&queueRunAgent, "agent", "", "Override agent/runtime for spawned polecats")
	queueRunCmd.Flags().StringVar(&queueRunAccount, "account", "", "Claude Code account handle to use")
	queueRunCmd.Flags().BoolVar(&queueRunForce, "force", false, "Force spawn even if polecat has unread mail")

	queueListCmd.Flags().BoolVar(&queueListJSON, "json", false, "Output in JSON format")
	queueListCmd.Flags().BoolVar(&queueListPending, "pending", false, "Show only pending beads")
	queueListCmd.Flags().BoolVar(&queueListRunning, "running", false, "Show only running beads")

	queueCmd.AddCommand(queueStatusCmd)
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueRunCmd)
	queueCmd.AddCommand(queueClearCmd)

	rootCmd.AddCommand(queueCmd)
}

func runQueueStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	ops := beads.NewRealBeadsOps(townRoot)
	q := queue.New(ops)

	// Load queue
	items, err := q.Load()
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}

	// Count running polecats
	running := countRunningPolecats()

	// Get capacity from config
	capacity := config.GetQueueMaxPolecats(townRoot)

	// Display status
	fmt.Println("Queue Status:")
	fmt.Println()
	fmt.Printf("  Pending:  %d beads\n", len(items))
	fmt.Printf("  Running:  %d polecats\n", running)
	if capacity > 0 {
		fmt.Printf("  Capacity: %d/%d\n", running, capacity)
	} else {
		fmt.Printf("  Capacity: unlimited\n")
	}

	// Show breakdown by rig if there are pending items
	if len(items) > 0 {
		byRig := make(map[string]int)
		for _, item := range items {
			byRig[item.RigName]++
		}
		fmt.Println()
		fmt.Println("  By rig:")
		for rigName, count := range byRig {
			fmt.Printf("    %s: %d\n", rigName, count)
		}
	}

	return nil
}

func runQueueList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	ops := beads.NewRealBeadsOps(townRoot)
	q := queue.New(ops)

	items, err := q.Load()
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("Queue is empty")
		return nil
	}

	// Group by rig
	byRig := make(map[string][]queue.QueueItem)
	for _, item := range items {
		byRig[item.RigName] = append(byRig[item.RigName], item)
	}

	// Collect wisp IDs for batch lookup of bonded titles
	var wispIDs []string
	for _, item := range items {
		if strings.HasPrefix(item.BeadID, "gt-wisp-") {
			wispIDs = append(wispIDs, item.BeadID)
		}
	}

	// Batch lookup bonded titles
	bondedTitles := getBondedBeadTitles(townRoot, wispIDs)

	fmt.Printf("Queued beads: %d\n\n", len(items))
	for rigName, rigItems := range byRig {
		fmt.Printf("%s (%d):\n", style.Bold.Render(rigName), len(rigItems))
		for _, item := range rigItems {
			// For wisps, show the bonded bead's title instead
			title := item.Title
			if bondedTitle, ok := bondedTitles[item.BeadID]; ok && bondedTitle != "" {
				title = bondedTitle
			}
			if title != "" {
				fmt.Printf("  %s  %s\n", item.BeadID, style.Dim.Render(title))
			} else {
				fmt.Printf("  %s\n", item.BeadID)
			}
		}
	}

	return nil
}

func runQueueRun(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	ops := beads.NewRealBeadsOps(townRoot)
	q := queue.New(ops)

	// Load queue
	items, err := q.Load()
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("Queue is empty, nothing to dispatch")
		return nil
	}

	fmt.Printf("Found %d queued bead(s)\n", len(items))

	// Determine capacity: flag > config > unlimited
	capacity := queueRunMaxPolecats
	if capacity == 0 {
		capacity = config.GetQueueMaxPolecats(townRoot)
	}

	// Pre-allocate polecat names to avoid race condition when spawning in parallel.
	// Each rig has its own name pool, so we group by rig and allocate upfront.
	preAllocatedNames, err := preAllocatePolecatNames(townRoot, items)
	if err != nil {
		return fmt.Errorf("pre-allocating names: %w", err)
	}

	// Create spawner that uses pre-allocated names
	spawner := &queue.RealSpawner{
		SpawnInFunc: func(rigName, beadID string) error {
			// Look up pre-allocated name for this bead
			polecatName, ok := preAllocatedNames[beadID]
			if !ok {
				return fmt.Errorf("no pre-allocated name for bead %s", beadID)
			}

			spawnOpts := SlingSpawnOptions{
				Force:    queueRunForce,
				Account:  queueRunAccount,
				HookBead: beadID,
				Agent:    queueRunAgent,
				Create:   true,
			}
			info, err := SpawnPolecatForSlingWithName(rigName, polecatName, spawnOpts)
			if err != nil {
				return err
			}
			fmt.Printf("%s Spawned %s for %s\n", style.Bold.Render("✓"), info.AgentID(), beadID)

			// Wake witness and refinery
			wakeRigAgents(townRoot, rigName)
			return nil
		},
	}

	// Determine parallelism: flag > config > default (5)
	parallelism := queueRunSpawnBatchSize
	if parallelism <= 0 {
		parallelism = config.GetPolecatSpawnBatchSize(townRoot)
	}

	// Create dispatcher with options
	dispatcher := queue.NewDispatcher(q, spawner).
		WithDryRun(queueRunDryRun).
		WithParallelism(parallelism)

	// Apply capacity limit if set
	if capacity > 0 {
		// Count running polecats and calculate available slots
		running := countRunningPolecats()
		available := capacity - running
		if available <= 0 {
			fmt.Printf("Capacity reached: %d/%d polecats running\n", running, capacity)
			return nil
		}
		fmt.Printf("Capacity: %d/%d polecats running, %d slots available\n", running, capacity, available)
		dispatcher = dispatcher.WithLimit(available)
	}

	// Dispatch
	result, err := dispatcher.Dispatch()

	// Report results
	if queueRunDryRun {
		fmt.Printf("\nDry run - would dispatch %d bead(s):\n", len(result.Dispatched))
		for _, id := range result.Dispatched {
			fmt.Printf("  %s\n", id)
		}
		if len(result.Skipped) > 0 {
			fmt.Printf("\nWould skip %d bead(s) (capacity limit):\n", len(result.Skipped))
			for _, id := range result.Skipped {
				fmt.Printf("  %s\n", id)
			}
		}
	} else {
		fmt.Printf("\nDispatched: %d\n", len(result.Dispatched))
		if len(result.Skipped) > 0 {
			fmt.Printf("Skipped (capacity): %d\n", len(result.Skipped))
		}
		if len(result.Errors) > 0 {
			fmt.Printf("Errors: %d\n", len(result.Errors))
			for _, e := range result.Errors {
				fmt.Printf("  %s %v\n", style.Warning.Render("!"), e)
			}
		}
	}

	return err
}

func runQueueClear(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	ops := beads.NewRealBeadsOps(townRoot)
	q := queue.New(ops)

	cleared, err := q.Clear()
	if err != nil {
		return fmt.Errorf("clearing queue: %w", err)
	}

	fmt.Printf("Cleared %d bead(s) from queue\n", cleared)
	return nil
}

// countRunningPolecats counts the number of active polecat sessions across all rigs.
func countRunningPolecats() int {
	// Use factory.Agents() for local polecats (global across all rigs)
	agents := factory.Agents()
	agentIDs, err := agents.List()
	if err != nil {
		return 0
	}

	count := 0
	for _, aid := range agentIDs {
		if aid.Role == "polecat" {
			count++
		}
	}
	return count
}

// getBondedBeadTitles returns the titles of beads bonded to wisps (batch lookup).
// For wisps, the bonded bead is the dependent with dependency_type "blocks".
func getBondedBeadTitles(townRoot string, beadIDs []string) map[string]string {
	result := make(map[string]string)
	if len(beadIDs) == 0 {
		return result
	}

	// Run bd show with all IDs at once
	args := append([]string{"--no-daemon", "show", "--json"}, beadIDs...)
	cmd := exec.Command("bd", args...)
	cmd.Dir = townRoot

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return result
	}

	// Parse the JSON array
	var issues []struct {
		ID         string `json:"id"`
		Dependents []struct {
			Title          string `json:"title"`
			DependencyType string `json:"dependency_type"`
		} `json:"dependents"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return result
	}

	// Extract bonded bead titles
	for _, issue := range issues {
		for _, dep := range issue.Dependents {
			if dep.DependencyType == "blocks" {
				result[issue.ID] = dep.Title
				break
			}
		}
	}

	return result
}

// preAllocatePolecatNames allocates polecat names for all queue items upfront.
// This prevents race conditions when spawning polecats in parallel, since each
// call to SpawnPolecatForSling would otherwise create its own NamePool instance
// and potentially allocate the same name.
//
// Uses AllocateNames() which reconciles the pool state only once, then allocates
// all names without re-reconciling. This is critical because ReconcilePool()
// resets InUse based on existing directories, and no directories exist until
// polecats are actually created.
//
// Returns a map of beadID -> polecatName.
func preAllocatePolecatNames(townRoot string, items []queue.QueueItem) (map[string]string, error) {
	result := make(map[string]string)

	// Group items by rig - each rig has its own name pool
	byRig := make(map[string][]string) // rigName -> []beadID
	for _, item := range items {
		byRig[item.RigName] = append(byRig[item.RigName], item.BeadID)
	}

	// Load rig config for rig lookups
	rigsConfigPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	// For each rig, allocate all names at once using AllocateNames
	for rigName, beadIDs := range byRig {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			return nil, fmt.Errorf("rig '%s' not found: %w", rigName, err)
		}

		// Create ONE manager per rig
		polecatGit := git.NewGit(r.Path)
		agents := agent.Default()
		polecatMgr := polecat.NewManager(agents, r, polecatGit)

		// Allocate all names at once - this reconciles once then allocates N names
		names, err := polecatMgr.AllocateNames(len(beadIDs))
		if err != nil {
			return nil, fmt.Errorf("allocating names for rig %s: %w", rigName, err)
		}

		// Map bead IDs to allocated names
		for i, beadID := range beadIDs {
			result[beadID] = names[i]
		}
	}

	return result, nil
}
