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
	epicSlingWorkers string // --workers flag: "crew" or "polecats"
	epicSlingDryRun  bool   // --dry-run flag
)

var epicSlingCmd = &cobra.Command{
	Use:   "sling [epic-id]",
	Short: "Dispatch epic subtasks to workers",
	Long: `Dispatch epic subtasks to workers for execution.

This command:
1. Gets all ready subtasks (no blockers) for the epic
2. Dispatches each to a worker based on --workers flag
3. Workers branch from integration/<epic>
4. Workers submit via gt mq submit (auto-targets integration branch)

WORKER TYPES:

  --workers=crew      Round-robin assign to crew members (default if crew exist)
  --workers=polecats  Spawn fresh polecat per task

If the rig has crew members, crew is used by default. Otherwise, polecats.

EXAMPLES:

  gt epic sling gt-epic-abc12
  gt epic sling gt-epic-abc12 --workers=crew
  gt epic sling gt-epic-abc12 --workers=polecats
  gt epic sling                          # Uses hooked epic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicSling,
}

func init() {
	epicSlingCmd.Flags().StringVar(&epicSlingWorkers, "workers", "", "Worker type: crew or polecats")
	epicSlingCmd.Flags().BoolVarP(&epicSlingDryRun, "dry-run", "n", false, "Show what would be done")
}

func runEpicSling(cmd *cobra.Command, args []string) error {
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

	// Validate state
	if fields.EpicState != beads.EpicStateReady && fields.EpicState != beads.EpicStateInProgress {
		return fmt.Errorf("epic is in '%s' state, expected 'ready' or 'in_progress'", fields.EpicState)
	}

	// Get subtasks
	subtasks, err := bd.GetEpicSubtasks(epicID)
	if err != nil {
		return fmt.Errorf("getting subtasks: %w", err)
	}

	if len(subtasks) == 0 {
		return fmt.Errorf("epic has no subtasks")
	}

	// Filter to ready subtasks (not blocked, not closed, not in_progress)
	var readySubtasks []*beads.Issue
	for _, st := range subtasks {
		if st.Status == "closed" || st.Status == "in_progress" || st.Status == "hooked" {
			continue
		}

		// Check if blocked
		blocked := isBeadBlocked(st.ID)
		if !blocked {
			readySubtasks = append(readySubtasks, st)
		}
	}

	if len(readySubtasks) == 0 {
		fmt.Printf("%s No ready subtasks to dispatch.\n", style.Dim.Render("○"))
		fmt.Println("Subtasks are either blocked, in progress, or completed.")
		return nil
	}

	fmt.Printf("%s Found %d ready subtask(s) to dispatch:\n", style.Bold.Render("📋"), len(readySubtasks))
	for _, st := range readySubtasks {
		fmt.Printf("  - %s: %s\n", st.ID, st.Title)
	}

	// Determine worker type
	workerType := epicSlingWorkers
	if workerType == "" {
		workerType = detectWorkerType(rigPath)
	}

	fmt.Printf("\n%s Worker type: %s\n", style.Bold.Render("→"), workerType)

	if epicSlingDryRun {
		fmt.Printf("\n%s Dry run - no changes made\n", style.Dim.Render("○"))
		fmt.Println("Would dispatch:")
		for _, st := range readySubtasks {
			if workerType == "crew" {
				fmt.Printf("  gt sling %s %s/crew/<member>\n", st.ID, rigName)
			} else {
				fmt.Printf("  gt sling %s %s\n", st.ID, rigName)
			}
		}
		return nil
	}

	// Dispatch subtasks
	var crewMembers []string
	if workerType == "crew" {
		crewMembers = listCrewMembers(rigPath)
		if len(crewMembers) == 0 {
			fmt.Printf("%s No crew members found, falling back to polecats\n",
				style.Warning.Render("⚠"))
			workerType = "polecats"
		}
	}

	fmt.Printf("\n%s Dispatching...\n", style.Bold.Render("→"))

	dispatchedCount := 0
	for i, st := range readySubtasks {
		var target string
		if workerType == "crew" {
			// Round-robin crew assignment
			crewMember := crewMembers[i%len(crewMembers)]
			target = fmt.Sprintf("%s/crew/%s", rigName, crewMember)
		} else {
			// Polecat - just use rig name
			target = rigName
		}

		fmt.Printf("  %s %s → %s\n", style.Bold.Render("→"), st.ID, target)

		slingArgs := []string{"sling", st.ID, target}
		slingCmd := exec.Command("gt", slingArgs...)
		slingCmd.Stdout = os.Stdout
		slingCmd.Stderr = os.Stderr

		if err := slingCmd.Run(); err != nil {
			fmt.Printf("  %s Failed to dispatch %s: %v\n",
				style.Warning.Render("⚠"), st.ID, err)
			continue
		}

		dispatchedCount++
	}

	// Update epic state to in_progress
	if fields.EpicState == beads.EpicStateReady {
		fields.EpicState = beads.EpicStateInProgress
		if err := bd.UpdateEpicFields(epicID, fields); err != nil {
			fmt.Printf("%s Could not update epic state: %v\n",
				style.Warning.Render("⚠"), err)
		}
	}

	fmt.Printf("\n%s Dispatched %d/%d subtasks\n",
		style.Bold.Render("✓"), dispatchedCount, len(readySubtasks))

	remaining := len(subtasks) - dispatchedCount
	for _, st := range subtasks {
		if st.Status == "closed" {
			remaining--
		}
	}
	if remaining > 0 {
		fmt.Printf("  %d subtask(s) still pending (blocked or already in progress)\n", remaining)
	}

	fmt.Println()
	fmt.Printf("Check progress: gt epic status %s\n", epicID)

	return nil
}

// detectWorkerType detects whether to use crew or polecats.
func detectWorkerType(rigPath string) string {
	crewPath := filepath.Join(rigPath, "crew")
	if _, err := os.Stat(crewPath); os.IsNotExist(err) {
		return "polecats"
	}

	entries, err := os.ReadDir(crewPath)
	if err != nil {
		return "polecats"
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return "crew"
		}
	}

	return "polecats"
}

// listCrewMembers returns all crew member names in a rig.
func listCrewMembers(rigPath string) []string {
	crewPath := filepath.Join(rigPath, "crew")
	entries, err := os.ReadDir(crewPath)
	if err != nil {
		return nil
	}

	var members []string
	for _, entry := range entries {
		if entry.IsDir() {
			members = append(members, entry.Name())
		}
	}
	return members
}

// isBeadBlocked checks if a bead is blocked by dependencies.
func isBeadBlocked(beadID string) bool {
	// Run bd blocked and check if this bead is in the output
	blockedCmd := exec.Command("bd", "blocked", "--json")
	out, err := blockedCmd.Output()
	if err != nil {
		return false // Assume not blocked on error
	}

	return strings.Contains(string(out), beadID)
}
