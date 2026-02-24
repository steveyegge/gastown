package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/convoy"
	"github.com/steveyegge/gastown/internal/workspace"
)

// convoyStageJSON controls whether output is machine-readable JSON.
var convoyStageJSON bool

// convoyStageLaunch controls whether to launch the convoy immediately after staging.
// When true, the staged convoy is transitioned to open and Wave 1 is dispatched.
// Set by `gt convoy stage --launch` or when `gt convoy launch` delegates to stage.
var convoyStageLaunch bool

func init() {
	convoyStageCmd.Flags().BoolVar(&convoyStageJSON, "json", false, "Output machine-readable JSON")
	convoyStageCmd.Flags().BoolVar(&convoyStageLaunch, "launch", false, "Launch the convoy immediately after staging (transition to open)")
}

// ---------------------------------------------------------------------------
// JSON output types (gt-csl.4.3)
// ---------------------------------------------------------------------------

// StageResult is the top-level JSON output for gt convoy stage --json.
type StageResult struct {
	Status   string         `json:"status"`    // "staged_ready", "staged_warnings", or "error"
	ConvoyID string         `json:"convoy_id"` // empty if errors prevented creation
	Restaged bool           `json:"restaged"`  // true if an existing convoy was updated in place
	Errors   []FindingJSON  `json:"errors"`
	Warnings []FindingJSON  `json:"warnings"`
	Waves    []WaveJSON     `json:"waves"`
	Tree     []TreeNodeJSON `json:"tree"`
}

// FindingJSON is the JSON representation of a StagingFinding.
type FindingJSON struct {
	Category     string   `json:"category"`
	BeadIDs      []string `json:"bead_ids"`
	Message      string   `json:"message"`
	SuggestedFix string   `json:"suggested_fix,omitempty"`
}

// WaveJSON is the JSON representation of a Wave with task details.
type WaveJSON struct {
	Number int        `json:"number"`
	Tasks  []TaskJSON `json:"tasks"`
}

// TaskJSON is the JSON representation of a task within a wave.
type TaskJSON struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Rig       string   `json:"rig"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// TreeNodeJSON is the JSON representation of a DAG node in a nested tree.
type TreeNodeJSON struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Type     string         `json:"type"`
	Status   string         `json:"status"`
	Rig      string         `json:"rig,omitempty"`
	Children []TreeNodeJSON `json:"children,omitempty"`
}

// StageInputKind identifies the type of input provided to gt convoy stage.
type StageInputKind int

const (
	StageInputEpic   StageInputKind = iota // single epic ID → walk children
	StageInputTasks                        // one or more task IDs → analyze as-is
	StageInputConvoy                       // single convoy ID → read tracked beads
)

// StageInput represents parsed and validated input for gt convoy stage.
type StageInput struct {
	Kind    StageInputKind
	IDs     []string // bead IDs to process
	RawArgs []string // original args for error messages
}

// classifyBeadType returns the StageInputKind for a given bead type string.
func classifyBeadType(beadType string) StageInputKind {
	switch beadType {
	case "epic":
		return StageInputEpic
	case "convoy":
		return StageInputConvoy
	default:
		return StageInputTasks
	}
}

// validateStageArgs checks args for basic validity before any bd calls.
// Returns error for empty args, flag-like args, etc.
func validateStageArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("gt convoy stage requires at least one bead ID\n\nUsage: gt convoy stage <epic-id | task-id... | convoy-id>")
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			return fmt.Errorf("invalid bead ID %q: looks like a CLI flag, not a bead ID", arg)
		}
	}
	return nil
}

// resolveInputKind determines the StageInputKind from a map of bead types.
// beadTypes maps bead ID → type string (e.g. "epic", "task", "convoy").
// Returns error if types are mixed (e.g., epic + task) or if multiple
// epics/convoys are provided (only one is allowed).
func resolveInputKind(beadTypes map[string]string) (*StageInput, error) {
	if len(beadTypes) == 0 {
		return nil, fmt.Errorf("no bead types to resolve")
	}

	// Classify each bead and collect IDs per kind.
	kindCounts := make(map[StageInputKind][]string)
	for id, typ := range beadTypes {
		kind := classifyBeadType(typ)
		kindCounts[kind] = append(kindCounts[kind], id)
	}

	// Check for mixed types.
	if len(kindCounts) > 1 {
		var parts []string
		for kind, ids := range kindCounts {
			var label string
			switch kind {
			case StageInputEpic:
				label = "epic"
			case StageInputConvoy:
				label = "convoy"
			default:
				label = "task"
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", label, strings.Join(ids, ", ")))
		}
		// Sort for deterministic error messages.
		sort.Strings(parts)
		return nil, fmt.Errorf("mixed input types: %s\n  Use separate invocations for different input types", strings.Join(parts, " + "))
	}

	// Single kind — extract it.
	var kind StageInputKind
	var ids []string
	for k, v := range kindCounts {
		kind = k
		ids = v
	}

	// Sort IDs for deterministic output.
	sort.Strings(ids)

	// Epics and convoys must be singular.
	if kind == StageInputEpic && len(ids) > 1 {
		return nil, fmt.Errorf("only one epic ID allowed, got %d: %s\n  To stage multiple epics, run gt convoy stage once per epic", len(ids), strings.Join(ids, ", "))
	}
	if kind == StageInputConvoy && len(ids) > 1 {
		return nil, fmt.Errorf("only one convoy ID allowed, got %d: %s", len(ids), strings.Join(ids, ", "))
	}

	return &StageInput{
		Kind:    kind,
		IDs:     ids,
		RawArgs: ids,
	}, nil
}

var convoyStageCmd = &cobra.Command{
	Use:   "stage <epic-id | task-id... | convoy-id>",
	Short: "Stage a convoy: analyze dependencies, compute waves, create staged convoy",
	Long: `Analyze bead dependencies, compute execution waves, and create a staged convoy.

Three input forms:
  gt convoy stage <epic-id>           Walk epic's children, analyze all descendants
  gt convoy stage <task1> <task2>...  Analyze exactly the given tasks
  gt convoy stage <convoy-id>         Re-analyze an existing convoy's tracked beads

The staged convoy can later be launched with 'gt convoy launch'.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConvoyStage,
}

