package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/opencode"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	hooksSyncDryRun   bool
	hooksSyncProvider string
)

var hooksSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Regenerate all hooks configuration files",
	Long: `Regenerate all hooks configuration files from templates.

For Claude (.claude/settings.json):
  For each target (mayor, deacon, rig/crew, rig/witness, etc.):
  1. Load base config
  2. Apply role override (if exists)
  3. Apply rig+role override (if exists)
  4. Merge hooks section into existing settings.json (preserving all fields)
  5. Write updated settings.json

For OpenCode (.opencode/plugins/gastown.js):
  Installs the Gas Town plugin to all agents with .opencode directories.

Examples:
  gt hooks sync                    # Regenerate all hooks files
  gt hooks sync --provider claude  # Regenerate only Claude hooks
  gt hooks sync --provider opencode # Regenerate only OpenCode plugins
  gt hooks sync --dry-run          # Show what would change without writing`,
	RunE: runHooksSync,
}

func init() {
	hooksCmd.AddCommand(hooksSyncCmd)
	hooksSyncCmd.Flags().BoolVar(&hooksSyncDryRun, "dry-run", false, "Show what would change without writing")
	hooksSyncCmd.Flags().StringVar(&hooksSyncProvider, "provider", "", "Provider to sync (claude, opencode, or empty for both)")
}

func runHooksSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Validate provider
	provider := strings.ToLower(hooksSyncProvider)
	if provider != "" && provider != "claude" && provider != "opencode" {
		return fmt.Errorf("invalid provider %q: must be 'claude', 'opencode', or empty", hooksSyncProvider)
	}

	if hooksSyncDryRun {
		fmt.Println("Dry run - showing what would change...")
		fmt.Println()
	} else {
		fmt.Println("Syncing hooks...")
	}

	allOK := true

	// Sync Claude hooks if provider is empty or "claude"
	if provider == "" || provider == "claude" {
		if err := syncClaudeHooks(townRoot); err != nil {
			fmt.Printf("%s Claude sync error: %v\n", style.Error.Render("Error:"), err)
			allOK = false
		}
	}

	// Sync OpenCode plugins if provider is empty or "opencode"
	if provider == "" || provider == "opencode" {
		if err := syncOpenCodePlugins(townRoot); err != nil {
			fmt.Printf("%s OpenCode sync error: %v\n", style.Error.Render("Error:"), err)
			allOK = false
		}
	}

	if !allOK {
		return fmt.Errorf("some sync operations failed")
	}

	return nil
}

// syncClaudeHooks syncs all Claude hooks configuration files.
func syncClaudeHooks(townRoot string) error {
	targets, err := hooks.DiscoverTargets(townRoot)
	if err != nil {
		return fmt.Errorf("discovering Claude targets: %w", err)
	}

	updated := 0
	unchanged := 0
	created := 0
	errors := 0

	for _, target := range targets {
		result, err := syncClaudeTarget(target, hooksSyncDryRun)
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Error.Render("✖"), target.DisplayKey(), err)
			errors++
			continue
		}

		relPath, pathErr := filepath.Rel(townRoot, target.Path)
		if pathErr != nil {
			relPath = target.Path
		}

		switch result {
		case syncCreated:
			if hooksSyncDryRun {
				fmt.Printf("  %s %s %s\n", style.Warning.Render("~"), relPath, style.Dim.Render("(would create)"))
			} else {
				fmt.Printf("  %s %s %s\n", style.Success.Render("✓"), relPath, style.Dim.Render("(created)"))
			}
			created++
		case syncUpdated:
			if hooksSyncDryRun {
				fmt.Printf("  %s %s %s\n", style.Warning.Render("~"), relPath, style.Dim.Render("(would update)"))
			} else {
				fmt.Printf("  %s %s %s\n", style.Success.Render("✓"), relPath, style.Dim.Render("(updated)"))
			}
			updated++
		case syncUnchanged:
			fmt.Printf("  %s %s %s\n", style.Dim.Render("·"), relPath, style.Dim.Render("(unchanged)"))
			unchanged++
		}
	}

	// Summary
	fmt.Println()
	total := updated + unchanged + created + errors
	providerLabel := "Claude"
	if hooksSyncDryRun {
		fmt.Printf("Would sync %d %s targets (%d to create, %d to update, %d unchanged",
			total, providerLabel, created, updated, unchanged)
	} else {
		fmt.Printf("Synced %d %s targets (%d created, %d updated, %d unchanged",
			total, providerLabel, created, updated, unchanged)
	}
	if errors > 0 {
		fmt.Printf(", %s", style.Error.Render(fmt.Sprintf("%d errors", errors)))
	}
	fmt.Println(")")

	return nil
}

