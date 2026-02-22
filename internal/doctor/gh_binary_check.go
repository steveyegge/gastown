package doctor

import (
	"fmt"
	"os/exec"
	"strings"
)

// GhBinaryCheck verifies that the GitHub CLI (gh) is installed.
// This is only required for rigs using fork workflow mode.
type GhBinaryCheck struct {
	BaseCheck
}

// NewGhBinaryCheck creates a new gh binary check.
func NewGhBinaryCheck() *GhBinaryCheck {
	return &GhBinaryCheck{
		BaseCheck: BaseCheck{
			CheckName:        "gh-binary",
			CheckDescription: "Check that GitHub CLI (gh) is installed (required for fork workflow)",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if gh is available in PATH and can execute.
func (c *GhBinaryCheck) Run(ctx *CheckContext) *CheckResult {
	// Check if gh is in PATH
	path, err := exec.LookPath("gh")
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "gh not installed (only needed for fork workflow)",
		}
	}

	// Check gh version
	cmd := exec.Command("gh", "--version")
	output, err := cmd.Output()
	if err != nil {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusWarning,
			Message: "gh found but version check failed",
			FixHint: "Try reinstalling: https://cli.github.com/",
		}
	}

	// Parse version from first line (format: "gh version X.Y.Z ...")
	version := strings.TrimSpace(string(output))
	if idx := strings.Index(version, "\n"); idx != -1 {
		version = version[:idx]
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%s (%s)", version, path),
	}
}