func runConvoyStage(cmd *cobra.Command, args []string) error {
	// Step 1: Validate args.
	if err := validateStageArgs(args); err != nil {
		return err
	}

	// Step 2: Resolve bead types via bd show for each arg.
	beadTypes := make(map[string]string)
	beadResults := make(map[string]*bdShowResult)
	for _, arg := range args {
		result, err := bdShow(arg)
		if err != nil {
			return fmt.Errorf("cannot resolve bead %s: %w", arg, err)
		}
		beadTypes[arg] = result.IssueType
		beadResults[arg] = result
	}

	// Step 3: Determine input kind.
	input, err := resolveInputKind(beadTypes)
	if err != nil {
		return err
	}

	// Step 3b: Detect re-stage scenario.
	// If input is a convoy that is already staged, we update in place.
	isRestage := false
	restageConvoyID := ""
	if input.Kind == StageInputConvoy {
		convoyResult := beadResults[input.IDs[0]]
		if strings.HasPrefix(convoyResult.Status, "staged_") {
			isRestage = true
			restageConvoyID = input.IDs[0]
		}
	}

	// Step 4: Collect beads and deps.
	beads, deps, err := collectBeads(input)
	if err != nil {
		return err
	}

	// Step 5: Build the DAG.
	dag := buildConvoyDAG(beads, deps)

	// Step 5b: Detect overlapping convoys (epic or task-list input only).
	// When the user stages from an epic (or task list), check if an existing
	// convoy already tracks these beads. This prevents duplicate convoys.
	if !isRestage && input.Kind != StageInputConvoy {
		slingableIDs := dagSlingableIDs(dag)
		overlaps, err := findOverlappingConvoys(slingableIDs)
		if err != nil {
			return fmt.Errorf("checking for overlapping convoys: %w", err)
		}
		autoRestage, autoConvoyID, err := handleOverlappingConvoys(overlaps)
		if err != nil {
			return err
		}
		if autoRestage {
			isRestage = true
			restageConvoyID = autoConvoyID
			if !convoyStageJSON {
				fmt.Printf("Re-staging existing convoy %s\n", autoConvoyID)
			}
		}
	}

	// Step 6: Detect errors.
	errFindings := detectErrors(dag)

	// Step 7: Detect warnings.
	warnFindings := detectWarnings(dag, input)

	// Step 8: Categorize findings.
	allFindings := append(errFindings, warnFindings...)
	errs, warns := categorizeFindings(allFindings)

	// Step 9: Choose status.
	status := chooseStatus(errs, warns)

	// --- JSON mode: build result and output ---
	if convoyStageJSON {
		return runConvoyStageJSON(dag, input, errs, warns, status, isRestage, restageConvoyID)
	}

	// Step 10: If errors, render and return.
	if len(errs) > 0 {
		fmt.Fprint(os.Stderr, renderErrors(errs))
		return fmt.Errorf("convoy staging failed: %d error(s) found", len(errs))
	}

	// Step 11: Compute waves (only when no errors).
	waves, err := computeWaves(dag)
	if err != nil {
		return err
	}

	// Step 12: Render DAG tree and print.
	treeOutput := renderDAGTree(dag, input)
	fmt.Print(treeOutput)

	// Step 13: Render wave table and print.
	waveOutput := renderWaveTable(waves, dag)
	fmt.Print(waveOutput)

	// Step 14: If warnings, render and print.
	if len(warns) > 0 {
		warnOutput := renderWarnings(warns)
		fmt.Print(warnOutput)
	}

	// Step 15: Create or update the staged convoy.
	var convoyID string
	if isRestage {
		// Re-stage: update existing convoy in place.
		if err := updateStagedConvoy(restageConvoyID, dag, waves, status); err != nil {
			return err
		}
		convoyID = restageConvoyID
		fmt.Printf("Convoy updated: %s (status: %s)\n", restageConvoyID, status)
	} else {
		// First stage: create a new convoy.
		var err error
		convoyID, err = createStagedConvoy(dag, waves, status)
		if err != nil {
			return err
		}
		fmt.Printf("Convoy created: %s (status: %s)\n", convoyID, status)
	}

	// Step 16: If --launch flag is set, transition to open immediately.
	if convoyStageLaunch {
		if err := transitionConvoyToOpen(convoyID, convoyLaunchForce); err != nil {
			return err
		}
		fmt.Printf("Convoy launched: %s (status: open)\n", convoyID)
	}

	return nil
}

