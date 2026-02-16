package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HooksPathAllRigsCheck verifies all clones across all rigs have core.hooksPath set.
// This runs globally (without --rig) to ensure the pre-push hook is active everywhere.
// The pre-push hook enforces integration branch landing guardrails.
type HooksPathAllRigsCheck struct {
	FixableCheck
	unconfiguredClones []string
}

// NewHooksPathAllRigsCheck creates a new global hooks path check.
func NewHooksPathAllRigsCheck() *HooksPathAllRigsCheck {
	return &HooksPathAllRigsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hooks-path-all-rigs",
				CheckDescription: "Check core.hooksPath is set for all clones across all rigs",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run checks all clones in all rigs for core.hooksPath configuration.
func (c *HooksPathAllRigsCheck) Run(ctx *CheckContext) *CheckResult {
	rigs := findAllRigs(ctx.TownRoot)
	if len(rigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs found",
		}
	}

	c.unconfiguredClones = nil
	totalClones := 0

	for _, rigPath := range rigs {
		clonePaths := findRigClones(rigPath)
		for _, clonePath := range clonePaths {
			// Skip if no .githooks directory (repo doesn't use hooks)
			if _, err := os.Stat(filepath.Join(clonePath, ".githooks")); os.IsNotExist(err) {
				continue
			}
			totalClones++

			cmd := exec.Command("git", "-C", clonePath, "config", "--get", "core.hooksPath")
			output, err := cmd.Output()
			if err != nil || strings.TrimSpace(string(output)) != ".githooks" {
				c.unconfiguredClones = append(c.unconfiguredClones, clonePath)
			}
		}
	}

	if len(c.unconfiguredClones) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d clone(s) have hooks configured", totalClones),
		}
	}

	var details []string
	for _, clonePath := range c.unconfiguredClones {
		relPath, _ := filepath.Rel(ctx.TownRoot, clonePath)
		if relPath == "" {
			relPath = clonePath
		}
		details = append(details, relPath)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d clone(s) missing core.hooksPath across all rigs", len(c.unconfiguredClones)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to configure hooks",
	}
}

// Fix configures core.hooksPath for all unconfigured clones.
func (c *HooksPathAllRigsCheck) Fix(ctx *CheckContext) error {
	for _, clonePath := range c.unconfiguredClones {
		cmd := exec.Command("git", "-C", clonePath, "config", "core.hooksPath", ".githooks")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to configure hooks for %s: %w", clonePath, err)
		}
	}
	return nil
}

// findRigClones returns all git clone paths within a rig.
func findRigClones(rigPath string) []string {
	var clones []string

	// Mayor clone
	clones = append(clones, filepath.Join(rigPath, "mayor", "rig"))
	// Refinery clone
	clones = append(clones, filepath.Join(rigPath, "refinery", "rig"))

	// Crew clones
	crewDir := filepath.Join(rigPath, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				clones = append(clones, filepath.Join(crewDir, entry.Name()))
			}
		}
	}

	// Polecat clones
	polecatDir := filepath.Join(rigPath, "polecats")
	if entries, err := os.ReadDir(polecatDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				// Polecats have nested structure: polecats/<name>/<rigname>/
				subDir := filepath.Join(polecatDir, entry.Name())
				if subEntries, err := os.ReadDir(subDir); err == nil {
					for _, subEntry := range subEntries {
						if subEntry.IsDir() {
							clones = append(clones, filepath.Join(subDir, subEntry.Name()))
						}
					}
				}
			}
		}
	}

	// Filter to only existing git repos
	var valid []string
	for _, clonePath := range clones {
		if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
			valid = append(valid, clonePath)
		}
	}
	return valid
}
