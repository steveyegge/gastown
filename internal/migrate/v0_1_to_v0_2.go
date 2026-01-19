package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// V0_1_to_V0_2 defines the migration from 0.1.x to 0.2.0.
var V0_1_to_V0_2 = Migration{
	ID:          "v0_1_to_v0_2",
	FromPattern: "0.1.x",
	ToVersion:   "0.2.0",
	Description: "Migrate to two-level beads and directory reorganization",
	Steps: []Step{
		&CreateMayorDirectoryStep{},
		&MoveConfigFilesStep{},
		&CreateRigSettingsStep{},
		&CreateRuntimeDirectoriesStep{},
		&MigrateAgentBeadsStep{},
		&CleanupLegacyStep{},
	},
}

func init() {
	RegisterMigration(V0_1_to_V0_2)
}

// CreateMayorDirectoryStep creates the mayor/ directory if it doesn't exist.
// This is the first step in the 0.1 to 0.2 migration, establishing the new
// directory structure for town-level configuration files.
//
// Check: Returns true if mayor/ directory does not exist.
// Execute: Creates the mayor/ directory with 0755 permissions.
// Rollback: Removes the mayor/ directory only if it's empty (preserves moved configs).
// Verify: Confirms mayor/ exists and is a directory.
type CreateMayorDirectoryStep struct {
	BaseStep
}

func (s *CreateMayorDirectoryStep) ID() string {
	return "create-mayor-directory"
}

func (s *CreateMayorDirectoryStep) Description() string {
	return "Create mayor/ directory"
}

func (s *CreateMayorDirectoryStep) Check(ctx *Context) (bool, error) {
	mayorDir := filepath.Join(ctx.TownRoot, "mayor")
	_, err := os.Stat(mayorDir)
	if err == nil {
		return false, nil // Already exists
	}
	if os.IsNotExist(err) {
		return true, nil // Needs to be created
	}
	return false, err
}

func (s *CreateMayorDirectoryStep) Execute(ctx *Context) error {
	mayorDir := filepath.Join(ctx.TownRoot, "mayor")
	return os.MkdirAll(mayorDir, 0755)
}

func (s *CreateMayorDirectoryStep) Rollback(ctx *Context) error {
	mayorDir := filepath.Join(ctx.TownRoot, "mayor")
	// Only remove if empty (don't remove configs that were moved)
	entries, err := os.ReadDir(mayorDir)
	if err != nil {
		return nil // Doesn't exist, nothing to do
	}
	if len(entries) == 0 {
		return os.Remove(mayorDir)
	}
	return nil
}