// runConvoyStageJSON handles the --json output path for convoy staging.
// It builds a StageResult, marshals it, and prints to stdout.
// On errors, it still outputs JSON but returns an error for non-zero exit code.
func runConvoyStageJSON(dag *ConvoyDAG, input *StageInput, errs, warns []StagingFinding, status string, isRestage bool, restageConvoyID string) error {
	result := StageResult{
		Errors:   buildFindingsJSON(errs),
		Warnings: buildFindingsJSON(warns),
		Tree:     buildTreeJSON(dag, input),
	}

	if len(errs) > 0 {
		// Errors: no convoy created, status is "error", no waves.
		result.Status = "error"
		result.Waves = []WaveJSON{}

		out, err := renderJSON(result)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return fmt.Errorf("convoy staging failed: %d error(s) found", len(errs))
	}

	// No errors: compute waves and create/update convoy.
	waves, err := computeWaves(dag)
	if err != nil {
		return err
	}

	result.Status = status
	result.Waves = buildWavesJSON(waves, dag)

	if isRestage {
		if err := updateStagedConvoy(restageConvoyID, dag, waves, status); err != nil {
			return err
		}
		result.ConvoyID = restageConvoyID
		result.Restaged = true
	} else {
		convoyID, err := createStagedConvoy(dag, waves, status)
		if err != nil {
			return err
		}
		result.ConvoyID = convoyID
	}

	out, err := renderJSON(result)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// ---------------------------------------------------------------------------
// Overlapping convoy detection (prevent duplicate convoy creation)
// ---------------------------------------------------------------------------

// overlappingConvoy describes a convoy that tracks beads overlapping with a
// staging request.
type overlappingConvoy struct {
	ID           string // convoy bead ID (e.g., "hq-cv-abc")
	Status       string // convoy status (e.g., "staged_ready", "open")
	OverlapCount int    // number of slingable IDs also tracked by this convoy
}

// dagSlingableIDs returns sorted slingable bead IDs from a DAG.
func dagSlingableIDs(dag *ConvoyDAG) []string {
	var ids []string
	for _, node := range dag.Nodes {
		if isSlingableType(node.Type) {
			ids = append(ids, node.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// convoyTrackedBeadIDs returns the set of bead IDs tracked by a convoy.
// It calls `bd dep list <convoyID> --direction=down --type=tracks --json`
// against the town beads directory and unwraps external:prefix:id references.
func convoyTrackedBeadIDs(townBeads, convoyID string) (map[string]bool, error) {
	townRoot := filepath.Dir(townBeads)
	depCmd := exec.Command("bd", "dep", "list", convoyID, "--direction=down", "--type=tracks", "--json")
	depCmd.Dir = townRoot
	out, err := depCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd dep list %s --direction=down --type=tracks: %w", convoyID, err)
	}

	var tracked []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &tracked); err != nil {
		return nil, fmt.Errorf("parsing tracked deps for %s: %w", convoyID, err)
	}

	ids := make(map[string]bool, len(tracked))
	for _, t := range tracked {
		ids[beads.ExtractIssueID(t.ID)] = true
	}
	return ids, nil
}

// findOverlappingConvoys lists all staged and open convoys and checks which
// ones track beads that overlap with the given slingable IDs.
// Only staged_* and open convoys are considered; closed convoys are skipped.
func findOverlappingConvoys(slingableIDs []string) ([]overlappingConvoy, error) {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return nil, err
	}

	// List all convoys (--all includes every status).
	listCmd := exec.Command("bd", "list", "--type=convoy", "--all", "--json")
	listCmd.Dir = townBeads
	out, err := listCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &convoys); err != nil {
		return nil, fmt.Errorf("parsing convoy list: %w", err)
	}

	// Build set of slingable IDs for fast lookup.
	slingSet := make(map[string]bool, len(slingableIDs))
	for _, id := range slingableIDs {
		slingSet[id] = true
	}

	var overlaps []overlappingConvoy
	for _, c := range convoys {
		status := normalizeConvoyStatus(c.Status)
		// Only consider staged and open convoys.
		if !isStagedStatus(status) && status != convoyStatusOpen {
			continue
		}

		trackedIDs, err := convoyTrackedBeadIDs(townBeads, c.ID)
		if err != nil {
			// Skip convoys whose tracked beads can't be resolved.
			continue
		}

		// Compute intersection.
		overlapCount := 0
		for id := range trackedIDs {
			if slingSet[id] {
				overlapCount++
			}
		}
		if overlapCount > 0 {
			overlaps = append(overlaps, overlappingConvoy{
				ID:           c.ID,
				Status:       status,
				OverlapCount: overlapCount,
			})
		}
	}

	return overlaps, nil
}

// handleOverlappingConvoys decides what to do when existing convoys overlap
// with the beads being staged.
//
// Returns:
//   - (true, convoyID, nil): auto re-stage the given convoy
//   - (false, "", nil): no overlaps, proceed with fresh creation
//   - (false, "", error): cannot proceed (open convoy or ambiguous)
func handleOverlappingConvoys(overlaps []overlappingConvoy) (bool, string, error) {
	if len(overlaps) == 0 {
		return false, "", nil
	}

	// Separate by status.
	var staged []overlappingConvoy
	var open []overlappingConvoy
	for _, o := range overlaps {
		if isStagedStatus(o.Status) {
			staged = append(staged, o)
		} else if o.Status == convoyStatusOpen {
			open = append(open, o)
		}
	}

	// Open convoy overlap → error.
	if len(open) > 0 {
		o := open[0]
		return false, "", fmt.Errorf(
			"cannot stage: open convoy %s already tracks %d of these beads — close it first or wait for completion\n"+
				"  gt convoy close %s --reason \"re-staging\"",
			o.ID, o.OverlapCount, o.ID)
	}

	// Multiple staged overlaps → ambiguous.
	if len(staged) > 1 {
		ids := make([]string, len(staged))
		for i, s := range staged {
			ids[i] = s.ID
		}
		sort.Strings(ids)
		return false, "", fmt.Errorf(
			"ambiguous: %d staged convoys overlap with these beads (%s)\n"+
				"  Specify which convoy to re-stage: gt convoy stage <convoy-id>",
			len(staged), strings.Join(ids, ", "))
	}

	// Exactly one staged overlap → auto re-stage.
	return true, staged[0].ID, nil
}

// createStagedConvoy creates a convoy with the given staged status.
// It generates a convoy ID, builds a title and description, then runs
// `bd create` to create the convoy and `bd dep add` for each slingable bead.
// Convoys live in the town HQ beads database (hq-cv-* prefix), so all bd
// commands run against getTownBeadsDir(), matching gt convoy create behavior.
// Returns the convoy ID.
func createStagedConvoy(dag *ConvoyDAG, waves []Wave, status string) (string, error) {
	// Convoys live in the town HQ beads database.
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return "", err
	}

	// Ensure custom types (including 'convoy') are registered in town beads.
	if err := beads.EnsureCustomTypes(townBeads); err != nil {
		return "", fmt.Errorf("ensuring custom types: %w", err)
	}

	// Generate convoy ID.
	convoyID := fmt.Sprintf("hq-cv-%s", generateShortID())

	// Count slingable tasks and unique rigs.
	taskCount := 0
	rigSet := make(map[string]bool)
	var slingableIDs []string
	for _, node := range dag.Nodes {
		if isSlingableType(node.Type) {
			taskCount++
			slingableIDs = append(slingableIDs, node.ID)
			if node.Rig != "" {
				rigSet[node.Rig] = true
			}
		}
	}
	rigCount := len(rigSet)

	// Sort slingable IDs for determinism.
	sort.Strings(slingableIDs)

	// Build title and description.
	title := fmt.Sprintf("Staged: %d beads across %d rigs", taskCount, rigCount)
	description := fmt.Sprintf("Staged convoy: %d tasks, %d waves. Staged at %s",
		taskCount, len(waves), time.Now().UTC().Format(time.RFC3339))

	// Create the convoy via bd create in town beads, then set status via bd update.
	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + title,
		"--description=" + description,
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}
	if out, err := BdCmd(createArgs...).Dir(townBeads).WithAutoCommit().CombinedOutput(); err != nil {
		return "", fmt.Errorf("bd create convoy: %w\noutput: %s", err, out)
	}

	// Set the staged status.
	statusCmd := exec.Command("bd", "update", convoyID, "--status="+status)
	statusCmd.Dir = townBeads
	if out, err := statusCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("bd update convoy status: %w\noutput: %s", err, out)
	}

	// Track each slingable bead via bd dep add.
	for _, beadID := range slingableIDs {
		if out, err := BdCmd("dep", "add", convoyID, beadID, "--type=tracks").
			Dir(townBeads).WithAutoCommit().
			CombinedOutput(); err != nil {
			return "", fmt.Errorf("bd dep add %s %s: %w\noutput: %s", convoyID, beadID, err, out)
		}
	}

	return convoyID, nil
}

// updateStagedConvoy updates an existing staged convoy in place.
// It updates the status and description via `bd update` commands.
// It also reconciles tracked beads: adds new slingable beads and removes
// stale ones that are no longer in the DAG (e.g., tasks removed from the epic).
// Convoy beads live in the town HQ beads database, so all bd commands
// run against getTownBeadsDir().
func updateStagedConvoy(existingConvoyID string, dag *ConvoyDAG, waves []Wave, status string) error {
	// Convoys live in the town HQ beads database.
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Reconcile tracked beads: compare current tracking with desired slingable set.
	desiredIDs := dagSlingableIDs(dag)
	desiredSet := make(map[string]bool, len(desiredIDs))
	for _, id := range desiredIDs {
		desiredSet[id] = true
	}

	currentIDs, err := convoyTrackedBeadIDs(townBeads, existingConvoyID)
	if err != nil {
		return fmt.Errorf("reading tracked beads for %s: %w", existingConvoyID, err)
	}

	// Add new beads not currently tracked.
	for _, id := range desiredIDs {
		if !currentIDs[id] {
			if out, err := BdCmd("dep", "add", existingConvoyID, id, "--type=tracks").
				Dir(townBeads).WithAutoCommit().
				CombinedOutput(); err != nil {
				return fmt.Errorf("bd dep add %s %s: %w\noutput: %s", existingConvoyID, id, err, out)
			}
		}
	}

	// Remove stale beads no longer in the DAG.
	for id := range currentIDs {
		if !desiredSet[id] {
			if out, err := BdCmd("dep", "remove", existingConvoyID, id, "--type=tracks").
				Dir(townBeads).WithAutoCommit().
				CombinedOutput(); err != nil {
				return fmt.Errorf("bd dep remove %s %s: %w\noutput: %s", existingConvoyID, id, err, out)
			}
		}
	}

	// Update status.
	if out, err := BdCmd("update", existingConvoyID, "--status="+status).
		Dir(townBeads).WithAutoCommit().
		CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --status: %w\noutput: %s", existingConvoyID, err, out)
	}

	// Update description with new wave count + timestamp.
	description := fmt.Sprintf("Staged convoy: %d tasks, %d waves. Re-staged at %s",
		len(desiredIDs), len(waves), time.Now().UTC().Format(time.RFC3339))
	if out, err := BdCmd("update", existingConvoyID, "--description="+description).
		Dir(townBeads).WithAutoCommit().
		CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --description: %w\noutput: %s", existingConvoyID, err, out)
	}

	return nil
}

