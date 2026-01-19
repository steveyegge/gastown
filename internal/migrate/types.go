// Package migrate provides workspace migration between Gas Town versions.
// It handles version upgrades with atomic transactions, backups, and rollback support.
package migrate

import (
	"time"

	"github.com/steveyegge/gastown/internal/upgrade"
)

// Context provides context for migration operations.
type Context struct {
	// TownRoot is the root directory of the Gas Town workspace.
	TownRoot string

	// FromVersion is the version we're migrating from.
	FromVersion *upgrade.Version

	// ToVersion is the version we're migrating to.
	ToVersion *upgrade.Version

	// DryRun indicates whether to simulate the migration without making changes.
	DryRun bool

	// Verbose enables detailed output during migration.
	Verbose bool

	// BackupDir is where backups are stored during migration.
	BackupDir string

	// Logger receives progress messages during migration.
	Logger func(format string, args ...interface{})

	// StepState stores per-step state for rollback purposes.
	// This allows steps to store state needed for rollback without
	// storing it on the step struct itself (which would cause race
	// conditions if migrations are run concurrently).
	// Keys are step IDs, values are step-specific state.
	StepState map[string]interface{}
}

// Log writes a message to the context logger if set.
func (c *Context) Log(format string, args ...interface{}) {
	if c.Logger != nil {
		c.Logger(format, args...)
	}
}

// SetStepState stores state for a step, keyed by step ID.
// This is used to store rollback information that persists across Execute and Rollback calls.
func (c *Context) SetStepState(stepID string, state interface{}) {
	if c.StepState == nil {
		c.StepState = make(map[string]interface{})
	}
	c.StepState[stepID] = state
}

// GetStepState retrieves state for a step by ID.
// Returns nil if no state exists for that step.
func (c *Context) GetStepState(stepID string) interface{} {
	if c.StepState == nil {
		return nil
	}
	return c.StepState[stepID]
}

// BackupManifest describes the contents of a migration backup.
type BackupManifest struct {
	// Timestamp when the backup was created.
	Timestamp time.Time `json:"timestamp"`

	// FromVersion is the workspace version before migration.
	FromVersion string `json:"from_version"`

	// ToVersion is the target version of the migration.
	ToVersion string `json:"to_version"`

	// TownRoot is the workspace root path.
	TownRoot string `json:"town_root"`

	// Files lists all files/directories included in the backup.
	Files []BackupFile `json:"files"`

	// MigrationID identifies the specific migration being applied.
	MigrationID string `json:"migration_id"`

	// StepsCompleted tracks which steps were completed before backup.
	StepsCompleted []string `json:"steps_completed,omitempty"`
}

// BackupFile describes a single backed up file or directory.
type BackupFile struct {
	// OriginalPath is the path relative to town root.
	OriginalPath string `json:"original_path"`

	// BackupPath is the path within the backup directory.
	BackupPath string `json:"backup_path"`

	// IsDirectory indicates if this is a directory.
	IsDirectory bool `json:"is_directory,omitempty"`

	// Size is the file size in bytes (0 for directories).
	Size int64 `json:"size,omitempty"`
}

// StepResult describes the outcome of a single migration step.
type StepResult struct {
	// StepID identifies the step that was executed.
	StepID string `json:"step_id"`

	// Success indicates whether the step completed successfully.
	Success bool `json:"success"`

	// Skipped indicates the step was skipped (already done).
	Skipped bool `json:"skipped,omitempty"`

	// Message provides details about the result.
	Message string `json:"message,omitempty"`

	// Error contains error details if the step failed.
	Error string `json:"error,omitempty"`

	// Duration is how long the step took.
	Duration time.Duration `json:"duration,omitempty"`

	// Changes lists what was modified by this step.
	Changes []string `json:"changes,omitempty"`
}

// MigrationResult describes the outcome of a complete migration.
type MigrationResult struct {
	// Success indicates whether the entire migration succeeded.
	Success bool `json:"success"`

	// MigrationID identifies which migration was run.
	MigrationID string `json:"migration_id"`

	// FromVersion is the version before migration.
	FromVersion string `json:"from_version"`

	// ToVersion is the version after migration.
	ToVersion string `json:"to_version"`

	// Steps contains results for each step.
	Steps []StepResult `json:"steps"`

	// BackupPath is where the backup was stored.
	BackupPath string `json:"backup_path,omitempty"`

	// StartTime is when the migration started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the migration completed.
	EndTime time.Time `json:"end_time"`

	// Error contains the overall error if migration failed.
	Error string `json:"error,omitempty"`

	// RolledBack indicates whether a rollback was performed.
	RolledBack bool `json:"rolled_back,omitempty"`
}

// Duration returns the total migration duration.
func (r *MigrationResult) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// FailedStep returns the first failed step, or nil if all succeeded.
func (r *MigrationResult) FailedStep() *StepResult {
	for i := range r.Steps {
		if !r.Steps[i].Success && !r.Steps[i].Skipped {
			return &r.Steps[i]
		}
	}
	return nil
}

// CompletedSteps returns the count of successfully completed steps.
func (r *MigrationResult) CompletedSteps() int {
	count := 0
	for _, s := range r.Steps {
		if s.Success && !s.Skipped {
			count++
		}
	}
	return count
}

// SkippedSteps returns the count of skipped steps.
func (r *MigrationResult) SkippedSteps() int {
	count := 0
	for _, s := range r.Steps {
		if s.Skipped {
			count++
		}
	}
	return count
}

// CheckResult describes the result of checking whether migration is needed.
type CheckResult struct {
	// NeedsMigration indicates whether a migration is required.
	NeedsMigration bool `json:"needs_migration"`

	// CurrentVersion is the detected workspace version.
	CurrentVersion string `json:"current_version"`

	// TargetVersion is the version to migrate to.
	TargetVersion string `json:"target_version,omitempty"`

	// MigrationPath lists the migrations that need to be applied.
	MigrationPath []string `json:"migration_path,omitempty"`

	// Message provides a human-readable explanation.
	Message string `json:"message"`

	// LayoutType describes the detected workspace layout.
	LayoutType string `json:"layout_type,omitempty"`
}

// WorkspaceLayout describes the detected layout of a Gas Town workspace.
type WorkspaceLayout struct {
	// Type identifies the layout version (e.g., "0.1.x", "0.2.x").
	Type string

	// TownRoot is the workspace root path.
	TownRoot string

	// ConfigPath is where town.json is located.
	ConfigPath string

	// RigsPath is where rigs.json is located.
	RigsPath string

	// BeadsPath is where the beads database is located.
	BeadsPath string

	// Version is the gt_version from town.json if present.
	Version string

	// Rigs lists detected rig directories.
	Rigs []string

	// HasMayorDir indicates if mayor/ directory exists.
	HasMayorDir bool

	// HasLegacyTownJSON indicates if town.json exists at root (0.1.x style).
	HasLegacyTownJSON bool
}
