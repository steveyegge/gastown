package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	stopAll      bool
	stopRig      string
	stopGraceful bool
)

var stopCmd = &cobra.Command{
	Use:     "stop",
	GroupID: GroupServices,
	Short:   "Emergency stop for sessions",
	Long: `Emergency stop command for Gas Town sessions.

Stops all running polecat sessions across rigs. Use for emergency shutdown
when you need to halt all agent activity immediately.

Examples:
  gt stop --all              # Kill ALL sessions across all rigs
  gt stop --rig wyvern       # Kill all sessions in the wyvern rig
  gt stop --all --graceful   # Try graceful shutdown first`,
	RunE: runStop,
}

func init() {
	stopCmd.Flags().BoolVar(&stopAll, "all", false, "Stop all sessions across all rigs")
	stopCmd.Flags().StringVar(&stopRig, "rig", "", "Stop all sessions in a specific rig")
	stopCmd.Flags().BoolVar(&stopGraceful, "graceful", false, "Try graceful shutdown before force kill")
	rootCmd.AddCommand(stopCmd)
}

// StopResult tracks what was stopped.
type StopResult struct {
	Rig       string
	Polecat   string
	SessionID string
	Success   bool
	Error     string
}

func runStop(cmd *cobra.Command, args []string) error {
	if !stopAll && stopRig == "" {
		return fmt.Errorf("must specify --all or --rig <name>")
	}

	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Get rigs to stop
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	rigs, err := rigMgr.DiscoverRigs()
	if err != nil {
		return fmt.Errorf("discovering rigs: %w", err)
	}

	// Filter by rig if specified
	if stopRig != "" {
		var filtered []*rig.Rig
		for _, r := range rigs {
			if r.Name == stopRig {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("rig '%s' not found", stopRig)
		}
		rigs = filtered
	}

	// Determine force mode
	force := !stopGraceful

	if stopAll {
		fmt.Printf("%s Stopping ALL Gas Town sessions...\n\n",
			style.Bold.Render("🛑"))
	} else {
		fmt.Printf("%s Stopping sessions in rig '%s'...\n\n",
			style.Bold.Render("🛑"), stopRig)
	}

	// Stop sessions in each rig
	t := tmux.NewTmux()
	var results []StopResult
	stopped := 0

	for _, r := range rigs {
		mgr := session.NewManager(t, r)
		infos, err := mgr.List()
		if err != nil {
			continue
		}

		for _, info := range infos {
			result := StopResult{
				Rig:       r.Name,
				Polecat:   info.Polecat,
				SessionID: info.SessionID,
			}

			// Capture output before stopping (best effort)
			output, _ := mgr.Capture(info.Polecat, 50)

			// Stop the session
			err := mgr.Stop(info.Polecat, force)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				fmt.Printf("  %s [%s] %s: %s\n",
					style.Dim.Render("✗"),
					r.Name, info.Polecat,
					style.Dim.Render(err.Error()))
			} else {
				result.Success = true
				stopped++
				fmt.Printf("  %s [%s] %s: stopped\n",
					style.Bold.Render("✓"),
					r.Name, info.Polecat)

				// Log kill event
				agent := fmt.Sprintf("%s/%s", r.Name, info.Polecat)
				logger := townlog.NewLogger(townRoot)
				_ = logger.Log(townlog.EventKill, agent, "gt stop")

				// Log kill event to activity feed
				_ = events.LogFeed(events.TypeKill, "gt", events.KillPayload(r.Name, info.Polecat, "gt stop"))

				// Log captured output (truncated)
				if len(output) > 200 {
					output = output[len(output)-200:]
				}
				if output != "" {
					fmt.Printf("      %s\n", style.Dim.Render("(output captured)"))
				}
			}

			results = append(results, result)
		}
	}

	// Summary
	fmt.Println()
	if stopped == 0 {
		fmt.Println("No active sessions to stop.")
	} else {
		fmt.Printf("%s %d session(s) stopped.\n",
			style.Bold.Render("✓"), stopped)
	}

	// Report failures
	failures := 0
	for _, r := range results {
		if !r.Success {
			failures++
		}
	}
	if failures > 0 {
		fmt.Printf("%s %d session(s) failed to stop.\n",
			style.Dim.Render("⚠"), failures)
	}

	return nil
}
