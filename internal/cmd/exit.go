package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"os/exec"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/workspace"
)

var exitCmd = &cobra.Command{
	Use:         "exit",
	GroupID:     GroupWork,
	Annotations: map[string]string{AnnotationPolecatSafe: "true"},
	Short:       "Save work and exit (dispatch-and-kill model)",
	Long: `Save all work and exit the polecat session cleanly.

Lightweight alternative to gt done for the dispatch-and-kill model:
1. Auto-commits any uncommitted work (safety net)
2. Pushes branch to origin
3. Persists a completion note to the bead
4. Exits — the daemon reaper handles the rest

Does NOT:
- Submit to merge queue (no MR beads)
- Notify witnesses (we don't use them)
- Transition to IDLE (polecats are fire-and-forget)
- Close the bead (archivist does this)

Examples:
  gt exit                              # Auto-save, push, exit
  gt exit --notes "Added type hints to 21 files"
  gt exit --issue sbx-gastown-abc      # Explicit issue ID`,
	RunE:         runExit,
	SilenceUsage: true,
}

var (
	exitNotes string
	exitIssue string
)

func init() {
	exitCmd.Flags().StringVar(&exitNotes, "notes", "", "Completion notes to persist on the bead")
	exitCmd.Flags().StringVar(&exitIssue, "issue", "", "Issue ID (default: auto-detect from branch name)")
	rootCmd.AddCommand(exitCmd)
}

func runExit(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	g := git.NewGit(cwd)

	branch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("cannot determine current branch: %w", err)
	}

	// Auto-detect issue from branch name
	issueID := exitIssue
	if issueID == "" {
		issueID = parseBranchName(branch).Issue
	}

	// Fallback: query for hooked beads assigned to this agent.
	// Modern polecat branches (polecat/<worker>-<timestamp>) don't embed the
	// issue ID, so parseBranchName returns "". Query beads directly for
	// status=hooked + assignee — same pattern gt done uses (hq-l6mm5).
	if issueID == "" {
		sender := detectSender()
		if sender != "" {
			bd := beads.New(beads.ResolveBeadsDir(townRoot))
			if hookIssue := findHookedBeadForAgent(bd, sender); hookIssue != "" {
				issueID = hookIssue
				fmt.Printf("%s Issue resolved from hooked bead: %s\n", style.Bold.Render("✓"), issueID)
			}
		}
	}

	// Determine rig name
	rigName := os.Getenv("GT_RIG")
	if rigName == "" {
		if rel, err := filepath.Rel(townRoot, cwd); err == nil {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 0 {
				rigName = parts[0]
			}
		}
	}

	// 1. AUTO-COMMIT SAFETY NET
	// Check for uncommitted work and commit it to prevent loss
	workStatus, err := g.CheckUncommittedWork()
	if err == nil && workStatus.HasUncommittedChanges && !workStatus.CleanExcludingRuntime() {
		fmt.Printf("\n%s Uncommitted changes detected — auto-saving\n", style.Bold.Render("⚠"))
		fmt.Printf("  Files: %s\n\n", workStatus.String())

		if addErr := g.Add("-A"); addErr != nil {
			style.PrintWarning("auto-save: git add failed: %v", addErr)
		} else {
			// Unstage overlay files
			_ = g.ResetFiles("CLAUDE.local.md")
			if claudeData, readErr := os.ReadFile(filepath.Join(cwd, "CLAUDE.md")); readErr == nil {
				if strings.Contains(string(claudeData), templates.PolecatLifecycleMarker) {
					_ = g.ResetFiles("CLAUDE.md")
				}
			}
			autoMsg := "fix: auto-save uncommitted work (gt exit safety net)"
			if issueID != "" {
				autoMsg = fmt.Sprintf("fix: auto-save uncommitted work (%s)", issueID)
			}
			if commitErr := g.Commit(autoMsg); commitErr != nil {
				style.PrintWarning("auto-save: git commit failed: %v", commitErr)
			} else {
				fmt.Printf("%s Auto-committed uncommitted work\n", style.Bold.Render("✓"))
			}
		}
	}

	// 2. PUSH TO ORIGIN
	defaultBranch := "main"
	aheadCount, err := g.CommitsAhead("origin/"+defaultBranch, branch)
	if err == nil && aheadCount > 0 {
		fmt.Printf("%s Pushing %d commit(s) to origin...\n", style.Bold.Render("→"), aheadCount)
		if pushErr := g.Push("origin", "HEAD", false); pushErr != nil {
			style.PrintWarning("push failed: %v — work is committed locally but not on remote", pushErr)
		} else {
			fmt.Printf("%s Branch pushed\n", style.Bold.Render("✓"))
		}
	} else {
		// Try push anyway — CommitsAhead may fail on detached HEAD or missing remote ref
		if pushErr := g.Push("origin", "HEAD", false); pushErr != nil {
			fmt.Printf("%s Nothing to push or push failed\n", style.Dim.Render("○"))
		} else {
			fmt.Printf("%s Branch pushed\n", style.Bold.Render("✓"))
		}
	}

	// 3. PERSIST COMPLETION NOTES TO BEAD
	if issueID != "" {
		notes := exitNotes
		if notes == "" {
			notes = fmt.Sprintf("Polecat exit: branch %s pushed. Rig: %s.", branch, rigName)
		}

		bdCmd := exec.Command("bd", "update", issueID, "--notes", notes)
		bdCmd.Dir = townRoot
		bdCmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(townRoot))
		util.SetDetachedProcessGroup(bdCmd)
		if err := bdCmd.Run(); err != nil {
			style.PrintWarning("could not persist notes to %s: %v", issueID, err)
		} else {
			fmt.Printf("%s Notes persisted to %s\n", style.Bold.Render("✓"), issueID)
		}
	} else {
		fmt.Printf("%s No issue ID detected — skipping bead update\n", style.Dim.Render("○"))
	}

	// 4. EXIT
	fmt.Printf("\n%s Work saved. Exiting — daemon reaper handles cleanup.\n", style.Bold.Render("✓"))

	// Signal to Claude Code to exit the session
	// gt exit returns 0 — the polecat template tells the agent to run /exit after
	return nil
}
