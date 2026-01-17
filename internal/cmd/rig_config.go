// Package cmd provides CLI commands for the gt tool.
// This file implements the gt rig config commands for viewing and manipulating
// rig configuration across property layers.
package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wisp"
)

var rigConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage rig configuration",
	Long: `View and manage rig configuration across property layers.

Configuration is looked up through multiple layers:
1. Wisp layer (transient, local) - .beads-wisp/config/
2. Bead layer (persistent, synced) - rig identity bead labels
3. Town defaults - ~/gt/settings/config.json
4. System defaults - compiled-in fallbacks

Most properties use override semantics (first non-nil wins).
Integer properties like priority_adjustment use stacking semantics (values add).`,
	RunE: requireSubcommand,
}

var rigConfigShowCmd = &cobra.Command{
	Use:   "show <rig>",
	Short: "Show effective configuration for a rig",
	Long: `Show the effective configuration for a rig.

By default, shows only the resolved values. Use --layers to see
which layer each value comes from.

Example output:
  gt rig config show gastown --layers
  Key                 Value        Source
  status              parked       wisp
  priority_adjustment 10           bead
  auto_restart        true         system
  max_polecats        4            town`,
	Args: cobra.ExactArgs(1),
	RunE: runRigConfigShow,
}

var rigConfigSetCmd = &cobra.Command{
	Use:   "set <rig> <key> [value]",
	Short: "Set a configuration value",
	Long: `Set a configuration value in the wisp layer (local, ephemeral).

Use --global to set in the bead layer (persistent, synced globally).
Use --block to explicitly block a key (prevents inheritance).

Examples:
  gt rig config set gastown status parked           # Wisp layer
  gt rig config set gastown status docked --global  # Bead layer
  gt rig config set gastown auto_restart --block    # Block inheritance`,
	Args: cobra.RangeArgs(2, 3),
	RunE: runRigConfigSet,
}

var rigConfigUnsetCmd = &cobra.Command{
	Use:   "unset <rig> <key>",
	Short: "Remove a configuration value from the wisp layer",
	Long: `Remove a configuration value from the wisp layer.

This clears both regular values and blocked markers for the key.
Values set in the bead layer remain unchanged.

Example:
  gt rig config unset gastown status`,
	Args: cobra.ExactArgs(2),
	RunE: runRigConfigUnset,
}

// Flags
var (
	rigConfigShowLayers bool
	rigConfigSetGlobal  bool
	rigConfigSetBlock   bool
)

func init() {
	rigCmd.AddCommand(rigConfigCmd)
	rigConfigCmd.AddCommand(rigConfigShowCmd)
	rigConfigCmd.AddCommand(rigConfigSetCmd)
	rigConfigCmd.AddCommand(rigConfigUnsetCmd)
	rigConfigCmd.AddCommand(rigConfigProtectedBranchesCmd)

	rigConfigShowCmd.Flags().BoolVar(&rigConfigShowLayers, "layers", false, "Show which layer each value comes from")

	rigConfigSetCmd.Flags().BoolVar(&rigConfigSetGlobal, "global", false, "Set in bead layer (persistent, synced)")
	rigConfigSetCmd.Flags().BoolVar(&rigConfigSetBlock, "block", false, "Block inheritance for this key")
}

func runRigConfigShow(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Collect all known keys
	allKeys := getConfigKeys(townRoot, r)

	if rigConfigShowLayers {
		// Show with sources
		fmt.Printf("%-25s %-15s %s\n", "Key", "Value", "Source")
		fmt.Printf("%-25s %-15s %s\n", "---", "-----", "------")
		for _, key := range allKeys {
			result := r.GetConfigWithSource(key)
			valueStr := formatValue(result.Value)
			sourceStr := string(result.Source)
			if result.Source == rig.SourceBlocked {
				valueStr = "(blocked)"
			}
			fmt.Printf("%-25s %-15s %s\n", key, valueStr, sourceStr)
		}
	} else {
		// Show only effective values
		fmt.Printf("%-25s %s\n", "Key", "Value")
		fmt.Printf("%-25s %s\n", "---", "-----")
		for _, key := range allKeys {
			result := r.GetConfigWithSource(key)
			if result.Source == rig.SourceNone {
				continue // Skip unset keys
			}
			valueStr := formatValue(result.Value)
			if result.Source == rig.SourceBlocked {
				valueStr = "(blocked)"
			}
			fmt.Printf("%-25s %s\n", key, valueStr)
		}
	}

	return nil
}