// syncOpenCodePlugins syncs all OpenCode plugin files.
func syncOpenCodePlugins(townRoot string) error {
	targets := discoverOpenCodeSyncTargets(townRoot)

	updated := 0
	unchanged := 0
	created := 0
	errors := 0

	for _, target := range targets {
		result, err := syncOpenCodeTarget(target, hooksSyncDryRun)
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Error.Render("✖"), target.DisplayKey(), err)
			errors++
			continue
		}

		relPath, pathErr := filepath.Rel(townRoot, target.Path)
		if pathErr != nil {
			relPath = target.Path
		}

		switch result {
		case syncCreated:
			if hooksSyncDryRun {
				fmt.Printf("  %s %s %s\n", style.Warning.Render("~"), relPath, style.Dim.Render("(would create)"))
			} else {
				fmt.Printf("  %s %s %s\n", style.Success.Render("✓"), relPath, style.Dim.Render("(created)"))
			}
			created++
		case syncUpdated:
			if hooksSyncDryRun {
				fmt.Printf("  %s %s %s\n", style.Warning.Render("~"), relPath, style.Dim.Render("(would update)"))
			} else {
				fmt.Printf("  %s %s %s\n", style.Success.Render("✓"), relPath, style.Dim.Render("(updated)"))
			}
			updated++
		case syncUnchanged:
			fmt.Printf("  %s %s %s\n", style.Dim.Render("·"), relPath, style.Dim.Render("(unchanged)"))
			unchanged++
		}
	}

	// Summary
	fmt.Println()
	total := updated + unchanged + created + errors
	providerLabel := "OpenCode"
	if hooksSyncDryRun {
		fmt.Printf("Would sync %d %s targets (%d to create, %d to update, %d unchanged",
			total, providerLabel, created, updated, unchanged)
	} else {
		fmt.Printf("Synced %d %s targets (%d created, %d updated, %d unchanged",
			total, providerLabel, created, updated, unchanged)
	}
	if errors > 0 {
		fmt.Printf(", %s", style.Error.Render(fmt.Sprintf("%d errors", errors)))
	}
	fmt.Println(")")

	return nil
}

type syncResult int

const (
	syncUnchanged syncResult = iota
	syncUpdated
	syncCreated
)

// OpenCodeSyncTarget represents an OpenCode plugin target for syncing.
type OpenCodeSyncTarget struct {
	Path string // Full path to .opencode/plugins/gastown.js
	Key  string // Override key: "gastown/crew", "mayor", etc.
	Rig  string // Rig name or empty for town-level
	Role string // crew, witness, refinery, polecats, mayor, deacon
}

// DisplayKey returns a human-readable label for the target.
func (t OpenCodeSyncTarget) DisplayKey() string {
	if t.Rig != "" {
		return t.Rig + "/" + t.Role
	}
	return t.Role
}

// syncClaudeTarget syncs a single Claude target's .claude/settings.json.
func syncClaudeTarget(target hooks.Target, dryRun bool) (syncResult, error) {
	// Compute expected hooks for this target
	expected, err := hooks.ComputeExpected(target.Key)
	if err != nil {
		return 0, fmt.Errorf("computing expected config: %w", err)
	}

	// Load existing settings (returns zero-value if file doesn't exist)
	current, err := hooks.LoadSettings(target.Path)
	if err != nil {
		return 0, fmt.Errorf("loading current settings: %w", err)
	}

	// Check if the file exists
	_, statErr := os.Stat(target.Path)
	fileExists := statErr == nil

	// Compare hooks sections
	if fileExists && hooks.HooksEqual(expected, &current.Hooks) {
		return syncUnchanged, nil
	}

	if dryRun {
		if fileExists {
			return syncUpdated, nil
		}
		return syncCreated, nil
	}

	// Update hooks section, preserving all other fields (including unknown ones)
	current.Hooks = *expected

	// Ensure enabledPlugins map exists with beads disabled (Gas Town standard)
	if current.EnabledPlugins == nil {
		current.EnabledPlugins = make(map[string]bool)
	}
	current.EnabledPlugins["beads@beads-marketplace"] = false

	// Create .claude directory if needed
	claudeDir := filepath.Dir(target.Path)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return 0, fmt.Errorf("creating .claude directory: %w", err)
	}

	// Write settings.json using MarshalSettings to preserve unknown fields
	data, err := hooks.MarshalSettings(current)
	if err != nil {
		return 0, fmt.Errorf("marshaling settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(target.Path, data, 0644); err != nil {
		return 0, fmt.Errorf("writing settings: %w", err)
	}

	if fileExists {
		return syncUpdated, nil
	}
	return syncCreated, nil
}

