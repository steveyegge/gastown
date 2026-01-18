package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Refinery command flags
var (
	refineryStatusJSON    bool
	refineryQueueJSON     bool
	refineryAgentOverride string
)

var refineryCmd = &cobra.Command{
	Use:     "refinery",
	Aliases: []string{"ref"},
	GroupID: GroupAgents,
	Short:   "Manage the Refinery (merge queue processor)",
	RunE:    requireSubcommand,
	Long: `Manage the Refinery - the per-rig merge queue processor.

The Refinery serializes all merges to main for a rig:
  - Receives MRs submitted by polecats (via gt done)
  - Rebases work branches onto latest main
  - Runs validation (tests, builds, checks)
  - Merges to main when clear
  - If conflict: spawns FRESH polecat to re-implement (original is gone)

Work flows: Polecat completes → gt done → MR in queue → Refinery merges.
The polecat is already nuked by the time the Refinery processes.

One Refinery per rig. Persistent agent that processes work as it arrives.

Role shortcuts: "refinery" in mail/nudge addresses resolves to this rig's Refinery.`,
}

var refineryStartCmd = &cobra.Command{
	Use:     "start [rig]",
	Aliases: []string{"spawn"},
	Short:   "Start the refinery",
	Long: `Start the Refinery for a rig.

Launches the merge queue processor which monitors for polecat work branches
and merges them to the appropriate target branches.

If rig is not specified, infers it from the current directory.

Examples:
  gt refinery start greenplace
  gt refinery start greenplace --foreground
  gt refinery start              # infer rig from cwd`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryStart,
}

var refineryStopCmd = &cobra.Command{
	Use:   "stop [rig]",
	Short: "Stop the refinery",
	Long: `Stop a running Refinery.

Gracefully stops the refinery, completing any in-progress merge first.
If rig is not specified, infers it from the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryStop,
}

var refineryStatusCmd = &cobra.Command{
	Use:   "status [rig]",
	Short: "Show refinery status",
	Long: `Show the status of a rig's Refinery.

Displays running state, current work, queue length, and statistics.
If rig is not specified, infers it from the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryStatus,
}

var refineryQueueCmd = &cobra.Command{
	Use:   "queue [rig]",
	Short: "Show merge queue",
	Long: `Show the merge queue for a rig.

Lists all pending merge requests waiting to be processed.
If rig is not specified, infers it from the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryQueue,
}

var refineryAttachCmd = &cobra.Command{
	Use:   "attach [rig]",
	Short: "Attach to refinery session",
	Long: `Attach to a running Refinery's Claude session.

Allows interactive access to the Refinery agent for debugging
or manual intervention.

If rig is not specified, infers it from the current directory.

Examples:
  gt refinery attach greenplace
  gt refinery attach          # infer rig from cwd`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryAttach,
}

var refineryRestartCmd = &cobra.Command{
	Use:   "restart [rig]",
	Short: "Restart the refinery",
	Long: `Restart the Refinery for a rig.

Stops the current session (if running) and starts a fresh one.
If rig is not specified, infers it from the current directory.

Examples:
  gt refinery restart greenplace
  gt refinery restart          # infer rig from cwd`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryRestart,
}

var refineryClaimCmd = &cobra.Command{
	Use:   "claim <mr-id>",
	Short: "Claim an MR for processing",
	Long: `Claim a merge request for processing by this refinery worker.

When running multiple refinery workers in parallel, each worker must claim
an MR before processing to prevent double-processing. Claims expire after
10 minutes if not processed (for crash recovery).

The worker ID is automatically determined from the GT_REFINERY_WORKER
environment variable, or defaults to "refinery-1".

Examples:
  gt refinery claim gt-abc123
  GT_REFINERY_WORKER=refinery-2 gt refinery claim gt-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runRefineryClaim,
}

var refineryReleaseCmd = &cobra.Command{
	Use:   "release <mr-id>",
	Short: "Release a claimed MR back to the queue",
	Long: `Release a claimed merge request back to the queue.

Called when processing fails and the MR should be retried by another worker.
This clears the claim so other workers can pick up the MR.

Examples:
  gt refinery release gt-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runRefineryRelease,
}

