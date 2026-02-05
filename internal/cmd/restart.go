package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Restart strategy constants.
const (
	// StrategyGraceful notifies agents to save state (commit WIP, write
	// checkpoint), waits 15s, then sends Ctrl-C and kills processes.
	// State is preserved in beads so polecats resume assigned work on
	// restart.
	StrategyGraceful = "graceful"

	// StrategyDrain waits for all polecats to finish their current work
	// before stopping. Blocks the terminal until all agents are idle
	// (30 minute timeout). Then proceeds with a graceful shutdown.
	// Zero work loss.
	StrategyDrain = "drain"

	// StrategyImmediate skips the Ctrl-C interrupt and kills processes
	// directly. Use when agents are stuck or unresponsive. Bead state is
	// preserved so work can be resumed, but in-progress WIP is likely lost.
	StrategyImmediate = "immediate"

	// StrategyClean kills all processes, nukes polecat worktrees, and
	// clears agent beads. Only infrastructure is restarted - no polecats
	// or crew are restored. Use for a fresh start when something is broken.
	StrategyClean = "clean"
)

var validRestartStrategies = []string{StrategyGraceful, StrategyDrain, StrategyImmediate, StrategyClean}

func isValidRestartStrategy(s string) bool {
	for _, v := range validRestartStrategies {
		if s == v {
			return true
		}
	}
	return false
}

var restartCmd = &cobra.Command{
	Use:     "restart",
	GroupID: GroupServices,
	Short:   "Restart all Gas Town services",
	Long: `Restart all Gas Town services by stopping then starting them.

Use this when you need to:
  • Update Gas Town (gt) after pulling new changes
  • Update Claude Code after a new release
  • Reload configuration or settings

Strategies (--strategy):

  graceful (default)
    Notify all agents to save state (commit WIP, write checkpoint),
    wait 15s, then send Ctrl-C and stop everything. Work state is
    preserved via hook beads so polecats resume assigned work after
    restart. Use --no-save on gt down to skip the notification.
    Use for: routine restarts, updates, config reloads.

  drain
    Wait for all agents to finish their current work before stopping.
    Blocks the terminal until all polecats are idle (30m timeout), then
    proceeds with a graceful shutdown. Zero work loss guaranteed.
    Use for: active work in progress, zero-tolerance for lost work.

  immediate
    Skip the Ctrl-C interrupt and kill all processes directly. Bead state
    is preserved so polecats can resume from their last committed state,
    but any uncommitted work in progress will be lost.
    Use for: stuck or unresponsive agents, urgent restarts.

  clean
    Kill all processes, nuke all polecat worktrees, and clear agent beads.
    Only infrastructure is restarted - no polecats or crew are restored.
    This gives you a completely fresh start.
    Use for: broken state, corrupted worktrees, starting over.`,
	Example: `  gt restart                        # Graceful restart (default)
  gt restart --strategy drain       # Wait for work to finish first
  gt restart --strategy immediate   # Kill now, resume from last commit
  gt restart --strategy clean       # Fresh start, nuke everything
  gt restart -s drain               # Short flag`,
	RunE: runRestart,
}

// RestartOptions configures the behavior of runRestartWithOptions.
type RestartOptions struct {
	Quiet    bool   // Only show errors
	Strategy string // One of: graceful, drain, immediate, clean
}

var (
	restartQuiet    bool
	restartStrategy string
)

func init() {
	restartCmd.Flags().BoolVarP(&restartQuiet, "quiet", "q", false, "Only show errors")
	restartCmd.Flags().StringVarP(&restartStrategy, "strategy", "s", StrategyGraceful,
		"Restart strategy: graceful, drain, immediate, clean")
	rootCmd.AddCommand(restartCmd)
}

// restartOptionsFromFlags creates RestartOptions from the package-level flag variables.
func restartOptionsFromFlags() RestartOptions {
	return RestartOptions{
		Quiet:    restartQuiet,
		Strategy: restartStrategy,
	}
}

func runRestart(cmd *cobra.Command, args []string) error {
	return runRestartWithOptions(restartOptionsFromFlags())
}

func runRestartWithOptions(opts RestartOptions) error {
	strategy := strings.ToLower(opts.Strategy)

	if !isValidRestartStrategy(strategy) {
		return fmt.Errorf("invalid strategy %q (valid: %s)", opts.Strategy,
			strings.Join(validRestartStrategies, ", "))
	}

	if !opts.Quiet {
		fmt.Printf("%s Restarting Gas Town (strategy: %s)...\n", style.Info.Render("ℹ"), strategy)
		fmt.Println()
	}

	switch strategy {
	case StrategyDrain:
		return restartDrain(opts)
	case StrategyGraceful:
		return restartGraceful(opts)
	case StrategyImmediate:
		return restartImmediate(opts)
	case StrategyClean:
		return restartClean(opts)
	}

	return nil
}

// restartGraceful notifies agents to save state, waits briefly, then kills and restarts.
func restartGraceful(opts RestartOptions) error {
	return restartWithDown(opts, DownOptions{
		Quiet:      opts.Quiet,
		Polecats:   true,
		Force:      false, // Graceful: send Ctrl-C first
		NotifySave: true,  // Nudge agents to save state before killing
	}, true) // restore polecats/crew
}

