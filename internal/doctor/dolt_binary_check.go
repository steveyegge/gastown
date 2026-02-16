package doctor

import (
	"fmt"
	"os/exec"
	"strings"
)

// DoltBinaryCheck verifies that the dolt binary is installed and accessible in PATH.
// Dolt is required for the beads storage backend (dolt sql-server).
type DoltBinaryCheck struct {
	BaseCheck
}

// NewDoltBinaryCheck creates a new dolt binary availability check.
func NewDoltBinaryCheck() *DoltBinaryCheck {
	return &DoltBinaryCheck{
		BaseCheck: BaseCheck{
			CheckName:        "dolt-binary",
			CheckDescription: "Check that dolt is installed and in PATH",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if dolt is available in PATH and reports its version.
func (c *DoltBinaryCheck) Run(ctx *CheckContext) *CheckResult {
	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: "dolt not found in PATH",
			Details: []string{
				"Dolt is required for the beads storage backend",
				"Install from: https://github.com/dolthub/dolt#installation",
			},
			FixHint: "Install dolt: https://github.com/dolthub/dolt#installation",
		}
	}

	// Get version for the OK message
	cmd := exec.Command(doltPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: fmt.Sprintf("dolt found at %s but 'dolt version' failed: %v", doltPath, err),
			Details: []string{
				strings.TrimSpace(string(output)),
			},
			FixHint: "Reinstall dolt: https://github.com/dolthub/dolt#installation",
		}
	}

	ver := strings.TrimSpace(string(output))
	// dolt version outputs "dolt version X.Y.Z"
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: ver,
	}
}
