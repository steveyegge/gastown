package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/upgrade"
)

// Engine runs migrations with atomic transactions and rollback support.
type Engine struct {
	townRoot string
	backup   *BackupManager
	verifier *Verifier
	logger   func(format string, args ...interface{})
}

// NewEngine creates a new migration engine for the given workspace.
func NewEngine(townRoot string) *Engine {
	return &Engine{
		townRoot: townRoot,
		backup:   NewBackupManager(townRoot),
		verifier: NewVerifier(townRoot),
		logger:   func(format string, args ...interface{}) {},
	}
}

// SetLogger sets the logging function for migration progress.
func (e *Engine) SetLogger(logger func(format string, args ...interface{})) {
	e.logger = logger
}

// Run executes a migration with backup, rollback support, and verification.
func (e *Engine) Run(migration *Migration, dryRun bool) (*MigrationResult, error) {
	result := &MigrationResult{
		MigrationID: migration.ID,
		StartTime:   time.Now(),
		Steps:       []StepResult{},
	}

	// Get current version
	fromVersion, err := GetWorkspaceVersion(e.townRoot)
	if err != nil {
		result.Error = fmt.Sprintf("detecting workspace version: %v", err)
		result.EndTime = time.Now()
		return result, fmt.Errorf("detecting workspace version: %w", err)
	}
	result.FromVersion = fromVersion
	result.ToVersion = migration.ToVersion

	// Parse versions
	fromVer, err := upgrade.ParseVersion(fromVersion)
	if err != nil {
		result.Error = fmt.Sprintf("parsing from version: %v", err)
		result.EndTime = time.Now()
		return result, fmt.Errorf("parsing from version: %w", err)
	}

	toVer, err := upgrade.ParseVersion(migration.ToVersion)
	if err != nil {
		result.Error = fmt.Sprintf("parsing to version: %v", err)
		result.EndTime = time.Now()
		return result, fmt.Errorf("parsing to version: %w", err)
	}

	// Create context
	ctx := &Context{
		TownRoot:    e.townRoot,
		FromVersion: fromVer,
		ToVersion:   toVer,
		DryRun:      dryRun,
		Logger:      e.logger,
	}

	if dryRun {
		e.logger("DRY RUN - no changes will be made\n")
	}

	// Step 1: Create backup (unless dry run)
	var backupDir string
	if !dryRun {
		e.logger("Creating backup...\n")
		var manifest *BackupManifest
		backupDir, manifest, err = e.backup.CreateBackup(migration.ID, fromVersion, migration.ToVersion)
		if err != nil {
			result.Error = fmt.Sprintf("creating backup: %v", err)
			result.EndTime = time.Now()
			return result, fmt.Errorf("creating backup: %w", err)
		}
		result.BackupPath = backupDir
		ctx.BackupDir = backupDir
		e.logger("  Backed up %d files to %s\n", len(manifest.Files), filepath.Base(backupDir))
	}

	// Step 2: Execute migration steps
	var completedSteps []Step
	for i, step := range migration.Steps {
		stepResult := StepResult{
			StepID: step.ID(),
		}
		stepStart := time.Now()

		e.logger("[%d/%d] %s...", i+1, len(migration.Steps), step.Description())

		// Check if step needs to run
		needed, err := step.Check(ctx)
		if err != nil {
			stepResult.Success = false
			stepResult.Error = err.Error()
			stepResult.Duration = time.Since(stepStart)
			result.Steps = append(result.Steps, stepResult)
			e.logger(" ERROR\n")
			e.logger("  Check failed: %v\n", err)

			// Rollback on failure
			if !dryRun && len(completedSteps) > 0 {
				e.rollbackSteps(ctx, completedSteps)
			}
			result.Error = fmt.Sprintf("step %s check failed: %v", step.ID(), err)
			result.EndTime = time.Now()
			return result, fmt.Errorf("step %s check failed: %w", step.ID(), err)
		}

		if !needed {
			stepResult.Success = true
			stepResult.Skipped = true
			stepResult.Message = "already done"
			stepResult.Duration = time.Since(stepStart)
			result.Steps = append(result.Steps, stepResult)
			e.logger(" SKIPPED\n")
			continue
		}

		// Execute step
		if !dryRun {
			if err := step.Execute(ctx); err != nil {
				stepResult.Success = false
				stepResult.Error = err.Error()
				stepResult.Duration = time.Since(stepStart)
				result.Steps = append(result.Steps, stepResult)
				e.logger(" FAILED\n")
				e.logger("  %v\n", err)

				// Rollback on failure
				if len(completedSteps) > 0 {
					e.rollbackSteps(ctx, completedSteps)
					result.RolledBack = true
				}
				result.Error = fmt.Sprintf("step %s failed: %v", step.ID(), err)
				result.EndTime = time.Now()
				return result, fmt.Errorf("step %s failed: %w", step.ID(), err)
			}

			// Verify step
			if err := step.Verify(ctx); err != nil {
				stepResult.Success = false
				stepResult.Error = fmt.Sprintf("verification failed: %v", err)
				stepResult.Duration = time.Since(stepStart)
				result.Steps = append(result.Steps, stepResult)
				e.logger(" VERIFY FAILED\n")
				e.logger("  %v\n", err)

				// Rollback on failure
				completedSteps = append(completedSteps, step) // Include this step in rollback
				e.rollbackSteps(ctx, completedSteps)
				result.RolledBack = true
				result.Error = fmt.Sprintf("step %s verification failed: %v", step.ID(), err)
				result.EndTime = time.Now()
				return result, fmt.Errorf("step %s verification failed: %w", step.ID(), err)
			}
		}

		stepResult.Success = true
		stepResult.Duration = time.Since(stepStart)
		result.Steps = append(result.Steps, stepResult)
		completedSteps = append(completedSteps, step)
		e.logger(" OK\n")
	}

	// Step 3: Update workspace version
	if !dryRun {
		e.logger("Updating workspace version to %s...", migration.ToVersion)
		if err := e.updateWorkspaceVersion(migration.ToVersion); err != nil {
			e.logger(" FAILED\n")
			// Rollback on failure
			e.rollbackSteps(ctx, completedSteps)
			result.RolledBack = true
			result.Error = fmt.Sprintf("updating workspace version: %v", err)
			result.EndTime = time.Now()
			return result, fmt.Errorf("updating workspace version: %w", err)
		}
		e.logger(" OK\n")
	}

	// Step 4: Post-migration verification
	if !dryRun {
		e.logger("Running verification...\n")
		verifyResult := e.verifier.VerifyQuick()
		if !verifyResult.Success {
			e.logger("  WARNINGS:\n")
			for _, err := range verifyResult.Errors {
				e.logger("    - %s\n", err)
			}
			// Don't fail on verification warnings, just report them
		} else {
			e.logger("  All checks passed\n")
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	return result, nil
}

// rollbackSteps reverses completed steps in reverse order.
func (e *Engine) rollbackSteps(ctx *Context, steps []Step) {
	e.logger("\nRolling back %d step(s)...\n", len(steps))
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		e.logger("  Rolling back %s...", step.ID())
		if err := step.Rollback(ctx); err != nil {
			e.logger(" FAILED: %v\n", err)
		} else {
			e.logger(" OK\n")
		}
	}
}

// Rollback restores the workspace from the most recent backup.
func (e *Engine) Rollback() error {
	backupDir, manifest, err := e.backup.GetLatestBackup()
	if err != nil {
		return fmt.Errorf("finding backup: %w", err)
	}

	e.logger("Restoring from backup: %s\n", filepath.Base(backupDir))
	e.logger("  Created: %s\n", manifest.Timestamp.Format(time.RFC3339))
	e.logger("  From version: %s\n", manifest.FromVersion)
	e.logger("  Files: %d\n", len(manifest.Files))

	if err := e.backup.RestoreBackup(backupDir); err != nil {
		return fmt.Errorf("restoring backup: %w", err)
	}

	e.logger("Backup restored successfully\n")
	return nil
}

// updateWorkspaceVersion updates the gt_version in mayor/town.json atomically.
// Uses write-to-temp-then-rename pattern for crash safety.
func (e *Engine) updateWorkspaceVersion(version string) error {
	configPath := filepath.Join(e.townRoot, "mayor", "town.json")

	// Get original file's permissions
	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("stat town.json: %w", err)
	}
	originalMode := info.Mode()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading town.json: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing town.json: %w", err)
	}

	config["gt_version"] = version

	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling town.json: %w", err)
	}

	// Add trailing newline for POSIX compliance
	newData = append(newData, '\n')

	// Write to temp file first for atomicity, preserving original permissions
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, originalMode); err != nil {
		return fmt.Errorf("writing temp town.json: %w", err)
	}

	// Atomically rename temp file to actual config
	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath) // Clean up temp file on failure
		return fmt.Errorf("renaming town.json: %w", err)
	}

	return nil
}