// ConvoyDAG represents an in-memory dependency graph for convoy staging.
type ConvoyDAG struct {
	Nodes map[string]*ConvoyDAGNode
}

// ConvoyDAGNode represents a single bead in the DAG.
type ConvoyDAGNode struct {
	ID        string
	Title     string
	Type      string // "epic", "task", "bug", etc.
	Status    string
	Rig       string
	BlockedBy []string // IDs of beads that block this one (execution edges)
	Blocks    []string // IDs of beads this one blocks
	Children  []string // parent-child children (hierarchy only, not execution)
	Parent    string   // parent-child parent
}

// detectCycles checks the DAG for cycles in execution edges (blocks/conditional-blocks/waits-for).
// Returns the cycle path as []string if a cycle is found, or nil if acyclic.
// Only considers BlockedBy/Blocks edges (execution edges), NOT parent-child.
//
// Uses DFS with 3-color marking:
//   - white (0): unvisited
//   - gray  (1): on the current recursion stack
//   - black (2): fully explored
func detectCycles(dag *ConvoyDAG) []string {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int)     // default zero = white
	parent := make(map[string]string) // tracks DFS parent for cycle extraction

	// Sort node IDs for deterministic traversal order.
	ids := make([]string, 0, len(dag.Nodes))
	for id := range dag.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// extractCycle walks back from the back-edge target through the DFS
	// parent chain to reconstruct the cycle path.
	extractCycle := func(from, to string) []string {
		// from -> to is the back-edge. The cycle is: to -> ... -> from -> to.
		path := []string{to}
		cur := from
		for cur != to {
			path = append(path, cur)
			cur = parent[cur]
		}
		// Reverse so the cycle reads in traversal order.
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		return path
	}

	var dfs func(id string) []string
	dfs = func(id string) []string {
		color[id] = gray
		node := dag.Nodes[id]
		if node == nil {
			color[id] = black
			return nil
		}

		// Sort neighbors for deterministic traversal.
		neighbors := make([]string, len(node.Blocks))
		copy(neighbors, node.Blocks)
		sort.Strings(neighbors)

		for _, next := range neighbors {
			switch color[next] {
			case white:
				parent[next] = id
				if cycle := dfs(next); cycle != nil {
					return cycle
				}
			case gray:
				// Back-edge found → cycle.
				return extractCycle(id, next)
			}
			// black → already fully explored, skip.
		}

		color[id] = black
		return nil
	}

	for _, id := range ids {
		if color[id] == white {
			if cycle := dfs(id); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// Wave represents a group of tasks that can execute in parallel.
type Wave struct {
	Number int
	Tasks  []string // bead IDs, sorted for determinism
}

// isSlingableType delegates to the canonical convoy.IsSlingableType, which
// handles empty types (legacy beads that default to "task").
func isSlingableType(beadType string) bool {
	return convoy.IsSlingableType(beadType)
}

// computeWaves assigns each slingable task to an execution wave using Kahn's algorithm.
// Wave 1 = tasks with no unsatisfied blocking deps within the staged set.
// Wave N+1 = tasks whose blockers are ALL in wave N or earlier.
// Epics and non-slingable types are excluded from waves.
// Parent-child deps do NOT create execution edges.
// Returns error if the DAG contains no slingable tasks.
func computeWaves(dag *ConvoyDAG) ([]Wave, error) {
	// Step 1: Filter to slingable types only.
	slingable := make(map[string]*ConvoyDAGNode)
	for id, node := range dag.Nodes {
		if isSlingableType(node.Type) {
			slingable[id] = node
		}
	}
	if len(slingable) == 0 {
		return nil, fmt.Errorf("no slingable tasks in DAG (need task, bug, feature, or chore)")
	}

	// Step 2: Calculate in-degree for each slingable node.
	// Only count BlockedBy entries that reference other slingable nodes.
	inDegree := make(map[string]int, len(slingable))
	for id, node := range slingable {
		deg := 0
		for _, blocker := range node.BlockedBy {
			if _, ok := slingable[blocker]; ok {
				deg++
			}
		}
		inDegree[id] = deg
	}

	// Step 3-6: Kahn's algorithm — peel off waves of in-degree-0 nodes.
	var waves []Wave
	processed := 0
	waveNum := 0

	for processed < len(slingable) {
		// Collect nodes with in-degree 0.
		var ready []string
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, id)
			}
		}

		if len(ready) == 0 {
			// All remaining nodes have dependencies — cycle (should be
			// caught by detectCycles before reaching here).
			return nil, fmt.Errorf("cycle detected among remaining %d slingable nodes", len(slingable)-processed)
		}

		// Step 7: Sort within each wave for determinism.
		sort.Strings(ready)
		waveNum++

		waves = append(waves, Wave{
			Number: waveNum,
			Tasks:  ready,
		})

		// Remove processed nodes and decrement in-degrees of their dependents.
		for _, id := range ready {
			delete(inDegree, id)
			processed++

			// Decrement in-degree of nodes this one blocks (that are slingable).
			for _, blocked := range slingable[id].Blocks {
				if _, ok := inDegree[blocked]; ok {
					inDegree[blocked]--
				}
			}
		}
	}

	return waves, nil
}

// BeadInfo represents raw bead data from bd show output.
type BeadInfo struct {
	ID     string
	Title  string
	Type   string // "epic", "task", "bug", etc.
	Status string
	Rig    string // resolved rig name
}

// DepInfo represents a raw dependency from bd dep list output.
type DepInfo struct {
	IssueID     string // the dependent bead
	DependsOnID string // the bead it depends on
	Type        string // "blocks", "parent-child", "waits-for", "conditional-blocks", "tracks", "related", etc.
}

// buildConvoyDAG constructs a ConvoyDAG from raw bead and dependency data.
// Edge classification:
//   - blocks, conditional-blocks, waits-for → execution edges (BlockedBy/Blocks)
//   - parent-child → hierarchy metadata (Children/Parent), NOT execution edges
//   - related, tracks, discovered-from, etc. → ignored
func buildConvoyDAG(beads []BeadInfo, deps []DepInfo) *ConvoyDAG {
	dag := &ConvoyDAG{Nodes: make(map[string]*ConvoyDAGNode)}

	// Create nodes from beads.
	for _, b := range beads {
		dag.Nodes[b.ID] = &ConvoyDAGNode{
			ID:     b.ID,
			Title:  b.Title,
			Type:   b.Type,
			Status: b.Status,
			Rig:    b.Rig,
		}
	}

	// Process deps.
	for _, d := range deps {
		from := dag.Nodes[d.DependsOnID] // the blocker
		to := dag.Nodes[d.IssueID]       // the blocked
		if from == nil || to == nil {
			continue // skip deps referencing beads not in our set
		}

		switch d.Type {
		case "blocks", "conditional-blocks", "waits-for":
			// Execution edges.
			from.Blocks = append(from.Blocks, to.ID)
			to.BlockedBy = append(to.BlockedBy, from.ID)
		case "parent-child":
			// Hierarchy only.
			from.Children = append(from.Children, to.ID)
			to.Parent = from.ID
		default:
			// related, tracks, discovered-from, etc. — ignored.
		}
	}

	return dag
}

