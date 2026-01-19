package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
)

// BeadsCustomTypesCheck verifies that the beads database has Gas Town custom types configured.
// Without these types, creating beads with Gas Town-specific types (agent, molecule, etc.) fails.
type BeadsCustomTypesCheck struct {
	FixableCheck
}

// NewBeadsCustomTypesCheck creates a new beads custom types check.
func NewBeadsCustomTypesCheck() *BeadsCustomTypesCheck {
	return &BeadsCustomTypesCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "beads-custom-types",
				CheckDescription: "Check that beads has Gas Town custom types configured",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if the beads database has custom types configured.
func (c *BeadsCustomTypesCheck) Run(ctx *CheckContext) *CheckResult {
	// Skip if not in a rig context
	if ctx.RigName == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Not in a rig context (skipped)",
		}
	}

	// Resolve beads directory (follows redirects)
	beadsDir := beads.ResolveBeadsDir(ctx.RigPath())
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No beads database (skipped)",
		}
	}

	// Get current custom types configuration
	cmd := exec.Command("bd", "config", "get", "types.custom")
	cmd.Dir = ctx.RigPath()
	output, err := cmd.Output()
	if err != nil {
		// Command failed - types may not be configured at all
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not read beads types.custom config",
			FixHint: "Run 'gt doctor --fix' or 'bd config set types.custom \"" + constants.BeadsCustomTypes + "\"'",
		}
	}

	configuredTypes := strings.TrimSpace(string(output))
	if configuredTypes == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Beads custom types not configured",
			FixHint: "Run 'gt doctor --fix' or 'bd config set types.custom \"" + constants.BeadsCustomTypes + "\"'",
		}
	}

	// Check if all required types are present
	requiredTypes := constants.BeadsCustomTypesList()
	configuredSet := make(map[string]bool)
	for _, t := range strings.Split(configuredTypes, ",") {
		configuredSet[strings.TrimSpace(t)] = true
	}

	var missingTypes []string
	for _, required := range requiredTypes {
		if !configuredSet[required] {
			missingTypes = append(missingTypes, required)
		}
	}

	if len(missingTypes) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Missing %d custom type(s)", len(missingTypes)),
			Details: []string{
				"Missing types: " + strings.Join(missingTypes, ", "),
				"Current config: " + configuredTypes,
			},
			FixHint: "Run 'gt doctor --fix' to add missing types",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Beads custom types configured correctly",
	}
}

// Fix configures the beads database with all Gas Town custom types.
func (c *BeadsCustomTypesCheck) Fix(ctx *CheckContext) error {
	if ctx.RigName == "" {
		return nil // No rig context
	}

	// Resolve beads directory (follows redirects)
	beadsDir := beads.ResolveBeadsDir(ctx.RigPath())
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil // No beads database
	}

	// Set the custom types
	cmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	cmd.Dir = ctx.RigPath()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setting types.custom: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// BeadsCustomTypesTownCheck verifies that the town-level beads database has custom types configured.
type BeadsCustomTypesTownCheck struct {
	FixableCheck
}

// NewBeadsCustomTypesTownCheck creates a new town-level beads custom types check.
func NewBeadsCustomTypesTownCheck() *BeadsCustomTypesTownCheck {
	return &BeadsCustomTypesTownCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "town-beads-custom-types",
				CheckDescription: "Check that town beads has Gas Town custom types configured",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if the town-level beads database has custom types configured.
func (c *BeadsCustomTypesTownCheck) Run(ctx *CheckContext) *CheckResult {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No town-level beads database (skipped)",
		}
	}

	// Get current custom types configuration
	cmd := exec.Command("bd", "config", "get", "types.custom")
	cmd.Dir = ctx.TownRoot
	output, err := cmd.Output()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not read town beads types.custom config",
			FixHint: "Run 'gt doctor --fix' or 'bd config set types.custom \"" + constants.BeadsCustomTypes + "\"'",
		}
	}

	configuredTypes := strings.TrimSpace(string(output))
	if configuredTypes == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Town beads custom types not configured",
			FixHint: "Run 'gt doctor --fix' or 'bd config set types.custom \"" + constants.BeadsCustomTypes + "\"'",
		}
	}

	// Check if all required types are present
	requiredTypes := constants.BeadsCustomTypesList()
	configuredSet := make(map[string]bool)
	for _, t := range strings.Split(configuredTypes, ",") {
		configuredSet[strings.TrimSpace(t)] = true
	}

	var missingTypes []string
	for _, required := range requiredTypes {
		if !configuredSet[required] {
			missingTypes = append(missingTypes, required)
		}
	}

	if len(missingTypes) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Town beads missing %d custom type(s)", len(missingTypes)),
			Details: []string{
				"Missing types: " + strings.Join(missingTypes, ", "),
				"Current config: " + configuredTypes,
			},
			FixHint: "Run 'gt doctor --fix' to add missing types",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Town beads custom types configured correctly",
	}
}

// Fix configures the town-level beads database with all Gas Town custom types.
func (c *BeadsCustomTypesTownCheck) Fix(ctx *CheckContext) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil // No beads database
	}

	cmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	cmd.Dir = ctx.TownRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setting types.custom: %s", strings.TrimSpace(string(output)))
	}

	return nil
}
