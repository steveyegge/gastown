package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var restartCmd = &cobra.Command{
	Use:     "restart",
	GroupID: GroupServices,
	Short:   "Restart all Gas Town services",
	Long: `Restart all Gas Town services by stopping then starting them.

Use this when you need to:
  • Update Gas Town (gt) after pulling new changes
  • Update Claude Code after a new release
  • Reload configuration or settings

All agents are stopped and restarted, preserving their work state:
  • Infrastructure: Daemon, Deacon, Mayor, Witnesses, Refineries
  • Workers: All polecats and crew members

Work is preserved via hook beads - polecats resume their assigned work
after restart. This is a safe operation that brings everything down
into a resumable state.

Flags:
  --wait   Wait for agents to finish current work before stopping.
           Blocks until all polecats reach a checkpoint or complete.
           Use this for graceful restarts during active work.

  --now    Skip graceful interrupt (Ctrl-C) and kill immediately.
           Use when agents are stuck or unresponsive to interrupts.

  --infra  Only restart infrastructure (daemon, deacon, mayor,
           witnesses, refineries). Leave polecats and crew running.`,
	Example: `  gt restart           # Stop all, restart all (default)
  gt restart --wait    # Wait for work to finish, then restart
  gt restart --now     # Immediate shutdown for stuck agents
  gt restart --infra   # Only restart infrastructure`,
	RunE: runRestart,
}

// RestartOptions configures the behavior of runRestartWithOptions.
type RestartOptions struct {
	Quiet bool // Only show errors
	Wait  bool // Wait for agents to finish work before stopping
	Now   bool // Skip graceful interrupt (Ctrl-C), kill immediately
	Infra bool // Only restart infrastructure, leave polecats/crew running
}

var (
	restartQuiet bool
	restartWait  bool
	restartNow   bool
	restartInfra bool
)

func init() {
	restartCmd.Flags().BoolVarP(&restartQuiet, "quiet", "q", false, "Only show errors")
	restartCmd.Flags().BoolVarP(&restartWait, "wait", "w", false, "Wait for agents to finish work before stopping")
	restartCmd.Flags().BoolVarP(&restartNow, "now", "n", false, "Skip graceful interrupt, kill immediately")
	restartCmd.Flags().BoolVar(&restartInfra, "infra", false, "Only restart infrastructure, leave polecats/crew running")
	rootCmd.AddCommand(restartCmd)
}

// restartOptionsFromFlags creates RestartOptions from the package-level flag variables.
func restartOptionsFromFlags() RestartOptions {
	return RestartOptions{
		Quiet: restartQuiet,
		Wait:  restartWait,
		Now:   restartNow,
		Infra: restartInfra,
	}
}

func runRestart(cmd *cobra.Command, args []string) error {
	return runRestartWithOptions(restartOptionsFromFlags())
}

func runRestartWithOptions(opts RestartOptions) error {
	// --wait and --now are mutually exclusive
	if opts.Wait && opts.Now {
		return fmt.Errorf("--wait and --now are mutually exclusive")
	}

	if !opts.Quiet {
		fmt.Printf("%s Restarting Gas Town...\n", style.Info.Render("ℹ"))
		fmt.Println()
	}

	// Phase 0: If --wait, wait for agents to finish work
	if opts.Wait {
		if !opts.Quiet {
			fmt.Println("Waiting for agents to finish work...")
		}
		if err := waitForAgentsToFinish(opts.Quiet); err != nil {
			return fmt.Errorf("wait failed: %w", err)
		}
		if !opts.Quiet {
			fmt.Println()
		}
	}

	// Phase 1: Stop services
	// By default, stop everything (including polecats) for a clean restart
	// Use --infra to only stop infrastructure
	downOpts := DownOptions{
		Quiet:    opts.Quiet,
		Polecats: !opts.Infra, // Stop polecats unless --infra
		Force:    opts.Now,    // --now maps to force shutdown
		All:      false,
		Nuke:     false,
		DryRun:   false,
	}

	if !opts.Quiet {
		if opts.Infra {
			fmt.Println("Stopping infrastructure...")
		} else {
			fmt.Println("Stopping all services...")
		}
	}

	if err := runDownWithOptions(downOpts); err != nil {
		// Continue with startup even if some services failed to stop
		if !opts.Quiet {
			fmt.Printf("%s Some services failed to stop, continuing with startup...\n", style.Warning.Render("⚠"))
		}
	}

	fmt.Println()

	// Phase 2: Start services
	// By default, restore everything (including polecats with work)
	// Use --infra to only start infrastructure
	upOpts := UpOptions{
		Quiet:   opts.Quiet,
		Restore: !opts.Infra, // Restore polecats/crew unless --infra
	}

	if !opts.Quiet {
		if opts.Infra {
			fmt.Println("Starting infrastructure...")
		} else {
			fmt.Println("Starting all services...")
		}
	}

	if err := runUpWithOptions(upOpts); err != nil {
		return fmt.Errorf("restart failed during startup: %w", err)
	}

	return nil
}

// waitForAgentsToFinish blocks until all polecats have finished their current work.
// This polls agent beads and waits for all to reach idle/done state.
func waitForAgentsToFinish(quiet bool) error {
	// Poll interval and timeout
	pollInterval := 5 * time.Second
	timeout := 30 * time.Minute
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for agents to finish (30m)")
		}

		// Check if any polecats are still working
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

	// Get all rigs
	rigs := discoverRigs(townRoot)
	if len(rigs) == 0 {
		return 0, nil
	}

	working := 0
	for _, rigName := range rigs {
		// Get beads instance for this rig
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
			// Count polecats that have work assigned (hook_bead set)
			if fields.RoleType == "polecat" && fields.HookBead != "" {
				working++
			}
		}
	}

	return working, nil
}
