package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/opencode"
)

// HooksSyncCheck verifies all hooks files match what gt hooks sync would generate.
// Supports both Claude (.claude/settings.json) and OpenCode (.opencode/plugins/gastown.js).
type HooksSyncCheck struct {
	FixableCheck
	outOfSyncClaude   []hooks.Target
	outOfSyncOpenCode []OpenCodePluginTarget
}

// OpenCodePluginTarget represents an OpenCode plugin target for sync checking.
type OpenCodePluginTarget struct {
	Path string // Full path to .opencode/plugins/gastown.js
	Key  string // Override key: "gastown/crew", "mayor", etc.
	Rig  string // Rig name or empty for town-level
	Role string // crew, witness, refinery, polecats, mayor, deacon
}

// DisplayKey returns a human-readable label for the target.
func (t OpenCodePluginTarget) DisplayKey() string {
	if t.Rig != "" {
		return t.Rig + "/" + t.Role
	}
	return t.Role
}

// NewHooksSyncCheck creates a new hooks sync validation check.
func NewHooksSyncCheck() *HooksSyncCheck {
	return &HooksSyncCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hooks-sync",
				CheckDescription: "Verify hooks files are in sync (Claude and OpenCode)",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Run checks all managed hooks files for sync status.
func (c *HooksSyncCheck) Run(ctx *CheckContext) *CheckResult {
	c.outOfSyncClaude = nil
	c.outOfSyncOpenCode = nil

	// Check Claude targets
	claudeTargets, err := hooks.DiscoverTargets(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Failed to discover Claude targets: %v", err),
			Category: c.Category(),
		}
	}

	var details []string

	// Check each Claude target
	for _, target := range claudeTargets {
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
			c.outOfSyncClaude = append(c.outOfSyncClaude, target)
			if !fileExists {
				details = append(details, fmt.Sprintf("%s (claude): missing", target.DisplayKey()))
			} else {
				details = append(details, fmt.Sprintf("%s (claude): out of sync", target.DisplayKey()))
			}
		}
	}

	// Check OpenCode targets
	opencodeTargets := discoverOpenCodeTargets(ctx.TownRoot)
	for _, target := range opencodeTargets {
		expected, err := getExpectedOpenCodePlugin()
		if err != nil {
			details = append(details, fmt.Sprintf("%s (opencode): error computing expected: %v", target.DisplayKey(), err))
			continue
		}

		current, err := os.ReadFile(target.Path)
		if err != nil && !os.IsNotExist(err) {
			details = append(details, fmt.Sprintf("%s (opencode): error loading: %v", target.DisplayKey(), err))
			continue
		}

		fileExists := err == nil
		if !fileExists || !openCodePluginEqual(expected, current) {
			c.outOfSyncOpenCode = append(c.outOfSyncOpenCode, target)
			if !fileExists {
				details = append(details, fmt.Sprintf("%s (opencode): missing", target.DisplayKey()))
			} else {
				details = append(details, fmt.Sprintf("%s (opencode): out of sync", target.DisplayKey()))
			}
		}
	}

	totalTargets := len(claudeTargets) + len(opencodeTargets)
	totalOutOfSync := len(c.outOfSyncClaude) + len(c.outOfSyncOpenCode)

	if totalOutOfSync == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  fmt.Sprintf("All %d hook targets in sync (%d Claude, %d OpenCode)", totalTargets, len(claudeTargets), len(opencodeTargets)),
			Category: c.Category(),
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d target(s) out of sync (%d Claude, %d OpenCode)", totalOutOfSync, len(c.outOfSyncClaude), len(c.outOfSyncOpenCode)),
		Details:  details,
		FixHint:  "Run 'gt hooks sync' to regenerate hooks files",
		Category: c.Category(),
	}
}

