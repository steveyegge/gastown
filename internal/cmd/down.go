package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var downCmd = &cobra.Command{
	Use:     "down",
	GroupID: GroupServices,
	Short:   "Stop all Gas Town services",
	Long: `Stop all Gas Town long-lived services.

This gracefully shuts down all infrastructure agents:

  • Witnesses - Per-rig polecat managers
  • Mayor     - Global work coordinator
  • Boot      - Deacon's watchdog
  • Deacon    - Health orchestrator
  • Daemon    - Go background process

Polecats are NOT stopped by this command - use 'gt swarm stop' or
kill individual polecats with 'gt polecat kill'.

This is useful for:
  • Taking a break (stop token consumption)
  • Clean shutdown before system maintenance
  • Resetting the town to a clean state`,
	RunE: runDown,
}

var (
	downQuiet bool
	downForce bool
	downAll   bool
)

func init() {
	downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
	downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
	downCmd.Flags().BoolVarP(&downAll, "all", "a", false, "Also kill the tmux server")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()
	allOK := true

	// Stop in reverse order of startup

	// 1. Stop witnesses first
	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-witness", rigName)
		if err := stopSession(t, sessionName); err != nil {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
		}
	}

	// 2. Stop town-level sessions (Mayor, Boot, Deacon) in correct order
	for _, ts := range session.TownSessions() {
		stopped, err := session.StopTownSession(t, ts, downForce)
		if err != nil {
			printDownStatus(ts.Name, false, err.Error())
			allOK = false
		} else if stopped {
			printDownStatus(ts.Name, true, "stopped")
		} else {
			printDownStatus(ts.Name, true, "not running")
		}
	}

	// 3. Stop Daemon last
	running, _, _ := daemon.IsRunning(townRoot)
	if running {
		if err := daemon.StopDaemon(townRoot); err != nil {
			printDownStatus("Daemon", false, err.Error())
			allOK = false
		} else {
			printDownStatus("Daemon", true, "stopped")
		}
	} else {
		printDownStatus("Daemon", true, "not running")
	}

	// 4. Kill tmux server if --all
	if downAll {
		if err := t.KillServer(); err != nil {
			printDownStatus("Tmux server", false, err.Error())
			allOK = false
		} else {
			printDownStatus("Tmux server", true, "killed")
		}
	}

	fmt.Println()
	if allOK {
		fmt.Printf("%s All services stopped\n", style.Bold.Render("✓"))
		// Log halt event with stopped services
		stoppedServices := []string{"daemon", "deacon", "boot", "mayor"}
		for _, rigName := range rigs {
			stoppedServices = append(stoppedServices, fmt.Sprintf("%s/witness", rigName))
		}
		if downAll {
			stoppedServices = append(stoppedServices, "tmux-server")
		}
		_ = events.LogFeed(events.TypeHalt, "gt", events.HaltPayload(stoppedServices))
	} else {
		fmt.Printf("%s Some services failed to stop\n", style.Bold.Render("✗"))
		return fmt.Errorf("not all services stopped")
	}

	return nil
}

func printDownStatus(name string, ok bool, detail string) {
	if downQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// stopSession gracefully stops a tmux session.
func stopSession(t *tmux.Tmux, sessionName string) error {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return err
	}
	if !running {
		return nil // Already stopped
	}

	// Try graceful shutdown first (Ctrl-C, best-effort interrupt)
	if !downForce {
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Kill the session
	return t.KillSession(sessionName)
}
