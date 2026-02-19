package doctor

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/tmuxinator"
)

// TmuxinatorCheck verifies that tmuxinator is installed.
// Tmuxinator provides declarative YAML-based session configuration
// and is preferred over raw tmux calls for session creation.
type TmuxinatorCheck struct {
	BaseCheck
}

// NewTmuxinatorCheck creates a new tmuxinator availability check.
func NewTmuxinatorCheck() *TmuxinatorCheck {
	return &TmuxinatorCheck{
		BaseCheck: BaseCheck{
			CheckName:        "tmuxinator",
			CheckDescription: "Check if tmuxinator is installed for declarative session management",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if tmuxinator is available on PATH.
func (c *TmuxinatorCheck) Run(ctx *CheckContext) *CheckResult {
	if !tmuxinator.IsAvailable() {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "tmuxinator not installed (sessions will use raw tmux fallback)",
			FixHint: "Install with: gem install tmuxinator",
		}
	}

	version, err := tmuxinator.Version()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "tmuxinator found but version check failed",
			Details: []string{err.Error()},
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("tmuxinator %s", version),
	}
}
