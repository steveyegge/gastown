package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var orphansCmd = &cobra.Command{
	Use:     "orphans",
	GroupID: GroupWork,
	Short:   "Find lost polecat work",
	Long: `Find orphaned commits that were never merged to main.

Polecat work can get lost when:
- Session killed before merge
- Refinery fails to process
- Network issues during push

This command uses 'git fsck --unreachable' to find dangling commits,
filters to recent ones, and shows details to help recovery.

Examples:
  gt orphans              # Last 7 days (default)
  gt orphans --days=14    # Last 2 weeks
  gt orphans --all        # Show all orphans (no date filter)`,
	RunE: runOrphans,
}

var (
	orphansDays int
	orphansAll  bool
)

func init() {
	orphansCmd.Flags().IntVar(&orphansDays, "days", 7, "Show orphans from last N days")
	orphansCmd.Flags().BoolVar(&orphansAll, "all", false, "Show all orphans (no date filter)")

	rootCmd.AddCommand(orphansCmd)
}

// OrphanCommit represents an unreachable commit
type OrphanCommit struct {
	SHA     string
	Date    time.Time
	Author  string
	Subject string
}

func runOrphans(cmd *cobra.Command, args []string) error {
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

	// We need to run from the mayor's clone (main git repo for the rig)
	mayorPath := r.Path + "/mayor/rig"

	fmt.Printf("Scanning for orphaned commits in %s...\n\n", rigName)

	// Run git fsck
	orphans, err := findOrphanCommits(mayorPath)
	if err != nil {
		return fmt.Errorf("finding orphans: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned commits found\n", style.Bold.Render("✓"))
		return nil
	}

	// Filter by date unless --all
	cutoff := time.Now().AddDate(0, 0, -orphansDays)
	var filtered []OrphanCommit

	for _, o := range orphans {
		if orphansAll || o.Date.After(cutoff) {
			filtered = append(filtered, o)
		}
	}

	if len(filtered) == 0 {
		fmt.Printf("%s No orphaned commits in the last %d days\n", style.Bold.Render("✓"), orphansDays)
		fmt.Printf("%s Use --days=N or --all to see older orphans\n", style.Dim.Render("Hint:"))
		return nil
	}

	// Display results
	fmt.Printf("%s Found %d orphaned commit(s):\n\n", style.Warning.Render("⚠"), len(filtered))

	for _, o := range filtered {
		age := formatAge(o.Date)
		fmt.Printf("  %s %s\n", style.Bold.Render(o.SHA[:8]), o.Subject)
		fmt.Printf("    %s by %s\n\n", style.Dim.Render(age), o.Author)
	}

	// Recovery hints
	fmt.Printf("%s\n", style.Dim.Render("To recover a commit:"))
	fmt.Printf("%s\n", style.Dim.Render("  git cherry-pick <sha>     # Apply to current branch"))
	fmt.Printf("%s\n", style.Dim.Render("  git show <sha>            # View full commit"))
	fmt.Printf("%s\n", style.Dim.Render("  git branch rescue <sha>   # Create branch from commit"))

	return nil
}

// findOrphanCommits runs git fsck and parses orphaned commits
func findOrphanCommits(repoPath string) ([]OrphanCommit, error) {
	// Run git fsck to find unreachable objects
	fsckCmd := exec.Command("git", "fsck", "--unreachable", "--no-reflogs")
	fsckCmd.Dir = repoPath

	var fsckOut, fsckErr bytes.Buffer
	fsckCmd.Stdout = &fsckOut
	fsckCmd.Stderr = &fsckErr

	if err := fsckCmd.Run(); err != nil {
		// git fsck returns non-zero if there are issues, but we still get output
		// Only fail if we got no output at all
		if fsckOut.Len() == 0 {
			// Include stderr in error message for debugging
			errMsg := strings.TrimSpace(fsckErr.String())
			if errMsg != "" {
				return nil, fmt.Errorf("git fsck failed: %w (%s)", err, errMsg)
			}
			return nil, fmt.Errorf("git fsck failed: %w", err)
		}
	}

	// Parse commit SHAs from output
	var commitSHAs []string
	scanner := bufio.NewScanner(&fsckOut)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "unreachable commit <sha>"
		if strings.HasPrefix(line, "unreachable commit ") {
			sha := strings.TrimPrefix(line, "unreachable commit ")
			commitSHAs = append(commitSHAs, sha)
		}
	}

	if len(commitSHAs) == 0 {
		return nil, nil
	}

	// Get details for each commit
	var orphans []OrphanCommit
	for _, sha := range commitSHAs {
		commit, err := getCommitDetails(repoPath, sha)
		if err != nil {
			continue // Skip commits we can't parse
		}

		// Skip stash-like and routine sync commits
		if isNoiseCommit(commit.Subject) {
			continue
		}

		orphans = append(orphans, commit)
	}

	return orphans, nil
}

// getCommitDetails retrieves commit metadata
func getCommitDetails(repoPath, sha string) (OrphanCommit, error) {
	// Format: timestamp|author|subject
	cmd := exec.Command("git", "log", "-1", "--format=%at|%an|%s", sha)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		return OrphanCommit{}, err
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
	if len(parts) < 3 {
		return OrphanCommit{}, fmt.Errorf("unexpected format")
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return OrphanCommit{}, err
	}

	return OrphanCommit{
		SHA:     sha,
		Date:    time.Unix(timestamp, 0),
		Author:  parts[1],
		Subject: parts[2],
	}, nil
}

// isNoiseCommit returns true for stash-related or routine sync commits
func isNoiseCommit(subject string) bool {
	// Git stash creates commits with these prefixes
	noisePrefixes := []string{
		"WIP on ",
		"index on ",
		"On ",              // "On branch: message"
		"stash@{",          // Direct stash reference
		"untracked files ", // Stash with untracked
		"bd sync:",         // Beads sync commits (routine)
		"bd sync: ",        // Beads sync commits (routine)
	}

	for _, prefix := range noisePrefixes {
		if strings.HasPrefix(subject, prefix) {
			return true
		}
	}

	return false
}

// formatAge returns a human-readable age string
func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
