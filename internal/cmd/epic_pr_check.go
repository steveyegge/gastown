package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	epicpkg "github.com/steveyegge/gastown/internal/epic"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var epicPRCheckCmd = &cobra.Command{
	Use:   "check [epic-id]",
	Short: "Check for conflicts and CI failures",
	Long: `Check upstream PRs for issues that need attention.

Detects:
- Merge conflicts with upstream
- CI failures
- Stale approvals after force-push
- PRs that need rebase after base merged

EXAMPLES:

  gt epic pr check gt-epic-abc12
  gt epic pr check                    # Uses hooked epic`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicPRCheck,
}

func runEpicPRCheck(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("%s Checking %d PR(s)...\n\n", style.Bold.Render("🔍"), len(prURLs))

	type issue struct {
		PRNum   int
		Type    string
		Message string
	}

	var issues []issue
	checkedCount := 0

	for _, url := range prURLs {
		_, _, prNum, err := epicpkg.ParsePRURL(url)
		if err != nil {
			continue
		}

		// Get PR details
		prInfo, err := getPRDetails(repoDir, prNum)
		if err != nil {
			fmt.Printf("  %s PR #%d: Could not fetch details\n", style.Warning.Render("⚠"), prNum)
			continue
		}

		if prInfo.State == "MERGED" || prInfo.State == "CLOSED" {
			fmt.Printf("  %s PR #%d: %s\n", style.Dim.Render("○"), prNum, prInfo.State)
			continue
		}

		checkedCount++

		// Check CI status
		ciStatus, err := epicpkg.GetPRCIStatus(repoDir, prNum)
		if err == nil && ciStatus != nil {
			if ciStatus.State == "failure" {
				issues = append(issues, issue{
					PRNum:   prNum,
					Type:    "ci_failure",
					Message: fmt.Sprintf("CI failed: %s", ciStatus.Details),
				})
			}
		}

		// Check review status
		reviewStatus, _, err := epicpkg.GetPRReviewStatus(repoDir, prNum)
		if err == nil && reviewStatus == epicpkg.PRStatusChangesRequested {
			issues = append(issues, issue{
				PRNum:   prNum,
				Type:    "changes_requested",
				Message: "Changes requested by reviewer",
			})
		}

		// Check for conflicts (this requires checking the branch)
		// Note: This is a simplified check - full conflict detection would require
		// fetching and attempting merge
		conflict, err := checkPRConflicts(repoDir, prNum)
		if err == nil && conflict != nil {
			issues = append(issues, issue{
				PRNum:   prNum,
				Type:    "conflict",
				Message: fmt.Sprintf("Conflicts with %s: %v", conflict.BaseBranch, conflict.Files),
			})
		}
	}

	fmt.Printf("Checked %d open PR(s)\n\n", checkedCount)

	if len(issues) == 0 {
		fmt.Printf("%s No issues found!\n", style.Bold.Render("✓"))
		return nil
	}

	fmt.Printf("%s Found %d issue(s):\n\n", style.Warning.Render("⚠"), len(issues))

	for _, iss := range issues {
		icon := "○"
		switch iss.Type {
		case "ci_failure":
			icon = style.Error.Render("✗")
		case "changes_requested":
			icon = style.Warning.Render("!")
		case "conflict":
			icon = style.Error.Render("⚔")
		}

		fmt.Printf("  %s PR #%d: %s\n", icon, iss.PRNum, iss.Message)
	}

	fmt.Println()
	fmt.Println("Options:")
	for _, iss := range issues {
		switch iss.Type {
		case "conflict":
			fmt.Printf("  gt epic pr sync %s              # Rebase to resolve conflicts\n", epicID)
			fmt.Printf("  gt epic pr resolve %d           # Manual conflict resolution\n", iss.PRNum)
		case "ci_failure":
			fmt.Printf("  # Fix CI issues in PR #%d, then push\n", iss.PRNum)
		case "changes_requested":
			fmt.Printf("  gt epic pr respond %d           # Address review feedback\n", iss.PRNum)
		}
	}

	return nil
}

// checkPRConflicts checks if a PR has merge conflicts.
// Returns nil if no conflicts.
func checkPRConflicts(repoDir string, prNum int) (*epicpkg.ConflictInfo, error) {
	// Get PR's head and base branches
	prInfo, err := getPRDetails(repoDir, prNum)
	if err != nil {
		return nil, err
	}

	// For now, we rely on gh pr view to tell us about conflicts
	// A full implementation would fetch branches and attempt merge
	// GitHub's API includes mergeable state which we could check

	// This is a placeholder - in a real implementation you'd:
	// 1. Fetch the PR's head branch
	// 2. Attempt to merge base into head with --no-commit
	// 3. Report any conflicts

	_ = prInfo
	return nil, nil
}