func runRigConfigSet(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	key := args[1]

	// Validate: --block requires no value, otherwise value is required
	if rigConfigSetBlock {
		if len(args) > 2 {
			return fmt.Errorf("--block does not take a value")
		}
	} else {
		if len(args) < 3 {
			return fmt.Errorf("value is required (use --block to block inheritance instead)")
		}
	}

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	if rigConfigSetBlock {
		// Block inheritance via wisp layer
		wispCfg := wisp.NewConfig(townRoot, r.Name)
		if err := wispCfg.Block(key); err != nil {
			return fmt.Errorf("blocking %s: %w", key, err)
		}
		fmt.Printf("%s Blocked %s for rig %s\n", style.Success.Render("✓"), key, rigName)
		return nil
	}

	value := args[2]

	if rigConfigSetGlobal {
		// Set in bead layer (rig identity bead labels)
		if err := setBeadLabel(townRoot, r, key, value); err != nil {
			return fmt.Errorf("setting bead label: %w", err)
		}
		fmt.Printf("%s Set %s=%s in bead layer for rig %s\n", style.Success.Render("✓"), key, value, rigName)
	} else {
		// Set in wisp layer
		wispCfg := wisp.NewConfig(townRoot, r.Name)
		// Try to parse as appropriate type
		var typedValue interface{} = value
		if b, err := strconv.ParseBool(value); err == nil {
			typedValue = b
		} else if i, err := strconv.Atoi(value); err == nil {
			typedValue = i
		}
		if err := wispCfg.Set(key, typedValue); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		fmt.Printf("%s Set %s=%s in wisp layer for rig %s\n", style.Success.Render("✓"), key, value, rigName)
	}

	return nil
}

func runRigConfigUnset(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	key := args[1]

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	wispCfg := wisp.NewConfig(townRoot, r.Name)
	if err := wispCfg.Unset(key); err != nil {
		return fmt.Errorf("unsetting %s: %w", key, err)
	}

	fmt.Printf("%s Unset %s from wisp layer for rig %s\n", style.Success.Render("✓"), key, rigName)
	return nil
}