// StagingFinding represents an error or warning found during convoy staging analysis.
type StagingFinding struct {
	Severity     string   // "error" or "warning"
	Category     string   // "cycle", "no-rig", "orphan", "parked-rig", "cross-rig", "capacity", "missing-branch"
	BeadIDs      []string // affected bead IDs
	Message      string   // human-readable description
	SuggestedFix string   // actionable fix suggestion
}

// categorizeFindings splits findings into errors and warnings by severity.
func categorizeFindings(findings []StagingFinding) (errors, warnings []StagingFinding) {
	for _, f := range findings {
		switch f.Severity {
		case "error":
			errors = append(errors, f)
		default:
			warnings = append(warnings, f)
		}
	}
	return
}

// detectErrors runs all error detection checks on the DAG.
// Returns findings with severity="error" for fatal issues.
func detectErrors(dag *ConvoyDAG) []StagingFinding {
	var findings []StagingFinding

	// Check for cycles
	cyclePath := detectCycles(dag)
	if cyclePath != nil {
		findings = append(findings, StagingFinding{
			Severity:     "error",
			Category:     "cycle",
			BeadIDs:      cyclePath,
			Message:      fmt.Sprintf("dependency cycle detected: %s", strings.Join(cyclePath, " → ")),
			SuggestedFix: fmt.Sprintf("remove one blocking dependency in the chain: %s", strings.Join(cyclePath, " → ")),
		})
	}

	// Check for beads with no valid rig
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue // epics don't need rigs
		}
		if node.Rig == "" {
			findings = append(findings, StagingFinding{
				Severity:     "error",
				Category:     "no-rig",
				BeadIDs:      []string{node.ID},
				Message:      fmt.Sprintf("bead %s has no valid rig (prefix not mapped in routes.jsonl or resolves to empty)", node.ID),
				SuggestedFix: fmt.Sprintf("add a routes.jsonl entry mapping the prefix of %s to a rig, or check that the bead ID has a valid prefix", node.ID),
			})
		}
	}

	// Sort findings by bead ID for determinism
	sort.Slice(findings, func(i, j int) bool {
		if len(findings[i].BeadIDs) == 0 || len(findings[j].BeadIDs) == 0 {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].BeadIDs[0] < findings[j].BeadIDs[0]
	})

	return findings
}

// chooseStatus determines the convoy status based on analysis results.
// Returns "" if errors found (no convoy should be created).
func chooseStatus(errors, warnings []StagingFinding) string {
	if len(errors) > 0 {
		return "" // no convoy
	}
	if len(warnings) > 0 {
		return "staged_warnings"
	}
	return "staged_ready"
}

// renderErrors formats error findings for console output.
func renderErrors(findings []StagingFinding) string {
	if len(findings) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("Errors:\n")
	for i, f := range findings {
		buf.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, f.Category, f.Message))
		if len(f.BeadIDs) > 0 {
			buf.WriteString(fmt.Sprintf("     Affected: %s\n", strings.Join(f.BeadIDs, ", ")))
		}
		if f.SuggestedFix != "" {
			buf.WriteString(fmt.Sprintf("     Fix: %s\n", f.SuggestedFix))
		}
	}
	return buf.String()
}

// renderWaveTable produces the wave dispatch plan table as a string.
// Format:
//
//	Wave | ID | Title | Rig | Blocked By
//	─────┼────┼───────┼─────┼───────────
//	1    | gt-a  | Task A | gst | —
//	1    | gt-c  | Task C | gst | —
//	2    | gt-b  | Task B | gst | gt-a
//	3    | gt-d  | Task D | gst | gt-b
//
// Summary line at end: "N tasks across M waves (max parallelism: K in wave W)"
func renderWaveTable(waves []Wave, dag *ConvoyDAG) string {
	var buf strings.Builder

	// Header
	buf.WriteString(fmt.Sprintf("  %-6s %-15s %-30s %-12s %s\n", "Wave", "ID", "Title", "Rig", "Blocked By"))
	buf.WriteString("  " + strings.Repeat("─", 80) + "\n")

	totalTasks := 0
	maxParallel := 0
	maxWave := 0

	for _, wave := range waves {
		if len(wave.Tasks) > maxParallel {
			maxParallel = len(wave.Tasks)
			maxWave = wave.Number
		}
		for _, taskID := range wave.Tasks {
			node := dag.Nodes[taskID]
			if node == nil {
				continue
			}

			title := node.Title
			if len(title) > 28 {
				title = title[:28] + ".."
			}

			rig := node.Rig
			if rig == "" {
				rig = "?"
			}

			blockers := "—"
			if len(node.BlockedBy) > 0 {
				blockers = strings.Join(node.BlockedBy, ", ")
			}

			buf.WriteString(fmt.Sprintf("  %-6d %-15s %-30s %-12s %s\n", wave.Number, taskID, title, rig, blockers))
			totalTasks++
		}
	}

	// Summary line
	buf.WriteString(fmt.Sprintf("\n  %d tasks across %d waves (max parallelism: %d in wave %d)\n",
		totalTasks, len(waves), maxParallel, maxWave))

	return buf.String()
}

// ---------------------------------------------------------------------------
// bd JSON parsing types
// ---------------------------------------------------------------------------

// bdShowResult matches the JSON output of `bd show <id> --json`.
type bdShowResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	IssueType string `json:"issue_type"`
}

// bdDepResult matches the JSON output of `bd dep list <id> --json`.
// Each entry is a bead that the queried issue depends on.
// The IssueID is set by the caller (it's the bead we queried).
type bdDepResult struct {
	IssueID        string `json:"-"`               // set by caller
	DependsOnID    string `json:"id"`              // the dependency target
	Type           string `json:"dependency_type"` // blocks, parent-child, etc.
}

// ---------------------------------------------------------------------------
// bd shell-out helpers
// ---------------------------------------------------------------------------

// bdShow runs `bd show <id> --json` and returns the parsed bead info.
// Returns error if bd exits non-zero or returns no results.
func bdShow(beadID string) (*bdShowResult, error) {
	out, err := exec.Command("bd", "show", beadID, "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", beadID, err)
	}

	var results []bdShowResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("bd show %s: parse JSON: %w (raw: %s)", beadID, err, out)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("bd show %s: no results", beadID)
	}

	return &results[0], nil
}

// bdDepList runs `bd dep list <id> --json` and returns parsed deps.
// bd dep list returns the beads that <id> depends on. Each result's
// DependsOnID is the dependency target; IssueID is set to <id> by this func.
func bdDepList(beadID string) ([]bdDepResult, error) {
	out, err := exec.Command("bd", "dep", "list", beadID, "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bd dep list %s: %w", beadID, err)
	}

	var results []bdDepResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("bd dep list %s: parse JSON: %w (raw: %s)", beadID, err, out)
	}

	// Set the IssueID on each result (bd dep list returns deps OF beadID).
	for i := range results {
		results[i].IssueID = beadID
	}

	return results, nil
}

