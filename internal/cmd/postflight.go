package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	postflightRig         string
	postflightArchiveMail bool
	postflightDryRun      bool
)

// PostflightReport summarizes postflight check results.
type PostflightReport struct {
	MailArchived    int
	BranchesCleaned int
	Warnings        []string
}

var postflightCmd = &cobra.Command{
	Use:     "postflight",
	GroupID: GroupWorkspace,
	Short:   "Clean up workspace after batch work",
	Long: `Run postflight cleanup after completing batch work.

Postflight performs:
1. Archive old mail (with --archive-mail)
2. Clean up stale integration branches
3. Sync beads
4. Report on rig state

Use --dry-run to see what would be done without making changes.

Examples:
  gt postflight                    # Basic cleanup
  gt postflight --archive-mail     # Also archive old messages
  gt postflight --rig myrig        # Clean specific rig only
  gt postflight --dry-run          # Preview without changes`,
	RunE: runPostflight,
}

func init() {
	postflightCmd.Flags().StringVar(&postflightRig, "rig", "", "Clean specific rig only")
	postflightCmd.Flags().BoolVar(&postflightArchiveMail, "archive-mail", false, "Archive old mail messages")
	postflightCmd.Flags().BoolVar(&postflightDryRun, "dry-run", false, "Preview without making changes")
	rootCmd.AddCommand(postflightCmd)
}

func runPostflight(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	report := &PostflightReport{}

	// Get rigs to clean
	var rigs []*rig.Rig
	if postflightRig != "" {
		_, r, err := getRig(postflightRig)
		if err != nil {
			return fmt.Errorf("loading rig %s: %w", postflightRig, err)
		}
		rigs = []*rig.Rig{r}
	} else {
		allRigs, _, err := getAllRigs()
		if err != nil {
			return fmt.Errorf("listing rigs: %w", err)
		}
		rigs = allRigs
	}

	fmt.Printf("%s Running postflight cleanup...\n\n", style.Bold.Render("→"))

	// 1. Archive old mail (if requested)
	if postflightArchiveMail {
		fmt.Printf("%s Archiving old mail...\n", style.Dim.Render("•"))
		archived, archiveWarnings := archiveOldMail(townRoot, rigs, postflightDryRun)
		report.MailArchived = archived
		report.Warnings = append(report.Warnings, archiveWarnings...)
	}

	// 2. Clean stale integration branches
	fmt.Printf("%s Cleaning stale integration branches...\n", style.Dim.Render("•"))
	cleaned, branchWarnings := cleanIntegrationBranches(townRoot, rigs, postflightDryRun)
	report.BranchesCleaned = cleaned
	report.Warnings = append(report.Warnings, branchWarnings...)

	// 3. Sync beads
	fmt.Printf("%s Syncing beads...\n", style.Dim.Render("•"))
	if !postflightDryRun {
		if err := runBdSync(townRoot); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("bd sync failed: %v", err))
		}
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("(dry-run: would run bd sync)"))
	}

	// 4. Report rig state
	fmt.Printf("%s Checking rig state...\n", style.Dim.Render("•"))
	stateWarnings := reportRigState(townRoot, rigs)
	report.Warnings = append(report.Warnings, stateWarnings...)

	// Print report
	fmt.Println()
	printPostflightReport(report)

	return nil
}

// archiveOldMail moves old messages to archive.
func archiveOldMail(townRoot string, rigs []*rig.Rig, dryRun bool) (int, []string) {
	var totalArchived int
	var warnings []string
	cutoff := time.Now().Add(-24 * time.Hour)

	// Archive mayor/deacon mail at town level
	townBeadsDir := beads.ResolveBeadsDir(townRoot)
	for _, role := range []string{"mayor", "deacon"} {
		archived, err := archiveMailboxOlderThan(role, townRoot, townBeadsDir, cutoff, dryRun)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("archiving %s mail: %v", role, err))
		}
		totalArchived += archived
	}

	// Archive rig-level mail
	for _, r := range rigs {
		rigBeadsDir := beads.ResolveBeadsDir(r.Path)

		// Witness and refinery
		for _, role := range []string{r.Name + "/witness", r.Name + "/refinery"} {
			archived, err := archiveMailboxOlderThan(role, r.Path, rigBeadsDir, cutoff, dryRun)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("archiving %s mail: %v", role, err))
			}
			totalArchived += archived
		}

		// Crew workers
		crewMgr := crew.NewManager(r, git.NewGit(r.Path))
		workers, err := crewMgr.List()
		if err == nil {
			for _, w := range workers {
				identity := fmt.Sprintf("%s/crew/%s", r.Name, w.Name)
				archived, err := archiveMailboxOlderThan(identity, r.Path, rigBeadsDir, cutoff, dryRun)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("archiving %s mail: %v", identity, err))
				}
				totalArchived += archived
			}
		}

		// Polecats
		polecatMgr := polecat.NewManager(r, git.NewGit(r.Path), tmux.NewTmux())
		polecats, err := polecatMgr.List()
		if err == nil {
			for _, p := range polecats {
				identity := fmt.Sprintf("%s/polecats/%s", r.Name, p.Name)
				archived, err := archiveMailboxOlderThan(identity, r.Path, rigBeadsDir, cutoff, dryRun)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("archiving %s mail: %v", identity, err))
				}
				totalArchived += archived
			}
		}
	}

	if dryRun && totalArchived > 0 {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(dry-run: would archive %d messages)", totalArchived)))
	} else if totalArchived > 0 {
		fmt.Printf("  Archived %d messages\n", totalArchived)
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("No messages to archive"))
	}

	return totalArchived, warnings
}