// getConfigKeys returns all known configuration keys, sorted.
func getConfigKeys(townRoot string, r *rig.Rig) []string {
	keySet := make(map[string]bool)

	// System defaults
	for k := range rig.SystemDefaults {
		keySet[k] = true
	}

	// Wisp keys
	wispCfg := wisp.NewConfig(townRoot, r.Name)
	for _, k := range wispCfg.Keys() {
		keySet[k] = true
	}

	// Bead labels (from rig identity bead)
	prefix := "gt"
	if r.Config != nil && r.Config.Prefix != "" {
		prefix = r.Config.Prefix
	}
	rigBeadID := beads.RigBeadIDWithPrefix(prefix, r.Name)
	beadsDir := beads.ResolveBeadsDir(r.Path)
	bd := beads.NewWithBeadsDir(townRoot, beadsDir)
	if issue, err := bd.Show(rigBeadID); err == nil {
		for _, label := range issue.Labels {
			// Labels are in format "key:value"
			for i, c := range label {
				if c == ':' {
					keySet[label[:i]] = true
					break
				}
			}
		}
	}

	// Sort keys
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

// setBeadLabel sets a label on the rig identity bead.
func setBeadLabel(townRoot string, r *rig.Rig, key, value string) error {
	prefix := "gt"
	if r.Config != nil && r.Config.Prefix != "" {
		prefix = r.Config.Prefix
	}

	rigBeadID := beads.RigBeadIDWithPrefix(prefix, r.Name)
	beadsDir := beads.ResolveBeadsDir(r.Path)
	bd := beads.NewWithBeadsDir(townRoot, beadsDir)

	// Check if bead exists
	issue, err := bd.Show(rigBeadID)
	if err != nil {
		return fmt.Errorf("rig identity bead %s not found (run 'gt rig add' to create it)", rigBeadID)
	}

	// Build new labels list: remove existing key:* and add new key:value
	newLabels := make([]string, 0, len(issue.Labels)+1)
	keyPrefix := key + ":"
	for _, label := range issue.Labels {
		if len(label) > len(keyPrefix) && label[:len(keyPrefix)] == keyPrefix {
			continue // Remove old value for this key
		}
		newLabels = append(newLabels, label)
	}
	newLabels = append(newLabels, key+":"+value)

	// Update the bead
	return bd.Update(rigBeadID, beads.UpdateOptions{
		SetLabels: newLabels,
	})
}

// formatValue formats a config value for display.
func formatValue(v interface{}) string {
	if v == nil {
		return "(nil)"
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Protected branches subcommand
var rigConfigProtectedBranchesCmd = &cobra.Command{
	Use:   "protected-branches <rig> [branches...]",
	Short: "Get or set protected branches for a rig",
	Long: `Get or set protected branches for a specific rig.

With no branch arguments, shows the effective protected branches.
With branch arguments, overrides the town default for this rig.

Use --inherit to remove the rig override and use town defaults.

Protection includes:
  - Direct pushes to these branches are blocked
  - All merges to these branches require human approval

Examples:
  gt rig config protected-branches myrig           # Show effective branches
  gt rig config protected-branches myrig main      # Protect only main
  gt rig config protected-branches myrig --inherit # Use town defaults`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigConfigProtectedBranches,
}

var rigConfigProtectedBranchesInherit bool

func init() {
	rigConfigProtectedBranchesCmd.Flags().BoolVar(&rigConfigProtectedBranchesInherit, "inherit", false, "Remove rig override and use town defaults")
}

func runRigConfigProtectedBranches(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	branches := args[1:]

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Load settings
	townSettingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(townSettingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	rigSettingsPath := config.RigSettingsPath(r.Path)
	rigSettings, err := config.LoadRigSettings(rigSettingsPath)
	if err != nil {
		// Create new rig settings if doesn't exist
		rigSettings = config.NewRigSettings()
	}

	// Handle --inherit flag
	if rigConfigProtectedBranchesInherit {
		rigSettings.ProtectedBranches = nil
		if err := config.SaveRigSettings(rigSettingsPath, rigSettings); err != nil {
			return fmt.Errorf("saving rig settings: %w", err)
		}
		fmt.Printf("Rig %s now inherits protected branches from town.\n", style.Bold.Render(rigName))
		effectiveBranches := config.ResolveProtectedBranches(townSettings, rigSettings)
		if len(effectiveBranches) > 0 {
			fmt.Printf("Effective: %s\n", strings.Join(effectiveBranches, ", "))
		} else {
			fmt.Println("Effective: (none)")
		}
		return nil
	}

	// If no branches specified, show current state
	if len(branches) == 0 {
		effectiveBranches := config.ResolveProtectedBranches(townSettings, rigSettings)

		fmt.Printf("%s\n\n", style.Bold.Render("Protected Branches for "+rigName))

		if rigSettings.ProtectedBranches != nil {
			fmt.Printf("Rig override: %s\n", style.Bold.Render(strings.Join(rigSettings.ProtectedBranches, ", ")))
		} else {
			fmt.Println("Rig override: (none, using town default)")
		}

		if len(townSettings.ProtectedBranches) > 0 {
			fmt.Printf("Town default: %s\n", strings.Join(townSettings.ProtectedBranches, ", "))
		} else {
			fmt.Println("Town default: (none)")
		}

		fmt.Println()
		if len(effectiveBranches) > 0 {
			fmt.Printf("Effective: %s\n", style.Bold.Render(strings.Join(effectiveBranches, ", ")))
		} else {
			fmt.Println("Effective: (no protection)")
		}
		return nil
	}

	// Set rig-level protected branches
	rigSettings.ProtectedBranches = branches
	if err := config.SaveRigSettings(rigSettingsPath, rigSettings); err != nil {
		return fmt.Errorf("saving rig settings: %w", err)
	}

	// Update the pre-push hook in the rig's clones
	if err := installRigProtectedBranchesHooks(r, branches); err != nil {
		fmt.Printf("   %s Could not update pre-push hooks: %v\n", style.Dim.Render("⚠"), err)
	}

	fmt.Printf("Protected branches for %s: %s\n", style.Bold.Render(rigName), strings.Join(branches, ", "))
	return nil
}

// installRigProtectedBranchesHooks installs pre-push hooks in the rig's git clones.
func installRigProtectedBranchesHooks(r *rig.Rig, branches []string) error {
	// Find git directories in the rig that might need hooks
	// - refinery/rig/ (worktree on main)
	// - mayor/rig/ (mayor's clone)
	// - polecats/*/rig/ (polecat worktrees)
	// - crew/*/rig/ (crew clones)

	// For now, just install in refinery/rig which is the main worktree
	// Polecats are ephemeral and created from .repo.git
	refineryPath := r.Path + "/refinery/rig"
	return installProtectedBranchesHook(refineryPath, branches)
}