// bdListChildren runs `bd list --parent=<id> --json` and returns child beads.
// bd list is CWD-sensitive — it only searches the beads database in the current
// directory. We resolve the correct .beads directory from the bead's prefix via
// routes.jsonl so this works regardless of the caller's working directory.
func bdListChildren(parentID string) ([]bdShowResult, error) {
	cmd := exec.Command("bd", "list", "--parent="+parentID, "--json")
	if dir := beadsDirForID(parentID); dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list --parent=%s: %w", parentID, err)
	}

	// Handle empty output (no children).
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var results []bdShowResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("bd list --parent=%s: parse JSON: %w (raw: %s)", parentID, err, out)
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// collectBeads and variants
// ---------------------------------------------------------------------------

// rigFromBeadID extracts the rig name from a bead ID by looking up its prefix
// in routes.jsonl. Returns empty string if the prefix is not found or if the
// workspace cannot be resolved.
func rigFromBeadID(beadID string) string {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}
	return beads.GetRigNameForPrefix(townRoot, prefix)
}

// beadsDirForID resolves the .beads directory that owns a given bead ID by
// looking up its prefix in routes.jsonl. This is needed for CWD-sensitive bd
// commands like `bd list --parent=` which only search the local database.
// Returns empty string if the prefix or workspace cannot be resolved.
func beadsDirForID(beadID string) string {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}
	rigPath := beads.GetRigPathForPrefix(townRoot, prefix)
	if rigPath == "" {
		return ""
	}
	return beads.ResolveBeadsDir(rigPath)
}

// collectBeads gathers all beads for staging based on the input kind.
// For epic input: recursively walks parent-child tree via bd list --parent=<id> --json
// For task list input: validates each bead exists via bd show <id> --json
// For convoy input: reads tracked beads via bd dep list <id> --type=tracks --json
// Returns BeadInfo slice and DepInfo slice for all collected beads.
func collectBeads(input *StageInput) ([]BeadInfo, []DepInfo, error) {
	switch input.Kind {
	case StageInputEpic:
		return collectEpicBeads(input.IDs[0])
	case StageInputTasks:
		return collectTaskListBeads(input.IDs)
	case StageInputConvoy:
		return collectConvoyBeads(input.IDs[0])
	}
	return nil, nil, fmt.Errorf("unknown input kind: %d", input.Kind)
}

// collectEpicBeads recursively walks an epic's parent-child tree.
// Uses bd list --parent=<id> --json for each level.
// For each bead found, also fetches its deps via bd dep list <id> --json.
func collectEpicBeads(epicID string) ([]BeadInfo, []DepInfo, error) {
	// 1. Validate the root epic exists.
	root, err := bdShow(epicID)
	if err != nil {
		return nil, nil, fmt.Errorf("epic %s: %w", epicID, err)
	}

	var allBeads []BeadInfo
	var allDeps []DepInfo
	visited := make(map[string]bool)

	// BFS queue for recursive tree walk.
	queue := []bdShowResult{*root}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.ID] {
			continue
		}
		visited[current.ID] = true

		// Add bead info.
		allBeads = append(allBeads, BeadInfo{
			ID:     current.ID,
			Title:  current.Title,
			Type:   current.IssueType,
			Status: current.Status,
			Rig:    rigFromBeadID(current.ID),
		})

		// Fetch deps for this bead.
		deps, err := bdDepList(current.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("deps for %s: %w", current.ID, err)
		}
		for _, d := range deps {
			allDeps = append(allDeps, DepInfo{
				IssueID:     d.IssueID,
				DependsOnID: d.DependsOnID,
				Type:        d.Type,
			})
		}

		// List children and enqueue them.
		children, err := bdListChildren(current.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("children of %s: %w", current.ID, err)
		}
		for _, child := range children {
			if !visited[child.ID] {
				queue = append(queue, child)
			}
		}
	}

	return allBeads, allDeps, nil
}

// collectTaskListBeads validates and fetches info for explicit task IDs.
func collectTaskListBeads(taskIDs []string) ([]BeadInfo, []DepInfo, error) {
	var allBeads []BeadInfo
	var allDeps []DepInfo

	for _, id := range taskIDs {
		// Validate bead exists.
		result, err := bdShow(id)
		if err != nil {
			return nil, nil, fmt.Errorf("task %s: %w", id, err)
		}

		allBeads = append(allBeads, BeadInfo{
			ID:     result.ID,
			Title:  result.Title,
			Type:   result.IssueType,
			Status: result.Status,
			Rig:    rigFromBeadID(result.ID),
		})

		// Fetch deps.
		deps, err := bdDepList(id)
		if err != nil {
			return nil, nil, fmt.Errorf("deps for %s: %w", id, err)
		}
		for _, d := range deps {
			allDeps = append(allDeps, DepInfo{
				IssueID:     d.IssueID,
				DependsOnID: d.DependsOnID,
				Type:        d.Type,
			})
		}
	}

	return allBeads, allDeps, nil
}

// collectConvoyBeads reads tracked beads from an existing convoy.
func collectConvoyBeads(convoyID string) ([]BeadInfo, []DepInfo, error) {
	// 1. Validate convoy exists.
	_, err := bdShow(convoyID)
	if err != nil {
		return nil, nil, fmt.Errorf("convoy %s: %w", convoyID, err)
	}

	// 2. Get deps for the convoy — filter for "tracks" type.
	deps, err := bdDepList(convoyID)
	if err != nil {
		return nil, nil, fmt.Errorf("deps for convoy %s: %w", convoyID, err)
	}

	// Extract tracked bead IDs. bdDepList sets IssueID to the queried bead
	// (the convoy) and DependsOnID to the JSON "id" field (the tracked bead).
	var trackedIDs []string
	for _, d := range deps {
		if d.Type == "tracks" {
			trackedIDs = append(trackedIDs, d.DependsOnID)
		}
	}

	if len(trackedIDs) == 0 {
		return nil, nil, fmt.Errorf("convoy %s tracks no beads", convoyID)
	}

	// 3. Fetch each tracked bead + its deps.
	return collectTaskListBeads(trackedIDs)
}

// ---------------------------------------------------------------------------
// DAG tree display (ASCII, epic hierarchy)
// ---------------------------------------------------------------------------

// renderDAGTree renders the bead hierarchy as an ASCII tree.
//
// For epic input (StageInputEpic):
//   - Starts from the root epic (input.IDs[0])
//   - Recursively renders children with tree-drawing characters
//   - Shows hierarchy with indentation
//
// For task-list or convoy input:
//   - Flat list sorted by ID
//
// Each node shows: <id> [<type>] <title> (rig: <rig>) [<status>]
// Blocked tasks append: ← blocked by: <blocker1>, <blocker2>
func renderDAGTree(dag *ConvoyDAG, input *StageInput) string {
	var buf strings.Builder

	switch input.Kind {
	case StageInputEpic:
		if len(input.IDs) == 0 {
			return ""
		}
		rootID := input.IDs[0]
		root := dag.Nodes[rootID]
		if root == nil {
			return ""
		}
		// Render root node (no tree prefix).
		buf.WriteString(formatNodeLine(root))
		buf.WriteString("\n")
		// Render children recursively.
		children := sortedChildren(dag, rootID)
		for i, childID := range children {
			isLast := i == len(children)-1
			renderTreeNode(dag, childID, "", isLast, &buf)
		}

	default:
		// Flat list for task-list and convoy input.
		ids := sortedNodeIDs(dag)
		for _, id := range ids {
			node := dag.Nodes[id]
			if node == nil {
				continue
			}
			buf.WriteString("  ")
			buf.WriteString(formatNodeLine(node))
			buf.WriteString("\n")
		}
	}

	return buf.String()
}