func (s *CreateMayorDirectoryStep) Verify(ctx *Context) error {
	mayorDir := filepath.Join(ctx.TownRoot, "mayor")
	info, err := os.Stat(mayorDir)
	if err != nil {
		return fmt.Errorf("mayor/ not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("mayor/ is not a directory")
	}
	return nil
}

// MoveConfigFilesStep moves town.json, rigs.json, and accounts.json to mayor/.
// This step centralizes configuration files in the mayor/ directory for the 0.2 layout.
//
// Check: Returns true if legacy town.json exists at root and hasn't been moved.
// Execute: Moves each config file from root to mayor/, tracking moves for rollback.
// Rollback: Moves files back to their original locations in reverse order.
// Verify: Confirms mayor/town.json exists after migration.
type MoveConfigFilesStep struct {
	BaseStep
}

// moveConfigFilesState holds rollback state for MoveConfigFilesStep.
// Stored in Context.StepState to avoid race conditions.
type moveConfigFilesState struct {
	movedFiles []movedFile
}

type movedFile struct {
	src string
	dst string
}

func (s *MoveConfigFilesStep) ID() string {
	return "move-config-files"
}

func (s *MoveConfigFilesStep) Description() string {
	return "Move config files to mayor/"
}

func (s *MoveConfigFilesStep) Check(ctx *Context) (bool, error) {
	// Check if legacy town.json exists at root
	legacyTownJSON := filepath.Join(ctx.TownRoot, "town.json")
	if _, err := os.Stat(legacyTownJSON); os.IsNotExist(err) {
		// No legacy config, check if new location exists
		newTownJSON := filepath.Join(ctx.TownRoot, "mayor", "town.json")
		if _, err := os.Stat(newTownJSON); err == nil {
			return false, nil // Already migrated
		}
	}
	return true, nil
}

func (s *MoveConfigFilesStep) Execute(ctx *Context) error {
	state := &moveConfigFilesState{movedFiles: nil}

	// Files to move from root to mayor/ - use constant list for maintainability
	filesToMove := ConfigFilesToMove()

	for _, filename := range filesToMove {
		src := filepath.Join(ctx.TownRoot, filename)
		dst := filepath.Join(ctx.TownRoot, "mayor", filename)

		// Skip if source doesn't exist
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		// Skip if destination already exists
		if _, err := os.Stat(dst); err == nil {
			continue
		}

		// Move the file
		if err := os.Rename(src, dst); err != nil {
			// If rename fails (cross-device), copy and delete
			if err := copyAndDelete(src, dst); err != nil {
				return fmt.Errorf("moving %s: %w", filename, err)
			}
		}

		state.movedFiles = append(state.movedFiles, movedFile{src: src, dst: dst})
		ctx.Log("  Moved %s to mayor/%s\n", filename, filename)
	}

	// Store state for rollback
	ctx.SetStepState(s.ID(), state)
	return nil
}

func (s *MoveConfigFilesStep) Rollback(ctx *Context) error {
	// Retrieve state from context
	stateIface := ctx.GetStepState(s.ID())
	if stateIface == nil {
		return nil // No state, nothing to rollback
	}
	state, ok := stateIface.(*moveConfigFilesState)
	if !ok {
		ctx.Log("  Warning: unexpected state type in %s rollback, skipping\n", s.ID())
		return nil
	}

	// Move files back in reverse order
	for i := len(state.movedFiles) - 1; i >= 0; i-- {
		mf := state.movedFiles[i]
		if err := os.Rename(mf.dst, mf.src); err != nil {
			_ = copyAndDelete(mf.dst, mf.src)
		}
	}
	return nil
}

func (s *MoveConfigFilesStep) Verify(ctx *Context) error {
	// Verify town.json exists in mayor/
	newTownJSON := filepath.Join(ctx.TownRoot, "mayor", "town.json")
	if _, err := os.Stat(newTownJSON); err != nil {
		return fmt.Errorf("mayor/town.json not found: %w", err)
	}
	return nil
}

// CreateRigSettingsStep creates settings/ directories in each rig.
// The settings/ directory is a 0.2 feature for storing rig-specific configuration.
//
// Check: Returns true if any rig is missing its settings/ directory.
// Execute: Creates settings/ directory in each rig that doesn't have one.
// Rollback: Removes created directories only if they're empty.
// Verify: Confirms all rigs have settings/ directories.
type CreateRigSettingsStep struct {
	BaseStep
}

// createRigSettingsState holds rollback state for CreateRigSettingsStep.
type createRigSettingsState struct {
	createdDirs []string
}

func (s *CreateRigSettingsStep) ID() string {
	return "create-rig-settings"
}

func (s *CreateRigSettingsStep) Description() string {
	return "Create settings/ directories in rigs"
}

func (s *CreateRigSettingsStep) Check(ctx *Context) (bool, error) {
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		settingsDir := filepath.Join(rig, "settings")
		if _, err := os.Stat(settingsDir); os.IsNotExist(err) {
			return true, nil // At least one rig needs settings/
		}
	}
	return false, nil
}

func (s *CreateRigSettingsStep) Execute(ctx *Context) error {
	state := &createRigSettingsState{createdDirs: nil}

	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		settingsDir := filepath.Join(rig, "settings")
		if _, err := os.Stat(settingsDir); os.IsNotExist(err) {
			if err := os.MkdirAll(settingsDir, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", settingsDir, err)
			}
			state.createdDirs = append(state.createdDirs, settingsDir)
			ctx.Log("  Created %s/settings/\n", filepath.Base(rig))
		}
	}

	// Store state for rollback
	ctx.SetStepState(s.ID(), state)
	return nil
}

