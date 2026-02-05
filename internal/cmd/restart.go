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

  --force  Force immediate shutdown without graceful stop.
           Sends SIGKILL instead of SIGTERM. Use when agents are
           stuck or unresponsive.

  --infra  Only restart infrastructure (daemon, deacon, mayor,
           witnesses, refineries). Leave polecats and crew running.`,
	Example: `  gt restart           # Stop all, restart all (default)
  gt restart --wait    # Wait for work to finish, then restart
  gt restart --force   # Force kill unresponsive agents
  gt restart --infra   # Only restart infrastructure`,
	RunE: runRestart,
}

var (
	restartQuiet bool
	restartWait  bool
	restartForce bool
	restartInfra bool
)

func init() {
	restartCmd.Flags().BoolVarP(&restartQuiet, "quiet", "q", false, "Only show errors")
	restartCmd.Flags().BoolVarP(&restartWait, "wait", "w", false, "Wait for agents to finish work before stopping")
	restartCmd.Flags().BoolVarP(&restartForce, "force", "f", false, "Force immediate shutdown without graceful stop")
	restartCmd.Flags().BoolVar(&restartInfra, "infra", false, "Only restart infrastructure, leave polecats/crew running")
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) error {
	// --wait and --force are mutually exclusive
	if restartWait && restartForce {
		return fmt.Errorf("--wait and --force are mutually exclusive")
	}

	if !restartQuiet {
		fmt.Printf("%s Restarting Gas Town...\n", style.Info.Render("ℹ"))
		fmt.Println()
	}

	// Phase 0: If --wait, wait for agents to finish work
	if restartWait {
		if !restartQuiet {
			fmt.Println("Waiting for agents to finish work...")
		}
		if err := waitForAgentsToFinish(restartQuiet); err != nil {
			return fmt.Errorf("wait failed: %w", err)
		}
		if !restartQuiet {
			fmt.Println()
		}
	}

	// Phase 1: Stop services
	// By default, stop everything (including polecats) for a clean restart
	// Use --infra to only stop infrastructure
	downOpts := DownOptions{
		Quiet:    restartQuiet,
		Polecats: !restartInfra, // Stop polecats unless --infra
		Force:    restartForce,
		All:      false,
		Nuke:     false,
		DryRun:   false,
	}

	if !restartQuiet {
		if restartInfra {
			fmt.Println("Stopping infrastructure...")
		} else {
			fmt.Println("Stopping all services...")
		}
	}

	if err := runDownWithOptions(downOpts); err != nil {
		// Continue with startup even if some services failed to stop
		if !restartQuiet {
			fmt.Printf("%s Some services failed to stop, continuing with startup...\n", style.Warning.Render("⚠"))
		}
	}

	fmt.Println()

	// Phase 2: Start services
	// By default, restore everything (including polecats with work)
	// Use --infra to only start infrastructure
	upOpts := UpOptions{
		Quiet:   restartQuiet,
		Restore: !restartInfra, // Restore polecats/crew unless --infra
	}

	if !restartQuiet {
		if restartInfra {
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
