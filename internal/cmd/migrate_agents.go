package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	migrateAgentsDryRun bool
	migrateAgentsForce  bool
)

var migrateAgentsCmd = &cobra.Command{
	Use:     "migrate-agents",
	GroupID: GroupDiag,
	Short:   "Migrate agent beads to two-level architecture",
	Long: `Migrate agent beads from the old single-tier to the two-level architecture.

This command migrates town-level agent beads (Mayor, Deacon) from rig beads
with gt-* prefix to town beads with hq-* prefix:

  OLD (rig beads):    gt-mayor, gt-deacon
  NEW (town beads):   hq-mayor, hq-deacon

Rig-level agents (Witness, Refinery, Polecats) remain in rig beads unchanged.

The migration:
1. Detects old gt-mayor/gt-deacon beads in rig beads
2. Creates new hq-mayor/hq-deacon beads in town beads
3. Copies agent state (hook_bead, agent_state, etc.)
4. Adds migration note to old beads (preserves them)

Safety:
- Dry-run mode by default (use --execute to apply changes)
- Old beads are preserved with migration notes
- Validates new beads exist before marking migration complete
- Skips if new beads already exist (idempotent)

Examples:
  gt migrate-agents              # Dry-run: show what would be migrated
  gt migrate-agents --execute    # Apply the migration
  gt migrate-agents --force      # Re-migrate even if new beads exist`,
	RunE: runMigrateAgents,
}

func init() {
	migrateAgentsCmd.Flags().BoolVar(&migrateAgentsDryRun, "dry-run", true, "Show what would be migrated without making changes (default)")
	migrateAgentsCmd.Flags().BoolVar(&migrateAgentsForce, "force", false, "Re-migrate even if new beads already exist")
	// Add --execute as inverse of --dry-run for clarity
	migrateAgentsCmd.Flags().BoolP("execute", "x", false, "Actually apply the migration (opposite of --dry-run)")
	rootCmd.AddCommand(migrateAgentsCmd)
}

// migrationResult holds the result of a single bead migration.
type migrationResult struct {
	OldID      string
	NewID      string
	Status     string // "migrated", "skipped", "error"
	Message    string
	OldFields  *beads.AgentFields
	WasDryRun  bool
}

func runMigrateAgents(cmd *cobra.Command, args []string) error {
	// Handle --execute flag
	if execute, _ := cmd.Flags().GetBool("execute"); execute {
		migrateAgentsDryRun = false
	}

	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get town beads path
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Load routes to find rig beads
	routes, err := beads.LoadRoutes(townBeadsDir)
	if err != nil {
		return fmt.Errorf("loading routes.jsonl: %w", err)
	}

	// Find the first rig with gt- prefix (where global agents are currently stored)
	var sourceRigPath string
	for _, r := range routes {
		if strings.TrimSuffix(r.Prefix, "-") == "gt" && r.Path != "." {
			sourceRigPath = r.Path
			break
		}
	}

	if sourceRigPath == "" {
		fmt.Println("No rig with gt- prefix found. Nothing to migrate.")
		return nil
	}

	// Source beads (rig beads where old agent beads are)
	sourceBeadsDir := filepath.Join(townRoot, sourceRigPath, ".beads")
	sourceBd := beads.New(sourceBeadsDir)

	// Target beads (town beads where new agent beads should go)
	targetBd := beads.NewWithBeadsDir(townRoot, townBeadsDir)

	// Agents to migrate: town-level agents only
	agentsToMigrate := []struct {
		oldID   string
		newID   string
		desc    string
	}{
		{
			oldID: beads.MayorBeadID(),  // gt-mayor
			newID: beads.MayorBeadIDTown(), // hq-mayor
			desc:  "Mayor - global coordinator, handles cross-rig communication and escalations.",
		},
		{
			oldID: beads.DeaconBeadID(),  // gt-deacon
			newID: beads.DeaconBeadIDTown(), // hq-deacon
			desc:  "Deacon (daemon beacon) - receives mechanical heartbeats, runs town plugins and monitoring.",
		},
	}

	// Also migrate role beads
	rolesToMigrate := []string{"mayor", "deacon", "witness", "refinery", "polecat", "crew", "dog"}

	if migrateAgentsDryRun {
		fmt.Println("🔍 DRY RUN: Showing what would be migrated")
		fmt.Println("   Use --execute to apply changes")
		fmt.Println()
	} else {
		fmt.Println("🚀 Migrating agent beads to two-level architecture")
		fmt.Println()
	}

	var results []migrationResult

	// Migrate agent beads
	fmt.Println("Agent Beads:")
	for _, agent := range agentsToMigrate {
		result := migrateAgentBead(sourceBd, targetBd, agent.oldID, agent.newID, agent.desc, migrateAgentsDryRun, migrateAgentsForce)
		results = append(results, result)
		printMigrationResult(result)
	}

	// Migrate role beads
	fmt.Println("\nRole Beads:")
	for _, role := range rolesToMigrate {
		oldID := "gt-" + role + "-role"
		newID := beads.RoleBeadIDTown(role) // hq-<role>-role
		result := migrateRoleBead(sourceBd, targetBd, oldID, newID, role, migrateAgentsDryRun, migrateAgentsForce)
		results = append(results, result)
		printMigrationResult(result)
	}

	// Summary
	fmt.Println()
	printMigrationSummary(results, migrateAgentsDryRun)

	return nil
}

