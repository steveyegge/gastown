package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var epicReviewCmd = &cobra.Command{
	Use:   "review [epic-id]",
	Short: "Review completed epic work before upstream submission",
	Long: `Review completed work for an epic before creating upstream PRs.

This command:
1. Verifies all subtasks are merged to integration branch
2. Shows summary of changes
3. Checks compliance with CONTRIBUTING.md guidelines
4. Transitions epic to "review" state

Use this after all MRs have been merged to the integration branch,
but before creating upstream PRs.

EXAMPLES:

  gt epic review gt-epic-abc12
  gt epic review                  # Uses hooked epic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicReview,
}

func init() {
	// No flags for now
}

func runEpicReview(cmd *cobra.Command, args []string) error {
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

	// Count completed vs total
	completed := 0
	inProgress := 0
	for _, st := range subtasks {
		switch st.Status {
		case "closed":
			completed++
		case "in_progress", "hooked":
			inProgress++
		}
	}

	fmt.Printf("%s Epic Review: %s\n\n", style.Bold.Render("📝"), epic.Title)
	fmt.Printf("  ID: %s\n", epicID)
	fmt.Printf("  State: %s\n", fields.EpicState)
	fmt.Printf("  Subtasks: %d completed, %d in progress, %d total\n",
		completed, inProgress, len(subtasks))

	if fields.IntegrationBr != "" {
		fmt.Printf("  Integration Branch: %s\n", fields.IntegrationBr)
	}

	fmt.Println()

	// Show subtask status
	fmt.Printf("%s\n", style.Bold.Render("Subtasks:"))
	for _, st := range subtasks {
		status := "○"
		switch st.Status {
		case "closed":
			status = "✓"
		case "in_progress", "hooked":
			status = "▶"
		}
		fmt.Printf("  %s %s: %s\n", status, st.ID, st.Title)
	}

	// Check if all subtasks are complete
	if completed < len(subtasks) {
		fmt.Println()
		fmt.Printf("%s Not all subtasks are complete.\n", style.Warning.Render("⚠"))
		fmt.Printf("  Complete: %d/%d\n", completed, len(subtasks))
		fmt.Println()
		fmt.Println("Complete remaining subtasks before review:")
		for _, st := range subtasks {
			if st.Status != "closed" {
				fmt.Printf("  - %s: %s (%s)\n", st.ID, st.Title, st.Status)
			}
		}
		return nil
	}

	// Show CONTRIBUTING.md compliance check
	if fields.ContributingMD != "" {
		fmt.Println()
		fmt.Printf("%s CONTRIBUTING.md Guidelines:\n", style.Bold.Render("📋"))
		fmt.Printf("  File: %s\n", fields.ContributingMD)
		fmt.Println("  Review the integration branch against these guidelines.")
	}

	// Transition to review state
	if fields.EpicState != beads.EpicStateReview {
		fields.EpicState = beads.EpicStateReview
		fields.CompletedCount = completed
		if err := bd.UpdateEpicFields(epicID, fields); err != nil {
			return fmt.Errorf("updating epic state: %w", err)
		}
		fmt.Println()
		fmt.Printf("%s Epic transitioned to 'review' state\n", style.Bold.Render("✓"))
	}

	fmt.Println()
	fmt.Println("Review checklist:")
	fmt.Println("  [ ] Changes follow CONTRIBUTING.md guidelines")
	fmt.Println("  [ ] Tests pass")
	fmt.Println("  [ ] Documentation updated if needed")
	fmt.Println("  [ ] Commits are clean and well-organized")
	fmt.Println()
	fmt.Printf("When ready: gt epic submit %s\n", epicID)

	return nil
}