func (s *CreateRigSettingsStep) Rollback(ctx *Context) error {
	// Retrieve state from context
	stateIface := ctx.GetStepState(s.ID())
	if stateIface == nil {
		return nil
	}
	state, ok := stateIface.(*createRigSettingsState)
	if !ok {
		ctx.Log("  Warning: unexpected state type in %s rollback, skipping\n", s.ID())
		return nil
	}

	for _, dir := range state.createdDirs {
		// Only remove if empty
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}

func (s *CreateRigSettingsStep) Verify(ctx *Context) error {
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		settingsDir := filepath.Join(rig, "settings")
		if _, err := os.Stat(settingsDir); err != nil {
			return fmt.Errorf("%s/settings/ not found", filepath.Base(rig))
		}
	}
	return nil
}

// CreateRuntimeDirectoriesStep creates .runtime/ directories for gitignored state.
// The .runtime/ directory stores transient state that shouldn't be committed.
//
// Check: Returns true if town or any rig is missing its .runtime/ directory.
// Execute: Creates .runtime/ at town root and in each rig, updates .gitignore.
// Rollback: Removes created directories only if they're empty.
// Verify: Confirms town .runtime/ exists.
type CreateRuntimeDirectoriesStep struct {
	BaseStep
}

// createRuntimeDirectoriesState holds rollback state for CreateRuntimeDirectoriesStep.
type createRuntimeDirectoriesState struct {
	createdDirs []string
}

func (s *CreateRuntimeDirectoriesStep) ID() string {
	return "create-runtime-directories"
}

func (s *CreateRuntimeDirectoriesStep) Description() string {
	return "Create .runtime/ directories"
}

func (s *CreateRuntimeDirectoriesStep) Check(ctx *Context) (bool, error) {
	// Check town-level .runtime
	townRuntime := filepath.Join(ctx.TownRoot, ".runtime")
	if _, err := os.Stat(townRuntime); os.IsNotExist(err) {
		return true, nil
	}

	// Check rig-level .runtime
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		rigRuntime := filepath.Join(rig, ".runtime")
		if _, err := os.Stat(rigRuntime); os.IsNotExist(err) {
			return true, nil
		}
	}

	return false, nil
}

func (s *CreateRuntimeDirectoriesStep) Execute(ctx *Context) error {
	state := &createRuntimeDirectoriesState{createdDirs: nil}

	// Create town-level .runtime
	townRuntime := filepath.Join(ctx.TownRoot, ".runtime")
	if _, err := os.Stat(townRuntime); os.IsNotExist(err) {
		if err := os.MkdirAll(townRuntime, 0755); err != nil {
			return fmt.Errorf("creating town .runtime: %w", err)
		}
		state.createdDirs = append(state.createdDirs, townRuntime)
		ctx.Log("  Created .runtime/ at town root\n")
	}

	// Create rig-level .runtime directories
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		rigRuntime := filepath.Join(rig, ".runtime")
		if _, err := os.Stat(rigRuntime); os.IsNotExist(err) {
			if err := os.MkdirAll(rigRuntime, 0755); err != nil {
				return fmt.Errorf("creating %s/.runtime: %w", filepath.Base(rig), err)
			}
			state.createdDirs = append(state.createdDirs, rigRuntime)
			ctx.Log("  Created %s/.runtime/\n", filepath.Base(rig))
		}
	}

	// Ensure .runtime is in .gitignore
	if err := s.ensureGitignore(ctx.TownRoot); err != nil {
		ctx.Log("  Warning: could not update .gitignore: %v\n", err)
	}

	// Store state for rollback
	ctx.SetStepState(s.ID(), state)
	return nil
}