// restartDrain waits for work to finish, then does a graceful restart.
func restartDrain(opts RestartOptions) error {
	if !opts.Quiet {
		fmt.Println("Waiting for agents to finish work...")
	}
	if err := waitForAgentsToFinish(opts.Quiet); err != nil {
		return fmt.Errorf("drain failed: %w", err)
	}
	if !opts.Quiet {
		fmt.Println()
	}

	return restartGraceful(opts)
}

// restartImmediate skips Ctrl-C and kills directly, then restarts.
func restartImmediate(opts RestartOptions) error {
	return restartWithDown(opts, DownOptions{
		Quiet:    opts.Quiet,
		Polecats: true,
		Force:    true, // Immediate: skip Ctrl-C
	}, true) // restore polecats/crew
}

// restartClean nukes everything and starts fresh.
func restartClean(opts RestartOptions) error {
	// Phase 1: Kill everything immediately
	if err := restartWithDown(opts, DownOptions{
		Quiet:    opts.Quiet,
		Polecats: true,
		Force:    true, // Kill immediately
	}, false); err != nil { // Do NOT restore polecats/crew
		// restartWithDown already handled the error output
		// For clean strategy, continue even if down had issues
	}

	// Phase 2: Nuke polecat worktrees and clear agent beads
	if !opts.Quiet {
		fmt.Println("Cleaning up polecat state...")
	}
	if err := cleanPolecatState(opts.Quiet); err != nil {
		if !opts.Quiet {
			fmt.Printf("%s Failed to clean some polecat state: %v\n", style.Warning.Render("⚠"), err)
		}
	}

	return nil
}

// restartWithDown is the common pattern: stop services, then start services.
func restartWithDown(opts RestartOptions, downOpts DownOptions, restore bool) error {
	if !opts.Quiet {
		fmt.Println("Stopping all services...")
	}

	if err := runDownWithOptions(downOpts); err != nil {
		if !opts.Quiet {
			fmt.Printf("%s Some services failed to stop, continuing with startup...\n", style.Warning.Render("⚠"))
		}
	}

	fmt.Println()

	upOpts := UpOptions{
		Quiet:   opts.Quiet,
		Restore: restore,
	}

	if !opts.Quiet {
		if restore {
			fmt.Println("Starting all services...")
		} else {
			fmt.Println("Starting infrastructure...")
		}
	}

	if err := runUpWithOptions(upOpts); err != nil {
		return fmt.Errorf("restart failed during startup: %w", err)
	}

	return nil
}

// cleanPolecatState nukes polecat worktrees and clears agent beads.
func cleanPolecatState(quiet bool) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	rigs := discoverRigs(townRoot)
	for _, rigName := range rigs {
		_, r, err := getRig(rigName)
		if err != nil {
			continue
		}

		// Close and clear agent beads for polecats
		b := beads.New(r.Path)
		agents, err := b.ListAgentBeads()
		if err == nil {
			for id, agent := range agents {
				fields := beads.ParseAgentFields(agent.Description)
				if fields.RoleType == "polecat" {
					if err := b.CloseAndClearAgentBead(id, "clean restart"); err != nil {
						if !quiet {
							fmt.Printf("%s Failed to clear agent bead %s: %v\n", style.Warning.Render("⚠"), id, err)
						}
					}
				}
			}
		}

		// Remove polecat worktree directories
		polecatsDir := filepath.Join(townRoot, rigName, "polecats")
		entries, err := os.ReadDir(polecatsDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			path := filepath.Join(polecatsDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				if !quiet {
					fmt.Printf("%s Failed to remove %s: %v\n", style.Warning.Render("⚠"), path, err)
				}
			} else if !quiet {
				fmt.Printf("%s Removed polecat %s/%s\n", style.SuccessPrefix, rigName, entry.Name())
			}
		}
	}

	return nil
}

// waitForAgentsToFinish blocks until all polecats have finished their current work.
// This polls agent beads and waits for all to reach idle/done state.
func waitForAgentsToFinish(quiet bool) error {
	pollInterval := 5 * time.Second
	timeout := 30 * time.Minute
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for agents to finish (30m)")
		}

		working, err := countWorkingPolecats()
		if err != nil {
			return fmt.Errorf("checking polecat status: %w", err)
		}

		if working == 0 {
			if !quiet {
				fmt.Printf("%s All agents idle\n", style.SuccessPrefix)
			}
			return nil
		}

		if !quiet {
			fmt.Printf("%s Waiting for %d agent(s) to finish...\n", style.Dim.Render("○"), working)
		}

		time.Sleep(pollInterval)
	}
}

// countWorkingPolecats returns the number of polecats currently working.
// A polecat is "working" if it has a hook_bead set (actively assigned work).
func countWorkingPolecats() (int, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return 0, err
	}

	rigs := discoverRigs(townRoot)
	if len(rigs) == 0 {
		return 0, nil
	}

	working := 0
	for _, rigName := range rigs {
		_, r, err := getRig(rigName)
		if err != nil {
			continue
		}

		b := beads.New(r.Path)
		agents, err := b.ListAgentBeads()
		if err != nil {
			continue
		}

		for _, agent := range agents {
			fields := beads.ParseAgentFields(agent.Description)
			if fields.RoleType == "polecat" && fields.HookBead != "" {
				working++
			}
		}
	}

	return working, nil
}
