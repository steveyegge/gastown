package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

func init() {
	observeCmd.AddCommand(observeRemoveCmd)
}

var observeRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an observability source",
	Long:  `Remove a named observability source from town settings.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runObserveRemove,
}

func runObserveRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if settings.Observability == nil || settings.Observability.Sources == nil {
		return fmt.Errorf("source %q not found (no sources configured)", name)
	}

	if _, exists := settings.Observability.Sources[name]; !exists {
		return fmt.Errorf("source %q not found", name)
	}

	delete(settings.Observability.Sources, name)

	// Clean up empty containers.
	if len(settings.Observability.Sources) == 0 {
		settings.Observability.Sources = nil
	}
	if settings.Observability.Sources == nil && settings.Observability.TestOrchestration == nil {
		settings.Observability = nil
	}

	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Removed observability source %q\n", name)
	return nil
}
