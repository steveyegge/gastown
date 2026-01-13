package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Lifecycle flags
var (
	rigShutdownForce   bool
	rigShutdownNuclear bool
	rigStopForce       bool
	rigStopNuclear     bool
	rigRestartForce    bool
	rigRestartNuclear  bool
)

var rigBootCmd = &cobra.Command{
	Use:   "boot <rig>",
	Short: "Start witness and refinery for a rig",
	Long: `Start the witness and refinery agents for a rig.

This is the inverse of 'gt rig shutdown'. It starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig boot greenplace`,
	Args: cobra.ExactArgs(1),
	RunE: runRigBoot,
}

var rigStartCmd = &cobra.Command{
	Use:   "start <rig>...",
	Short: "Start witness and refinery on patrol for one or more rigs",
	Long: `Start the witness and refinery agents on patrol for one or more rigs.

This is similar to 'gt rig boot' but supports multiple rigs at once.
For each rig, it starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig start gastown
  gt rig start gastown beads
  gt rig start gastown beads myproject`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigStart,
}

var rigRebootCmd = &cobra.Command{
	Use:   "reboot <rig>",
	Short: "Restart witness and refinery for a rig",
	Long: `Restart the patrol agents (witness and refinery) for a rig.

This is equivalent to 'gt rig shutdown' followed by 'gt rig boot'.
Useful after polecats complete work and land their changes.

Examples:
  gt rig reboot greenplace
  gt rig reboot beads --force`,
	Args: cobra.ExactArgs(1),
	RunE: runRigReboot,
}

var rigShutdownCmd = &cobra.Command{
	Use:   "shutdown <rig>",
	Short: "Gracefully stop all rig agents",
	Long: `Stop all agents in a rig.

This command gracefully shuts down:
- All polecat sessions
- The refinery (if running)
- The witness (if running)

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig shutdown greenplace
  gt rig shutdown greenplace --force
  gt rig shutdown greenplace --nuclear  # DANGER: loses uncommitted work`,
	Args: cobra.ExactArgs(1),
	RunE: runRigShutdown,
}

var rigStopCmd = &cobra.Command{
	Use:   "stop <rig>...",
	Short: "Stop one or more rigs (shutdown semantics)",
	Long: `Stop all agents in one or more rigs.

This command is similar to 'gt rig shutdown' but supports multiple rigs.
For each rig, it gracefully shuts down:
- All polecat sessions
- The refinery (if running)
- The witness (if running)

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig stop gastown
  gt rig stop gastown beads
  gt rig stop --force gastown beads
  gt rig stop --nuclear gastown  # DANGER: loses uncommitted work`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigStop,
}

var rigRestartCmd = &cobra.Command{
	Use:   "restart <rig>...",
	Short: "Restart one or more rigs (stop then start)",
	Long: `Restart the patrol agents (witness and refinery) for one or more rigs.

This is equivalent to 'gt rig stop' followed by 'gt rig start' for each rig.
Useful after polecats complete work and land their changes.

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig restart gastown
  gt rig restart gastown beads
  gt rig restart --force gastown beads
  gt rig restart --nuclear gastown  # DANGER: loses uncommitted work`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigRestart,
}

