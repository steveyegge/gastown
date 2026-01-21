package doctor

import (
	"os/exec"
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
	path, err := exec.LookPath("sqlite3")
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "sqlite3 CLI not found",
			Details: []string{
				"sqlite3 is required for convoy features:",
				"  - gt convoy status (shows 0 tracked issues without it)",
				"  - gt sling duplicate convoy detection",
				"  - TUI convoy panels",
				"  - Daemon convoy completion detection",
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