// Fix runs gt hooks sync to bring all targets into sync.
func (c *HooksSyncCheck) Fix(ctx *CheckContext) error {
	if len(c.outOfSyncClaude) == 0 && len(c.outOfSyncOpenCode) == 0 {
		return nil
	}

	var errs []string

	// Fix Claude targets
	for _, target := range c.outOfSyncClaude {
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

		// Use filepath.Dir to get the directory, not string manipulation
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

	// Fix OpenCode targets
	for _, target := range c.outOfSyncOpenCode {
		expected, err := getExpectedOpenCodePlugin()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (opencode): %v", target.DisplayKey(), err))
			continue
		}

		// Create plugins directory if needed
		pluginsDir := filepath.Dir(target.Path)
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("%s (opencode): creating dir: %v", target.DisplayKey(), err))
			continue
		}

		// Write plugin file
		if err := os.WriteFile(target.Path, expected, 0644); err != nil {
			errs = append(errs, fmt.Sprintf("%s (opencode): write: %v", target.DisplayKey(), err))
			continue
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// discoverOpenCodeTargets finds all managed .opencode/plugins/gastown.js locations.
func discoverOpenCodeTargets(townRoot string) []OpenCodePluginTarget {
	var targets []OpenCodePluginTarget

	// Town-level targets
	if hasOpenCodeDir(filepath.Join(townRoot, "mayor")) {
		targets = append(targets, OpenCodePluginTarget{
			Path: filepath.Join(townRoot, "mayor", ".opencode", "plugins", "gastown.js"),
			Key:  "mayor",
			Role: "mayor",
		})
	}
	if hasOpenCodeDir(filepath.Join(townRoot, "deacon")) {
		targets = append(targets, OpenCodePluginTarget{
			Path: filepath.Join(townRoot, "deacon", ".opencode", "plugins", "gastown.js"),
			Key:  "deacon",
			Role: "deacon",
		})
	}

	// Scan rigs
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return targets
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "mayor" || entry.Name() == "deacon" ||
			entry.Name() == ".beads" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip directories that aren't rigs
		if !isRigDir(rigPath) {
			continue
		}

		// Rig-level
		if hasOpenCodeDir(rigPath) {
			targets = append(targets, OpenCodePluginTarget{
				Path: filepath.Join(rigPath, ".opencode", "plugins", "gastown.js"),
				Key:  rigName + "/rig",
				Rig:  rigName,
				Role: "rig",
			})
		}

		// Crew members
		crewDir := filepath.Join(rigPath, "crew")
		if info, err := os.Stat(crewDir); err == nil && info.IsDir() {
			if members, err := os.ReadDir(crewDir); err == nil {
				for _, m := range members {
					if m.IsDir() && !strings.HasPrefix(m.Name(), ".") {
						crewPath := filepath.Join(crewDir, m.Name())
						if hasOpenCodeDir(crewPath) {
							targets = append(targets, OpenCodePluginTarget{
								Path: filepath.Join(crewPath, ".opencode", "plugins", "gastown.js"),
								Key:  rigName + "/crew",
								Rig:  rigName,
								Role: "crew",
							})
						}
					}
				}
			}
		}

		// Polecats
		polecatsDir := filepath.Join(rigPath, "polecats")
		if info, err := os.Stat(polecatsDir); err == nil && info.IsDir() {
			if polecats, err := os.ReadDir(polecatsDir); err == nil {
				for _, p := range polecats {
					if p.IsDir() && !strings.HasPrefix(p.Name(), ".") {
						polecatPath := filepath.Join(polecatsDir, p.Name())
						if hasOpenCodeDir(polecatPath) {
							targets = append(targets, OpenCodePluginTarget{
								Path: filepath.Join(polecatPath, ".opencode", "plugins", "gastown.js"),
								Key:  rigName + "/polecats",
								Rig:  rigName,
								Role: "polecats",
							})
						}
					}
				}
			}
		}

		// Witness
		witnessDir := filepath.Join(rigPath, "witness")
		if info, err := os.Stat(witnessDir); err == nil && info.IsDir() {
			if hasOpenCodeDir(witnessDir) {
				targets = append(targets, OpenCodePluginTarget{
					Path: filepath.Join(witnessDir, ".opencode", "plugins", "gastown.js"),
					Key:  rigName + "/witness",
					Rig:  rigName,
					Role: "witness",
				})
			}
		}

		// Refinery
		refineryDir := filepath.Join(rigPath, "refinery")
		if info, err := os.Stat(refineryDir); err == nil && info.IsDir() {
			if hasOpenCodeDir(refineryDir) {
				targets = append(targets, OpenCodePluginTarget{
					Path: filepath.Join(refineryDir, ".opencode", "plugins", "gastown.js"),
					Key:  rigName + "/refinery",
					Rig:  rigName,
					Role: "refinery",
				})
			}
		}
	}

	return targets
}

// hasOpenCodeDir checks if a directory has an .opencode subdirectory.
func hasOpenCodeDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".opencode"))
	return err == nil && info.IsDir()
}

// isRigDir checks if a directory looks like a rig.
func isRigDir(path string) bool {
	for _, sub := range []string{"crew", "witness", "polecats", "refinery"} {
		info, err := os.Stat(filepath.Join(path, sub))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// getExpectedOpenCodePlugin returns the expected content of the gastown.js plugin.
func getExpectedOpenCodePlugin() ([]byte, error) {
	return opencode.GetPluginContent()
}

// openCodePluginEqual compares two plugin files for equality (ignoring whitespace differences).
func openCodePluginEqual(expected, actual []byte) bool {
	return strings.TrimSpace(string(expected)) == strings.TrimSpace(string(actual))
}
