package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicPRResolveCrew string
)

var epicPRResolveCmd = &cobra.Command{
	Use:   "resolve <pr-number>",
	Short: "Manual conflict resolution workflow",
	Long: `Start a manual conflict resolution workflow for a PR.

This command:
1. Checks out the conflicting PR branch
2. Creates a conflict-resolution work bead
3. Optionally slings to crew member for resolution
4. When resolved: commit, push, and mark bead done

EXAMPLES:

  gt epic pr resolve 102
  gt epic pr resolve 102 --crew dave`,
	Args: cobra.ExactArgs(1),
	RunE: runEpicPRResolve,
}

func init() {
	epicPRResolveCmd.Flags().StringVar(&epicPRResolveCrew, "crew", "", "Crew member to sling resolution work to")
}

func runEpicPRResolve(cmd *cobra.Command, args []string) error {
	prNumStr := args[0]
	var prNum int
	if _, err := fmt.Sscanf(prNumStr, "%d", &prNum); err != nil {
		return fmt.Errorf("invalid PR number: %s", prNumStr)
	}

	// Find town root and detect rig from cwd
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Detect rig from current directory
	cwd, _ := os.Getwd()
	rigName := detectRigFromPath(cwd, townRoot)
	if rigName == "" {
		return fmt.Errorf("could not detect rig from current directory")
	}

	rigPath := filepath.Join(townRoot, rigName)
	repoDir := filepath.Join(rigPath, "mayor", "rig")

	// Get PR details
	fmt.Printf("%s Checking PR #%d...\n\n", style.Bold.Render("🔍"), prNum)

	prInfo, err := getPRDetails(repoDir, prNum)
	if err != nil {
		return fmt.Errorf("fetching PR details: %w", err)
	}

	if prInfo.State == "MERGED" || prInfo.State == "CLOSED" {
		return fmt.Errorf("PR #%d is %s, no conflicts to resolve", prNum, prInfo.State)
	}

	// Get head branch
	headBranch, err := getPRHeadBranch(repoDir, prNum)
	if err != nil {
		return fmt.Errorf("getting PR branch: %w", err)
	}
	headBranch = strings.TrimSpace(headBranch)

	fmt.Printf("  PR: %s\n", prInfo.Title)
	fmt.Printf("  Branch: %s\n", headBranch)
	fmt.Printf("  Base: %s\n", prInfo.Base)

	// Check if there are actually conflicts
	// This is a simplified check - we'd need to attempt merge to truly know

	fmt.Println()
	fmt.Println("Conflict resolution workflow:")
	fmt.Println()

	if epicPRResolveCrew != "" {
		// Sling to crew member
		fmt.Printf("Creating conflict resolution bead for %s...\n", epicPRResolveCrew)

		// Create work bead
		desc := fmt.Sprintf(`Resolve merge conflicts in PR #%d

## Context
- Branch: %s
- Base: %s
- PR: %s

## Steps
1. Checkout branch: gh pr checkout %d
2. Rebase on base: git rebase %s
3. Resolve conflicts in each file
4. Continue rebase: git rebase --continue
5. Force push: git push --force-with-lease
6. Mark this bead done

conflict_pr: %d
`, prNum, headBranch, prInfo.Base, prInfo.Title, prNum, prInfo.Base, prNum)

		createArgs := []string{"create",
			"--title", fmt.Sprintf("Resolve conflicts in PR #%d", prNum),
			"--description", desc,
			"--type", "task",
			"--json",
		}
		createCmd := exec.Command("bd", createArgs...)
		createCmd.Dir = repoDir

		out, err := createCmd.Output()
		if err != nil {
			return fmt.Errorf("creating work bead: %w", err)
		}

		// Parse bead ID
		beadID := extractBeadIDFromJSON(string(out))
		if beadID == "" {
			return fmt.Errorf("could not parse created bead ID")
		}

		fmt.Printf("  Created: %s\n", beadID)

		// Sling to crew
		target := fmt.Sprintf("%s/crew/%s", rigName, epicPRResolveCrew)
		slingArgs := []string{"sling", beadID, target}
		slingCmd := exec.Command("gt", slingArgs...)
		slingCmd.Stdout = os.Stdout
		slingCmd.Stderr = os.Stderr

		if err := slingCmd.Run(); err != nil {
			return fmt.Errorf("slinging work: %w", err)
		}

		fmt.Printf("\n%s Work slung to %s\n", style.Bold.Render("✓"), epicPRResolveCrew)

	} else {
		// Manual resolution - show instructions
		fmt.Println("Manual resolution steps:")
		fmt.Println()
		fmt.Printf("  1. Checkout the PR:\n")
		fmt.Printf("     gh pr checkout %d\n", prNum)
		fmt.Println()
		fmt.Printf("  2. Start rebase:\n")
		fmt.Printf("     git rebase %s\n", prInfo.Base)
		fmt.Println()
		fmt.Println("  3. For each conflict:")
		fmt.Println("     - Edit the conflicting files")
		fmt.Println("     - git add <resolved-files>")
		fmt.Println("     - git rebase --continue")
		fmt.Println()
		fmt.Println("  4. Push the resolved branch:")
		fmt.Println("     git push --force-with-lease")
		fmt.Println()
		fmt.Println("  5. Verify PR is updated on GitHub")
		fmt.Println()

		// Offer to start
		fmt.Print("Start resolution now (checkout branch)? [y/N] ")
		var response string
		fmt.Scanln(&response)

		if strings.ToLower(strings.TrimSpace(response)) == "y" {
			checkoutCmd := exec.Command("gh", "pr", "checkout", fmt.Sprintf("%d", prNum))
			checkoutCmd.Dir = repoDir
			checkoutCmd.Stdout = os.Stdout
			checkoutCmd.Stderr = os.Stderr

			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("checking out PR: %w", err)
			}

			fmt.Printf("\n%s Checked out PR #%d\n", style.Bold.Render("✓"), prNum)
			fmt.Printf("Now run: git rebase %s\n", prInfo.Base)
		}
	}

	return nil
}

func extractBeadIDFromJSON(jsonStr string) string {
	// Simple extraction - look for "id": "xxx"
	if idx := strings.Index(jsonStr, `"id"`); idx >= 0 {
		start := strings.Index(jsonStr[idx:], `"`)
		if start >= 0 {
			start += idx + 1
			end := strings.Index(jsonStr[start:], `"`)
			if end >= 0 {
				start2 := strings.Index(jsonStr[start:], `"`)
				if start2 >= 0 {
					start2 += start + 1
					end2 := strings.Index(jsonStr[start2:], `"`)
					if end2 >= 0 {
						return jsonStr[start2 : start2+end2]
					}
				}
			}
		}
	}
	return ""
}
