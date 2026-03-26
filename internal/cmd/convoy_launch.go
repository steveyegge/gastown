package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/workspace"
)

// convoyLaunchForce controls whether to launch a convoy with warnings.
var convoyLaunchForce bool

// DispatchResult records the outcome of dispatching a single task.
type DispatchResult struct {
	BeadID  string
	Rig     string
	Success bool
	Error   error
}

// dispatchTaskDirect dispatches a single task to its rig.
// In production, this delegates to gt sling. Tests override this variable
// with a stub to avoid spawning real processes.
var dispatchTaskDirect = func(townRoot, beadID, rig string) error {
	cmd := exec.Command("gt", "sling", beadID, rig)
	cmd.Dir = townRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gt sling %s %s: %w\nstderr: %s", beadID, rig, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

var convoyLaunchCmd = &cobra.Command{
	Use:   "launch <convoy-id | epic-id | task-id...>",
	Short: "Launch a staged convoy: transition to open and dispatch Wave 1",
	Long: `Launch a staged convoy by transitioning its status from staged to open
and dispatching Wave 1 tasks.

For staged convoy-id input: transitions directly and dispatches.
For epic/task input: runs stage + launch in one step.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConvoyLaunch,
}

func init() {
	convoyLaunchCmd.Flags().BoolVar(&convoyLaunchForce, "force", false, "Launch even with warnings")
}

// transitionConvoyToOpen transitions a staged convoy to open status.
// If the convoy is staged_ready, it transitions unconditionally.
// If the convoy is staged_warnings and force is true, it transitions.
// If the convoy is staged_warnings and force is false, it returns an error.
// If the convoy is already open or closed, it returns an error.
func transitionConvoyToOpen(convoyID string, force bool) error {
	result, err := bdShow(convoyID)
	if err != nil {
		return fmt.Errorf("cannot resolve convoy %s: %w", convoyID, err)
	}

	status := normalizeConvoyStatus(result.Status)

	switch status {
	case convoyStatusStagedReady:
		// Transition directly to open.
		return bdUpdateStatus(convoyID, convoyStatusOpen)

	case convoyStatusStagedWarnings:
		if !force {
			return fmt.Errorf("convoy %s has warnings, use --force to launch", convoyID)
		}
		return bdUpdateStatus(convoyID, convoyStatusOpen)

	case convoyStatusOpen:
		return fmt.Errorf("convoy %s is already launched", convoyID)

	case convoyStatusClosed:
		return fmt.Errorf("convoy %s is closed", convoyID)

	default:
		return fmt.Errorf("convoy %s has unexpected status %q", convoyID, result.Status)
	}
}

// bdUpdateStatus runs `bd update <id> --status=<status>` against the town beads
// database, since convoys live at the HQ level.
func bdUpdateStatus(beadID, status string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("bd", "update", beadID, "--status="+status)
	cmd.Dir = townBeads
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --status=%s: %w\noutput: %s", beadID, status, err, out)
	}
	return nil
}

// collectBlockedRigsInDAG returns a map of parked/docked rig names to the
// bead IDs that target them. Only considers slingable nodes. (gt-4owfd.1)
func collectBlockedRigsInDAG(dag *ConvoyDAG, townRoot string) map[string][]string {
	blockedRigBeads := make(map[string][]string)
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig == "" {
			continue
		}
		if blocked, _ := IsRigParkedOrDocked(townRoot, node.Rig); blocked {
			blockedRigBeads[node.Rig] = append(blockedRigBeads[node.Rig], node.ID)
		}
	}
	return blockedRigBeads
}

// checkBlockedRigsForLaunch checks if any target rigs are parked or docked.
// Returns an error listing all blocked rigs if any are found and force is false.
// (gt-4owfd.1)
func checkBlockedRigsForLaunch(dag *ConvoyDAG, townRoot string, force bool) error {
	blockedRigBeads := collectBlockedRigsInDAG(dag, townRoot)
	if len(blockedRigBeads) == 0 {
		return nil
	}

	// Build sorted list of blocked rigs for deterministic output
	var rigs []string
	for rig := range blockedRigBeads {
		rigs = append(rigs, rig)
	}
	sort.Strings(rigs)

	if force {
		// Warn but proceed
		fmt.Printf("Warning: %d non-operational rig(s) in convoy: %s\n", len(rigs), strings.Join(rigs, ", "))
		fmt.Printf("  Proceeding with --force (tasks may fail)\n")
		return nil
	}

	// Build detailed error message
	var details []string
	for _, rig := range rigs {
		beadIDs := blockedRigBeads[rig]
		sort.Strings(beadIDs)
		details = append(details, fmt.Sprintf("  %s: %s", rig, strings.Join(beadIDs, ", ")))
	}

	return fmt.Errorf("cannot launch: %d target rig(s) are parked or docked:\n%s\n\nUse 'gt rig unpark' or 'gt rig undock' to restore, or --force to proceed anyway",
		len(rigs), strings.Join(details, "\n"))
}

// dispatchWave1 dispatches all tasks in Wave 1 of the computed waves.
// Individual task failures do not abort remaining dispatches (I-14).
// Returns a result for every Wave 1 task and a non-nil error only if waves
// are empty or contain no Wave 1.
func dispatchWave1(convoyID string, dag *ConvoyDAG, waves []Wave, townRoot string) ([]DispatchResult, error) {
	if len(waves) == 0 {
		return nil, fmt.Errorf("convoy %s: no waves to dispatch", convoyID)
	}

	wave1 := waves[0]
	if wave1.Number != 1 {
		return nil, fmt.Errorf("convoy %s: first wave has unexpected number %d", convoyID, wave1.Number)
	}

	var results []DispatchResult
	for _, taskID := range wave1.Tasks {
		node := dag.Nodes[taskID]
		rig := ""
		if node != nil {
			rig = node.Rig
		}

		err := dispatchTaskDirect(townRoot, taskID, rig)
		results = append(results, DispatchResult{
			BeadID:  taskID,
			Rig:     rig,
			Success: err == nil,
			Error:   err,
		})
	}

	return results, nil
}

// renderLaunchOutput formats the post-launch console output showing convoy ID,
// wave summary, dispatched tasks with status, and helpful hints.
func renderLaunchOutput(convoyID string, waves []Wave, results []DispatchResult, dag *ConvoyDAG) string {
	var b strings.Builder

	// Section 1: Convoy ID with status.
	fmt.Fprintf(&b, "Convoy launched: %s (status: open)\n", convoyID)
	b.WriteString("\n")

	// Section 2: Monitor command hint.
	fmt.Fprintf(&b, "  Monitor: gt convoy status %s\n", convoyID)
	b.WriteString("\n")

	// Section 3: Wave summary.
	totalTasks := 0
	for _, w := range waves {
		totalTasks += len(w.Tasks)
	}
	b.WriteString("Wave summary:\n")
	fmt.Fprintf(&b, "  %d waves, %d tasks total\n", len(waves), totalTasks)
	for _, w := range waves {
		status := "pending"
		if w.Number == 1 {
			status = "dispatched"
		}
		fmt.Fprintf(&b, "  Wave %d: %d tasks (%s)\n", w.Number, len(w.Tasks), status)
	}
	b.WriteString("\n")

	// Section 4: Dispatched tasks (Wave 1).
	b.WriteString("Dispatched (Wave 1):\n")

	// Sort results by BeadID for deterministic output.
	sorted := make([]DispatchResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].BeadID < sorted[j].BeadID
	})

	for _, r := range sorted {
		marker := "✓"
		if !r.Success {
			marker = "✗"
		}

		title := ""
		if node := dag.Nodes[r.BeadID]; node != nil {
			title = node.Title
		}

		rigInfo := ""
		if r.Rig != "" {
			rigInfo = fmt.Sprintf("  (rig: %s)", r.Rig)
		}

		errInfo := ""
		if r.Error != nil {
			errInfo = fmt.Sprintf("    error: %v", r.Error)
		}

		fmt.Fprintf(&b, "  %s %s  %s%s%s\n", marker, r.BeadID, title, rigInfo, errInfo)
	}
	b.WriteString("\n")

	// Section 5: TUI hint.
	b.WriteString("  Hint: gt convoy -i for interactive monitoring\n")
	b.WriteString("\n")

	// Section 6: Daemon explanation.
	b.WriteString("Subsequent waves will be dispatched automatically by the daemon as tasks complete.\n")

	return b.String()
}

// runConvoyLaunch is the handler for `gt convoy launch`.
func runConvoyLaunch(cmd *cobra.Command, args []string) error {
	// Step 1: Validate args.
	if err := validateStageArgs(args); err != nil {
		return err
	}

	// Step 2: Resolve bead types via bd show for each arg.
	beadTypes := make(map[string]*bdShowResult)
	for _, arg := range args {
		result, err := bdShow(arg)
		if err != nil {
			return fmt.Errorf("cannot resolve bead %s: %w", arg, err)
		}
		beadTypes[arg] = result
	}

	// Step 3: If single arg is a convoy with staged status, transition to open
	// and dispatch Wave 1.
	if len(args) == 1 {
		result := beadTypes[args[0]]
		if result.IssueType == "convoy" && isStagedStatus(normalizeConvoyStatus(result.Status)) {
			convoyID := args[0]

			if err := transitionConvoyToOpen(convoyID, convoyLaunchForce); err != nil {
				return err
			}

			// Rebuild DAG from tracked beads and dispatch Wave 1.
			trackedBeads, deps, err := collectConvoyBeads(convoyID)
			if err != nil {
				return fmt.Errorf("collect beads for dispatch: %w", err)
			}

			dag := buildConvoyDAG(trackedBeads, deps)
			waves, _, err := computeWaves(dag)
			if err != nil {
				return fmt.Errorf("compute waves for dispatch: %w", err)
			}

			townRoot, err := workspace.FindFromCwdOrError()
			if err != nil {
				return fmt.Errorf("resolve town root for dispatch: %w", err)
			}

			// For batch-pr convoys, create integration branch before dispatch.
			merge := convoyMergeFromFields(result.Description)
			if merge == "batch-pr" {
				if err := createBatchPRIntegrationBranch(convoyID, result.Description, dag, townRoot); err != nil {
					return fmt.Errorf("create integration branch: %w", err)
				}
			}

			// Check for parked/docked rigs before dispatch (gt-4owfd.1, #2120)
			if err := checkBlockedRigsForLaunch(dag, townRoot, convoyLaunchForce); err != nil {
				return err
			}

			results, err := dispatchWave1(convoyID, dag, waves, townRoot)
			if err != nil {
				return fmt.Errorf("dispatch wave 1: %w", err)
			}

			// Report results.
			fmt.Print(renderLaunchOutput(convoyID, waves, results, dag))
			return nil
		}
	}

	// Step 4: For non-convoy or non-staged input, delegate to stage+launch flow.
	// Set the --launch flag on convoyStageCmd and delegate to runConvoyStage.
	convoyStageLaunch = true
	defer func() { convoyStageLaunch = false }()
	return runConvoyStage(cmd, args)
}

// createBatchPRIntegrationBranch creates an integration branch for a batch-pr convoy.
// The branch is created on the primary rig (most common rig among slingable nodes)
// from the rig's default branch (or the convoy's base_branch if set).
// The branch name is stored on the convoy bead's description for downstream consumption.
func createBatchPRIntegrationBranch(convoyID, convoyDesc string, dag *ConvoyDAG, townRoot string) error {
	// 1. Determine primary rig from DAG.
	primaryRigName := primaryRigFromDAG(dag)
	if primaryRigName == "" {
		return fmt.Errorf("convoy %s: no rig found in DAG for integration branch", convoyID)
	}

	// 2. Load the rig.
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return fmt.Errorf("load rigs config: %w", err)
	}
	rigMgr := rig.NewManager(townRoot, rigsConfig, git.NewGit(townRoot))
	r, err := rigMgr.GetRig(primaryRigName)
	if err != nil {
		return fmt.Errorf("rig %q not found: %w", primaryRigName, err)
	}

	// 3. Determine base branch from convoy fields or rig default.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoyDesc})
	baseBranch := r.DefaultBranch()
	if convoyFields != nil && convoyFields.BaseBranch != "" {
		baseBranch = convoyFields.BaseBranch
	}

	// 4. Determine integration branch name: convoy/<convoy-id>.
	branchName := "convoy/" + convoyID

	// 5. Get git client for the rig and create the branch.
	g, err := getRigGit(r.Path)
	if err != nil {
		return fmt.Errorf("initializing git for rig %s: %w", primaryRigName, err)
	}

	fmt.Printf("Creating integration branch %s from %s on rig %s...\n", branchName, baseBranch, primaryRigName)

	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching from origin: %w", err)
	}

	if err := g.CreateBranchFrom(branchName, "origin/"+baseBranch); err != nil {
		return fmt.Errorf("creating branch %s: %w", branchName, err)
	}

	if err := g.Push("origin", branchName, false); err != nil {
		// Clean up local branch on push failure.
		_ = g.DeleteBranch(branchName, true)
		return fmt.Errorf("pushing branch %s to origin: %w", branchName, err)
	}

	// 6. Store integration branch name on convoy bead.
	newDesc := beads.AddIntegrationBranchField(convoyDesc, branchName)
	if err := bdUpdateConvoyDescription(convoyID, newDesc); err != nil {
		// Non-fatal: branch was created, just metadata update failed.
		fmt.Printf("  (warning: could not store integration branch on convoy: %v)\n", err)
	}

	fmt.Printf("  ✓ Integration branch: %s (rig: %s, base: %s)\n", branchName, primaryRigName, baseBranch)
	return nil
}

// primaryRigFromDAG returns the most common rig name among slingable nodes in the DAG.
// Returns empty string if no slingable nodes have a rig assignment.
func primaryRigFromDAG(dag *ConvoyDAG) string {
	rigCount := make(map[string]int)
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig != "" {
			rigCount[node.Rig]++
		}
	}

	var primaryRig string
	maxCount := 0
	for rigName, count := range rigCount {
		if count > maxCount {
			maxCount = count
			primaryRig = rigName
		}
	}
	return primaryRig
}

// bdUpdateConvoyDescription updates a convoy bead's description.
// Convoys live in the town HQ beads database.
func bdUpdateConvoyDescription(convoyID, description string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("bd", "update", convoyID, "--description="+description)
	cmd.Dir = townBeads
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --description: %w\noutput: %s", convoyID, err, out)
	}
	return nil
}