// archiveMailboxOlderThan archives messages older than cutoff.
func archiveMailboxOlderThan(identity, workDir, beadsDir string, cutoff time.Time, dryRun bool) (int, error) {
	mb := mail.NewMailboxWithBeadsDir(identity, workDir, beadsDir)
	messages, err := mb.List()
	if err != nil {
		return 0, err
	}

	var archived int
	for _, msg := range messages {
		if msg.Timestamp.Before(cutoff) {
			if !dryRun {
				if err := mb.Archive(msg.ID); err != nil {
					return archived, err
				}
			}
			archived++
		}
	}

	return archived, nil
}

// cleanIntegrationBranches removes merged integration branches.
func cleanIntegrationBranches(townRoot string, rigs []*rig.Rig, dryRun bool) (int, []string) {
	var totalCleaned int
	var warnings []string

	for _, r := range rigs {
		// Check mayor clone for integration branches
		mayorClone := filepath.Join(r.Path, "mayor", "rig")
		if _, err := os.Stat(mayorClone); err != nil {
			continue
		}

		g := git.NewGit(mayorClone)

		// Get all int/* branches
		branches, err := g.ListBranches("int/*")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("listing branches in %s: %v", r.Name, err))
			continue
		}

		// Find merged int/* branches
		for _, branch := range branches {
			// Check if branch is merged to main using git merge-base
			merged, err := isBranchMerged(mayorClone, branch, "main")
			if err != nil {
				continue
			}

			if merged {
				if dryRun {
					fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(dry-run: would delete %s in %s)", branch, r.Name)))
				} else {
					if err := g.DeleteBranch(branch, false); err != nil {
						warnings = append(warnings, fmt.Sprintf("deleting branch %s in %s: %v", branch, r.Name, err))
						continue
					}
					fmt.Printf("  Deleted %s in %s\n", branch, r.Name)
				}
				totalCleaned++
			}
		}
	}

	if totalCleaned == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("No stale integration branches"))
	}

	return totalCleaned, warnings
}

// reportRigState prints a summary of rig state.
func reportRigState(townRoot string, rigs []*rig.Rig) []string {
	var warnings []string
	t := tmux.NewTmux()

	for _, r := range rigs {
		var status []string

		// Count polecats
		polecatMgr := polecat.NewManager(r, git.NewGit(r.Path), t)
		polecats, err := polecatMgr.List()
		if err == nil {
			status = append(status, fmt.Sprintf("%d polecats", len(polecats)))
		}

		// Count crew workers
		crewMgr := crew.NewManager(r, git.NewGit(r.Path))
		workers, err := crewMgr.List()
		if err == nil {
			status = append(status, fmt.Sprintf("%d crew", len(workers)))
		}

		// Check services
		var services []string
		if hasSession, _ := t.HasSession(fmt.Sprintf("gt-%s-witness", r.Name)); hasSession {
			services = append(services, "witness")
		}
		if hasSession, _ := t.HasSession(fmt.Sprintf("gt-%s-refinery", r.Name)); hasSession {
			services = append(services, "refinery")
		}
		if len(services) > 0 {
			status = append(status, strings.Join(services, "+"))
		}

		fmt.Printf("  %s: %s\n", r.Name, strings.Join(status, ", "))
	}

	return warnings
}

// printPostflightReport prints a summary of the postflight results.
func printPostflightReport(report *PostflightReport) {
	fmt.Printf("%s Postflight Summary\n", style.Bold.Render("📋"))
	if postflightArchiveMail {
		fmt.Printf("  Mail archived: %d\n", report.MailArchived)
	}
	fmt.Printf("  Branches cleaned: %d\n", report.BranchesCleaned)

	if len(report.Warnings) > 0 {
		fmt.Printf("\n%s Warnings (%d)\n", style.WarningPrefix, len(report.Warnings))
		for _, w := range report.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	} else {
		fmt.Printf("\n%s Workspace cleanup complete\n", style.Bold.Render("✓"))
	}
}

// isBranchMerged checks if a branch is merged into target using git merge-base.
func isBranchMerged(repoPath, branch, target string) (bool, error) {
	// Get the merge-base between branch and target
	cmd := exec.Command("git", "merge-base", "--is-ancestor", branch, target)
	cmd.Dir = repoPath
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means not an ancestor (not merged)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	// Exit code 0 means branch is an ancestor of target (merged)
	return true, nil
}
