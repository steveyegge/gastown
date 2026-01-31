// ABOUTME: Command to check and manage rate limit state for polecats.
// ABOUTME: Shows current rate limit status and allows manual clearing.

package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/ratelimit"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var rateLimitCmd = &cobra.Command{
	Use:     "rate-limit",
	Aliases: []string{"ratelimit", "rl"},
	GroupID: GroupDiag,
	Short:   "Check and manage rate limit state",
	Long: `Check and manage rate limit state for polecats.

When polecats hit Claude API rate limits, Gas Town tracks this state and
implements exponential backoff to avoid wasted spawn attempts.

Subcommands:
  status  - Show current rate limit state (default)
  clear   - Manually clear rate limit state

Examples:
  gt rate-limit            # Show rate limit status
  gt rate-limit status     # Same as above
  gt rate-limit clear      # Clear rate limit state to allow immediate spawns`,
	RunE: runRateLimitStatus,
}

var rateLimitStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current rate limit state",
	RunE:  runRateLimitStatus,
}

var rateLimitClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear rate limit state",
	Long: `Clear rate limit state to allow immediate spawns.

Use this when:
  - You know the rate limit has been lifted
  - You want to retry spawning despite the backoff
  - You've switched to a different account`,
	RunE: runRateLimitClear,
}

var (
	rateLimitRig  string
	rateLimitJSON bool
)

func init() {
	rootCmd.AddCommand(rateLimitCmd)
	rateLimitCmd.AddCommand(rateLimitStatusCmd)
	rateLimitCmd.AddCommand(rateLimitClearCmd)

	// Flags for all rate-limit commands
	rateLimitCmd.PersistentFlags().StringVarP(&rateLimitRig, "rig", "r", "", "Target rig (default: current)")
	rateLimitCmd.PersistentFlags().BoolVar(&rateLimitJSON, "json", false, "Output in JSON format")
}

func runRateLimitStatus(cmd *cobra.Command, args []string) error {
	rigPath, err := resolveRigPath(rateLimitRig)
	if err != nil {
		return err
	}

	tracker := ratelimit.NewTracker(rigPath)
	if err := tracker.Load(); err != nil {
		return fmt.Errorf("loading rate limit state: %w", err)
	}

	state := tracker.State()

	if rateLimitJSON {
		output := map[string]interface{}{
			"limited":          state.Limited,
			"detected_at":      state.DetectedAt,
			"consecutive_hits": state.ConsecutiveHits,
			"backoff_until":    state.BackoffUntil,
			"account":          state.Account,
			"source":           state.Source,
			"should_defer":     tracker.ShouldDefer(),
			"time_until_ready": tracker.TimeUntilReady().String(),
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	rigName := filepath.Base(rigPath)
	fmt.Printf("Rate Limit Status for %s\n", style.Bold.Render(rigName))
	fmt.Println()

	if !state.Limited {
		fmt.Printf("  %s No rate limit active\n", style.Success.Render("✓"))
		fmt.Println("  Polecats can spawn immediately.")
		return nil
	}

	fmt.Printf("  %s Rate limit detected\n", style.Warning.Render("⚠"))
	fmt.Println()

	if !state.DetectedAt.IsZero() {
		fmt.Printf("  Detected: %s\n", state.DetectedAt.Format(time.RFC3339))
	}
	if state.Source != "" {
		fmt.Printf("  Source:   %s\n", state.Source)
	}
	if state.Account != "" {
		fmt.Printf("  Account:  %s\n", state.Account)
	}
	fmt.Printf("  Hits:     %d consecutive\n", state.ConsecutiveHits)

	if tracker.ShouldDefer() {
		waitTime := tracker.TimeUntilReady().Round(time.Second)
		fmt.Printf("\n  Backoff active: %s until retry allowed\n", style.Warning.Render(waitTime.String()))
		fmt.Printf("  Ready at: %s\n", state.BackoffUntil.Format(time.RFC3339))
	} else {
		fmt.Printf("\n  %s Backoff period has elapsed\n", style.Success.Render("✓"))
		fmt.Println("  Spawns will be attempted, but may hit rate limit again.")
	}

	fmt.Println()
	fmt.Printf("Use %s to clear the rate limit state.\n", style.Dim.Render("gt rate-limit clear"))

	return nil
}

func runRateLimitClear(cmd *cobra.Command, args []string) error {
	rigPath, err := resolveRigPath(rateLimitRig)
	if err != nil {
		return err
	}

	tracker := ratelimit.NewTracker(rigPath)
	if err := tracker.Load(); err != nil {
		return fmt.Errorf("loading rate limit state: %w", err)
	}

	state := tracker.State()
	if !state.Limited {
		fmt.Printf("%s No rate limit was active.\n", style.Dim.Render("ℹ"))
		return nil
	}

	tracker.Clear()
	if err := tracker.Save(); err != nil {
		return fmt.Errorf("saving rate limit state: %w", err)
	}

	rigName := filepath.Base(rigPath)
	fmt.Printf("%s Rate limit cleared for %s\n", style.Success.Render("✓"), rigName)
	fmt.Println("  Polecat spawns will now be attempted immediately.")
	fmt.Println()
	fmt.Printf("  %s If the rate limit is still active, spawns will fail again.\n", style.Warning.Render("Note:"))

	return nil
}

// resolveRigPath returns the rig path for the specified rig name, or the current rig.
func resolveRigPath(rigName string) (string, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	if rigName == "" {
		// Try to detect rig from current directory
		rigs, err := rigMgr.DiscoverRigs()
		if err == nil && len(rigs) == 1 {
			return rigs[0].Path, nil
		}
		// Fall back to asking for explicit rig
		return "", fmt.Errorf("could not detect rig - use --rig to specify")
	}

	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return "", fmt.Errorf("rig '%s' not found: %w", rigName, err)
	}

	return r.Path, nil
}
