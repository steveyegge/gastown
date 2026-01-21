package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/templates"
)

// OpenCodeCommandsCheck validates that town-level .opencode/commands/ is provisioned.
// OpenCode slash commands live in .opencode/commands and should exist at the town root.
type OpenCodeCommandsCheck struct {
	FixableCheck
	townRoot        string
	missingCommands []string
}

// NewOpenCodeCommandsCheck creates a new OpenCode commands check.
func NewOpenCodeCommandsCheck() *OpenCodeCommandsCheck {
	return &OpenCodeCommandsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "opencode-commands-provisioned",
				CheckDescription: "Check .opencode/commands/ is provisioned at town level",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if town-level OpenCode slash commands are provisioned.
func (c *OpenCodeCommandsCheck) Run(ctx *CheckContext) *CheckResult {
	c.townRoot = ctx.TownRoot
	c.missingCommands = nil

	missing, err := templates.MissingCommandsOpenCode(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Error checking OpenCode commands: %v", err),
		}
	}

	if len(missing) == 0 {
		names, _ := templates.CommandNamesOpenCode()
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Town-level OpenCode commands provisioned (%s)", strings.Join(names, ", ")),
		}
	}

	c.missingCommands = missing
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Missing town-level OpenCode commands: %s", strings.Join(missing, ", ")),
		Details: []string{
			fmt.Sprintf("Expected at: %s/.opencode/commands/", ctx.TownRoot),
			"OpenCode slash commands should be provisioned at the town root",
		},
		FixHint: "Run 'gt doctor --fix' to provision missing OpenCode commands",
	}
}

// Fix provisions missing OpenCode slash commands at town level.
func (c *OpenCodeCommandsCheck) Fix(ctx *CheckContext) error {
	if len(c.missingCommands) == 0 {
		return nil
	}

	return templates.ProvisionCommandsOpenCode(c.townRoot)
}
