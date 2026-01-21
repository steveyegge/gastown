package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/migrate"
)

// MigrationCheck verifies if a workspace migration is needed.
type MigrationCheck struct {
	BaseCheck
}

// NewMigrationCheck creates a new migration check.
func NewMigrationCheck() *MigrationCheck {
	return &MigrationCheck{
		BaseCheck: BaseCheck{
			CheckName:        "migration",
			CheckDescription: "Check if workspace migration is needed",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks if a workspace migration is needed.
func (c *MigrationCheck) Run(ctx *CheckContext) *CheckResult {
	migrateResult, err := migrate.NeedsMigration(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not check migration status: %v", err),
		}
	}

	if !migrateResult.NeedsMigration {
		msg := "Workspace is up to date"
		if migrateResult.CurrentVersion != "" {
			msg = fmt.Sprintf("Workspace is up to date (%s)", migrateResult.CurrentVersion)
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: msg,
		}
	}

	// Migration is needed - build details only when needed
	details := []string{
		fmt.Sprintf("Current layout: %s", migrateResult.LayoutType),
		fmt.Sprintf("Current version: %s", migrateResult.CurrentVersion),
		fmt.Sprintf("Target version: %s", migrateResult.TargetVersion),
	}

	if len(migrateResult.MigrationPath) > 0 {
		details = append(details, fmt.Sprintf("Migration path: %s", strings.Join(migrateResult.MigrationPath, " -> ")))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Migration needed: %s -> %s", migrateResult.CurrentVersion, migrateResult.TargetVersion),
		Details: details,
		FixHint: "Run 'gt migrate' to migrate your workspace",
	}
}
