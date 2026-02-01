package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	epicpkg "github.com/steveyegge/gastown/internal/epic"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicSubmitRemote     string // --remote flag
	epicSubmitDryRun     bool   // --dry-run flag
	epicSubmitSequential bool   // --sequential flag
	epicSubmitSingle     bool   // --single flag: combine all into one PR
)

var epicSubmitCmd = &cobra.Command{
	Use:   "submit [epic-id]",
	Short: "Create stacked upstream PRs for epic",
	Long: `Create stacked upstream PRs for an epic's subtasks.

This command:
1. Groups subtasks by dependency
2. Creates PRs with correct base branches (stacking)
3. Root tasks target upstream/main
4. Dependent tasks target their dependency's PR branch
5. Records PR URLs in the epic

STACKED PR STRATEGY:

When subtasks have dependencies, PRs are created as a stack:

  Subtask dependencies:
    implement-api (no deps) → PR #101 → targets main
    add-tests (needs: implement-api) → PR #102 → targets PR #101's branch
    update-docs (needs: implement-api) → PR #103 → targets PR #101's branch

When #101 merges, GitHub auto-retargets #102 and #103 to main.

FLAGS:

  --remote <name>   Upstream remote name (default: origin)
  --dry-run         Preview without creating PRs
  --sequential      Wait for each PR to merge before creating next
  --single          Combine all subtasks into one PR

EXAMPLES:

  gt epic submit gt-epic-abc12
  gt epic submit gt-epic-abc12 --dry-run
  gt epic submit gt-epic-abc12 --remote upstream
  gt epic submit gt-epic-abc12 --single`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicSubmit,
}

func init() {
	epicSubmitCmd.Flags().StringVar(&epicSubmitRemote, "remote", "origin", "Upstream remote name")
	epicSubmitCmd.Flags().BoolVarP(&epicSubmitDryRun, "dry-run", "n", false, "Preview without creating PRs")
	epicSubmitCmd.Flags().BoolVar(&epicSubmitSequential, "sequential", false, "Wait for each PR to merge before next")
	epicSubmitCmd.Flags().BoolVar(&epicSubmitSingle, "single", false, "Combine all into one PR")
}

