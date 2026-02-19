package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	observeAddPath       string
	observeAddScope      string
	observeAddKind       string
	observeAddServiceID  string
	observeAddEnvID      string
	observeAddRedaction  string
)

func init() {
	observeCmd.AddCommand(observeAddCmd)

	observeAddCmd.Flags().StringVar(&observeAddPath, "path", "", "File path to tail (required)")
	observeAddCmd.Flags().StringVar(&observeAddScope, "scope", "dev", "Source scope: dev, ci, all")
	observeAddCmd.Flags().StringVar(&observeAddKind, "kind", "log", "Source kind: log, metric, trace, test_output")
	observeAddCmd.Flags().StringVar(&observeAddServiceID, "service-id", "", "Service identifier")
	observeAddCmd.Flags().StringVar(&observeAddEnvID, "env-id", "", "Environment identifier")
	observeAddCmd.Flags().StringVar(&observeAddRedaction, "redaction-policy", "standard", "Redaction policy: none, standard, strict")
	_ = observeAddCmd.MarkFlagRequired("path")
}

var observeAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an observability source",
	Long: `Add a new observability source to town settings.

The source will tail the specified file and emit events to the feed.

Examples:
  gt observe add app-logs --path /var/log/app.log --kind log --service-id myapp
  gt observe add test-out --path /tmp/test.log --kind test_output --scope dev`,
	Args: cobra.ExactArgs(1),
	RunE: runObserveAdd,
}

func runObserveAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate inputs.
	switch observeAddScope {
	case "dev", "ci", "all":
	default:
		return fmt.Errorf("invalid scope %q: must be dev, ci, or all", observeAddScope)
	}
	switch observeAddKind {
	case "log", "metric", "trace", "test_output":
	default:
		return fmt.Errorf("invalid kind %q: must be log, metric, trace, or test_output", observeAddKind)
	}
	switch observeAddRedaction {
	case "none", "standard", "strict":
	default:
		return fmt.Errorf("invalid redaction-policy %q: must be none, standard, or strict", observeAddRedaction)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if settings.Observability == nil {
		settings.Observability = &config.ObservabilityConfig{}
	}
	if settings.Observability.Sources == nil {
		settings.Observability.Sources = make(map[string]*config.ObservabilitySourceConfig)
	}

	if _, exists := settings.Observability.Sources[name]; exists {
		return fmt.Errorf("source %q already exists; remove it first with: gt observe remove %s", name, name)
	}

	settings.Observability.Sources[name] = &config.ObservabilitySourceConfig{
		Scope:           observeAddScope,
		ServiceID:       observeAddServiceID,
		EnvID:           observeAddEnvID,
		SourceKind:      observeAddKind,
		Path:            observeAddPath,
		RedactionPolicy: observeAddRedaction,
	}

	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Added observability source %q (%s, %s) → %s\n", name, observeAddKind, observeAddScope, observeAddPath)
	return nil
}