// renderTreeNode is a recursive helper for tree rendering.
// It renders a single node with the appropriate tree-drawing prefix,
// then recurses into its children.
func renderTreeNode(dag *ConvoyDAG, nodeID string, prefix string, isLast bool, buf *strings.Builder) {
	node := dag.Nodes[nodeID]
	if node == nil {
		return
	}

	// Draw the branch connector.
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	buf.WriteString(prefix)
	buf.WriteString(connector)
	buf.WriteString(formatNodeLine(node))
	buf.WriteString("\n")

	// Determine the prefix for children of this node.
	childPrefix := prefix + "│   "
	if isLast {
		childPrefix = prefix + "    "
	}

	// Recurse into children.
	children := sortedChildren(dag, nodeID)
	for i, childID := range children {
		childIsLast := i == len(children)-1
		renderTreeNode(dag, childID, childPrefix, childIsLast, buf)
	}
}

// formatNodeLine formats a single node as: <id> [<type>] <title> (rig: <rig>) [<status>]
// For nodes with blockers, appends: ← blocked by: <blocker1>, <blocker2>
func formatNodeLine(node *ConvoyDAGNode) string {
	var sb strings.Builder
	sb.WriteString(node.ID)
	sb.WriteString(" [")
	sb.WriteString(node.Type)
	sb.WriteString("] ")
	sb.WriteString(node.Title)

	if node.Rig != "" {
		sb.WriteString(" (rig: ")
		sb.WriteString(node.Rig)
		sb.WriteString(")")
	}

	sb.WriteString(" [")
	sb.WriteString(node.Status)
	sb.WriteString("]")

	if len(node.BlockedBy) > 0 {
		sorted := make([]string, len(node.BlockedBy))
		copy(sorted, node.BlockedBy)
		sort.Strings(sorted)
		sb.WriteString(" ← blocked by: ")
		sb.WriteString(strings.Join(sorted, ", "))
	}

	return sb.String()
}

// sortedChildren returns the Children of the given node, sorted alphabetically.
func sortedChildren(dag *ConvoyDAG, nodeID string) []string {
	node := dag.Nodes[nodeID]
	if node == nil || len(node.Children) == 0 {
		return nil
	}
	children := make([]string, len(node.Children))
	copy(children, node.Children)
	sort.Strings(children)
	return children
}

// sortedNodeIDs returns all node IDs in the DAG, sorted alphabetically.
func sortedNodeIDs(dag *ConvoyDAG) []string {
	ids := make([]string, 0, len(dag.Nodes))
	for id := range dag.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---------------------------------------------------------------------------
// Warning detection (gt-csl.3.4)
// ---------------------------------------------------------------------------

// waveCapacityThreshold is the maximum number of tasks in a wave before a
// capacity warning is emitted.
const waveCapacityThreshold = 5

// detectWarnings runs all warning checks on the DAG.
// Returns findings with severity="warning".
func detectWarnings(dag *ConvoyDAG, input *StageInput) []StagingFinding {
	var findings []StagingFinding

	findings = append(findings, detectOrphans(dag, input)...)
	findings = append(findings, detectParkedRigs(dag)...)
	findings = append(findings, detectCrossRig(dag)...)
	findings = append(findings, estimateCapacity(dag)...)
	findings = append(findings, detectMissingBranches(dag)...)

	// Sort findings by first bead ID for determinism.
	sort.Slice(findings, func(i, j int) bool {
		if len(findings[i].BeadIDs) == 0 || len(findings[j].BeadIDs) == 0 {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].BeadIDs[0] < findings[j].BeadIDs[0]
	})

	return findings
}

// detectOrphans finds slingable tasks that are completely isolated in the
// wave graph (in-degree 0 AND out-degree 0 among slingable nodes).
// Only applies to epic input — task-list and convoy input never warn about
// orphans because isolation is expected.
func detectOrphans(dag *ConvoyDAG, input *StageInput) []StagingFinding {
	if input.Kind != StageInputEpic {
		return nil
	}

	// Build slingable set.
	slingable := make(map[string]*ConvoyDAGNode)
	for id, node := range dag.Nodes {
		if isSlingableType(node.Type) {
			slingable[id] = node
		}
	}

	var findings []StagingFinding
	for id, node := range slingable {
		// Calculate in-degree among slingable nodes.
		inDeg := 0
		for _, blocker := range node.BlockedBy {
			if _, ok := slingable[blocker]; ok {
				inDeg++
			}
		}

		// Calculate out-degree among slingable nodes.
		outDeg := 0
		for _, blocked := range node.Blocks {
			if _, ok := slingable[blocked]; ok {
				outDeg++
			}
		}

		if inDeg == 0 && outDeg == 0 {
			findings = append(findings, StagingFinding{
				Severity:     "warning",
				Category:     "orphan",
				BeadIDs:      []string{id},
				Message:      fmt.Sprintf("task %s has no blocking dependencies with other staged tasks (isolated in wave graph)", id),
				SuggestedFix: fmt.Sprintf("add a blocking dependency for %s, or verify it should be staged independently", id),
			})
		}
	}

	return findings
}

// isRigParkedFn is a seam for tests. Production uses IsRigParked.
var isRigParkedFn = func(townRoot, rigName string) bool {
	return IsRigParked(townRoot, rigName)
}

// detectParkedRigs warns about slingable nodes whose target rig is parked
// in the wisp layer (gt-4owfd.1). This uses the actual IsRigParked() check
// rather than string matching on rig names.
func detectParkedRigs(dag *ConvoyDAG) []StagingFinding {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// Can't resolve town root — skip parked rig detection
		return nil
	}

	// Group beads by parked rig to consolidate warnings
	parkedRigBeads := make(map[string][]string)
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig == "" {
			continue // already caught by no-rig errors
		}
		if isRigParkedFn(townRoot, node.Rig) {
			parkedRigBeads[node.Rig] = append(parkedRigBeads[node.Rig], node.ID)
		}
	}

	// Sort rig names for deterministic output with multiple parked rigs
	rigNames := make([]string, 0, len(parkedRigBeads))
	for rigName := range parkedRigBeads {
		rigNames = append(rigNames, rigName)
	}
	sort.Strings(rigNames)

	var findings []StagingFinding
	for _, rigName := range rigNames {
		beadIDs := parkedRigBeads[rigName]
		// Sort bead IDs for determinism
		sort.Strings(beadIDs)
		findings = append(findings, StagingFinding{
			Severity:     "warning",
			Category:     "parked-rig",
			BeadIDs:      beadIDs,
			Message:      fmt.Sprintf("%d bead(s) target parked rig %q: %s", len(beadIDs), rigName, strings.Join(beadIDs, ", ")),
			SuggestedFix: fmt.Sprintf("unpark the rig: gt rig unpark %s", rigName),
		})
	}
	return findings
}

