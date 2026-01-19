package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/migrate"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/upgrade"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	migrateCmdCheck    bool // --check: only check if migration is needed
	migrateCmdDryRun   bool // --dry-run: preview changes without executing
	migrateCmdExecute  bool // --execute: run the migration
	migrateCmdForce    bool // --force: skip confirmation prompts
	migrateCmdRollback bool // --rollback: undo the last migration
	migrateCmdStatus   bool // --status: show migration status
)

var migrateCmd = &cobra.Command{
	Use:     "migrate",
	GroupID: GroupDiag,
	Short:   "Migrate workspace to a new version layout",
	Long: `Migrate workspace to a new version layout.

This command handles workspace structure changes between major versions.
Migrations are atomic - if a step fails, changes are rolled back automatically.

The migration process:
1. Creates a backup of critical configuration files
2. Executes migration steps in order
3. Verifies the migration succeeded
4. Updates the workspace version

Examples:
  gt migrate              # Interactive migration (prompts for confirmation)
  gt migrate --check      # Check if migration is needed
  gt migrate --dry-run    # Preview changes without executing
  gt migrate --execute    # Run migration without confirmation prompt
  gt migrate --force      # Run migration without confirmation (same as --execute)
  gt migrate --rollback   # Restore from latest backup
  gt migrate --status     # Show current migration status`,
	RunE: runMigrate,
}

func init() {
	migrateCmd.Flags().BoolVar(&migrateCmdCheck, "check", false, "Check if migration is needed")
	migrateCmd.Flags().BoolVarP(&migrateCmdDryRun, "dry-run", "n", false, "Preview changes without executing")
	migrateCmd.Flags().BoolVar(&migrateCmdExecute, "execute", false, "Execute the migration")
	migrateCmd.Flags().BoolVarP(&migrateCmdForce, "force", "f", false, "Skip confirmation prompts")
	migrateCmd.Flags().BoolVar(&migrateCmdRollback, "rollback", false, "Restore from latest backup")
	migrateCmd.Flags().BoolVar(&migrateCmdStatus, "status", false, "Show current migration status")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	engine := migrate.NewEngine(townRoot)
	engine.SetLogger(func(format string, args ...interface{}) {
		fmt.Printf(format, args...)
	})

	// Handle rollback
	if migrateCmdRollback {
		return runMigrateRollback(engine)
	}

	// Handle status
	if migrateCmdStatus {
		return runMigrateStatus(engine)
	}

	// Check if migration is needed
	checkResult, err := engine.Check()
	if err != nil {
		return fmt.Errorf("checking migration status: %w", err)
	}

	// Handle --check flag
	if migrateCmdCheck {
		return runMigrateCheck(checkResult)
	}

	// If no migration needed, we're done
	if !checkResult.NeedsMigration {
		fmt.Printf("%s Workspace is up to date (version %s)\n",
			style.SuccessPrefix, checkResult.CurrentVersion)
		return nil
	}

	// Get the migration to run
	currentVer, err := upgrade.ParseVersion(checkResult.CurrentVersion)
	if err != nil {
		return fmt.Errorf("parsing current version: %w", err)
	}

	migration, err := migrate.FindMigration(currentVer)
	if err != nil {
		return fmt.Errorf("finding migration: %w", err)
	}

	// Handle --dry-run
	if migrateCmdDryRun {
		return runMigrateDryRun(engine, migration)
	}

	// Interactive or --execute mode
	return runMigrateExecute(engine, migration, checkResult)
}

func runMigrateCheck(checkResult *migrate.CheckResult) error {
	if !checkResult.NeedsMigration {
		fmt.Printf("%s No migration needed\n", style.SuccessPrefix)
		fmt.Printf("  Layout: %s\n", checkResult.LayoutType)
		fmt.Printf("  Version: %s\n", checkResult.CurrentVersion)
		return nil
	}

	fmt.Printf("%s Migration required\n", style.WarningPrefix)
	fmt.Printf("  Current: %s (%s layout)\n", checkResult.CurrentVersion, checkResult.LayoutType)
	fmt.Printf("  Target: %s\n", checkResult.TargetVersion)
	fmt.Printf("  Path: %s\n", strings.Join(checkResult.MigrationPath, " -> "))
	fmt.Println()
	fmt.Printf("Run 'gt migrate --dry-run' to preview changes\n")
	fmt.Printf("Run 'gt migrate --execute' to run the migration\n")
	return nil
}

