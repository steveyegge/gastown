package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Sqlite3Check verifies that sqlite3 CLI is available.
// sqlite3 is required for convoy-related database queries including:
// - gt convoy status (tracking issue progress)
// - gt sling duplicate convoy detection
// - TUI convoy panels
// - Daemon convoy completion detection
type Sqlite3Check struct {
	BaseCheck
}

// NewSqlite3Check creates a new sqlite3 availability check.
func NewSqlite3Check() *Sqlite3Check {
	return &Sqlite3Check{
		BaseCheck: BaseCheck{
			CheckName:        "sqlite3-available",
			CheckDescription: "Check sqlite3 CLI is installed (required for convoy features)",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if sqlite3 is available in PATH.
func (c *Sqlite3Check) Run(ctx *CheckContext) *CheckResult {
	// Detect backend to determine if sqlite3 is actually required
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		// No beads directory, skip check
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No beads database (skipped)",
		}
	}

	backend, err := detectBackend(beadsDir)
	if err != nil {
		// Can't detect backend, assume SQLite for backwards compatibility
		backend = "sqlite"
	}

	path, lookupErr := exec.LookPath("sqlite3")
	if lookupErr != nil {
		// sqlite3 not found
		if backend == "dolt" {
			// Not required for Dolt backend
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "sqlite3 not required (using Dolt backend)",
				Details: []string{
					"Note: The convoy watcher and TUI features have been updated",
					"to use bd CLI instead of direct sqlite3 access.",
				},
			}
		}

		// Required for SQLite backend
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "sqlite3 CLI not found",
			Details: []string{
				"sqlite3 is required for SQLite backend features:",
				"  - gt convoy status",
				"  - gt sling duplicate convoy detection",
				"  - TUI convoy panels",
				"  - Daemon convoy completion detection",
				"Note: These features are being migrated to use bd CLI instead.",
			},
			FixHint: "Install sqlite3: apt install sqlite3 (Debian/Ubuntu) or brew install sqlite3 (macOS)",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "sqlite3 found at " + path,
	}
}
