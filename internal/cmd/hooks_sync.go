package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var hooksSyncDryRun bool

var hooksSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Regenerate all .claude/settings.json files",
	Long: `Regenerate all .claude/settings.json files from the base config and overrides.

For each target (mayor, deacon, rig/crew, rig/witness, etc.):
1. Load base config
2. Apply role override (if exists)
3. Apply rig+role override (if exists)
4. Merge hooks section into existing settings.json (preserving all fields)
5. Write updated settings.json

Examples:
  gt hooks sync             # Regenerate all settings.json files
  gt hooks sync --dry-run   # Show what would change without writing`,
	RunE: runHooksSync,
}

func init() {
	hooksCmd.AddCommand(hooksSyncCmd)
	hooksSyncCmd.Flags().BoolVar(&hooksSyncDryRun, "dry-run", false, "Show what would change without writing")
}

func runHooksSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	targets, err := hooks.DiscoverTargets(townRoot)
	if err != nil {
		return fmt.Errorf("discovering targets: %w", err)
	}

	if hooksSyncDryRun {
		fmt.Println("Dry run - showing what would change...")
		fmt.Println()
	} else {
		fmt.Println("Syncing hooks...")
	}

	updated := 0
	unchanged := 0
	created := 0
	errors := 0

	for _, target := range targets {
		result, err := syncTarget(target, hooksSyncDryRun)
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
	if hooksSyncDryRun {
		fmt.Printf("Would sync %d targets (%d to create, %d to update, %d unchanged",
			total, created, updated, unchanged)
	} else {
		fmt.Printf("Synced %d targets (%d created, %d updated, %d unchanged",
			total, created, updated, unchanged)
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

// syncTarget syncs a single target's .claude/settings.json.
// Uses MarshalSettings/UnmarshalSettings to preserve unknown fields.
func syncTarget(target hooks.Target, dryRun bool) (syncResult, error) {
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