func runMigrateStatus(engine *migrate.Engine) error {
	status, err := engine.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	fmt.Println("Migration Status")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Current version: %s\n", status.CurrentVersion)
	fmt.Printf("Layout type: %s\n", status.LayoutType)
	fmt.Printf("Migration needed: %v\n", status.NeedsMigration)

	if status.NeedsMigration && len(status.AvailablePath) > 0 {
		fmt.Printf("Migration path: %s\n", strings.Join(status.AvailablePath, " -> "))
	}

	fmt.Printf("Backups: %d\n", status.BackupCount)
	if status.LatestBackupDir != "" {
		fmt.Printf("Latest backup: %s\n", status.LatestBackupDir)
	}

	// List available migrations
	fmt.Println()
	fmt.Println("Available Migrations:")
	for _, info := range migrate.ListMigrations() {
		fmt.Printf("  %s: %s -> %s (%d steps)\n",
			info.ID, info.FromPattern, info.ToVersion, info.StepCount)
		fmt.Printf("    %s\n", info.Description)
	}

	return nil
}

func runMigrateDryRun(engine *migrate.Engine, migration *migrate.Migration) error {
	fmt.Println("DRY RUN - Previewing migration changes")
	fmt.Println(strings.Repeat("-", 40))

	preview, err := engine.Preview(migration)
	if err != nil {
		return fmt.Errorf("generating preview: %w", err)
	}

	fmt.Printf("Migration: %s\n", preview.MigrationID)
	fmt.Printf("From: %s -> To: %s\n", preview.FromVersion, preview.ToVersion)
	fmt.Printf("Description: %s\n", preview.Description)
	fmt.Println()
	fmt.Println("Steps:")

	for i, step := range preview.Steps {
		icon := " "
		switch step.Status {
		case "pending":
			icon = "○"
		case "skipped":
			icon = "✓"
		case "error":
			icon = "✗"
		}

		fmt.Printf("  %s [%d] %s: %s", icon, i+1, step.ID, step.Description)
		if step.Message != "" {
			fmt.Printf(" (%s)", step.Message)
		}
		fmt.Println()
	}

	fmt.Println()
	pending := preview.PendingStepCount()
	if pending == 0 {
		fmt.Printf("%s All steps already completed\n", style.SuccessPrefix)
	} else {
		fmt.Printf("%d step(s) will be executed\n", pending)
		fmt.Println()
		fmt.Println("Run 'gt migrate --execute' to apply these changes")
	}

	return nil
}

func runMigrateExecute(engine *migrate.Engine, migration *migrate.Migration, checkResult *migrate.CheckResult) error {
	fmt.Println("Workspace Migration")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Current version: %s\n", checkResult.CurrentVersion)
	fmt.Printf("Target version: %s\n", migration.ToVersion)
	fmt.Printf("Migration: %s\n", migration.Description)
	fmt.Println()

	// Preview steps
	preview, err := engine.Preview(migration)
	if err != nil {
		return fmt.Errorf("generating preview: %w", err)
	}

	fmt.Println("Steps to execute:")
	for i, step := range preview.Steps {
		status := "pending"
		if step.Status == "skipped" {
			status = "already done"
		}
		fmt.Printf("  [%d] %s (%s)\n", i+1, step.Description, status)
	}
	fmt.Println()

	// Confirm unless --force
	if !migrateCmdForce && !migrateCmdExecute {
		fmt.Println("This will modify your workspace. A backup will be created first.")
		fmt.Print("Continue? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Migration cancelled")
			return nil
		}
	}

	// Run the migration
	fmt.Println()
	fmt.Println("Running migration...")
	fmt.Println()

	result, err := engine.Run(migration, false)
	if err != nil {
		if result != nil && result.RolledBack {
			fmt.Println()
			fmt.Printf("%s Migration failed and was rolled back\n", style.ErrorPrefix)
			fmt.Printf("  Error: %s\n", result.Error)
			if result.BackupPath != "" {
				fmt.Printf("  Backup preserved at: %s\n", result.BackupPath)
			}
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print summary
	fmt.Println()
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("%s Migration completed successfully!\n", style.SuccessPrefix)
	fmt.Printf("  Version: %s -> %s\n", result.FromVersion, result.ToVersion)
	fmt.Printf("  Steps: %d completed, %d skipped\n",
		result.CompletedSteps(), result.SkippedSteps())
	fmt.Printf("  Duration: %s\n", result.Duration().Round(100*1e6))
	fmt.Printf("  Backup: %s\n", result.BackupPath)
	fmt.Println()
	fmt.Println("Run 'gt doctor' to verify workspace health")

	return nil
}

func runMigrateRollback(engine *migrate.Engine) error {
	fmt.Println("Rollback Migration")
	fmt.Println(strings.Repeat("-", 40))

	// Confirm unless --force
	if !migrateCmdForce {
		fmt.Println("This will restore the workspace from the latest backup.")
		fmt.Print("Continue? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Rollback cancelled")
			return nil
		}
	}

	if err := engine.Rollback(); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s Rollback completed successfully\n", style.SuccessPrefix)
	fmt.Println("Run 'gt doctor' to verify workspace health")

	return nil
}
