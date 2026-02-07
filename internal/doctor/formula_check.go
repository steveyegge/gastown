package doctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/formula"
)

// FormulaCheck verifies that embedded formulas are accessible.
// Since formulas now resolve from embedded as a fallback, this check
// simply verifies the embedded formulas are available.
type FormulaCheck struct {
	BaseCheck
}

// NewFormulaCheck creates a new formula check.
func NewFormulaCheck() *FormulaCheck {
	return &FormulaCheck{
		BaseCheck: BaseCheck{
			CheckName:        "formulas",
			CheckDescription: "Check embedded formulas are accessible",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks if embedded formulas are accessible.
func (c *FormulaCheck) Run(ctx *CheckContext) *CheckResult {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not read embedded formulas: %v", err),
		}
	}

	if len(names) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No embedded formulas found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d embedded formulas available", len(names)),
	}
}

// LegacyProvisionedFormulasCheck detects and offers to clean up legacy
// provisioned formulas that match their embedded versions exactly.
// These were created by old versions of `gt install` and are now redundant.
type LegacyProvisionedFormulasCheck struct {
	FixableCheck
	legacyFormulas []string // paths to formulas that can be cleaned up
}

// NewLegacyProvisionedFormulasCheck creates a new legacy formula check.
func NewLegacyProvisionedFormulasCheck() *LegacyProvisionedFormulasCheck {
	return &LegacyProvisionedFormulasCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "legacy-formulas",
				CheckDescription: "Check for legacy provisioned formulas that match embedded",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run scans for legacy provisioned formulas that can be cleaned up.
func (c *LegacyProvisionedFormulasCheck) Run(ctx *CheckContext) *CheckResult {
	c.legacyFormulas = nil

	// Scan town-level formulas
	townFormulasDir := filepath.Join(ctx.TownRoot, ".beads", "formulas")
	c.scanForLegacyFormulas(townFormulasDir)

	// Scan rig-level formulas
	rigDirs := c.discoverRigDirs(ctx.TownRoot)
	for _, rigDir := range rigDirs {
		rigFormulasDir := filepath.Join(rigDir, ".beads", "formulas")
		c.scanForLegacyFormulas(rigFormulasDir)
	}

	if len(c.legacyFormulas) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No legacy provisioned formulas found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d legacy provisioned formulas that match embedded versions", len(c.legacyFormulas)),
		Details: c.legacyFormulas,
	}
}

// Fix removes the legacy provisioned formulas that match embedded exactly.
func (c *LegacyProvisionedFormulasCheck) Fix(ctx *CheckContext) error {
	if len(c.legacyFormulas) == 0 {
		return nil
	}

	var removed []string
	var errors []string

	for _, path := range c.legacyFormulas {
		if err := os.Remove(path); err != nil {
			errors = append(errors, fmt.Sprintf("  %s: %v", path, err))
		} else {
			removed = append(removed, path)
		}
	}

	if len(removed) > 0 {
		fmt.Printf("Removed %d legacy provisioned formulas:\n", len(removed))
		for _, p := range removed {
			fmt.Printf("  - %s\n", p)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove some formulas:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// scanForLegacyFormulas scans a directory for formulas that match embedded exactly
func (c *LegacyProvisionedFormulasCheck) scanForLegacyFormulas(formulasDir string) {
	entries, err := os.ReadDir(formulasDir)
	if err != nil {
		return // Directory doesn't exist, nothing to scan
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".formula.toml")

		// Check if this formula exists in embedded
		if !formula.EmbeddedFormulaExists(name) {
			continue // Custom formula, not legacy
		}

		// Compare content
		path := filepath.Join(formulasDir, entry.Name())
		localContent, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		embeddedContent, err := formula.GetEmbeddedFormula(name)
		if err != nil {
			continue
		}

		// If content matches exactly, it's a legacy provisioned formula
		if bytes.Equal(localContent, embeddedContent) {
			c.legacyFormulas = append(c.legacyFormulas, path)
		}
	}
}

// discoverRigDirs returns paths to all rig directories in the town
func (c *LegacyProvisionedFormulasCheck) discoverRigDirs(townRoot string) []string {
	var rigDirs []string

	// Read rigs.json to get registered rigs
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	content, err := os.ReadFile(rigsConfigPath)
	if err != nil {
		return rigDirs
	}

	// Simple JSON parsing for rig names
	lines := strings.Split(string(content), "\n")
	inRigs := false
	braceDepth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, `"rigs"`) {
			inRigs = true
			continue
		}
		if inRigs {
			if strings.Contains(trimmed, "{") {
				braceDepth++
			}
			if strings.Contains(trimmed, "}") {
				braceDepth--
				if braceDepth <= 0 {
					inRigs = false
				}
			}
			if braceDepth == 1 && strings.Contains(trimmed, `":`) {
				parts := strings.Split(trimmed, `"`)
				if len(parts) >= 2 {
					rigName := parts[1]
					if rigName != "" && rigName != "rigs" {
						rigPath := filepath.Join(townRoot, rigName)
						if info, err := os.Stat(rigPath); err == nil && info.IsDir() {
							rigDirs = append(rigDirs, rigPath)
						}
					}
				}
			}
		}
	}

	return rigDirs
}
