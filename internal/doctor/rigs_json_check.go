package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RigsJSONCheck verifies that rigs.json exists and the PrefixRegistry is populated.
// A missing rigs.json causes silent failures in session name parsing, crew cycling,
// nudge routing, and session-hygiene (which killed 7 sessions due to this).
type RigsJSONCheck struct {
	FixableCheck
	canonicalPath string
	fallbackPath  string
	townRoot      string
}

// NewRigsJSONCheck creates a new rigs.json existence check.
func NewRigsJSONCheck() *RigsJSONCheck {
	return &RigsJSONCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "rigs-json",
				CheckDescription: "Check that rigs.json exists for PrefixRegistry",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// CanFix returns true if the canonical path is missing but fallback exists.
func (c *RigsJSONCheck) CanFix() bool {
	if c.canonicalPath == "" {
		return false
	}
	// Can fix if canonical is missing but fallback exists (copy it back).
	if _, err := os.Stat(c.canonicalPath); os.IsNotExist(err) {
		if _, err := os.Stat(c.fallbackPath); err == nil {
			return true
		}
	}
	return false
}

// Fix copies rigs.json from fallback to canonical location.
func (c *RigsJSONCheck) Fix(ctx *CheckContext) error {
	src, err := os.Open(c.fallbackPath)
	if err != nil {
		return fmt.Errorf("reading fallback rigs.json: %w", err)
	}
	defer src.Close()

	// Ensure mayor directory exists
	mayorDir := filepath.Dir(c.canonicalPath)
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		return fmt.Errorf("creating mayor dir: %w", err)
	}

	dst, err := os.Create(c.canonicalPath)
	if err != nil {
		return fmt.Errorf("writing canonical rigs.json: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying rigs.json: %w", err)
	}
	return nil
}

// Run checks that rigs.json exists at the canonical or fallback location.
func (c *RigsJSONCheck) Run(ctx *CheckContext) *CheckResult {
	c.townRoot = ctx.TownRoot
	c.canonicalPath = filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	c.fallbackPath = filepath.Join(ctx.TownRoot, "rigs.json")

	// Check canonical location
	if _, err := os.Stat(c.canonicalPath); err == nil {
		// Also verify the fallback copy exists for resilience
		if _, err := os.Stat(c.fallbackPath); os.IsNotExist(err) {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusWarning,
				Message: "rigs.json exists but no fallback copy at town root",
				Details: []string{
					fmt.Sprintf("Canonical: %s (exists)", c.canonicalPath),
					fmt.Sprintf("Fallback: %s (missing)", c.fallbackPath),
					"Git operations in mayor/ can delete rigs.json",
				},
				FixHint: fmt.Sprintf("cp %s %s", c.canonicalPath, c.fallbackPath),
			}
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "rigs.json present with fallback copy",
		}
	}

	// Canonical missing — check fallback
	if _, err := os.Stat(c.fallbackPath); err == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "rigs.json missing from mayor/ (using fallback at town root)",
			Details: []string{
				fmt.Sprintf("Canonical: %s (MISSING)", c.canonicalPath),
				fmt.Sprintf("Fallback: %s (exists)", c.fallbackPath),
				"Likely deleted by git operation in mayor worktree",
			},
			FixHint: "Run 'gt doctor --fix' to restore from fallback",
		}
	}

	// Both missing — critical
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: "rigs.json not found — PrefixRegistry is empty, session parsing broken",
		Details: []string{
			fmt.Sprintf("Canonical: %s (MISSING)", c.canonicalPath),
			fmt.Sprintf("Fallback: %s (MISSING)", c.fallbackPath),
			"Session cycling, nudge routing, and session-hygiene will fail silently",
		},
		FixHint: "Restore rigs.json or run 'gt rig list' to regenerate",
	}
}
