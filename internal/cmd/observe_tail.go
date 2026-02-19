package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/observe"
	"github.com/steveyegge/gastown/internal/workspace"
)

var observeTailJSON bool

func init() {
	observeCmd.AddCommand(observeTailCmd)
	observeTailCmd.Flags().BoolVar(&observeTailJSON, "json", false, "Output events as JSON")
}

var observeTailCmd = &cobra.Command{
	Use:   "tail <name>",
	Short: "Tail events from an observability source",
	Long: `Stream events from a named observability source in real-time.
Useful for debugging source configuration, redaction, and severity filtering.

Examples:
  gt observe tail app-logs
  gt observe tail app-logs --json`,
	Args: cobra.ExactArgs(1),
	RunE: runObserveTail,
}

func runObserveTail(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("no observability sources configured")
	}
	srcCfg, ok := settings.Observability.Sources[name]
	if !ok {
		return fmt.Errorf("source %q not found", name)
	}

	src, err := observe.NewSource(name, srcCfg)
	if err != nil {
		return fmt.Errorf("creating source %q: %w", name, err)
	}
	defer src.Close()

	fmt.Fprintf(os.Stderr, "Tailing %s (%s)... press Ctrl-C to stop\n", name, srcCfg.Path)

	// Handle interrupt.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case ev, ok := <-src.Events():
			if !ok {
				return nil
			}
			if observeTailJSON {
				data, _ := json.Marshal(map[string]interface{}{
					"ts":      ev.Time.Format("15:04:05"),
					"type":    ev.Type,
					"actor":   ev.Actor,
					"message": ev.Message,
				})
				fmt.Println(string(data))
			} else {
				fmt.Printf("[%s] %s\n", ev.Time.Format("15:04:05"), ev.Message)
			}
		case <-sigCh:
			return nil
		}
	}
}