// syncOpenCodeTarget syncs a single OpenCode target's plugin file.
func syncOpenCodeTarget(target OpenCodeSyncTarget, dryRun bool) (syncResult, error) {
	// Get expected plugin content
	expected, err := opencode.GetPluginContent()
	if err != nil {
		return 0, fmt.Errorf("getting expected plugin content: %w", err)
	}

	// Check if file exists and read current content
	current, err := os.ReadFile(target.Path)
	fileExists := err == nil

	if fileExists {
		// Compare content
		if strings.TrimSpace(string(expected)) == strings.TrimSpace(string(current)) {
			return syncUnchanged, nil
		}
	}

	if dryRun {
		if fileExists {
			return syncUpdated, nil
		}
		return syncCreated, nil
	}

	// Create plugins directory if needed
	pluginsDir := filepath.Dir(target.Path)
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return 0, fmt.Errorf("creating plugins directory: %w", err)
	}

	// Write plugin file
	if err := os.WriteFile(target.Path, expected, 0644); err != nil {
		return 0, fmt.Errorf("writing plugin file: %w", err)
	}

	// Also ensure package.json exists
	opencodeDir := filepath.Dir(pluginsDir)
	packageJsonPath := filepath.Join(opencodeDir, "package.json")
	if _, err := os.Stat(packageJsonPath); os.IsNotExist(err) {
		// Use opencode.EnsurePluginAt to create package.json and install dependencies
		if err := opencode.EnsurePluginAt(opencodeDir, "plugins", "gastown.js"); err != nil {
			// Don't fail if dependency installation fails, just warn
			fmt.Printf("  %s Warning: %v\n", style.Warning.Render("⚠"), err)
		}
	}

	if fileExists {
		return syncUpdated, nil
	}
	return syncCreated, nil
}

// discoverOpenCodeSyncTargets finds all OpenCode plugin locations for syncing.
// This discovers all .opencode directories that exist in the workspace.
func discoverOpenCodeSyncTargets(townRoot string) []OpenCodeSyncTarget {
	var targets []OpenCodeSyncTarget

	// Town-level targets
	for _, agent := range []string{"mayor", "deacon"} {
		agentPath := filepath.Join(townRoot, agent)
		if hasOpenCodeDir(agentPath) {
			targets = append(targets, OpenCodeSyncTarget{
				Path: filepath.Join(agentPath, ".opencode", "plugins", "gastown.js"),
				Key:  agent,
				Role: agent,
			})
		}
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
		if !isRigForSync(rigPath) {
			continue
		}

		// Rig-level
		if hasOpenCodeDir(rigPath) {
			targets = append(targets, OpenCodeSyncTarget{
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
							targets = append(targets, OpenCodeSyncTarget{
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
							targets = append(targets, OpenCodeSyncTarget{
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
				targets = append(targets, OpenCodeSyncTarget{
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
				targets = append(targets, OpenCodeSyncTarget{
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

// isRigForSync checks if a directory looks like a rig.
func isRigForSync(path string) bool {
	for _, sub := range []string{"crew", "witness", "polecats", "refinery"} {
		info, err := os.Stat(filepath.Join(path, sub))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// syncTarget is a backward compatibility alias for syncClaudeTarget.
// It is used by rig.go and other callers that expect the old function signature.
// Deprecated: Use syncClaudeTarget instead.
func syncTarget(target hooks.Target, dryRun bool) (syncResult, error) {
	return syncClaudeTarget(target, dryRun)
}
