package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

func init() {
	observeCmd.AddCommand(observeStatusCmd)
}

var observeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check accessibility of observability sources",
	Long:  `Check whether each configured observability source file exists and is readable.`,
	RunE:  runObserveStatus,
}

func runObserveStatus(cmd *cobra.Command, args []string) error {
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
		return nil
	}

	names := make([]string, 0, len(settings.Observability.Sources))
	for name := range settings.Observability.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tLAST MODIFIED\tPATH")
	for _, name := range names {
		src := settings.Observability.Sources[name]
		info, err := os.Stat(src.Path)
		status := "reachable"
		modTime := "-"
		if err != nil {
			status = "unreachable"
		} else {
			modTime = info.ModTime().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, status, modTime, src.Path)
	}
	_ = w.Flush()

	return nil
}
