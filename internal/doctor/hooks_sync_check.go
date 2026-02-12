package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/hooks"
)

// HooksSyncCheck verifies all settings.local.json files match what gt hooks sync would generate.
type HooksSyncCheck struct {
	FixableCheck
	outOfSync []hooks.Target
}

// NewHooksSyncCheck creates a new hooks sync validation check.
func NewHooksSyncCheck() *HooksSyncCheck {
	return &HooksSyncCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hooks-sync",
				CheckDescription: "Verify hooks settings.local.json files are in sync",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Run checks all managed settings.local.json files for sync status.
func (c *HooksSyncCheck) Run(ctx *CheckContext) *CheckResult {
	c.outOfSync = nil

	targets, err := hooks.DiscoverTargets(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Failed to discover targets: %v", err),
			Category: c.Category(),
		}
	}

	var details []string
	for _, target := range targets {
		expected, err := hooks.ComputeExpected(target.Key)
		if err != nil {
			details = append(details, fmt.Sprintf("%s: error computing expected: %v", target.DisplayKey(), err))
			continue
		}

		current, err := hooks.LoadSettings(target.Path)
		if err != nil {
			details = append(details, fmt.Sprintf("%s: error loading: %v", target.DisplayKey(), err))
			continue
		}

		// Check if file exists
		_, statErr := os.Stat(target.Path)
		fileExists := statErr == nil

		if !fileExists || !hooks.HooksEqual(expected, &current.Hooks) {
			c.outOfSync = append(c.outOfSync, target)
			if !fileExists {
				details = append(details, fmt.Sprintf("%s: missing", target.DisplayKey()))
			} else {
				details = append(details, fmt.Sprintf("%s: out of sync", target.DisplayKey()))
			}
		}
	}

	if len(c.outOfSync) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  fmt.Sprintf("All %d hook targets in sync", len(targets)),
			Category: c.Category(),
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d target(s) out of sync", len(c.outOfSync)),
		Details:  details,
		FixHint:  "Run 'gt hooks sync' to regenerate settings.local.json files",
		Category: c.Category(),
	}
}

// Fix runs gt hooks sync to bring all targets into sync.
func (c *HooksSyncCheck) Fix(ctx *CheckContext) error {
	if len(c.outOfSync) == 0 {
		return nil
	}

	var errs []string
	for _, target := range c.outOfSync {
		expected, err := hooks.ComputeExpected(target.Key)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.DisplayKey(), err))
			continue
		}

		current, err := hooks.LoadSettings(target.Path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.DisplayKey(), err))
			continue
		}

		current.Hooks = *expected

		if current.EnabledPlugins == nil {
			current.EnabledPlugins = make(map[string]bool)
		}
		current.EnabledPlugins["beads@beads-marketplace"] = false

		claudeDir := filepath.Dir(target.Path)
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("%s: creating dir: %v", target.DisplayKey(), err))
			continue
		}

		data, err := hooks.MarshalSettings(current)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: marshal: %v", target.DisplayKey(), err))
			continue
		}
		data = append(data, '\n')

		if err := os.WriteFile(target.Path, data, 0644); err != nil {
			errs = append(errs, fmt.Sprintf("%s: write: %v", target.DisplayKey(), err))
			continue
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
