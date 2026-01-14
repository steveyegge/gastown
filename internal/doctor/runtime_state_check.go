package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeStateCheck verifies the validity of .runtime/*.json files.
// These files contain transient state for agents (witness, refinery, namepool, etc.)
// and can become corrupted or stale after crashes.
type RuntimeStateCheck struct {
	FixableCheck
	invalidFiles []string // files that failed validation
	townRoot     string
}

// NewRuntimeStateCheck creates a new runtime state check.
func NewRuntimeStateCheck() *RuntimeStateCheck {
	return &RuntimeStateCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "runtime-state",
				CheckDescription: "Verify .runtime/*.json files are valid and not corrupted",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks if all runtime state files are valid JSON.
func (c *RuntimeStateCheck) Run(ctx *CheckContext) *CheckResult {
	c.townRoot = ctx.TownRoot
	c.invalidFiles = nil

	// Check town-level runtime
	c.checkRuntimeDir(ctx.TownRoot)

	// Check each rig's runtime directories
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsData, err := os.ReadFile(rigsPath)
	if err == nil {
		var rigs map[string]interface{}
		if json.Unmarshal(rigsData, &rigs) == nil {
			for rigName := range rigs {
				rigPath := filepath.Join(ctx.TownRoot, rigName)
				c.checkRuntimeDir(rigPath)

				// Check witness runtime
				witnessRuntimePath := filepath.Join(rigPath, "witness", ".runtime")
				c.checkRuntimeDir(witnessRuntimePath)

				// Check refinery runtime
				refineryRuntimePath := filepath.Join(rigPath, "refinery", "rig", ".runtime")
				c.checkRuntimeDir(refineryRuntimePath)

				// Check crew runtimes
				crewDir := filepath.Join(rigPath, "crew")
				if entries, err := os.ReadDir(crewDir); err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							c.checkRuntimeDir(filepath.Join(crewDir, entry.Name(), ".runtime"))
						}
					}
				}
			}
		}
	}

	if len(c.invalidFiles) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All runtime state files are valid",
		}
	}

	// Build relative paths for display
	var details []string
	for _, f := range c.invalidFiles {
		rel, _ := filepath.Rel(ctx.TownRoot, f)
		if rel == "" {
			rel = f
		}
		details = append(details, rel)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d runtime state file(s) are invalid or corrupted", len(c.invalidFiles)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to reset invalid runtime state files",
	}
}

// checkRuntimeDir checks all JSON files in a .runtime directory.
func (c *RuntimeStateCheck) checkRuntimeDir(basePath string) {
	runtimePath := basePath
	if !strings.HasSuffix(basePath, ".runtime") {
		runtimePath = filepath.Join(basePath, ".runtime")
	}

	entries, err := os.ReadDir(runtimePath)
	if err != nil {
		return // Directory doesn't exist or not accessible
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(runtimePath, entry.Name())
		if !c.isValidJSONFile(filePath) {
			c.invalidFiles = append(c.invalidFiles, filePath)
		}
	}
}

// isValidJSONFile checks if a file contains valid JSON.
func (c *RuntimeStateCheck) isValidJSONFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	// Empty file is invalid
	if len(data) == 0 {
		return false
	}

	// Try to parse as JSON
	var obj interface{}
	return json.Unmarshal(data, &obj) == nil
}

// Fix removes invalid runtime state files.
// This is safe because runtime state is transient and will be regenerated
// when the relevant service starts.
func (c *RuntimeStateCheck) Fix(ctx *CheckContext) error {
	for _, f := range c.invalidFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			rel, _ := filepath.Rel(ctx.TownRoot, f)
			return fmt.Errorf("failed to remove %s: %w", rel, err)
		}
	}
	return nil
}
