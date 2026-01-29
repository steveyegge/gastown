package doctor

import (
	"fmt"
	"os"

	"github.com/steveyegge/gastown/internal/copilot"
)

// CopilotTrustCheck warns when the town root is not trusted by Copilot CLI.
type CopilotTrustCheck struct {
	FixableCheck
	configPath string
}

// NewCopilotTrustCheck creates a new Copilot trusted folders check.
func NewCopilotTrustCheck() *CopilotTrustCheck {
	return &CopilotTrustCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "copilot-trusted-folders",
				CheckDescription: "Verify Copilot config trusts the town root",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if Copilot config trusts the town root.
func (c *CopilotTrustCheck) Run(ctx *CheckContext) *CheckResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not resolve home directory for Copilot config",
			Details: []string{err.Error()},
			FixHint: "Set HOME and rerun 'gt doctor --fix'",
		}
	}

	configPath := copilot.ConfigPath(homeDir)
	c.configPath = configPath

	cfg, exists, err := copilot.LoadConfig(configPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Copilot config is invalid JSON",
			Details: []string{fmt.Sprintf("%s: %v", configPath, err)},
			FixHint: "Fix ~/.copilot/config.json then rerun 'gt doctor --fix'",
		}
	}

	if !exists || cfg == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Copilot config not found",
		}
	}

	for _, folder := range cfg.TrustedFolders {
		if folder == ctx.TownRoot {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "Copilot trusted_folders includes town root",
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Copilot trusted_folders missing town root",
		Details: []string{fmt.Sprintf("missing: %s", ctx.TownRoot)},
		FixHint: "Run 'gt doctor --fix' to add the town root to trusted_folders",
	}
}

// Fix adds the town root to trusted_folders if Copilot config exists.
func (c *CopilotTrustCheck) Fix(ctx *CheckContext) error {
	configPath := c.configPath
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving home directory: %w", err)
		}
		configPath = copilot.ConfigPath(homeDir)
	}

	updated, exists, err := copilot.EnsureTrustedFolder(configPath, ctx.TownRoot)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if !updated {
		return nil
	}
	return nil
}
