package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

var patrolReportSummary string

var patrolReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Close patrol cycle with summary and start next cycle",
	Long: `Close the current patrol cycle, recording a summary of observations,
then automatically start a new patrol cycle.

This replaces the old squash+new pattern with a single command that:
  1. Closes the current patrol root wisp with the summary
  2. Creates a new patrol wisp for the next cycle

The summary is stored on the patrol root wisp for audit purposes.

Examples:
  gt patrol report --summary "All clear, no issues"
  gt patrol report --summary "Dolt latency elevated, filed escalation"`,
	RunE: runPatrolReport,
}

func init() {
	patrolReportCmd.Flags().StringVar(&patrolReportSummary, "summary", "", "Brief summary of patrol observations (required)")
	_ = patrolReportCmd.MarkFlagRequired("summary")
}

func runPatrolReport(cmd *cobra.Command, args []string) error {
	// Resolve role
	roleInfo, err := GetRole()
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	roleName := string(roleInfo.Role)

	// Build config based on role
	var cfg PatrolConfig
	switch roleInfo.Role {
	case RoleDeacon:
		cfg = PatrolConfig{
			RoleName:      "deacon",
			PatrolMolName: "mol-deacon-patrol",
			BeadsDir:      roleInfo.TownRoot,
			Assignee:      "deacon",
		}
	case RoleWitness:
		cfg = PatrolConfig{
			RoleName:      "witness",
			PatrolMolName: "mol-witness-patrol",
			BeadsDir:      roleInfo.WorkDir,
			Assignee:      roleInfo.Rig + "/witness",
		}
	case RoleRefinery:
		cfg = PatrolConfig{
			RoleName:      "refinery",
			PatrolMolName: "mol-refinery-patrol",
			BeadsDir:      roleInfo.WorkDir,
			Assignee:      roleInfo.Rig + "/refinery",
			ExtraVars:     buildRefineryPatrolVars(roleInfo),
		}
	default:
		return fmt.Errorf("unsupported role for patrol report: %q", roleName)
	}

	// Find the active patrol
	patrolID, _, hasPatrol, findErr := findActivePatrol(cfg)
	if findErr != nil {
		return fmt.Errorf("finding active patrol: %w", findErr)
	}
	if !hasPatrol {
		return fmt.Errorf("no active patrol found for %s", cfg.RoleName)
	}

	// Close the current patrol root with the summary
	b := beads.New(cfg.BeadsDir)

	// Update the description with the patrol summary
	desc := fmt.Sprintf("Patrol report: %s", patrolReportSummary)
	if err := b.Update(patrolID, beads.UpdateOptions{
		Description: &desc,
	}); err != nil {
		style.PrintWarning("could not update patrol summary: %v", err)
	}

	// Close the patrol root
	if err := b.ForceCloseWithReason("patrol cycle complete: "+patrolReportSummary, patrolID); err != nil {
		return fmt.Errorf("closing patrol %s: %w", patrolID, err)
	}

	fmt.Printf("%s Closed patrol %s\n", style.Success.Render("✓"), patrolID)

	// Start next cycle
	newPatrolID, err := autoSpawnPatrol(cfg)
	if err != nil {
		if newPatrolID != "" {
			fmt.Fprintf(os.Stderr, "warning: %s\n", err.Error())
			fmt.Printf("New patrol: %s\n", newPatrolID)
			return nil
		}
		return fmt.Errorf("starting next patrol cycle: %w", err)
	}

	fmt.Printf("%s Started new patrol: %s\n", style.Success.Render("✓"), newPatrolID)
	return nil
}
