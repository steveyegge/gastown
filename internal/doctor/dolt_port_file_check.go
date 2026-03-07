package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/doltserver"
)

// DoltPortFileCheck verifies that all rig .beads/ directories have a
// dolt-server.port file pointing to the shared server's port. Port file
// drift causes bd to think no server is running and spawn orphan embedded
// Dolt servers with empty databases.
type DoltPortFileCheck struct {
	FixableCheck
}

// NewDoltPortFileCheck creates a new dolt port file check.
func NewDoltPortFileCheck() *DoltPortFileCheck {
	return &DoltPortFileCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "dolt-port-files",
				CheckDescription: "Check that all rig .beads/ dirs have correct dolt-server.port",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks if all rig .beads/dolt-server.port files point to the shared server port.
func (c *DoltPortFileCheck) Run(ctx *CheckContext) *CheckResult {
	config := doltserver.DefaultConfig(ctx.TownRoot)

	// Only meaningful for local servers
	if config.IsRemote() {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "remote server — port files not applicable",
		}
	}

	// Check if server is actually running
	running, _, err := doltserver.IsRunning(ctx.TownRoot)
	if err != nil || !running {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "server not running — skipped",
		}
	}

	drifted := doltserver.CheckPortFiles(ctx.TownRoot, config.Port)
	if len(drifted) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("all port files point to %d", config.Port),
		}
	}

	// Build details
	var details []string
	for _, d := range drifted {
		if d.CurrentPort == 0 {
			details = append(details, fmt.Sprintf("%s: missing", shortPath(ctx.TownRoot, d.BeadsDir)))
		} else {
			details = append(details, fmt.Sprintf("%s: %d (expected %d)", shortPath(ctx.TownRoot, d.BeadsDir), d.CurrentPort, d.ExpectedPort))
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d port file(s) drifted from shared port %d", len(drifted), config.Port),
		Details: details,
		FixHint: "Run 'gt doctor --fix' or 'gt dolt start' to sync port files",
	}
}

// Fix syncs all port files to the shared server port.
func (c *DoltPortFileCheck) Fix(ctx *CheckContext) error {
	config := doltserver.DefaultConfig(ctx.TownRoot)
	return doltserver.SyncPortFiles(ctx.TownRoot, config.Port)
}

// shortPath returns a path relative to townRoot for display.
func shortPath(townRoot, path string) string {
	if strings.HasPrefix(path, townRoot+"/") {
		return path[len(townRoot)+1:]
	}
	return path
}
