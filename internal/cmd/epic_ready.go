package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicReadyDryRun bool // --dry-run flag
)

var epicReadyCmd = &cobra.Command{
	Use:   "ready [epic-id]",
	Short: "Parse plan and create subtasks for an epic",
	Long: `Parse the epic plan and create subtask beads.

This command:
1. Parses the plan from the epic description
2. Creates subtask beads for each step
3. Wires dependencies based on Needs: declarations
4. Creates an integration branch for the epic
5. Creates a convoy to track the subtasks
6. Transitions epic to "ready" state

The plan format uses molecule step syntax:

  ## Step: implement-api
  Implement the core API changes
  Tier: opus

  ## Step: add-tests
  Write comprehensive tests
  Needs: implement-api
  Tier: sonnet

If no epic-id is provided, uses the currently hooked bead if it's an epic.

EXAMPLES:

  gt epic ready gt-epic-abc12
  gt epic ready                   # Uses hooked epic
  gt epic ready gt-epic-abc12 --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicReady,
}

func init() {
	epicReadyCmd.Flags().BoolVarP(&epicReadyDryRun, "dry-run", "n", false, "Show what would be done without making changes")
}

func runEpicReady(cmd *cobra.Command, args []string) error {
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
		// Try to get from hooked bead
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID prefix
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig from epic ID: %w", err)
	}

	rigPath := filepath.Join(townRoot, rigName)
	beadsDir := filepath.Join(rigPath, "mayor", "rig")

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
	if fields.EpicState != beads.EpicStateDrafting {
		return fmt.Errorf("epic is in '%s' state, expected 'drafting'", fields.EpicState)
	}

	// Parse plan from description
	planContent := beads.ExtractPlanContent(epic.Description)
	if planContent == "" {
		return fmt.Errorf("epic has no plan content")
	}

	steps, err := beads.ParseMoleculeSteps(planContent)
	if err != nil {
		return fmt.Errorf("parsing plan: %w", err)
	}

	if len(steps) == 0 {
		return fmt.Errorf("no steps found in plan")
	}

	fmt.Printf("%s Parsed %d step(s) from plan:\n", style.Bold.Render("📋"), len(steps))
	for _, step := range steps {
		deps := ""
		if len(step.Needs) > 0 {
			deps = fmt.Sprintf(" (needs: %s)", strings.Join(step.Needs, ", "))
		}
		tier := ""
		if step.Tier != "" {
			tier = fmt.Sprintf(" [%s]", step.Tier)
		}
		fmt.Printf("  - %s: %s%s%s\n", step.Ref, step.Title, tier, deps)
	}

	if epicReadyDryRun {
		fmt.Printf("\n%s Dry run - no changes made\n", style.Dim.Render("○"))
		fmt.Println("Would:")
		fmt.Printf("  1. Create %d subtask beads as children of %s\n", len(steps), epicID)
		fmt.Printf("  2. Wire dependencies based on Needs: declarations\n")
		fmt.Printf("  3. Create integration branch: integration/%s\n", epicID)
		fmt.Printf("  4. Create convoy to track subtasks\n")
		fmt.Printf("  5. Transition epic to 'ready' state\n")
		return nil
	}

	// Create subtask beads
	fmt.Printf("\n%s Creating subtasks...\n", style.Bold.Render("→"))

	subtaskIDs := make(map[string]string) // step.Ref -> bead ID
	var createdIssues []*beads.Issue

	for _, step := range steps {
		// Build description with metadata
		description := step.Instructions
		if step.Tier != "" {
			description += fmt.Sprintf("\n\ntier: %s", step.Tier)
		}
		description += fmt.Sprintf("\ninstantiated_from: %s\nstep: %s", epicID, step.Ref)

		// Create subtask bead
		createOpts := beads.CreateOptions{
			Title:       step.Title,
			Type:        "task",
			Description: description,
			Parent:      epicID,
		}

		subtask, err := bd.Create(createOpts)
		if err != nil {
			// Cleanup created issues on failure
			for _, created := range createdIssues {
				_ = bd.Close(created.ID)
			}
			return fmt.Errorf("creating subtask '%s': %w", step.Ref, err)
		}

		createdIssues = append(createdIssues, subtask)
		subtaskIDs[step.Ref] = subtask.ID
		fmt.Printf("  %s %s: %s\n", style.Bold.Render("✓"), subtask.ID, step.Title)
	}

	// Wire dependencies
	fmt.Printf("\n%s Wiring dependencies...\n", style.Bold.Render("→"))

	for _, step := range steps {
		if len(step.Needs) == 0 {
			continue
		}

		subtaskID := subtaskIDs[step.Ref]
		for _, need := range step.Needs {
			depID, ok := subtaskIDs[need]
			if !ok {
				fmt.Printf("  %s Unknown dependency '%s' for step '%s'\n",
					style.Warning.Render("⚠"), need, step.Ref)
				continue
			}

			if err := bd.AddDependency(subtaskID, depID); err != nil {
				fmt.Printf("  %s Could not add dependency %s -> %s: %v\n",
					style.Warning.Render("⚠"), subtaskID, depID, err)
			} else {
				fmt.Printf("  %s %s needs %s\n", style.Bold.Render("✓"), step.Ref, need)
			}
		}
	}

	// Create integration branch
	integrationBranch := fmt.Sprintf("integration/%s", epicID)
	fmt.Printf("\n%s Creating integration branch: %s\n", style.Bold.Render("→"), integrationBranch)

	mqCmd := exec.Command("gt", "mq", "integration", "create", epicID)
	mqCmd.Dir = beadsDir
	mqCmd.Stdout = os.Stdout
	mqCmd.Stderr = os.Stderr

	if err := mqCmd.Run(); err != nil {
		fmt.Printf("  %s Could not create integration branch: %v\n",
			style.Warning.Render("⚠"), err)
		fmt.Println("  You can create it manually: gt mq integration create", epicID)
	} else {
		fields.IntegrationBr = integrationBranch
	}

	// Create convoy to track subtasks
	fmt.Printf("\n%s Creating convoy...\n", style.Bold.Render("→"))

	var subtaskIDList []string
	for _, id := range subtaskIDs {
		subtaskIDList = append(subtaskIDList, id)
	}

	convoyArgs := append([]string{"convoy", "create", epic.Title}, subtaskIDList...)
	convoyCmd := exec.Command("gt", convoyArgs...)
	convoyCmd.Stdout = os.Stdout
	convoyCmd.Stderr = os.Stderr

	if err := convoyCmd.Run(); err != nil {
		fmt.Printf("  %s Could not create convoy: %v\n",
			style.Warning.Render("⚠"), err)
	}

	// Update epic state and fields
	fields.EpicState = beads.EpicStateReady
	fields.SubtaskCount = len(steps)
	fields.CompletedCount = 0

	if err := bd.UpdateEpicFields(epicID, fields); err != nil {
		return fmt.Errorf("updating epic state: %w", err)
	}

	fmt.Printf("\n%s Epic %s is ready!\n", style.Bold.Render("✓"), epicID)
	fmt.Printf("  Subtasks: %d\n", len(steps))
	fmt.Printf("  State: %s\n", fields.EpicState)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  gt epic sling %s           # Dispatch subtasks to workers\n", epicID)
	fmt.Printf("  gt epic status %s          # Check progress\n", epicID)

	return nil
}

// getHookedEpicID gets the epic ID from the currently hooked bead.
func getHookedEpicID() (string, error) {
	// Run gt hook to get hooked bead
	hookCmd := exec.Command("gt", "hook", "--json")
	out, err := hookCmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting hooked bead: %w", err)
	}

	// Parse the output to find epic ID
	outStr := strings.TrimSpace(string(out))
	if outStr == "" || outStr == "null" || outStr == "[]" {
		return "", fmt.Errorf("no bead hooked")
	}

	// Simple parse - look for ID that contains "epic"
	if strings.Contains(outStr, "-epic-") {
		// Extract the ID
		for _, part := range strings.Fields(outStr) {
			if strings.Contains(part, "-epic-") {
				// Clean up any JSON formatting
				part = strings.Trim(part, `"`,)
				part = strings.TrimSuffix(part, ",")
				return part, nil
			}
		}
	}

	return "", fmt.Errorf("hooked bead is not an epic")
}

// getRigFromBeadID extracts the rig name from a bead ID prefix.
func getRigFromBeadID(beadID string) (string, error) {
	// Map prefixes to rig names
	prefixToRig := map[string]string{
		"gt": "gastown",
		"bd": "beads",
		"mi": "missioncontrol",
		"gp": "greenplace",
	}

	// Get prefix (everything before first hyphen)
	parts := strings.SplitN(beadID, "-", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid bead ID format")
	}

	prefix := parts[0]
	if rig, ok := prefixToRig[prefix]; ok {
		return rig, nil
	}

	// Try to find from routes.jsonl or fall back to workspace detection
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", err
	}

	// List rigs and try to match
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rigName := entry.Name()
		if strings.HasPrefix(rigName, ".") {
			continue
		}
		// Check if this rig has a beads directory
		beadsPath := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
		if _, err := os.Stat(beadsPath); err == nil {
			// This is a rig - check if the prefix matches
			if strings.HasPrefix(strings.ToLower(rigName), prefix) {
				return rigName, nil
			}
		}
	}

	return "", fmt.Errorf("could not find rig for prefix '%s'", prefix)
}