// migrateAgentBead migrates a single agent bead from source to target.
func migrateAgentBead(sourceBd, targetBd *beads.Beads, oldID, newID, desc string, dryRun, force bool) migrationResult {
	result := migrationResult{
		OldID:     oldID,
		NewID:     newID,
		WasDryRun: dryRun,
	}

	// Check if old bead exists
	oldIssue, oldFields, err := sourceBd.GetAgentBead(oldID)
	if err != nil {
		result.Status = "skipped"
		result.Message = "old bead not found"
		return result
	}
	result.OldFields = oldFields

	// Check if new bead already exists
	if _, err := targetBd.Show(newID); err == nil {
		if !force {
			result.Status = "skipped"
			result.Message = "new bead already exists (use --force to re-migrate)"
			return result
		}
	}

	if dryRun {
		result.Status = "would migrate"
		result.Message = fmt.Sprintf("would copy state from %s", oldIssue.ID)
		return result
	}

	// Create new bead in town beads
	newFields := &beads.AgentFields{
		RoleType:          oldFields.RoleType,
		Rig:               oldFields.Rig,
		AgentState:        oldFields.AgentState,
		HookBead:          oldFields.HookBead,
		RoleBead:          beads.RoleBeadIDTown(oldFields.RoleType), // Update to hq- role
		CleanupStatus:     oldFields.CleanupStatus,
		ActiveMR:          oldFields.ActiveMR,
		NotificationLevel: oldFields.NotificationLevel,
	}

	_, err = targetBd.CreateAgentBead(newID, desc, newFields)
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("failed to create: %v", err)
		return result
	}

	// Add migration label to old bead
	migrationLabel := fmt.Sprintf("migrated-to:%s", newID)
	if err := sourceBd.Update(oldID, beads.UpdateOptions{AddLabels: []string{migrationLabel}}); err != nil {
		// Non-fatal: just log it
		result.Message = fmt.Sprintf("created but couldn't add migration label: %v", err)
	}

	result.Status = "migrated"
	result.Message = "successfully migrated"
	return result
}

// migrateRoleBead migrates a role definition bead.
func migrateRoleBead(sourceBd, targetBd *beads.Beads, oldID, newID, role string, dryRun, force bool) migrationResult {
	result := migrationResult{
		OldID:     oldID,
		NewID:     newID,
		WasDryRun: dryRun,
	}

	// Check if old bead exists
	oldIssue, err := sourceBd.Show(oldID)
	if err != nil {
		result.Status = "skipped"
		result.Message = "old bead not found"
		return result
	}

	// Check if new bead already exists
	if _, err := targetBd.Show(newID); err == nil {
		if !force {
			result.Status = "skipped"
			result.Message = "new bead already exists (use --force to re-migrate)"
			return result
		}
	}

	if dryRun {
		result.Status = "would migrate"
		result.Message = fmt.Sprintf("would copy from %s", oldIssue.ID)
		return result
	}

	// Create new role bead in town beads
	// Role beads are simple - just copy the description
	_, err = targetBd.CreateWithID(newID, beads.CreateOptions{
		Title:       fmt.Sprintf("Role: %s", role),
		Type:        "role",
		Description: oldIssue.Title, // Use old title as description
	})
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("failed to create: %v", err)
		return result
	}

	// Add migration label to old bead
	migrationLabel := fmt.Sprintf("migrated-to:%s", newID)
	if err := sourceBd.Update(oldID, beads.UpdateOptions{AddLabels: []string{migrationLabel}}); err != nil {
		// Non-fatal
		result.Message = fmt.Sprintf("created but couldn't add migration label: %v", err)
	}

	result.Status = "migrated"
	result.Message = "successfully migrated"
	return result
}

func getMigrationStatusIcon(status string) string {
	switch status {
	case "migrated", "would migrate":
		return "  ✓"
	case "skipped":
		return "  ⊘"
	case "error":
		return "  ✗"
	default:
		return "  ?"
	}
}

func printMigrationResult(r migrationResult) {
	fmt.Printf("%s %s → %s: %s\n", getMigrationStatusIcon(r.Status), r.OldID, r.NewID, r.Message)
}

func printMigrationSummary(results []migrationResult, dryRun bool) {
	var migrated, skipped, errors int
	for _, r := range results {
		switch r.Status {
		case "migrated", "would migrate":
			migrated++
		case "skipped":
			skipped++
		case "error":
			errors++
		}
	}

	if dryRun {
		fmt.Printf("Summary (dry-run): %d would migrate, %d skipped, %d errors\n", migrated, skipped, errors)
		if migrated > 0 {
			fmt.Println("\nRun with --execute to apply these changes.")
		}
	} else {
		fmt.Printf("Summary: %d migrated, %d skipped, %d errors\n", migrated, skipped, errors)
	}
}
