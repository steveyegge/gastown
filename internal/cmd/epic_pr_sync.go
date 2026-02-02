package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	epicpkg "github.com/steveyegge/gastown/internal/epic"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicPRSyncDryRun bool
	epicPRSyncRemote string
)

var epicPRSyncCmd = &cobra.Command{
	Use:   "sync [epic-id]",
	Short: "Rebase PR branches on upstream",
	Long: `Rebase all PR branches on their base branches.

This command:
1. Fetches upstream main
2. Rebases integration branch on upstream/main
3. For each PR branch, rebases on its base:
   - Root PRs: rebase on main
   - Dependent PRs: rebase on their base PR's branch
4. Force-pushes updated branches
5. Reports conflicts if any (doesn't auto-resolve)

EXAMPLES:

  gt epic pr sync gt-epic-abc12
  gt epic pr sync gt-epic-abc12 --dry-run
  gt epic pr sync gt-epic-abc12 --remote upstream`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicPRSync,
}

func init() {
	epicPRSyncCmd.Flags().BoolVarP(&epicPRSyncDryRun, "dry-run", "n", false, "Preview without making changes")
	epicPRSyncCmd.Flags().StringVar(&epicPRSyncRemote, "remote", "origin", "Upstream remote name")
}

func runEpicPRSync(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	rigPath := filepath.Join(townRoot, rigName)
	beadsDir := filepath.Join(rigPath, "mayor", "rig")
	repoDir := filepath.Join(rigPath, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, fields, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Parse PR URLs
	prURLs := epicpkg.ParseUpstreamPRs(fields.UpstreamPRs)
	if len(prURLs) == 0 {
		fmt.Println("No upstream PRs found for this epic.")
		return nil
	}

	fmt.Printf("%s Syncing %d PR branch(es)...\n\n", style.Bold.Render("🔄"), len(prURLs))

	// Step 1: Fetch upstream
	fmt.Printf("%s Fetching %s...\n", style.Bold.Render("→"), epicPRSyncRemote)
	if epicPRSyncDryRun {
		fmt.Printf("  Would run: git fetch %s\n", epicPRSyncRemote)
	} else {
		if err := epicpkg.FetchUpstream(repoDir, epicPRSyncRemote); err != nil {
			return fmt.Errorf("fetching upstream: %w", err)
		}
		fmt.Printf("  %s Done\n", style.Bold.Render("✓"))
	}

	// Get default branch
	defaultBranch := getDefaultBranch(repoDir)

	// Step 2: Rebase integration branch if it exists
	if fields.IntegrationBr != "" {
		fmt.Printf("\n%s Rebasing integration branch: %s\n", style.Bold.Render("→"), fields.IntegrationBr)
		if epicPRSyncDryRun {
			fmt.Printf("  Would rebase %s onto %s/%s\n", fields.IntegrationBr, epicPRSyncRemote, defaultBranch)
		} else {
			result, err := epicpkg.RebaseBranch(repoDir, fields.IntegrationBr, fmt.Sprintf("%s/%s", epicPRSyncRemote, defaultBranch))
			if err != nil {
				return fmt.Errorf("rebasing integration branch: %w", err)
			}
			if !result.Success {
				fmt.Printf("  %s Conflicts detected:\n", style.Warning.Render("⚠"))
				for _, f := range result.Conflicts.Files {
					fmt.Printf("    - %s\n", f)
				}
				fmt.Println()
				fmt.Println("Resolve conflicts manually:")
				fmt.Printf("  gt epic pr resolve %s\n", epicID)
				return nil
			}
			fmt.Printf("  %s Rebased %d commit(s)\n", style.Bold.Render("✓"), result.CommitCount)
		}
	}

	// Step 3: For each PR, get branch and rebase
	var conflicts []struct {
		PRNum  int
		Branch string
		Files  []string
	}

	for _, url := range prURLs {
		_, _, prNum, err := epicpkg.ParsePRURL(url)
		if err != nil {
			continue
		}

		// Get PR details to find head branch and base
		prInfo, err := getPRDetails(repoDir, prNum)
		if err != nil {
			fmt.Printf("  %s PR #%d: Could not fetch details\n", style.Warning.Render("⚠"), prNum)
			continue
		}

		if prInfo.State == "MERGED" || prInfo.State == "CLOSED" {
			fmt.Printf("  %s PR #%d: %s (skipped)\n", style.Dim.Render("○"), prNum, prInfo.State)
			continue
		}

		// Get head branch name
		headBranch, err := getPRHeadBranch(repoDir, prNum)
		if err != nil {
			fmt.Printf("  %s PR #%d: Could not determine branch\n", style.Warning.Render("⚠"), prNum)
			continue
		}

		fmt.Printf("\n%s Rebasing PR #%d: %s\n", style.Bold.Render("→"), prNum, headBranch)
		fmt.Printf("  Base: %s\n", prInfo.Base)

		if epicPRSyncDryRun {
			fmt.Printf("  Would rebase %s onto %s\n", headBranch, prInfo.Base)
			fmt.Printf("  Would force-push to %s\n", epicPRSyncRemote)
			continue
		}

		// Rebase
		result, err := epicpkg.RebaseBranch(repoDir, headBranch, prInfo.Base)
		if err != nil {
			fmt.Printf("  %s Error: %v\n", style.Error.Render("✗"), err)
			continue
		}

		if !result.Success {
			conflicts = append(conflicts, struct {
				PRNum  int
				Branch string
				Files  []string
			}{prNum, headBranch, result.Conflicts.Files})
			fmt.Printf("  %s Conflicts detected\n", style.Warning.Render("⚠"))
			continue
		}

		// Force push
		if err := epicpkg.ForcePushBranch(repoDir, epicPRSyncRemote, headBranch); err != nil {
			fmt.Printf("  %s Push failed: %v\n", style.Error.Render("✗"), err)
			continue
		}

		fmt.Printf("  %s Rebased %d commit(s) and pushed\n", style.Bold.Render("✓"), result.CommitCount)
	}

	// Summary
	fmt.Println()
	if len(conflicts) > 0 {
		fmt.Printf("%s %d branch(es) have conflicts:\n\n", style.Warning.Render("⚠"), len(conflicts))
		for _, c := range conflicts {
			fmt.Printf("  PR #%d (%s):\n", c.PRNum, c.Branch)
			for _, f := range c.Files {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println()
		fmt.Println("Resolve conflicts with:")
		for _, c := range conflicts {
			fmt.Printf("  gt epic pr resolve %d\n", c.PRNum)
		}
	} else {
		fmt.Printf("%s All branches synced successfully!\n", style.Bold.Render("✓"))
	}

	return nil
}

// getPRHeadBranch gets the head branch name for a PR.
func getPRHeadBranch(repoDir string, prNum int) (string, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNum), "--json", "headRefName", "-q", ".headRefName")
	cmd.Dir = repoDir

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}
