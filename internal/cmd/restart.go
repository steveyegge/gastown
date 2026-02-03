package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var restartCmd = &cobra.Command{
	Use:     "restart",
	GroupID: GroupServices,
	Short:   "Restart all Gas Town services",
	Long: `Restart all Gas Town services by stopping then starting them.

This is equivalent to running 'gt down && gt up' but in a single command.
Use this when services need to pick up new configuration or after updates.

Services restarted:
  • Daemon     - Go background process
  • Deacon     - Health orchestrator
  • Mayor      - Global work coordinator
  • Witnesses  - Per-rig polecat managers
  • Refineries - Per-rig merge queue processors

Polecats and crew are NOT restarted by default. Use --restore to also
restart polecats with pinned work and crew from settings.

Use --polecats to also stop/restart all polecat sessions.`,
	Example: `  gt restart              # Restart infrastructure
  gt restart --restore    # Also restore crew and polecats with work
  gt restart --polecats   # Also restart all polecats`,
	RunE: runRestart,
}

var (
	restartQuiet    bool
	restartRestore  bool
	restartPolecats bool
)

func init() {
	restartCmd.Flags().BoolVarP(&restartQuiet, "quiet", "q", false, "Only show errors")
	restartCmd.Flags().BoolVar(&restartRestore, "restore", false, "Also restore crew (from settings) and polecats (from hooks)")
	restartCmd.Flags().BoolVarP(&restartPolecats, "polecats", "p", false, "Also restart all polecat sessions")
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) error {
	if !restartQuiet {
		fmt.Printf("%s Restarting Gas Town...\n", style.Info.Render("ℹ"))
		fmt.Println()
	}

	// Phase 1: Stop services (reuse down logic)
	// Set the package-level flags for runDown
	savedDownQuiet := downQuiet
	savedDownPolecats := downPolecats
	savedDownForce := downForce
	savedDownAll := downAll
	savedDownNuke := downNuke
	savedDownDryRun := downDryRun

	downQuiet = restartQuiet
	downPolecats = restartPolecats
	downForce = false // Don't force kill during restart
	downAll = false
	downNuke = false
	downDryRun = false

	if !restartQuiet {
		fmt.Println("Stopping services...")
	}
	err := runDown(cmd, []string{})

	// Restore flags
	downQuiet = savedDownQuiet
	downPolecats = savedDownPolecats
	downForce = savedDownForce
	downAll = savedDownAll
	downNuke = savedDownNuke
	downDryRun = savedDownDryRun

	if err != nil {
		// Continue with startup even if some services failed to stop
		if !restartQuiet {
			fmt.Printf("%s Some services failed to stop, continuing with startup...\n", style.Warning.Render("⚠"))
		}
	}

	fmt.Println()

	// Phase 2: Start services (reuse up logic)
	savedUpQuiet := upQuiet
	savedUpRestore := upRestore

	upQuiet = restartQuiet
	upRestore = restartRestore

	if !restartQuiet {
		fmt.Println("Starting services...")
	}
	err = runUp(cmd, []string{})

	// Restore flags
	upQuiet = savedUpQuiet
	upRestore = savedUpRestore

	if err != nil {
		return fmt.Errorf("restart failed during startup: %w", err)
	}

	return nil
}
