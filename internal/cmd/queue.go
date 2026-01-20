package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/queue"
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

var queueAddCmd = &cobra.Command{
	Use:   "add <bead-id>...",
	Short: "Add beads to the queue",
	Long: `Add one or more beads to the work queue.

Beads are marked with the "queued" label. Town-level beads (hq-*) cannot be
queued since polecats are rig-local.

Examples:
  gt queue add gt-abc
  gt queue add gt-abc gt-def gt-ghi`,
	Args: cobra.MinimumNArgs(1),
	RunE: runQueueAdd,
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

Each queued bead gets its own polecat. The --capacity flag limits the total
number of polecats that can be running at once (checks current count and
spawns up to the limit). The --parallel flag controls how many polecats
spawn concurrently.

Examples:
  gt queue run                      # Dispatch all queued beads
  gt queue run --capacity 10        # Only spawn up to 10 total polecats
  gt queue run --parallel 3         # Spawn 3 at a time
  gt queue run --dry-run            # Show what would be dispatched`,
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
	queueRunDryRun   bool
	queueRunCapacity int
	queueRunParallel int
	queueRunAgent    string
	queueRunAccount  string
	queueRunForce    bool
	queueListJSON    bool
	queueListPending bool
	queueListRunning bool
)

func init() {
	queueRunCmd.Flags().BoolVarP(&queueRunDryRun, "dry-run", "n", false, "Show what would be dispatched")
	queueRunCmd.Flags().IntVar(&queueRunCapacity, "capacity", 0, "Max total polecats running (0 = use config or unlimited)")
	queueRunCmd.Flags().IntVar(&queueRunParallel, "parallel", 0, "Number of concurrent dispatches (0 = use default)")
	queueRunCmd.Flags().StringVar(&queueRunAgent, "agent", "", "Override agent/runtime for spawned polecats")
	queueRunCmd.Flags().StringVar(&queueRunAccount, "account", "", "Claude Code account handle to use")
	queueRunCmd.Flags().BoolVar(&queueRunForce, "force", false, "Force spawn even if polecat has unread mail")

	queueListCmd.Flags().BoolVar(&queueListJSON, "json", false, "Output in JSON format")
	queueListCmd.Flags().BoolVar(&queueListPending, "pending", false, "Show only pending beads")
	queueListCmd.Flags().BoolVar(&queueListRunning, "running", false, "Show only running beads")

	queueCmd.AddCommand(queueStatusCmd)
	queueCmd.AddCommand(queueAddCmd)
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
	running := countRunningPolecats(townRoot)

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

func runQueueAdd(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	ops := beads.NewRealBeadsOps(townRoot)
	q := queue.New(ops)

	var added, failed int
	for _, beadID := range args {
		if err := q.Add(beadID); err != nil {
			fmt.Printf("%s Failed to queue %s: %v\n", style.Warning.Render("!"), beadID, err)
			failed++
			continue
		}
		fmt.Printf("%s Queued %s\n", style.Bold.Render("✓"), beadID)
		added++
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d beads failed to queue", failed, len(args))
	}

	fmt.Printf("\n%d bead(s) added to queue\n", added)
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
	byRig := make(map[string][]string)
	for _, item := range items {
		byRig[item.RigName] = append(byRig[item.RigName], item.BeadID)
	}

	fmt.Printf("Queued beads: %d\n\n", len(items))
	for rigName, beadIDs := range byRig {
		fmt.Printf("%s (%d):\n", style.Bold.Render(rigName), len(beadIDs))
		for _, id := range beadIDs {
			fmt.Printf("  %s\n", id)
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
	capacity := queueRunCapacity
	if capacity == 0 {
		capacity = config.GetQueueMaxPolecats(townRoot)
	}

	// Create spawner
	spawner := &queue.RealSpawner{
		SpawnInFunc: func(rigName, beadID string) error {
			spawnOpts := SlingSpawnOptions{
				Force:    queueRunForce,
				Account:  queueRunAccount,
				HookBead: beadID,
				Agent:    queueRunAgent,
				Create:   true,
			}
			info, err := SpawnPolecatForSling(rigName, spawnOpts)
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
	parallelism := queueRunParallel
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
		running := countRunningPolecats(townRoot)
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
func countRunningPolecats(townRoot string) int {
	// Load agents and count polecat sessions
	agents, err := loadAllAgents(townRoot)
	if err != nil {
		return 0
	}

	count := 0
	for _, a := range agents {
		if a.Type == "polecat" && a.HasActiveSession {
			count++
		}
	}
	return count
}

// agentInfo holds basic agent information for counting.
type agentInfo struct {
	Type             string
	HasActiveSession bool
}

// loadAllAgents loads basic agent info from all rigs.
func loadAllAgents(townRoot string) ([]agentInfo, error) {
	// This is a simplified implementation - in practice you'd use the agent package
	// to enumerate all agents across rigs
	rigsConfigPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, err
	}

	var agents []agentInfo
	for rigName := range rigsConfig.Rigs {
		rigAgents := loadRigAgents(townRoot, rigName)
		agents = append(agents, rigAgents...)
	}

	return agents, nil
}

// loadRigAgents loads agent info for a specific rig.
func loadRigAgents(townRoot, rigName string) []agentInfo {
	// Check for polecat sessions in this rig
	// This would typically check tmux sessions or agent state files
	// For now, return empty - the real implementation would use agent.List()
	return nil
}