var refineryUnclaimedCmd = &cobra.Command{
	Use:   "unclaimed [rig]",
	Short: "List unclaimed MRs available for processing",
	Long: `List merge requests that are available for claiming.

Shows MRs that are not currently claimed by any worker, or have stale
claims (worker may have crashed). Useful for parallel refinery workers
to find work.

Examples:
  gt refinery unclaimed
  gt refinery unclaimed --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryUnclaimed,
}

var refineryUnclaimedJSON bool

var refineryReadyCmd = &cobra.Command{
	Use:   "ready [rig]",
	Short: "List MRs ready for processing (unclaimed and unblocked)",
	Long: `List merge requests ready for processing.

Shows MRs that are:
- Not currently claimed by any worker (or claim is stale)
- Not blocked by an open task (e.g., conflict resolution in progress)

This is the preferred command for finding work to process.

Examples:
  gt refinery ready
  gt refinery ready --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryReady,
}

var refineryReadyJSON bool

var refineryBlockedCmd = &cobra.Command{
	Use:   "blocked [rig]",
	Short: "List MRs blocked by open tasks",
	Long: `List merge requests blocked by open tasks.

Shows MRs waiting for conflict resolution or other blocking tasks to complete.
When the blocking task closes, the MR will appear in 'ready'.

Examples:
  gt refinery blocked
  gt refinery blocked --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRefineryBlocked,
}

var refineryBlockedJSON bool

func init() {
	// Start flags
	refineryStartCmd.Flags().StringVar(&refineryAgentOverride, "agent", "", "Agent alias to run the Refinery with (overrides town default)")

	// Attach flags
	refineryAttachCmd.Flags().StringVar(&refineryAgentOverride, "agent", "", "Agent alias to run the Refinery with (overrides town default)")

	// Restart flags
	refineryRestartCmd.Flags().StringVar(&refineryAgentOverride, "agent", "", "Agent alias to run the Refinery with (overrides town default)")

	// Status flags
	refineryStatusCmd.Flags().BoolVar(&refineryStatusJSON, "json", false, "Output as JSON")

	// Queue flags
	refineryQueueCmd.Flags().BoolVar(&refineryQueueJSON, "json", false, "Output as JSON")

	// Unclaimed flags
	refineryUnclaimedCmd.Flags().BoolVar(&refineryUnclaimedJSON, "json", false, "Output as JSON")

	// Ready flags
	refineryReadyCmd.Flags().BoolVar(&refineryReadyJSON, "json", false, "Output as JSON")

	// Blocked flags
	refineryBlockedCmd.Flags().BoolVar(&refineryBlockedJSON, "json", false, "Output as JSON")

	// Add subcommands
	refineryCmd.AddCommand(refineryStartCmd)
	refineryCmd.AddCommand(refineryStopCmd)
	refineryCmd.AddCommand(refineryRestartCmd)
	refineryCmd.AddCommand(refineryStatusCmd)
	refineryCmd.AddCommand(refineryQueueCmd)
	refineryCmd.AddCommand(refineryAttachCmd)
	refineryCmd.AddCommand(refineryClaimCmd)
	refineryCmd.AddCommand(refineryReleaseCmd)
	refineryCmd.AddCommand(refineryUnclaimedCmd)
	refineryCmd.AddCommand(refineryReadyCmd)
	refineryCmd.AddCommand(refineryBlockedCmd)

	rootCmd.AddCommand(refineryCmd)
}

// getRefineryManager creates a refinery manager for a rig.
// If rigName is empty, infers the rig from cwd.
func getRefineryManager(rigName string) (*refinery.Manager, *rig.Rig, string, error) {
	// Infer rig from cwd if not provided
	if rigName == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return nil, nil, "", fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return nil, nil, "", fmt.Errorf("could not determine rig: %w\nUsage: gt refinery <command> <rig>", err)
		}
	}

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return nil, nil, "", err
	}

	agentName, _ := config.ResolveRoleAgentName("refinery", townRoot, r.Path)
	mgr := factory.New(townRoot).RefineryManager(r, agentName)
	return mgr, r, rigName, nil
}

func runRefineryStart(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	// Infer rig from cwd if not provided
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig: %w\nUsage: gt refinery start <rig>", err)
		}
	}

	// Validate rig exists
	if _, _, err := getRig(rigName); err != nil {
		return err
	}

	fmt.Printf("Starting refinery for %s...\n", rigName)

	// Use factory.Start() with RefineryAddress (agent resolved automatically, with optional override)
	id := agent.RefineryAddress(rigName)
	if _, err := factory.Start(townRoot, id, "", factory.WithAgent(refineryAgentOverride)); err != nil {
		if err == agent.ErrAlreadyRunning {
			fmt.Printf("%s Refinery is already running\n", style.Dim.Render("⚠"))
			return nil
		}
		return fmt.Errorf("starting refinery: %w", err)
	}

	fmt.Printf("%s Refinery started for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt refinery status' to check progress"))
	return nil
}

func runRefineryStop(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	// Infer rig from cwd if not provided
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig: %w\nUsage: gt refinery stop <rig>", err)
		}
	}

	// Use factory.Agents().Stop() with RefineryAddress
	agents := factory.Agents()
	id := agent.RefineryAddress(rigName)

	if !agents.Exists(id) {
		fmt.Printf("%s Refinery is not running\n", style.Dim.Render("⚠"))
		return nil
	}

	if err := agents.Stop(id, true); err != nil {
		return fmt.Errorf("stopping refinery: %w", err)
	}

	fmt.Printf("%s Refinery stopped for %s\n", style.Bold.Render("✓"), rigName)
	return nil
}

func runRefineryStatus(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	mgr, _, rigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	ref, err := mgr.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	// JSON output
	if refineryStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ref)
	}

	// Human-readable output
	fmt.Printf("%s Refinery: %s\n\n", style.Bold.Render("⚙"), rigName)

	stateStr := string(ref.State)
	switch ref.State {
	case agent.StateRunning:
		stateStr = style.Bold.Render("● running")
	case agent.StateStopped:
		stateStr = style.Dim.Render("○ stopped")
	case agent.StatePaused:
		stateStr = style.Dim.Render("⏸ paused")
	}
	fmt.Printf("  State: %s\n", stateStr)

	if ref.StartedAt != nil {
		fmt.Printf("  Started: %s\n", ref.StartedAt.Format("2006-01-02 15:04:05"))
	}

	if ref.CurrentMR != nil {
		fmt.Printf("\n  %s\n", style.Bold.Render("Currently Processing:"))
		fmt.Printf("    Branch: %s\n", ref.CurrentMR.Branch)
		fmt.Printf("    Worker: %s\n", ref.CurrentMR.Worker)
		if ref.CurrentMR.IssueID != "" {
			fmt.Printf("    Issue:  %s\n", ref.CurrentMR.IssueID)
		}
	}

	// Get queue length
	queue, _ := mgr.Queue()
	pendingCount := 0
	for _, item := range queue {
		if item.Position > 0 { // Not currently processing
			pendingCount++
		}
	}
	fmt.Printf("\n  Queue: %d pending\n", pendingCount)

	return nil
}

func runRefineryQueue(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	mgr, _, rigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	queue, err := mgr.Queue()
	if err != nil {
		return fmt.Errorf("getting queue: %w", err)
	}

	// JSON output
	if refineryQueueJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(queue)
	}

	// Human-readable output
	fmt.Printf("%s Merge queue for '%s':\n\n", style.Bold.Render("📋"), rigName)

	if len(queue) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(empty)"))
		return nil
	}

	for _, item := range queue {
		status := ""
		prefix := fmt.Sprintf("  %d.", item.Position)

		if item.Position == 0 {
			prefix = "  ▶"
			status = style.Bold.Render("[processing]")
		} else {
			switch item.MR.Status {
			case refinery.MROpen:
				if item.MR.Error != "" {
					status = style.Dim.Render("[needs-rework]")
				} else {
					status = style.Dim.Render("[pending]")
				}
			case refinery.MRInProgress:
				status = style.Bold.Render("[processing]")
			case refinery.MRClosed:
				switch item.MR.CloseReason {
				case refinery.CloseReasonMerged:
					status = style.Bold.Render("[merged]")
				case refinery.CloseReasonRejected:
					status = style.Dim.Render("[rejected]")
				case refinery.CloseReasonConflict:
					status = style.Dim.Render("[conflict]")
				case refinery.CloseReasonSuperseded:
					status = style.Dim.Render("[superseded]")
				default:
					status = style.Dim.Render("[closed]")
				}
			}
		}

		issueInfo := ""
		if item.MR.IssueID != "" {
			issueInfo = fmt.Sprintf(" (%s)", item.MR.IssueID)
		}

		fmt.Printf("%s %s %s/%s%s %s\n",
			prefix,
			status,
			item.MR.Worker,
			item.MR.Branch,
			issueInfo,
			style.Dim.Render(item.Age))
	}

	return nil
}

func runRefineryAttach(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Use getRefineryManager to validate rig (and infer from cwd if needed)
	_, _, inferredRigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}
	rigName = inferredRigName

	// Check if session exists
	agents := factory.Agents()
	refineryID := agent.RefineryAddress(rigName)
	if !agents.Exists(refineryID) {
		// Auto-start if not running (agent resolved automatically, with optional override)
		fmt.Printf("Refinery not running for %s, starting...\n", rigName)
		if _, err := factory.Start(townRoot, refineryID, "", factory.WithAgent(refineryAgentOverride)); err != nil {
			return fmt.Errorf("starting refinery: %w", err)
		}
		fmt.Printf("%s Refinery started\n", style.Bold.Render("✓"))
	}

	// Smart attach: switches if inside tmux, attaches if outside
	return agents.Attach(refineryID)
}

func runRefineryRestart(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	_, _, inferredRigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}
	rigName = inferredRigName

	fmt.Printf("Restarting refinery for %s...\n", rigName)

	// Use factory.Start() with KillExisting (agent resolved automatically, with optional override)
	refineryID := agent.RefineryAddress(rigName)
	opts := []factory.StartOption{factory.WithKillExisting(), factory.WithAgent(refineryAgentOverride)}
	if _, err := factory.Start(townRoot, refineryID, "", opts...); err != nil {
		return fmt.Errorf("starting refinery: %w", err)
	}

	fmt.Printf("%s Refinery restarted for %s\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  %s\n", style.Dim.Render("Use 'gt refinery attach' to connect"))
	return nil
}

// getWorkerID returns the refinery worker ID from environment or default.
func getWorkerID() string {
	if id := os.Getenv("GT_REFINERY_WORKER"); id != "" {
		return id
	}
	return "refinery-1"
}

func runRefineryClaim(cmd *cobra.Command, args []string) error {
	mrID := args[0]
	workerID := getWorkerID()

	// Find beads from current working directory
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	eng := refinery.NewEngineer(r)
	if err := eng.ClaimMR(mrID, workerID); err != nil {
		return fmt.Errorf("claiming MR: %w", err)
	}

	fmt.Printf("%s Claimed %s for %s\n", style.Bold.Render("✓"), mrID, workerID)
	return nil
}

func runRefineryRelease(cmd *cobra.Command, args []string) error {
	mrID := args[0]

	// Find beads from current working directory
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	eng := refinery.NewEngineer(r)
	if err := eng.ReleaseMR(mrID); err != nil {
		return fmt.Errorf("releasing MR: %w", err)
	}

	fmt.Printf("%s Released %s back to queue\n", style.Bold.Render("✓"), mrID)
	return nil
}

func runRefineryUnclaimed(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	_, r, rigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	// Query beads for merge-request issues without assignee
	b := beads.New(r.Path)
	issues, err := b.List(beads.ListOptions{
		Status:   "open",
		Label:    "gt:merge-request",
		Priority: -1,
	})
	if err != nil {
		return fmt.Errorf("listing merge requests: %w", err)
	}

	// Filter for unclaimed (no assignee)
	var unclaimed []*refinery.MRInfo
	for _, issue := range issues {
		if issue.Assignee != "" {
			continue
		}
		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue
		}
		mr := &refinery.MRInfo{
			ID:       issue.ID,
			Branch:   fields.Branch,
			Target:   fields.Target,
			Worker:   fields.Worker,
			Priority: issue.Priority,
		}
		unclaimed = append(unclaimed, mr)
	}

	// JSON output
	if refineryUnclaimedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(unclaimed)
	}

	// Human-readable output
	fmt.Printf("%s Unclaimed MRs for '%s':\n\n", style.Bold.Render("📋"), rigName)

	if len(unclaimed) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(none available)"))
		return nil
	}

	for i, mr := range unclaimed {
		priority := fmt.Sprintf("P%d", mr.Priority)
		fmt.Printf("  %d. [%s] %s → %s\n", i+1, priority, mr.Branch, mr.Target)
		fmt.Printf("     ID: %s  Worker: %s\n", mr.ID, mr.Worker)
	}

	return nil
}

func runRefineryReady(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	_, r, rigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	// Create engineer for the rig (it has beads access for status checking)
	eng := refinery.NewEngineer(r)

	// Get ready MRs (unclaimed AND unblocked)
	ready, err := eng.ListReadyMRs()
	if err != nil {
		return fmt.Errorf("listing ready MRs: %w", err)
	}

	// JSON output
	if refineryReadyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ready)
	}

	// Human-readable output
	fmt.Printf("%s Ready MRs for '%s':\n\n", style.Bold.Render("🚀"), rigName)

	if len(ready) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(none ready)"))
		return nil
	}

	for i, mr := range ready {
		priority := fmt.Sprintf("P%d", mr.Priority)
		fmt.Printf("  %d. [%s] %s → %s\n", i+1, priority, mr.Branch, mr.Target)
		fmt.Printf("     ID: %s  Worker: %s\n", mr.ID, mr.Worker)
	}

	return nil
}

func runRefineryBlocked(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	_, r, rigName, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	// Create engineer for the rig (it has beads access for status checking)
	eng := refinery.NewEngineer(r)

	// Get blocked MRs
	blocked, err := eng.ListBlockedMRs()
	if err != nil {
		return fmt.Errorf("listing blocked MRs: %w", err)
	}

	// JSON output
	if refineryBlockedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(blocked)
	}

	// Human-readable output
	fmt.Printf("%s Blocked MRs for '%s':\n\n", style.Bold.Render("🚧"), rigName)

	if len(blocked) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(none blocked)"))
		return nil
	}

	for i, mr := range blocked {
		priority := fmt.Sprintf("P%d", mr.Priority)
		fmt.Printf("  %d. [%s] %s → %s\n", i+1, priority, mr.Branch, mr.Target)
		fmt.Printf("     ID: %s  Worker: %s\n", mr.ID, mr.Worker)
		if mr.BlockedBy != "" {
			fmt.Printf("     Blocked by: %s\n", mr.BlockedBy)
		}
	}

	return nil
}

// =============================================================================
// Auto-Start Helper (testable)
// =============================================================================

// AutoStartResult describes what happened during an auto-start check.
type AutoStartResult struct {
	AlreadyRunning bool  // true if agent was already running
	Started        bool  // true if agent was started
	Err            error // non-nil if start failed
}

// AgentExistsChecker checks if an agent exists.
type AgentExistsChecker interface {
	Exists(id agent.AgentID) bool
}

// AgentAutoStarter starts an agent.
type AgentAutoStarter func(id agent.AgentID) error

// ensureAgentRunning checks if an agent is running and starts it if not.
// This helper encapsulates the auto-start pattern for testing.
func ensureAgentRunning(id agent.AgentID, checker AgentExistsChecker, starter AgentAutoStarter) AutoStartResult {
	if checker.Exists(id) {
		return AutoStartResult{AlreadyRunning: true}
	}

	err := starter(id)
	if err != nil {
		return AutoStartResult{Err: err}
	}

	return AutoStartResult{Started: true}
}
