package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

func init() {
	observeCmd.AddCommand(observeListCmd)
}

var observeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured observability sources",
	Long:  `List all observability sources configured in town settings.`,
	RunE:  runObserveList,
}

func runObserveList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if settings.Observability == nil || len(settings.Observability.Sources) == 0 {
		fmt.Println("No observability sources configured.")
		fmt.Println()
		fmt.Println("Add one with:")
		fmt.Println("  gt observe add <name> --path /path/to/log --kind log")
		return nil
	}

	// Sort names for stable output.
	names := make([]string, 0, len(settings.Observability.Sources))
	for name := range settings.Observability.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCOPE\tKIND\tSERVICE\tPATH")
	for _, name := range names {
		src := settings.Observability.Sources[name]
		scope := src.Scope
		if scope == "" {
			scope = "dev"
		}
		kind := src.SourceKind
		if kind == "" {
			kind = "log"
		}
		svc := src.ServiceID
		if svc == "" {
			svc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, scope, kind, svc, src.Path)
	}
	w.Flush()

	return nil
}