func (s *CreateRuntimeDirectoriesStep) ensureGitignore(townRoot string) error {
	gitignorePath := filepath.Join(townRoot, ".gitignore")

	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := string(data)
	if strings.Contains(content, ".runtime") {
		return nil // Already present
	}

	// Append .runtime to gitignore
	newContent := content
	if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += "# Gas Town runtime state (not tracked)\n.runtime/\n"

	return os.WriteFile(gitignorePath, []byte(newContent), 0644)
}

func (s *CreateRuntimeDirectoriesStep) Rollback(ctx *Context) error {
	// Retrieve state from context
	stateIface := ctx.GetStepState(s.ID())
	if stateIface == nil {
		return nil
	}
	state, ok := stateIface.(*createRuntimeDirectoriesState)
	if !ok {
		ctx.Log("  Warning: unexpected state type in %s rollback, skipping\n", s.ID())
		return nil
	}

	for _, dir := range state.createdDirs {
		// Only remove if empty
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}

func (s *CreateRuntimeDirectoriesStep) Verify(ctx *Context) error {
	townRuntime := filepath.Join(ctx.TownRoot, ".runtime")
	if _, err := os.Stat(townRuntime); err != nil {
		return fmt.Errorf("town .runtime/ not found")
	}
	return nil
}

// MigrateAgentBeadsStep migrates agent beads from gt-* to hq-* prefix.
// This changes the naming convention for mayor and deacon agent beads routing.
//
// Check: Returns true if routes.jsonl contains old gt-mayor or gt-deacon references.
// Execute: Replaces gt-mayor/gt-deacon with hq-mayor/hq-deacon in routes.jsonl.
// Rollback: Restores the original routes.jsonl content from saved state.
// Verify: Confirms no old gt-* agent references remain in routes.jsonl.
type MigrateAgentBeadsStep struct {
	BaseStep
}

// migrateAgentBeadsState holds rollback state for MigrateAgentBeadsStep.
// Stores original file content to enable safe rollback without data corruption.
type migrateAgentBeadsState struct {
	originalContent []byte
	routesPath      string
}

func (s *MigrateAgentBeadsStep) ID() string {
	return "migrate-agent-beads"
}

func (s *MigrateAgentBeadsStep) Description() string {
	return "Migrate agent beads to hq-* prefix"
}

func (s *MigrateAgentBeadsStep) Check(ctx *Context) (bool, error) {
	// Check if old-style agent beads exist
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return false, nil // No beads database
	}

	// Check routes.jsonl for old gt-mayor, gt-deacon patterns
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	data, err := os.ReadFile(routesPath)
	if err != nil {
		return false, nil // No routes file
	}

	// Look for old-style agent references
	if strings.Contains(string(data), `"gt-mayor"`) ||
		strings.Contains(string(data), `"gt-deacon"`) {
		return true, nil
	}

	return false, nil
}

func (s *MigrateAgentBeadsStep) Execute(ctx *Context) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routesPath := filepath.Join(beadsDir, "routes.jsonl")

	data, err := os.ReadFile(routesPath)
	if err != nil {
		return nil // No routes file to migrate
	}

	// Store original content for rollback
	state := &migrateAgentBeadsState{
		originalContent: data,
		routesPath:      routesPath,
	}
	ctx.SetStepState(s.ID(), state)

	// Replace old prefixes with new ones
	content := string(data)
	replacements := map[string]string{
		`"gt-mayor"`:  `"hq-mayor"`,
		`"gt-deacon"`: `"hq-deacon"`,
	}

	for oldPrefix, newPrefix := range replacements {
		content = strings.ReplaceAll(content, oldPrefix, newPrefix)
	}

	if err := os.WriteFile(routesPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("updating routes.jsonl: %w", err)
	}

	ctx.Log("  Updated beads routing for hq-* prefix\n")
	return nil
}