func runEpicSubmit(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	rigPath := filepath.Join(townRoot, rigName)
	beadsDir := filepath.Join(rigPath, "mayor", "rig")
	repoDir := filepath.Join(rigPath, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, fields, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Validate state
	if fields.EpicState != beads.EpicStateReview && fields.EpicState != beads.EpicStateInProgress {
		fmt.Printf("%s Epic is in '%s' state.\n", style.Warning.Render("⚠"), fields.EpicState)
		fmt.Println("Consider running 'gt epic review' first.")
	}

	// Get subtasks
	subtasks, err := bd.GetEpicSubtasks(epicID)
	if err != nil {
		return fmt.Errorf("getting subtasks: %w", err)
	}

	if len(subtasks) == 0 {
		return fmt.Errorf("epic has no subtasks")
	}

	// Build dependency graph
	graph := epicpkg.NewDependencyGraph()
	subtaskMap := make(map[string]*beads.Issue)

	for _, st := range subtasks {
		graph.AddNode(st.ID)
		subtaskMap[st.ID] = st
	}

	// Add edges from dependencies
	for _, st := range subtasks {
		for _, depID := range st.DependsOn {
			if _, ok := subtaskMap[depID]; ok {
				graph.AddEdge(st.ID, depID)
			}
		}
	}

	// Get topological order
	order, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("dependency cycle detected: %w", err)
	}

	// Get default branch
	defaultBranch := getDefaultBranch(repoDir)

	fmt.Printf("%s Submit Plan for: %s\n\n", style.Bold.Render("📤"), epic.Title)

	// Show what will be created
	roots := graph.GetRoots()
	fmt.Printf("%s Root tasks (will target %s/%s):\n", style.Bold.Render("→"), epicSubmitRemote, defaultBranch)
	for _, id := range roots {
		st := subtaskMap[id]
		fmt.Printf("  [x] %s: %s\n", id, st.Title)
	}

	// Show dependent tasks grouped by what they depend on
	dependentTasks := make(map[string][]string) // base -> dependents
	for _, id := range order {
		deps := graph.GetDependencies(id)
		if len(deps) > 0 {
			// Takes first dependency as base
			base := deps[0]
			dependentTasks[base] = append(dependentTasks[base], id)
		}
	}

	for base, dependents := range dependentTasks {
		baseSt := subtaskMap[base]
		fmt.Printf("\n%s Dependent tasks (will stack on %s):\n", style.Bold.Render("→"), baseSt.Title)
		for _, id := range dependents {
			st := subtaskMap[id]
			fmt.Printf("  [x] %s: %s\n", id, st.Title)
		}
	}

	if epicSubmitDryRun {
		fmt.Printf("\n%s Dry run - no PRs created\n", style.Dim.Render("○"))
		fmt.Println("\nWould create stacked PRs:")
		prNum := 100                          // Mock PR number
		prBranches := make(map[string]string) // subtaskID -> branch name
		prNumbers := make(map[string]int)     // subtaskID -> PR number

		for _, id := range order {
			st := subtaskMap[id]
			branch := epicpkg.FormatPRBranchName(epicID, extractStepRef(st))
			prBranches[id] = branch

			deps := graph.GetDependencies(id)
			var base string
			if len(deps) > 0 {
				baseBranch := prBranches[deps[0]]
				base = fmt.Sprintf("PR #%d's branch (%s)", prNumbers[deps[0]], baseBranch)
			} else {
				base = fmt.Sprintf("%s/%s", epicSubmitRemote, defaultBranch)
			}

			prNum++
			prNumbers[id] = prNum
			fmt.Printf("  PR #%d: %s → %s\n", prNum, st.Title, base)
		}
		return nil
	}

	// Confirm
	fmt.Println()
	if epicSubmitSingle {
		fmt.Printf("Create single PR with all changes? [y/N] ")
	} else {
		fmt.Printf("Create %d stacked PRs? [y/N] ", len(order))
	}

	var response string
	if _, err := fmt.Scanln(&response); err != nil && err != io.EOF {
		return fmt.Errorf("reading response: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	// Create PRs
	fmt.Printf("\n%s Creating PRs...\n", style.Bold.Render("→"))

	var createdPRs []string
	prBranches := make(map[string]string)
	prNumbers := make(map[string]int)

	if epicSubmitSingle {
		// Single PR mode
		prURL, err := createSinglePR(repoDir, epic, subtasks, defaultBranch)
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}
		createdPRs = append(createdPRs, prURL)
		fmt.Printf("  %s %s\n", style.Bold.Render("✓"), prURL)
	} else {
		// Stacked PR mode
		for _, id := range order {
			st := subtaskMap[id]
			stepRef := extractStepRef(st)
			branch := epicpkg.FormatPRBranchName(epicID, stepRef)
			prBranches[id] = branch

			// Determine base
			var baseBranch string
			deps := graph.GetDependencies(id)
			if len(deps) > 0 {
				baseBranch = prBranches[deps[0]]
			} else {
				baseBranch = defaultBranch
			}

			// Create PR
			prURL, prNum, err := createStackedPR(repoDir, st, branch, baseBranch)
			if err != nil {
				fmt.Printf("  %s Failed to create PR for %s: %v\n",
					style.Warning.Render("⚠"), st.Title, err)
				continue
			}

			prNumbers[id] = prNum
			createdPRs = append(createdPRs, prURL)
			fmt.Printf("  %s PR #%d: %s\n", style.Bold.Render("✓"), prNum, st.Title)
		}
	}

	// Update epic with PR URLs
	if len(createdPRs) > 0 {
		fields.UpstreamPRs = epicpkg.FormatUpstreamPRs(createdPRs)
		fields.EpicState = beads.EpicStateSubmitted
		if err := bd.UpdateEpicFields(epicID, fields); err != nil {
			fmt.Printf("%s Could not update epic: %v\n", style.Warning.Render("⚠"), err)
		}
	}

	fmt.Printf("\n%s Created %d PR(s)\n", style.Bold.Render("✓"), len(createdPRs))
	for _, url := range createdPRs {
		fmt.Printf("  %s\n", url)
	}

	fmt.Println()
	fmt.Println("Track PR status: gt epic pr status", epicID)

	return nil
}

// extractStepRef extracts the step ref from a subtask's description.
func extractStepRef(st *beads.Issue) string {
	// Look for "step: <ref>" in description
	for _, line := range strings.Split(st.Description, "\n") {
		line = strings.TrimSpace(line)
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "step:") {
			// Extract the part after "step:" (case-insensitive)
			return strings.TrimSpace(line[5:]) // len("step:") == 5
		}
	}
	// Fall back to sanitized title
	return sanitizeForBranch(st.Title)
}

