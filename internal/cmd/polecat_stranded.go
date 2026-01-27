package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	polecatStrandedVerbose bool
)

var polecatStrandedCmd = &cobra.Command{
	Use:   "stranded",
	Short: "Find stranded work in polecat worktrees",
	Long: `Scan polecat worktrees for branches with unmerged commits.

After a crash, polecats may have commits on their local branches that were
never pushed to origin. This command detects such stranded work by comparing
each polecat's current branch against origin/main.

A worktree is considered to have stranded work if:
- It has commits not in origin/main (git rev-list --count origin/main..HEAD > 0)
- AND there's no active tmux session (polecat is dead)

Examples:
  gt polecat stranded           # List worktrees with stranded commits
  gt polecat stranded -v        # Include branch names and commit details`,
	RunE: runPolecatStranded,
}

func init() {
	polecatStrandedCmd.Flags().BoolVarP(&polecatStrandedVerbose, "verbose", "v", false, "Show detailed commit information")
	polecatCmd.AddCommand(polecatStrandedCmd)
}

// StrandedWorktree represents a polecat worktree with unmerged commits
type StrandedWorktree struct {
	Name       string   // Polecat name
	Path       string   // Full path to worktree
	Branch     string   // Current branch name
	Commits    int      // Number of commits not in origin/main
	HasSession bool     // Whether tmux session is active
	CommitInfo []string // Commit subjects (for verbose mode)
}

// runPolecatStranded scans polecat worktrees for stranded work
func runPolecatStranded(cmd *cobra.Command, args []string) error {
	// Find workspace to determine rig root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	rigName, r, err := findCurrentRig(townRoot)
	if err != nil {
		return fmt.Errorf("determining rig: %w", err)
	}

	fmt.Printf("Scanning polecat worktrees in %s for stranded work...\n\n", rigName)

	// Find stranded worktrees
	stranded, err := findStrandedWorktrees(r.Path)
	if err != nil {
		return fmt.Errorf("finding stranded worktrees: %w", err)
	}

	if len(stranded) == 0 {
		fmt.Printf("%s No stranded work found in polecat worktrees\n", style.Bold.Render("✓"))
		return nil
	}

	// Display results
	fmt.Printf("%s Found %d polecat worktree(s) with stranded commits:\n\n", style.Warning.Render("⚠"), len(stranded))

	for _, s := range stranded {
		sessionStatus := ""
		if s.HasSession {
			sessionStatus = style.Dim.Render(" (session active)")
		}
		fmt.Printf("  %s %s%s\n", style.Bold.Render(s.Name), style.Dim.Render(fmt.Sprintf("(%d commits)", s.Commits)), sessionStatus)
		fmt.Printf("    Path: %s\n", s.Path)
		if polecatStrandedVerbose {
			fmt.Printf("    Branch: %s\n", s.Branch)
			if len(s.CommitInfo) > 0 {
				fmt.Printf("    Commits:\n")
				for _, commit := range s.CommitInfo {
					fmt.Printf("      • %s\n", commit)
				}
			}
		}
		fmt.Println()
	}

	// Recovery hints
	fmt.Printf("%s\n", style.Dim.Render("To recover stranded work:"))
	fmt.Printf("%s\n", style.Dim.Render("  cd <path>                       # Enter the worktree"))
	fmt.Printf("%s\n", style.Dim.Render("  git log origin/main..HEAD       # View stranded commits"))
	fmt.Printf("%s\n", style.Dim.Render("  git push origin HEAD:main       # Push to main (if appropriate)"))
	fmt.Printf("%s\n", style.Dim.Render("  git format-patch origin/main    # Export as patches"))

	return nil
}

// findStrandedWorktrees scans polecat directories for worktrees with unmerged commits
func findStrandedWorktrees(rigPath string) ([]StrandedWorktree, error) {
	polecatsDir := rigPath + "/polecats"

	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No polecats directory
		}
		return nil, fmt.Errorf("reading polecats dir: %w", err)
	}

	var stranded []StrandedWorktree
	rigName := rigPath[strings.LastIndex(rigPath, "/")+1:]

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatName := entry.Name()

		// Try new structure first: polecats/<name>/<rigname>/
		clonePath := rigPath + "/polecats/" + polecatName + "/" + rigName
		if _, err := os.Stat(clonePath + "/.git"); os.IsNotExist(err) {
			// Try old structure: polecats/<name>/
			clonePath = rigPath + "/polecats/" + polecatName
			if _, err := os.Stat(clonePath + "/.git"); os.IsNotExist(err) {
				continue // Not a git worktree
			}
		}

		// Get the worktree's current branch
		branchCmd := exec.Command("git", "branch", "--show-current")
		branchCmd.Dir = clonePath
		branchOut, err := branchCmd.Output()
		if err != nil {
			continue // Can't get branch, skip
		}
		branch := strings.TrimSpace(string(branchOut))
		if branch == "" {
			continue // Detached HEAD, skip
		}

		// Count commits not in origin/main
		countCmd := exec.Command("git", "rev-list", "--count", "origin/main..HEAD")
		countCmd.Dir = clonePath
		countOut, err := countCmd.Output()
		if err != nil {
			// Try origin/master as fallback
			countCmd = exec.Command("git", "rev-list", "--count", "origin/master..HEAD")
			countCmd.Dir = clonePath
			countOut, err = countCmd.Output()
			if err != nil {
				continue // Can't compare to origin
			}
		}

		count, err := strconv.Atoi(strings.TrimSpace(string(countOut)))
		if err != nil || count == 0 {
			continue // No stranded commits
		}

		// Check for active tmux session
		sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
		hasSessionCmd := exec.Command("tmux", "has-session", "-t", sessionName)
		hasSession := hasSessionCmd.Run() == nil

		// Get commit info for verbose mode
		var commitInfo []string
		if polecatStrandedVerbose {
			logCmd := exec.Command("git", "log", "--oneline", "origin/main..HEAD", "--format=%h %s")
			logCmd.Dir = clonePath
			logOut, err := logCmd.Output()
			if err == nil {
				lines := strings.Split(strings.TrimSpace(string(logOut)), "\n")
				// Limit to first 5 commits
				maxCommits := 5
				if len(lines) < maxCommits {
					maxCommits = len(lines)
				}
				commitInfo = lines[:maxCommits]
				if len(lines) > 5 {
					commitInfo = append(commitInfo, fmt.Sprintf("... and %d more", len(lines)-5))
				}
			}
		}

		stranded = append(stranded, StrandedWorktree{
			Name:       polecatName,
			Path:       clonePath,
			Branch:     branch,
			Commits:    count,
			HasSession: hasSession,
			CommitInfo: commitInfo,
		})
	}

	return stranded, nil
}
