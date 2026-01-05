package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/templates"
)

// CommandsCheck validates that town-level .claude/commands/ is provisioned.
// All agents inherit these via Claude's directory traversal - no per-workspace copies needed.
type CommandsCheck struct {
	FixableCheck
	townRoot       string   // Cached for Fix
	missingCommands []string // Cached during Run for use in Fix
}

// NewCommandsCheck creates a new commands check.
func NewCommandsCheck() *CommandsCheck {
	return &CommandsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "commands-provisioned",
				CheckDescription: "Check .claude/commands/ is provisioned at town level",
			},
		},
	}
}

// Run checks if town-level slash commands are provisioned.
func (c *CommandsCheck) Run(ctx *CheckContext) *CheckResult {
	c.townRoot = ctx.TownRoot
	c.missingCommands = nil

	// Check town-level commands
	missing, err := templates.MissingCommands(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Error checking town-level commands: %v", err),
		}
	}

	if len(missing) == 0 {
		// Get command names for the success message
		names, _ := templates.CommandNames()
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Town-level slash commands provisioned (%s)", strings.Join(names, ", ")),
		}
	}

	c.missingCommands = missing
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Missing town-level slash commands: %s", strings.Join(missing, ", ")),
		Details: []string{
			fmt.Sprintf("Expected at: %s/.claude/commands/", ctx.TownRoot),
			"All agents inherit town-level commands via directory traversal",
		},
		FixHint: "Run 'gt doctor --fix' to provision missing commands",
	}
}

// Fix provisions missing slash commands at town level.
func (c *CommandsCheck) Fix(ctx *CheckContext) error {
	if len(c.missingCommands) == 0 {
		return nil
	}

	return templates.ProvisionCommands(c.townRoot)
}
