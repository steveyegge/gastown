package doctor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
)

// TownBeadsConfigCheck verifies town-level .beads/config.yaml exists when
// town beads are enabled.
type TownBeadsConfigCheck struct {
	FixableCheck
	missingConfig bool
}

// NewTownBeadsConfigCheck creates a town-level beads config check.
func NewTownBeadsConfigCheck() *TownBeadsConfigCheck {
	return &TownBeadsConfigCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "town-beads-config",
				CheckDescription: "Verify town .beads/config.yaml exists when beads are enabled",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if town-level config.yaml exists when town .beads exists.
func (c *TownBeadsConfigCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingConfig = false

	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No town .beads directory (beads not configured)",
			Category: c.CheckCategory,
		}
	} else if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Could not access town .beads directory: %v", err),
			Category: c.CheckCategory,
		}
	}

	configPath := filepath.Join(beadsDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.missingConfig = true
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusError,
			Message:  "Missing town .beads/config.yaml",
			Details:  []string{"Fix will create config.yaml without modifying existing metadata or configs."},
			FixHint:  "Run 'gt doctor --fix' to create config.yaml",
			Category: c.CheckCategory,
		}
	} else if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Could not read town .beads/config.yaml: %v", err),
			Category: c.CheckCategory,
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusOK,
		Message:  "Town beads config.yaml present",
		Category: c.CheckCategory,
	}
}

// Fix creates town-level .beads/config.yaml only when missing.
func (c *TownBeadsConfigCheck) Fix(ctx *CheckContext) error {
	if !c.missingConfig {
		return nil
	}
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	return beads.EnsureConfigYAMLFromMetadataIfMissing(beadsDir, "hq")
}
