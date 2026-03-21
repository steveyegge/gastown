package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/hooks"
)

// HooksSyncCheck verifies all settings.json files match what gt hooks sync would generate.
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
				CheckDescription: "Verify hooks settings.json files are in sync",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Run checks all managed settings.json files for sync status.
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
		if target.Provider == "gemini" {
			if detail := c.checkGeminiTarget(target); detail != "" {
				details = append(details, detail)
			}
			continue
		}

		// Claude targets: use base+override merge system
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
		FixHint:  "Run 'gt doctor --fix hooks-sync' to regenerate settings files",
		Category: c.Category(),
	}
}

// checkGeminiTarget compares an installed gemini settings file against the
// current template (with {{GT_BIN}} resolved). Returns a detail string if
// out of sync, or empty string if in sync.
func (c *HooksSyncCheck) checkGeminiTarget(target hooks.Target) string {
	expected, err := hooks.ComputeExpectedTemplate("gemini", "settings.json", target.Role)
	if err != nil {
		return fmt.Sprintf("%s: error computing expected template: %v", target.DisplayKey(), err)
	}

	actual, err := os.ReadFile(target.Path)
	if err != nil {
		c.outOfSync = append(c.outOfSync, target)
		return fmt.Sprintf("%s: cannot read: %v", target.DisplayKey(), err)
	}

	if !hooks.TemplateContentEqual(expected, actual) {
		c.outOfSync = append(c.outOfSync, target)
		return fmt.Sprintf("%s: out of sync", target.DisplayKey())
	}

	return ""
}

// Fix runs gt hooks sync to bring all targets into sync.
func (c *HooksSyncCheck) Fix(ctx *CheckContext) error {
	if len(c.outOfSync) == 0 {
		return nil
	}

	var errs []string
	for _, target := range c.outOfSync {
		if target.Provider == "gemini" {
			if err := c.fixGeminiTarget(target); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", target.DisplayKey(), err))
			}
			continue
		}

		// Claude targets: use base+override merge system
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

// fixGeminiTarget re-installs a gemini settings file from the current template.
func (c *HooksSyncCheck) fixGeminiTarget(target hooks.Target) error {
	content, err := hooks.ComputeExpectedTemplate("gemini", "settings.json", target.Role)
	if err != nil {
		return fmt.Errorf("computing template: %w", err)
	}

	dir := filepath.Dir(target.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}

	if err := os.WriteFile(target.Path, content, 0600); err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	return nil
}
