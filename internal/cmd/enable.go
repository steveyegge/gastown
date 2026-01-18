// ABOUTME: Command to enable Gas Town system-wide.
// ABOUTME: Sets the global state to enabled for all agentic coding tools.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var enableCmd = &cobra.Command{
	Use:     "enable",
	GroupID: GroupConfig,
	Short:   "Enable Gas Town system-wide",
	Long: `Enable Gas Town for all agentic coding tools.

When enabled:
  - Shell hooks set GT_TOWN_ROOT and GT_RIG environment variables
  - Claude Code SessionStart hooks run 'gt prime' for context
  - Git repos are auto-registered as rigs (configurable)

Use 'gt disable' to turn off. Use 'gt status --global' to check state.

Environment overrides:
  GASTOWN_DISABLED=1  - Disable for current session only
  GASTOWN_ENABLED=1   - Enable for current session only`,
	RunE: runEnable,
}

func init() {
	rootCmd.AddCommand(enableCmd)
}

func runEnable(cmd *cobra.Command, args []string) error {
	if err := state.Enable(Version); err != nil {
		return fmt.Errorf("enabling Gas Town: %w", err)
	}

	fmt.Printf("%s Gas Town enabled\n", style.Success.Render("✓"))
	fmt.Println()

	// Try to configure hooks for existing agents if we're in a Gas Town workspace
	configuredHooks := configureExistingAgentHooks()

	fmt.Println("Gas Town will now:")
	fmt.Println("  • Inject context into Claude Code sessions")
	fmt.Println("  • Set GT_TOWN_ROOT and GT_RIG environment variables")
	fmt.Println("  • Auto-register git repos as rigs (if configured)")

	if configuredHooks > 0 {
		fmt.Printf("  • Configured hooks for %d existing agent(s)\n", configuredHooks)
	}

	fmt.Println()
	fmt.Printf("Use %s to disable, %s to check status\n",
		style.Dim.Render("gt disable"),
		style.Dim.Render("gt status --global"))

	return nil
}

// configureExistingAgentHooks configures runtime hooks for existing agents in the workspace.
// Returns the number of agents configured.
func configureExistingAgentHooks() int {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return 0
	}

	configured := 0

	// Configure mayor hooks (mayor runs from town root)
	mayorDir := filepath.Join(townRoot, "mayor")
	if _, err := os.Stat(mayorDir); err == nil {
		mayorConfig := config.ResolveAgentConfig(townRoot, mayorDir)
		if err := runtime.EnsureSettingsForRole(townRoot, "mayor", mayorConfig); err == nil {
			configured++
		}
	}

	// Configure deacon hooks
	deaconDir := filepath.Join(townRoot, "deacon")
	if _, err := os.Stat(deaconDir); err == nil {
		deaconConfig := config.ResolveAgentConfig(townRoot, deaconDir)
		if err := runtime.EnsureSettingsForRole(deaconDir, "deacon", deaconConfig); err == nil {
			configured++
		}
	}

	// Configure hooks for registered rigs
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil || rigsConfig == nil {
		return configured
	}

	for rigName := range rigsConfig.Rigs {
		rigDir := filepath.Join(townRoot, rigName)
		if _, err := os.Stat(rigDir); os.IsNotExist(err) {
			continue
		}

		// Configure witness (witness runs from witness/rig or witness)
		witnessWorkDir := filepath.Join(rigDir, "witness", "rig")
		if _, err := os.Stat(witnessWorkDir); os.IsNotExist(err) {
			witnessWorkDir = filepath.Join(rigDir, "witness")
		}
		if _, err := os.Stat(witnessWorkDir); err == nil {
			witnessConfig := config.ResolveAgentConfig(townRoot, witnessWorkDir)
			if err := runtime.EnsureSettingsForRole(witnessWorkDir, "witness", witnessConfig); err == nil {
				configured++
			}
		}

		// Configure refinery (refinery runs from refinery/rig or mayor/rig)
		refineryRigDir := filepath.Join(rigDir, "refinery", "rig")
		if _, err := os.Stat(refineryRigDir); os.IsNotExist(err) {
			refineryRigDir = filepath.Join(rigDir, "mayor", "rig")
		}
		if _, err := os.Stat(refineryRigDir); err == nil {
			refineryConfig := config.ResolveAgentConfig(townRoot, refineryRigDir)
			if err := runtime.EnsureSettingsForRole(refineryRigDir, "refinery", refineryConfig); err == nil {
				configured++
			}
		}

		// Configure polecats
		polecatsDir := filepath.Join(rigDir, "polecats")
		if entries, err := os.ReadDir(polecatsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				polecatDir := filepath.Join(polecatsDir, entry.Name())
				polecatConfig := config.ResolveAgentConfig(townRoot, polecatDir)
				if err := runtime.EnsureSettingsForRole(polecatDir, "polecat", polecatConfig); err == nil {
					configured++
				}
			}
		}

		// Configure crew
		crewDir := filepath.Join(rigDir, "crew")
		if entries, err := os.ReadDir(crewDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				memberDir := filepath.Join(crewDir, entry.Name())
				memberConfig := config.ResolveAgentConfig(townRoot, memberDir)
				if err := runtime.EnsureSettingsForRole(memberDir, "crew", memberConfig); err == nil {
					configured++
				}
			}
		}
	}

	return configured
}