// Check determines if a migration is needed and returns details.
func (e *Engine) Check() (*CheckResult, error) {
	return NeedsMigration(e.townRoot)
}

// Status returns the current migration status of the workspace.
func (e *Engine) Status() (*MigrationStatus, error) {
	layout, err := DetectLayout(e.townRoot)
	if err != nil {
		return nil, err
	}

	check, err := NeedsMigration(e.townRoot)
	if err != nil {
		return nil, err
	}

	backups, _ := e.backup.ListBackups()

	return &MigrationStatus{
		CurrentVersion:  layout.Version,
		LayoutType:      layout.Type,
		NeedsMigration:  check.NeedsMigration,
		AvailablePath:   check.MigrationPath,
		BackupCount:     len(backups),
		LatestBackupDir: getLatestBackupDir(backups),
	}, nil
}

// MigrationStatus describes the current migration state of a workspace.
type MigrationStatus struct {
	CurrentVersion  string   `json:"current_version"`
	LayoutType      string   `json:"layout_type"`
	NeedsMigration  bool     `json:"needs_migration"`
	AvailablePath   []string `json:"available_path,omitempty"`
	BackupCount     int      `json:"backup_count"`
	LatestBackupDir string   `json:"latest_backup_dir,omitempty"`
}

func getLatestBackupDir(backups []string) string {
	if len(backups) == 0 {
		return ""
	}
	return filepath.Base(backups[len(backups)-1])
}