// detectCrossRig finds slingable nodes that are on a different rig than the
// primary rig (most common rig among slingable nodes).
func detectCrossRig(dag *ConvoyDAG) []StagingFinding {
	// Count rigs among slingable nodes.
	rigCount := make(map[string]int)
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig == "" {
			continue
		}
		rigCount[node.Rig]++
	}

	if len(rigCount) <= 1 {
		return nil // all same rig or no rigs
	}

	// Find primary rig (most common; tie-break alphabetically for determinism).
	primaryRig := ""
	primaryCount := 0
	for rig, count := range rigCount {
		if count > primaryCount || (count == primaryCount && rig < primaryRig) {
			primaryRig = rig
			primaryCount = count
		}
	}

	var findings []StagingFinding
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig == "" || node.Rig == primaryRig {
			continue
		}
		findings = append(findings, StagingFinding{
			Severity:     "warning",
			Category:     "cross-rig",
			BeadIDs:      []string{node.ID},
			Message:      fmt.Sprintf("task %s is on rig %q (primary rig is %q)", node.ID, node.Rig, primaryRig),
			SuggestedFix: fmt.Sprintf("verify cross-rig routing for %s or reassign to %s", node.ID, primaryRig),
		})
	}
	return findings
}

// estimateCapacity checks each wave for task counts exceeding the threshold
// and emits an informational warning.
func estimateCapacity(dag *ConvoyDAG) []StagingFinding {
	waves, err := computeWaves(dag)
	if err != nil {
		return nil // no slingable tasks → nothing to warn about
	}

	var findings []StagingFinding
	for _, wave := range waves {
		if len(wave.Tasks) > waveCapacityThreshold {
			findings = append(findings, StagingFinding{
				Severity: "warning",
				Category: "capacity",
				BeadIDs:  wave.Tasks,
				Message:  fmt.Sprintf("wave %d has %d tasks (threshold: %d) — may exceed parallel capacity", wave.Number, len(wave.Tasks), waveCapacityThreshold),
			})
		}
	}
	return findings
}

// detectMissingBranches warns about sub-epics that have children but no
// integration branch metadata. This is a simple heuristic — real branch
// checking comes later.
func detectMissingBranches(dag *ConvoyDAG) []StagingFinding {
	var findings []StagingFinding
	for _, node := range dag.Nodes {
		if node.Type != "epic" {
			continue
		}
		// Skip root-level epics (no parent) — only warn for sub-epics.
		if node.Parent == "" {
			continue
		}
		if len(node.Children) > 0 {
			findings = append(findings, StagingFinding{
				Severity:     "warning",
				Category:     "missing-branch",
				BeadIDs:      []string{node.ID},
				Message:      fmt.Sprintf("sub-epic %s has %d children but no integration branch", node.ID, len(node.Children)),
				SuggestedFix: fmt.Sprintf("create an integration branch for sub-epic %s", node.ID),
			})
		}
	}
	return findings
}

// renderWarnings formats warning findings for console output.
func renderWarnings(findings []StagingFinding) string {
	if len(findings) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("Warnings:\n")
	for i, f := range findings {
		buf.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, f.Category, f.Message))
		if len(f.BeadIDs) > 0 {
			buf.WriteString(fmt.Sprintf("     Affected: %s\n", strings.Join(f.BeadIDs, ", ")))
		}
		if f.SuggestedFix != "" {
			buf.WriteString(fmt.Sprintf("     Fix: %s\n", f.SuggestedFix))
		}
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// JSON output helpers (gt-csl.4.3)
// ---------------------------------------------------------------------------

// renderJSON marshals a StageResult to indented JSON.
func renderJSON(result StageResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}
	return string(data) + "\n", nil
}

// buildFindingsJSON converts StagingFinding slices to FindingJSON slices.
func buildFindingsJSON(findings []StagingFinding) []FindingJSON {
	out := make([]FindingJSON, 0, len(findings))
	for _, f := range findings {
		beadIDs := f.BeadIDs
		if beadIDs == nil {
			beadIDs = []string{}
		}
		out = append(out, FindingJSON{
			Category:     f.Category,
			BeadIDs:      beadIDs,
			Message:      f.Message,
			SuggestedFix: f.SuggestedFix,
		})
	}
	return out
}

// buildWavesJSON converts Wave slices plus DAG into WaveJSON slices.
func buildWavesJSON(waves []Wave, dag *ConvoyDAG) []WaveJSON {
	out := make([]WaveJSON, 0, len(waves))
	for _, w := range waves {
		tasks := make([]TaskJSON, 0, len(w.Tasks))
		for _, taskID := range w.Tasks {
			node := dag.Nodes[taskID]
			if node == nil {
				continue
			}
			var blockedBy []string
			if len(node.BlockedBy) > 0 {
				blockedBy = make([]string, len(node.BlockedBy))
				copy(blockedBy, node.BlockedBy)
				sort.Strings(blockedBy)
			}
			tasks = append(tasks, TaskJSON{
				ID:        node.ID,
				Title:     node.Title,
				Type:      node.Type,
				Rig:       node.Rig,
				BlockedBy: blockedBy,
			})
		}
		out = append(out, WaveJSON{
			Number: w.Number,
			Tasks:  tasks,
		})
	}
	return out
}

// buildTreeJSON converts the DAG into a nested tree structure for JSON output.
// For epic input, recursively builds from the root epic.
// For task-list or convoy input, returns a flat list of nodes.
func buildTreeJSON(dag *ConvoyDAG, input *StageInput) []TreeNodeJSON {
	switch input.Kind {
	case StageInputEpic:
		if len(input.IDs) == 0 {
			return []TreeNodeJSON{}
		}
		rootID := input.IDs[0]
		root := dag.Nodes[rootID]
		if root == nil {
			return []TreeNodeJSON{}
		}
		rootNode := buildTreeNodeJSON(dag, root)
		return []TreeNodeJSON{rootNode}

	default:
		// Flat list for task-list and convoy input, sorted by ID.
		ids := sortedNodeIDs(dag)
		out := make([]TreeNodeJSON, 0, len(ids))
		for _, id := range ids {
			node := dag.Nodes[id]
			if node == nil {
				continue
			}
			out = append(out, TreeNodeJSON{
				ID:     node.ID,
				Title:  node.Title,
				Type:   node.Type,
				Status: node.Status,
				Rig:    node.Rig,
			})
		}
		return out
	}
}

// buildTreeNodeJSON recursively converts a single DAG node into TreeNodeJSON.
func buildTreeNodeJSON(dag *ConvoyDAG, node *ConvoyDAGNode) TreeNodeJSON {
	tn := TreeNodeJSON{
		ID:     node.ID,
		Title:  node.Title,
		Type:   node.Type,
		Status: node.Status,
		Rig:    node.Rig,
	}

	children := sortedChildren(dag, node.ID)
	if len(children) > 0 {
		tn.Children = make([]TreeNodeJSON, 0, len(children))
		for _, childID := range children {
			childNode := dag.Nodes[childID]
			if childNode == nil {
				continue
			}
			tn.Children = append(tn.Children, buildTreeNodeJSON(dag, childNode))
		}
	}

	return tn
}
