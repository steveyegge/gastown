package doctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// BeadsDatabaseCheck verifies that the beads database is properly initialized.
// It detects when issues.db is empty or missing critical columns, and can
// auto-fix by triggering a re-import from the JSONL file.
type BeadsDatabaseCheck struct {
	FixableCheck
}

// NewBeadsDatabaseCheck creates a new beads database check.
func NewBeadsDatabaseCheck() *BeadsDatabaseCheck {
	return &BeadsDatabaseCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "beads-database",
				CheckDescription: "Verify beads database is properly initialized",
			},
		},
	}
}

// Run checks if the beads database is properly initialized.
func (c *BeadsDatabaseCheck) Run(ctx *CheckContext) *CheckResult {
	// Check town-level beads
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No .beads directory found at town root",
			FixHint: "Run 'bd init' to initialize beads",
		}
	}

	// Check if issues.db exists and has content
	issuesDB := filepath.Join(beadsDir, "issues.db")
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")

	dbInfo, dbErr := os.Stat(issuesDB)
	jsonlInfo, jsonlErr := os.Stat(issuesJSONL)

	// If no database file, that's OK - beads will create it
	if os.IsNotExist(dbErr) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No issues.db file (will be created on first use)",
		}
	}

	// If database file is empty but JSONL has content, this is the bug
	if dbErr == nil && dbInfo.Size() == 0 {
		if jsonlErr == nil && jsonlInfo.Size() > 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusError,
				Message: "issues.db is empty but issues.jsonl has content",
				Details: []string{
					"This can cause 'table issues has no column named pinned' errors",
					"The database needs to be rebuilt from the JSONL file",
				},
				FixHint: "Run 'gt doctor --fix' or delete issues.db and run 'bd sync --from-main'",
			}
		}
	}

	// Also check rig-level beads if a rig is specified
	// Follows redirect if present (rig root may redirect to mayor/rig/.beads)
	if ctx.RigName != "" {
		rigBeadsDir := beads.ResolveBeadsDir(ctx.RigPath())
		if _, err := os.Stat(rigBeadsDir); err == nil {
			rigDB := filepath.Join(rigBeadsDir, "issues.db")
			rigJSONL := filepath.Join(rigBeadsDir, "issues.jsonl")

			rigDBInfo, rigDBErr := os.Stat(rigDB)
			rigJSONLInfo, rigJSONLErr := os.Stat(rigJSONL)

			if rigDBErr == nil && rigDBInfo.Size() == 0 {
				if rigJSONLErr == nil && rigJSONLInfo.Size() > 0 {
					return &CheckResult{
						Name:    c.Name(),
						Status:  StatusError,
						Message: "Rig issues.db is empty but issues.jsonl has content",
						Details: []string{
							"Rig: " + ctx.RigName,
							"This can cause 'table issues has no column named pinned' errors",
						},
						FixHint: "Run 'gt doctor --fix' or delete the rig's issues.db",
					}
				}
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Beads database is properly initialized",
	}
}

// Fix attempts to rebuild the database from JSONL.
func (c *BeadsDatabaseCheck) Fix(ctx *CheckContext) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	issuesDB := filepath.Join(beadsDir, "issues.db")
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")

	// Check if we need to fix town-level database
	dbInfo, dbErr := os.Stat(issuesDB)
	jsonlInfo, jsonlErr := os.Stat(issuesJSONL)

	if dbErr == nil && dbInfo.Size() == 0 && jsonlErr == nil && jsonlInfo.Size() > 0 {
		// Delete the empty database file
		if err := os.Remove(issuesDB); err != nil {
			return err
		}

		// Run bd sync to rebuild from JSONL
		cmd := beads.Command(ctx.TownRoot, "sync", "--from-main")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	// Also fix rig-level if specified (follows redirect if present)
	if ctx.RigName != "" {
		rigBeadsDir := beads.ResolveBeadsDir(ctx.RigPath())
		rigDB := filepath.Join(rigBeadsDir, "issues.db")
		rigJSONL := filepath.Join(rigBeadsDir, "issues.jsonl")

		rigDBInfo, rigDBErr := os.Stat(rigDB)
		rigJSONLInfo, rigJSONLErr := os.Stat(rigJSONL)

		if rigDBErr == nil && rigDBInfo.Size() == 0 && rigJSONLErr == nil && rigJSONLInfo.Size() > 0 {
			if err := os.Remove(rigDB); err != nil {
				return err
			}

			cmd := beads.Command(ctx.RigPath(), "sync", "--from-main")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
	}

	return nil
}

// PrefixConflictCheck detects duplicate prefixes across rigs in routes.jsonl.
// Duplicate prefixes break prefix-based routing.
type PrefixConflictCheck struct {
	BaseCheck
}

// NewPrefixConflictCheck creates a new prefix conflict check.
func NewPrefixConflictCheck() *PrefixConflictCheck {
	return &PrefixConflictCheck{
		BaseCheck: BaseCheck{
			CheckName:        "prefix-conflict",
			CheckDescription: "Check for duplicate beads prefixes across rigs",
		},
	}
}

// Run checks for duplicate prefixes in routes.jsonl.
func (c *PrefixConflictCheck) Run(ctx *CheckContext) *CheckResult {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")

	// Check if routes.jsonl exists
	routesPath := filepath.Join(beadsDir, beads.RoutesFileName)
	if _, err := os.Stat(routesPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No routes.jsonl file (prefix routing not configured)",
		}
	}

	// Find conflicts
	conflicts, err := beads.FindConflictingPrefixes(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not check routes.jsonl: %v", err),
		}
	}

	if len(conflicts) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No prefix conflicts found",
		}
	}

	// Build details
	var details []string
	for prefix, paths := range conflicts {
		details = append(details, fmt.Sprintf("Prefix %q used by: %s", prefix, strings.Join(paths, ", ")))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("%d prefix conflict(s) found in routes.jsonl", len(conflicts)),
		Details: details,
		FixHint: "Use 'bd rename-prefix <new-prefix>' in one of the conflicting rigs to resolve",
	}
}