func init() {
	rigCmd.AddCommand(rigBootCmd)
	rigCmd.AddCommand(rigStartCmd)
	rigCmd.AddCommand(rigRebootCmd)
	rigCmd.AddCommand(rigShutdownCmd)
	rigCmd.AddCommand(rigStopCmd)
	rigCmd.AddCommand(rigRestartCmd)

	rigShutdownCmd.Flags().BoolVarP(&rigShutdownForce, "force", "f", false, "Force immediate shutdown")
	rigShutdownCmd.Flags().BoolVar(&rigShutdownNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")

	rigRebootCmd.Flags().BoolVarP(&rigShutdownForce, "force", "f", false, "Force immediate shutdown during reboot")

	rigStopCmd.Flags().BoolVarP(&rigStopForce, "force", "f", false, "Force immediate shutdown")
	rigStopCmd.Flags().BoolVar(&rigStopNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")

	rigRestartCmd.Flags().BoolVarP(&rigRestartForce, "force", "f", false, "Force immediate shutdown during restart")
	rigRestartCmd.Flags().BoolVar(&rigRestartNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")
}

func runRigBoot(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config and get rig
	rigsPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	fmt.Printf("Booting rig %s...\n", style.Bold.Render(rigName))

	var started []string
	var skipped []string

	t := tmux.NewTmux()

	// 1. Start the witness
	// Check actual tmux session, not state file (may be stale)
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
	witnessRunning, _ := t.HasSession(witnessSession)
	if witnessRunning {
		skipped = append(skipped, "witness (already running)")
	} else {
		fmt.Printf("  Starting witness...\n")
		witMgr := witness.NewManager(r)
		if err := witMgr.Start(false, "", nil); err != nil {
			if err == witness.ErrAlreadyRunning {
				skipped = append(skipped, "witness (already running)")
			} else {
				return fmt.Errorf("starting witness: %w", err)
			}
		} else {
			started = append(started, "witness")
		}
	}

	// 2. Start the refinery
	// Check actual tmux session, not state file (may be stale)
	refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
	refineryRunning, _ := t.HasSession(refinerySession)
	if refineryRunning {
		skipped = append(skipped, "refinery (already running)")
	} else {
		fmt.Printf("  Starting refinery...\n")
		refMgr := refinery.NewManager(r)
		if err := refMgr.Start(false); err != nil { // false = background mode
			return fmt.Errorf("starting refinery: %w", err)
		}
		started = append(started, "refinery")
	}

	// Report results
	if len(started) > 0 {
		fmt.Printf("%s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
	}
	if len(skipped) > 0 {
		fmt.Printf("%s Skipped: %s\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
	}

	return nil
}

func runRigStart(cmd *cobra.Command, args []string) error {
	// Find workspace once
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	t := tmux.NewTmux()

	var successRigs []string
	var failedRigs []string

	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failedRigs = append(failedRigs, rigName)
			continue
		}

		fmt.Printf("Starting rig %s...\n", style.Bold.Render(rigName))

		var started []string
		var skipped []string
		hasError := false

		// 1. Start the witness
		witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
		witnessRunning, _ := t.HasSession(witnessSession)
		if witnessRunning {
			skipped = append(skipped, "witness")
		} else {
			fmt.Printf("  Starting witness...\n")
			witMgr := witness.NewManager(r)
			if err := witMgr.Start(false, "", nil); err != nil {
				if err == witness.ErrAlreadyRunning {
					skipped = append(skipped, "witness")
				} else {
					fmt.Printf("  %s Failed to start witness: %v\n", style.Warning.Render("⚠"), err)
					hasError = true
				}
			} else {
				started = append(started, "witness")
			}
		}

		// 2. Start the refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
		refineryRunning, _ := t.HasSession(refinerySession)
		if refineryRunning {
			skipped = append(skipped, "refinery")
		} else {
			fmt.Printf("  Starting refinery...\n")
			refMgr := refinery.NewManager(r)
			if err := refMgr.Start(false); err != nil {
				fmt.Printf("  %s Failed to start refinery: %v\n", style.Warning.Render("⚠"), err)
				hasError = true
			} else {
				started = append(started, "refinery")
			}
		}

		// Report results for this rig
		if len(started) > 0 {
			fmt.Printf("  %s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
		}
		if len(skipped) > 0 {
			fmt.Printf("  %s Skipped: %s (already running)\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
		}

		if hasError {
			failedRigs = append(failedRigs, rigName)
		} else {
			successRigs = append(successRigs, rigName)
		}
		fmt.Println()
	}

	// Summary
	if len(successRigs) > 0 {
		fmt.Printf("%s Started rigs: %s\n", style.Success.Render("✓"), strings.Join(successRigs, ", "))
	}
	if len(failedRigs) > 0 {
		fmt.Printf("%s Failed rigs: %s\n", style.Warning.Render("⚠"), strings.Join(failedRigs, ", "))
		return fmt.Errorf("some rigs failed to start")
	}

	return nil
}

func runRigShutdown(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config and get rig
	rigsPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Check all polecats for uncommitted work (unless nuclear)
	if !rigShutdownNuclear {
		polecatGit := git.NewGit(r.Path)
		polecatMgr := polecat.NewManager(r, polecatGit)
		polecats, err := polecatMgr.List()
		if err == nil && len(polecats) > 0 {
			var problemPolecats []struct {
				name   string
				status *git.UncommittedWorkStatus
			}

			for _, p := range polecats {
				pGit := git.NewGit(p.ClonePath)
				status, err := pGit.CheckUncommittedWork()
				if err == nil && !status.Clean() {
					problemPolecats = append(problemPolecats, struct {
						name   string
						status *git.UncommittedWorkStatus
					}{p.Name, status})
				}
			}

			if len(problemPolecats) > 0 {
				fmt.Printf("\n%s Cannot shutdown - polecats have uncommitted work:\n\n", style.Warning.Render("⚠"))
				for _, pp := range problemPolecats {
					fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
				}
				fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
				return fmt.Errorf("refusing to shutdown with uncommitted work")
			}
		}
	}

	fmt.Printf("Shutting down rig %s...\n", style.Bold.Render(rigName))

	var errors []string

	// 1. Stop all polecat sessions
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)
	infos, err := polecatMgr.List()
	if err == nil && len(infos) > 0 {
		fmt.Printf("  Stopping %d polecat session(s)...\n", len(infos))
		if err := polecatMgr.StopAll(rigShutdownForce); err != nil {
			errors = append(errors, fmt.Sprintf("polecat sessions: %v", err))
		}
	}

	// 2. Stop the refinery
	refMgr := refinery.NewManager(r)
	refStatus, err := refMgr.Status()
	if err == nil && refStatus.State == refinery.StateRunning {
		fmt.Printf("  Stopping refinery...\n")
		if err := refMgr.Stop(); err != nil {
			errors = append(errors, fmt.Sprintf("refinery: %v", err))
		}
	}

	// 3. Stop the witness
	witMgr := witness.NewManager(r)
	witStatus, err := witMgr.Status()
	if err == nil && witStatus.State == witness.StateRunning {
		fmt.Printf("  Stopping witness...\n")
		if err := witMgr.Stop(); err != nil {
			errors = append(errors, fmt.Sprintf("witness: %v", err))
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\n%s Some agents failed to stop:\n", style.Warning.Render("⚠"))
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("shutdown incomplete")
	}

	fmt.Printf("%s Rig %s shut down successfully\n", style.Success.Render("✓"), rigName)
	return nil
}

func runRigReboot(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	fmt.Printf("Rebooting rig %s...\n\n", style.Bold.Render(rigName))

	// Shutdown first
	if err := runRigShutdown(cmd, args); err != nil {
		// If shutdown fails due to uncommitted work, propagate the error
		return err
	}

	fmt.Println() // Blank line between shutdown and boot

	// Boot
	if err := runRigBoot(cmd, args); err != nil {
		return fmt.Errorf("boot failed: %w", err)
	}

	fmt.Printf("\n%s Rig %s rebooted successfully\n", style.Success.Render("✓"), rigName)
	return nil
}

func runRigStop(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	// Track results
	var succeeded []string
	var failed []string

	// Process each rig
	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failed = append(failed, rigName)
			continue
		}

		// Check all polecats for uncommitted work (unless nuclear)
		if !rigStopNuclear {
			polecatGit := git.NewGit(r.Path)
			polecatMgr := polecat.NewManager(r, polecatGit)
			polecats, err := polecatMgr.List()
			if err == nil && len(polecats) > 0 {
				var problemPolecats []struct {
					name   string
					status *git.UncommittedWorkStatus
				}

				for _, p := range polecats {
					pGit := git.NewGit(p.ClonePath)
					status, err := pGit.CheckUncommittedWork()
					if err == nil && !status.Clean() {
						problemPolecats = append(problemPolecats, struct {
							name   string
							status *git.UncommittedWorkStatus
						}{p.Name, status})
					}
				}

				if len(problemPolecats) > 0 {
					fmt.Printf("\n%s Cannot stop %s - polecats have uncommitted work:\n", style.Warning.Render("⚠"), rigName)
					for _, pp := range problemPolecats {
						fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
					}
					failed = append(failed, rigName)
					continue
				}
			}
		}

		fmt.Printf("Stopping rig %s...\n", style.Bold.Render(rigName))

		var errors []string

		// 1. Stop all polecat sessions
		t := tmux.NewTmux()
		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err == nil && len(infos) > 0 {
			fmt.Printf("  Stopping %d polecat session(s)...\n", len(infos))
			if err := polecatMgr.StopAll(rigStopForce); err != nil {
				errors = append(errors, fmt.Sprintf("polecat sessions: %v", err))
			}
		}

		// 2. Stop the refinery
		refMgr := refinery.NewManager(r)
		refStatus, err := refMgr.Status()
		if err == nil && refStatus.State == refinery.StateRunning {
			fmt.Printf("  Stopping refinery...\n")
			if err := refMgr.Stop(); err != nil {
				errors = append(errors, fmt.Sprintf("refinery: %v", err))
			}
		}

		// 3. Stop the witness
		witMgr := witness.NewManager(r)
		witStatus, err := witMgr.Status()
		if err == nil && witStatus.State == witness.StateRunning {
			fmt.Printf("  Stopping witness...\n")
			if err := witMgr.Stop(); err != nil {
				errors = append(errors, fmt.Sprintf("witness: %v", err))
			}
		}

		if len(errors) > 0 {
			fmt.Printf("%s Some agents in %s failed to stop:\n", style.Warning.Render("⚠"), rigName)
			for _, e := range errors {
				fmt.Printf("  - %s\n", e)
			}
			failed = append(failed, rigName)
		} else {
			fmt.Printf("%s Rig %s stopped\n", style.Success.Render("✓"), rigName)
			succeeded = append(succeeded, rigName)
		}
	}

	// Summary
	if len(args) > 1 {
		fmt.Println()
		if len(succeeded) > 0 {
			fmt.Printf("%s Stopped: %s\n", style.Success.Render("✓"), strings.Join(succeeded, ", "))
		}
		if len(failed) > 0 {
			fmt.Printf("%s Failed: %s\n", style.Warning.Render("⚠"), strings.Join(failed, ", "))
			fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
			return fmt.Errorf("some rigs failed to stop")
		}
	} else if len(failed) > 0 {
		fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
		return fmt.Errorf("rig failed to stop")
	}

	return nil
}

func runRigRestart(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := townRoot + "/mayor/rigs.json"
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	t := tmux.NewTmux()

	// Track results
	var succeeded []string
	var failed []string

	// Process each rig
	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failed = append(failed, rigName)
			continue
		}

		fmt.Printf("Restarting rig %s...\n", style.Bold.Render(rigName))

		// Check all polecats for uncommitted work (unless nuclear)
		if !rigRestartNuclear {
			polecatGit := git.NewGit(r.Path)
			polecatMgr := polecat.NewManager(r, polecatGit)
			polecats, err := polecatMgr.List()
			if err == nil && len(polecats) > 0 {
				var problemPolecats []struct {
					name   string
					status *git.UncommittedWorkStatus
				}

				for _, p := range polecats {
					pGit := git.NewGit(p.ClonePath)
					status, err := pGit.CheckUncommittedWork()
					if err == nil && !status.Clean() {
						problemPolecats = append(problemPolecats, struct {
							name   string
							status *git.UncommittedWorkStatus
						}{p.Name, status})
					}
				}

				if len(problemPolecats) > 0 {
					fmt.Printf("\n%s Cannot restart %s - polecats have uncommitted work:\n", style.Warning.Render("⚠"), rigName)
					for _, pp := range problemPolecats {
						fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
					}
					failed = append(failed, rigName)
					continue
				}
			}
		}

		var stopErrors []string
		var startErrors []string

		// === STOP PHASE ===
		fmt.Printf("  Stopping...\n")

		// 1. Stop all polecat sessions
		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err == nil && len(infos) > 0 {
			fmt.Printf("    Stopping %d polecat session(s)...\n", len(infos))
			if err := polecatMgr.StopAll(rigRestartForce); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("polecat sessions: %v", err))
			}
		}

		// 2. Stop the refinery
		refMgr := refinery.NewManager(r)
		refStatus, err := refMgr.Status()
		if err == nil && refStatus.State == refinery.StateRunning {
			fmt.Printf("    Stopping refinery...\n")
			if err := refMgr.Stop(); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("refinery: %v", err))
			}
		}

		// 3. Stop the witness
		witMgr := witness.NewManager(r)
		witStatus, err := witMgr.Status()
		if err == nil && witStatus.State == witness.StateRunning {
			fmt.Printf("    Stopping witness...\n")
			if err := witMgr.Stop(); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("witness: %v", err))
			}
		}

		if len(stopErrors) > 0 {
			fmt.Printf("  %s Stop errors:\n", style.Warning.Render("⚠"))
			for _, e := range stopErrors {
				fmt.Printf("    - %s\n", e)
			}
			failed = append(failed, rigName)
			continue
		}

		// === START PHASE ===
		fmt.Printf("  Starting...\n")

		var started []string
		var skipped []string

		// 1. Start the witness
		witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
		witnessRunning, _ := t.HasSession(witnessSession)
		if witnessRunning {
			skipped = append(skipped, "witness")
		} else {
			fmt.Printf("    Starting witness...\n")
			if err := witMgr.Start(false, "", nil); err != nil {
				if err == witness.ErrAlreadyRunning {
					skipped = append(skipped, "witness")
				} else {
					fmt.Printf("    %s Failed to start witness: %v\n", style.Warning.Render("⚠"), err)
					startErrors = append(startErrors, fmt.Sprintf("witness: %v", err))
				}
			} else {
				started = append(started, "witness")
			}
		}

		// 2. Start the refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
		refineryRunning, _ := t.HasSession(refinerySession)
		if refineryRunning {
			skipped = append(skipped, "refinery")
		} else {
			fmt.Printf("    Starting refinery...\n")
			if err := refMgr.Start(false); err != nil {
				fmt.Printf("    %s Failed to start refinery: %v\n", style.Warning.Render("⚠"), err)
				startErrors = append(startErrors, fmt.Sprintf("refinery: %v", err))
			} else {
				started = append(started, "refinery")
			}
		}

		// Report results for this rig
		if len(started) > 0 {
			fmt.Printf("  %s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
		}
		if len(skipped) > 0 {
			fmt.Printf("  %s Skipped: %s (already running)\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
		}

		if len(startErrors) > 0 {
			fmt.Printf("  %s Start errors:\n", style.Warning.Render("⚠"))
			for _, e := range startErrors {
				fmt.Printf("    - %s\n", e)
			}
			failed = append(failed, rigName)
		} else {
			fmt.Printf("%s Rig %s restarted\n", style.Success.Render("✓"), rigName)
			succeeded = append(succeeded, rigName)
		}
		fmt.Println()
	}

	// Summary
	if len(args) > 1 {
		if len(succeeded) > 0 {
			fmt.Printf("%s Restarted: %s\n", style.Success.Render("✓"), strings.Join(succeeded, ", "))
		}
		if len(failed) > 0 {
			fmt.Printf("%s Failed: %s\n", style.Warning.Render("⚠"), strings.Join(failed, ", "))
			fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
			return fmt.Errorf("some rigs failed to restart")
		}
	} else if len(failed) > 0 {
		fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
		return fmt.Errorf("rig failed to restart")
	}

	return nil
}