func (s *MigrateAgentBeadsStep) Rollback(ctx *Context) error {
	// Retrieve state from context - restore original content if available
	stateIface := ctx.GetStepState(s.ID())
	if stateIface == nil {
		return nil // No state, nothing to rollback
	}
	state, ok := stateIface.(*migrateAgentBeadsState)
	if !ok {
		ctx.Log("  Warning: unexpected state type in %s rollback, skipping\n", s.ID())
		return nil
	}
	if state.originalContent == nil {
		return nil
	}

	// Restore original file content
	return os.WriteFile(state.routesPath, state.originalContent, 0644)
}

func (s *MigrateAgentBeadsStep) Verify(ctx *Context) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routesPath := filepath.Join(beadsDir, "routes.jsonl")

	data, err := os.ReadFile(routesPath)
	if err != nil {
		return nil // No routes file is ok
	}

	// Verify old patterns are gone
	if strings.Contains(string(data), `"gt-mayor"`) ||
		strings.Contains(string(data), `"gt-deacon"`) {
		return fmt.Errorf("old gt-* agent beads still present")
	}

	return nil
}

// CleanupLegacyStep removes empty legacy .gastown/ directories.
// This is the final cleanup step, removing directories that are no longer needed.
//
// Check: Returns true if any .gastown/ directories exist at town or rig level.
// Execute: Removes .gastown/ directories only if they're empty.
// Rollback: No-op (can't undo deletion, but directories were empty anyway).
// Verify: Always passes (non-empty directories are intentionally left alone).
type CleanupLegacyStep struct {
	BaseStep
}

func (s *CleanupLegacyStep) ID() string {
	return "cleanup-legacy"
}

func (s *CleanupLegacyStep) Description() string {
	return "Clean up legacy directories"
}

func (s *CleanupLegacyStep) Check(ctx *Context) (bool, error) {
	// Check for .gastown directories
	townGastown := filepath.Join(ctx.TownRoot, ".gastown")
	if _, err := os.Stat(townGastown); err == nil {
		return true, nil
	}

	// Check rigs for .gastown
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		rigGastown := filepath.Join(rig, ".gastown")
		if _, err := os.Stat(rigGastown); err == nil {
			return true, nil
		}
	}

	return false, nil
}

func (s *CleanupLegacyStep) Execute(ctx *Context) error {
	// Remove empty .gastown directories
	townGastown := filepath.Join(ctx.TownRoot, ".gastown")
	if info, err := os.Stat(townGastown); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(townGastown)
		if len(entries) == 0 {
			_ = os.Remove(townGastown)
			ctx.Log("  Removed empty .gastown/ at town root\n")
		} else {
			ctx.Log("  Warning: .gastown/ not empty, skipping removal\n")
		}
	}

	// Remove .gastown from rigs
	rigs := detectRigs(ctx.TownRoot)
	for _, rig := range rigs {
		rigGastown := filepath.Join(rig, ".gastown")
		if info, err := os.Stat(rigGastown); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(rigGastown)
			if len(entries) == 0 {
				_ = os.Remove(rigGastown)
				ctx.Log("  Removed empty %s/.gastown/\n", filepath.Base(rig))
			}
		}
	}

	return nil
}

func (s *CleanupLegacyStep) Rollback(ctx *Context) error {
	// Can't undo deletion, but directories were empty anyway
	return nil
}

func (s *CleanupLegacyStep) Verify(ctx *Context) error {
	// Verification passes even if .gastown directories remain (they might have content)
	return nil
}

// Helper function to copy and delete a file
func copyAndDelete(src, dst string) error {
	// Read source
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Get source permissions
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Write destination
	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return err
	}

	// Verify the copy
	dstData, err := os.ReadFile(dst)
	if err != nil {
		return err
	}

	// Simple content verification
	if len(data) != len(dstData) {
		return fmt.Errorf("copy verification failed: size mismatch")
	}

	// Verify JSON validity for .json files
	if strings.HasSuffix(src, ".json") {
		var v interface{}
		if err := json.Unmarshal(dstData, &v); err != nil {
			return fmt.Errorf("copy verification failed: invalid JSON")
		}
	}

	// Delete source
	return os.Remove(src)
}