// Preview returns a description of what a migration will do without executing.
func (e *Engine) Preview(migration *Migration) (*MigrationPreview, error) {
	fromVersion, err := GetWorkspaceVersion(e.townRoot)
	if err != nil {
		return nil, fmt.Errorf("detecting workspace version: %w", err)
	}

	fromVer, err := upgrade.ParseVersion(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing version: %w", err)
	}

	toVer, err := upgrade.ParseVersion(migration.ToVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing target version: %w", err)
	}

	ctx := &Context{
		TownRoot:    e.townRoot,
		FromVersion: fromVer,
		ToVersion:   toVer,
		DryRun:      true,
	}

	preview := &MigrationPreview{
		MigrationID: migration.ID,
		FromVersion: fromVersion,
		ToVersion:   migration.ToVersion,
		Description: migration.Description,
		Steps:       []StepPreview{},
	}

	for _, step := range migration.Steps {
		stepPreview := StepPreview{
			ID:          step.ID(),
			Description: step.Description(),
		}

		needed, err := step.Check(ctx)
		if err != nil {
			stepPreview.Status = "error"
			stepPreview.Message = err.Error()
		} else if needed {
			stepPreview.Status = "pending"
		} else {
			stepPreview.Status = "skipped"
			stepPreview.Message = "already done"
		}

		preview.Steps = append(preview.Steps, stepPreview)
	}

	return preview, nil
}

// MigrationPreview describes what a migration will do.
type MigrationPreview struct {
	MigrationID string        `json:"migration_id"`
	FromVersion string        `json:"from_version"`
	ToVersion   string        `json:"to_version"`
	Description string        `json:"description"`
	Steps       []StepPreview `json:"steps"`
}

// StepPreview describes what a single step will do.
type StepPreview struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pending", "skipped", or "error"
	Message     string `json:"message,omitempty"`
}

// PendingStepCount returns the number of steps that need to run.
func (p *MigrationPreview) PendingStepCount() int {
	count := 0
	for _, s := range p.Steps {
		if s.Status == "pending" {
			count++
		}
	}
	return count
}