// sanitizeForBranch converts a string to a valid branch name.
func sanitizeForBranch(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	// Remove invalid characters
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// getDefaultBranch gets the default branch for a repo.
func getDefaultBranch(repoDir string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}

	// Output is like "refs/remotes/origin/main"
	ref := strings.TrimSpace(string(out))
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// createSinglePR creates a single PR with all epic changes.
func createSinglePR(repoDir string, epic *beads.Issue, subtasks []*beads.Issue, baseBranch string) (string, error) {
	// Build PR body
	var body strings.Builder
	writeBody := func(s string) error {
		if _, err := body.WriteString(s); err != nil {
			return fmt.Errorf("building PR body: %w", err)
		}
		return nil
	}
	if err := writeBody("## Summary\n\n"); err != nil {
		return "", err
	}
	if err := writeBody(epic.Title); err != nil {
		return "", err
	}
	if err := writeBody("\n\n## Changes\n\n"); err != nil {
		return "", err
	}
	for _, st := range subtasks {
		if err := writeBody(fmt.Sprintf("- %s\n", st.Title)); err != nil {
			return "", err
		}
	}
	if err := writeBody("\n\n---\n\n"); err != nil {
		return "", err
	}
	if err := writeBody("Generated by [gt epic](https://github.com/steveyegge/gastown)\n"); err != nil {
		return "", err
	}

	// Create PR using gh
	cmd := exec.Command("gh", "pr", "create",
		"--title", epic.Title,
		"--body", body.String(),
		"--base", baseBranch,
	)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// createStackedPR creates a PR for a single subtask.
func createStackedPR(repoDir string, subtask *beads.Issue, branch, baseBranch string) (string, int, error) {
	// Build PR body
	var body strings.Builder
	writeBody := func(s string) error {
		if _, err := body.WriteString(s); err != nil {
			return fmt.Errorf("building PR body: %w", err)
		}
		return nil
	}
	if err := writeBody("## Summary\n\n"); err != nil {
		return "", 0, err
	}
	if err := writeBody(subtask.Title); err != nil {
		return "", 0, err
	}
	if subtask.Description != "" {
		if err := writeBody("\n\n## Details\n\n"); err != nil {
			return "", 0, err
		}
		// Include only non-metadata lines
		for _, line := range strings.Split(subtask.Description, "\n") {
			if !strings.Contains(line, ":") || strings.HasPrefix(line, "#") {
				if err := writeBody(line); err != nil {
					return "", 0, err
				}
				if err := writeBody("\n"); err != nil {
					return "", 0, err
				}
			}
		}
	}
	if err := writeBody("\n\n---\n\n"); err != nil {
		return "", 0, err
	}
	if err := writeBody("Generated by [gt epic](https://github.com/steveyegge/gastown)\n"); err != nil {
		return "", 0, err
	}

	// Create PR using gh
	cmd := exec.Command("gh", "pr", "create",
		"--title", subtask.Title,
		"--body", body.String(),
		"--base", baseBranch,
		"--head", branch,
	)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("%w: %s", err, stderr.String())
	}

	prURL := strings.TrimSpace(stdout.String())

	// Get PR number from URL
	_, _, prNum, err := epicpkg.ParsePRURL(prURL)
	if err != nil {
		prNum = 0
	}

	return prURL, prNum, nil
}

// epicPlanCmd shows the current plan for an epic.
var epicPlanCmd = &cobra.Command{
	Use:   "plan [epic-id]",
	Short: "Show current plan for an epic",
	Long: `Display the plan content from an epic's description.

This is useful for reviewing the plan before marking it ready,
or for verifying what was planned after subtasks are created.

EXAMPLES:

  gt epic plan gt-epic-abc12
  gt epic plan                    # Uses hooked epic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicPlan,
}

func runEpicPlan(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, fields, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	fmt.Printf("%s Epic Plan: %s\n\n", style.Bold.Render("📋"), epic.Title)
	fmt.Printf("  State: %s\n\n", fields.EpicState)

	planContent := beads.ExtractPlanContent(epic.Description)
	if planContent == "" {
		fmt.Println("  (No plan content yet)")
		fmt.Println()
		fmt.Println("Add plan content using step format:")
		fmt.Println("  bd update", epicID, "--description \"...plan...\"")
	} else {
		fmt.Println(planContent)
	}

	return nil
}

// epicStatusCmd shows status for an epic.
var epicStatusCmd = &cobra.Command{
	Use:   "status [epic-id]",
	Short: "Show epic progress",
	Long: `Display detailed status for an epic including subtask progress.

EXAMPLES:

  gt epic status gt-epic-abc12
  gt epic status                  # Uses hooked epic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicStatus,
}

func runEpicStatus(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, fields, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Get subtasks
	subtasks, err := bd.GetEpicSubtasks(epicID)
	if err != nil {
		return fmt.Errorf("getting subtasks: %w", err)
	}

	// Count by status
	statusCounts := make(map[string]int)
	for _, st := range subtasks {
		statusCounts[st.Status]++
	}

	fmt.Printf("%s Epic: %s\n\n", style.Bold.Render("📊"), epic.Title)
	fmt.Printf("  ID: %s\n", epicID)
	fmt.Printf("  State: %s\n", fields.EpicState)

	if fields.IntegrationBr != "" {
		fmt.Printf("  Integration Branch: %s\n", fields.IntegrationBr)
	}

	if fields.ContributingMD != "" {
		fmt.Printf("  CONTRIBUTING.md: %s\n", fields.ContributingMD)
	}

	// Progress bar
	total := len(subtasks)
	completed := statusCounts["closed"]
	if total > 0 {
		pct := completed * 100 / total
		bar := strings.Repeat("█", pct/5) + strings.Repeat("░", 20-pct/5)
		fmt.Printf("\n  Progress: [%s] %d%% (%d/%d)\n", bar, pct, completed, total)
	}

	// Status breakdown
	fmt.Printf("\n%s\n", style.Bold.Render("Subtasks:"))
	for _, st := range subtasks {
		status := "○"
		switch st.Status {
		case "closed":
			status = "✓"
		case "in_progress", "hooked":
			status = "▶"
		}

		assignee := ""
		if st.Assignee != "" {
			parts := strings.Split(st.Assignee, "/")
			assignee = fmt.Sprintf(" [@%s]", parts[len(parts)-1])
		}

		fmt.Printf("  %s %s: %s%s\n", status, st.ID, st.Title, assignee)
	}

	// Upstream PRs
	if fields.UpstreamPRs != "" {
		prs := epicpkg.ParseUpstreamPRs(fields.UpstreamPRs)
		fmt.Printf("\n%s\n", style.Bold.Render("Upstream PRs:"))
		for _, pr := range prs {
			fmt.Printf("  %s\n", pr)
		}
	}

	// Next actions
	fmt.Println()
	switch fields.EpicState {
	case beads.EpicStateDrafting:
		fmt.Println("Next: gt epic ready", epicID)
	case beads.EpicStateReady:
		fmt.Println("Next: gt epic sling", epicID)
	case beads.EpicStateInProgress:
		if completed == total {
			fmt.Println("Next: gt epic review", epicID)
		} else {
			fmt.Println("Waiting for subtasks to complete...")
		}
	case beads.EpicStateReview:
		fmt.Println("Next: gt epic submit", epicID)
	case beads.EpicStateSubmitted:
		fmt.Println("Track PRs: gt epic pr status", epicID)
	}

	return nil
}

// epicListCmd lists epics by state.
var epicListCmd = &cobra.Command{
	Use:   "list",
	Short: "List epics by state",
	Long: `List all epics, optionally filtered by state.

EXAMPLES:

  gt epic list                    # All epics
  gt epic list --state=drafting   # Only drafting epics
  gt epic list --json`,
	RunE: runEpicList,
}

var (
	epicListState string
	epicListJSON  bool
)

func init() {
	epicListCmd.Flags().StringVar(&epicListState, "state", "", "Filter by state (drafting, ready, in_progress, review, submitted)")
	epicListCmd.Flags().BoolVar(&epicListJSON, "json", false, "Output as JSON")
}

func runEpicList(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Find all rigs
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return err
	}

	var allEpics []*beads.Issue
	epicFieldsMap := make(map[string]*beads.EpicFields)

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		beadsDir := filepath.Join(townRoot, entry.Name(), "mayor", "rig")
		if _, err := os.Stat(filepath.Join(beadsDir, ".beads")); os.IsNotExist(err) {
			continue
		}

		bd := beads.New(beadsDir)
		epics, err := bd.ListEpicBeads()
		if err != nil {
			continue
		}

		for _, epic := range epics {
			fields := beads.ParseEpicFields(epic.Description)

			// Filter by state if specified
			if epicListState != "" && string(fields.EpicState) != epicListState {
				continue
			}

			allEpics = append(allEpics, epic)
			epicFieldsMap[epic.ID] = fields
		}
	}

	if epicListJSON {
		type epicInfo struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			State string `json:"state"`
		}
		var output []epicInfo
		for _, e := range allEpics {
			output = append(output, epicInfo{
				ID:    e.ID,
				Title: e.Title,
				State: string(epicFieldsMap[e.ID].EpicState),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	if len(allEpics) == 0 {
		fmt.Println("No epics found.")
		fmt.Println("Create an epic with: gt epic start <rig> \"Title\"")
		return nil
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Epics"))

	for _, epic := range allEpics {
		fields := epicFieldsMap[epic.ID]
		stateIcon := getStateIcon(fields.EpicState)
		fmt.Printf("  %s %s: %s [%s]\n", stateIcon, epic.ID, epic.Title, fields.EpicState)
	}

	return nil
}

func getStateIcon(state beads.EpicState) string {
	switch state {
	case beads.EpicStateDrafting:
		return "📝"
	case beads.EpicStateReady:
		return "✅"
	case beads.EpicStateInProgress:
		return "🔄"
	case beads.EpicStateReview:
		return "👀"
	case beads.EpicStateSubmitted:
		return "📤"
	case beads.EpicStateLanded:
		return "🎉"
	case beads.EpicStateClosed:
		return "❌"
	default:
		return "○"
	}
}

// epicCloseCmd closes an epic.
var epicCloseCmd = &cobra.Command{
	Use:   "close [epic-id]",
	Short: "Close or abandon an epic",
	Long: `Close an epic and optionally its subtasks.

EXAMPLES:

  gt epic close gt-epic-abc12
  gt epic close gt-epic-abc12 --reason "Abandoned"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicClose,
}

var epicCloseReason string

func init() {
	epicCloseCmd.Flags().StringVar(&epicCloseReason, "reason", "", "Reason for closing")
}

func runEpicClose(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, _, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Close subtasks
	subtasks, _ := bd.GetEpicSubtasks(epicID)
	closedCount := 0
	for _, st := range subtasks {
		if st.Status != "closed" {
			if err := bd.Close(st.ID); err == nil {
				closedCount++
			}
		}
	}

	// Close epic
	reason := epicCloseReason
	if reason == "" {
		reason = "Closed via gt epic close"
	}

	closeArgs := []string{"close", epicID, "-r", reason}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = beadsDir
	if err := closeCmd.Run(); err != nil {
		return fmt.Errorf("closing epic: %w", err)
	}

	fmt.Printf("%s Closed epic: %s\n", style.Bold.Render("✓"), epic.Title)
	if closedCount > 0 {
		fmt.Printf("  Also closed %d subtask(s)\n", closedCount)
	}

	return nil
}
