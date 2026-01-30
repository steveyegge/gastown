package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var prStackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage stacked PR dependencies",
	Long: `Manage stacked PR dependencies for coordinated merging.

Stacked PRs are PRs that depend on other PRs. When the base PR merges,
dependent PRs are automatically rebased onto the new main.

SUBCOMMANDS:
  set      Set the base branch/PR this branch depends on
  list     Show the PR dependency DAG
  sync     Rebase all dependent PRs when base updates
  clear    Remove dependency tracking for a branch

EXAMPLES:
  gt pr stack set fix/ci-fixes           # Current branch depends on fix/ci-fixes
  gt pr stack list                       # Show dependency graph
  gt pr stack sync                       # Rebase stale branches
  gt pr stack clear                      # Remove dependency for current branch`,
	RunE: requireSubcommand,
}

var prStackSetCmd = &cobra.Command{
	Use:   "set <base-branch>",
	Short: "Set the base branch this PR depends on",
	Long: `Set the base branch that the current branch depends on.

This registers the dependency in the branch's MR bead metadata and
adds the branch to the DAG for automated rebase tracking.

The base branch should be:
- Another feature branch (for stacked PRs)
- "main" to depend directly on main (default, no stacking)

EXAMPLES:
  gt pr stack set fix/ci-fixes           # Depend on fix/ci-fixes branch
  gt pr stack set feat/epic-workflow     # Depend on another feature branch`,
	Args: cobra.ExactArgs(1),
	RunE: runPRStackSet,
}

var prStackListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the PR dependency DAG",
	Long: `Show the dependency graph of all tracked PR branches.

Displays branches in topological order (dependencies first) with
their current status (up-to-date, needs-rebase, has-conflicts).

EXAMPLES:
  gt pr stack list
  gt pr stack list --json`,
	RunE: runPRStackList,
}

var prStackSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebase dependent PRs when base updates",
	Long: `Rebase all PR branches that need updating.

This command:
1. Checks which branches are behind their base
2. Rebases them in topological order (dependencies first)
3. Force-pushes updated branches
4. Reports any conflicts

EXAMPLES:
  gt pr stack sync
  gt pr stack sync --dry-run`,
	RunE: runPRStackSync,
}

var prStackClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove dependency tracking for current branch",
	Long: `Remove the current branch from dependency tracking.

This clears the depends_on field in the MR bead and removes
the branch from the DAG.`,
	RunE: runPRStackClear,
}

var (
	prStackListJSON   bool
	prStackSyncDryRun bool
)

func init() {
	prStackListCmd.Flags().BoolVar(&prStackListJSON, "json", false, "Output as JSON")
	prStackSyncCmd.Flags().BoolVarP(&prStackSyncDryRun, "dry-run", "n", false, "Preview without making changes")

	prStackCmd.AddCommand(prStackSetCmd)
	prStackCmd.AddCommand(prStackListCmd)
	prStackCmd.AddCommand(prStackSyncCmd)
	prStackCmd.AddCommand(prStackClearCmd)

	// Add as 'gt stack' command (can be aliased to 'gt pr stack' later)
	rootCmd.AddCommand(prStackCmd)
}

func runPRStackSet(cmd *cobra.Command, args []string) error {
	baseBranch := args[0]

	// Get current branch
	currentBranch, err := getCurrentBranchName()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if currentBranch == "main" || currentBranch == "master" {
		return fmt.Errorf("cannot set dependency for %s branch", currentBranch)
	}

	// Find rig context
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Get or create DAG orchestrator
	orch := getDAGOrchestrator(r)

	// Register the branch in the DAG
	if err := orch.RegisterBranch(currentBranch, baseBranch, "", "", ""); err != nil {
		return fmt.Errorf("registering branch: %w", err)
	}

	// Update MR bead if it exists
	if err := updateMRDependsOn(r, currentBranch, baseBranch); err != nil {
		// Not fatal - MR bead may not exist yet
		fmt.Printf("%s No MR bead found for %s (will be set when MR is created)\n",
			style.Dim.Render("ℹ"), currentBranch)
	}

	fmt.Printf("%s %s now depends on %s\n",
		style.Bold.Render("✓"), currentBranch, baseBranch)

	return nil
}

func runPRStackList(cmd *cobra.Command, args []string) error {
	// Find rig context
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	orch := getDAGOrchestrator(r)
	if err := orch.LoadDAG(); err != nil {
		// No DAG file yet
		fmt.Printf("%s No PR stack configured\n", style.Dim.Render("ℹ"))
		fmt.Println("Use 'gt pr stack set <base-branch>' to start stacking PRs")
		return nil
	}

	stats := orch.GetDAGStats()
	if stats["total"] == 0 {
		fmt.Printf("%s No PR stack configured\n", style.Dim.Render("ℹ"))
		return nil
	}

	// Get topological order
	order := orch.GetRebaseOrder()

	fmt.Printf("%s PR Stack (%d branches):\n\n", style.Bold.Render("📚"), stats["total"])

	for _, branch := range order {
		node, ok := orch.GetBranchStatus(branch)
		if !ok {
			continue
		}

		status := style.Bold.Render("✓")
		statusText := "up-to-date"
		if node.Status == refinery.BranchStatusNeedsRebase {
			status = style.Warning.Render("↻")
			statusText = "needs-rebase"
		}
		if node.Status == refinery.BranchStatusConflict {
			status = style.Error.Render("✗")
			statusText = "has-conflicts"
		}

		dep := "main"
		if node.DependsOn != "" {
			dep = node.DependsOn
		}

		fmt.Printf("  %s %s → %s [%s]\n", status, branch, dep, statusText)
	}

	// Show summary
	fmt.Println()
	if stats["needs_rebase"] > 0 {
		fmt.Printf("%s %d branch(es) need rebase. Run: gt pr stack sync\n",
			style.Warning.Render("⚠"), stats["needs_rebase"])
	}
	if stats["has_conflict"] > 0 {
		fmt.Printf("%s %d branch(es) have conflicts\n",
			style.Error.Render("✗"), stats["has_conflict"])
	}

	return nil
}

func runPRStackSync(cmd *cobra.Command, args []string) error {
	// Find rig context
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	orch := getDAGOrchestrator(r)
	if err := orch.LoadDAG(); err != nil {
		return fmt.Errorf("no PR stack configured: %w", err)
	}

	// Check for updates
	updates, err := orch.CheckBranchUpdates()
	if err != nil {
		return fmt.Errorf("checking updates: %w", err)
	}

	if len(updates) == 0 {
		fmt.Printf("%s All branches are up-to-date\n", style.Bold.Render("✓"))
		return nil
	}

	fmt.Printf("%s Found %d branch(es) needing rebase\n\n",
		style.Bold.Render("🔄"), len(updates))

	// Get rebase order
	order := orch.GetRebaseOrder()
	var rebased, conflicts []string

	for _, branch := range order {
		node, ok := orch.GetBranchStatus(branch)
		if !ok || node.Status != refinery.BranchStatusNeedsRebase {
			continue
		}

		fmt.Printf("  %s Rebasing %s onto %s...\n",
			style.Bold.Render("→"), branch, node.DependsOn)

		if prStackSyncDryRun {
			fmt.Printf("    Would rebase and force-push\n")
			continue
		}

		if err := orch.PerformRebase(branch); err != nil {
			fmt.Printf("    %s Conflict: %v\n", style.Error.Render("✗"), err)
			conflicts = append(conflicts, branch)
			continue
		}

		// Force push
		if err := forcePushBranch(r.Path, "origin", branch); err != nil {
			fmt.Printf("    %s Push failed: %v\n", style.Error.Render("✗"), err)
			continue
		}

		fmt.Printf("    %s Done\n", style.Bold.Render("✓"))
		rebased = append(rebased, branch)
	}

	// Summary
	fmt.Println()
	if len(rebased) > 0 {
		fmt.Printf("%s Rebased %d branch(es)\n", style.Bold.Render("✓"), len(rebased))
	}
	if len(conflicts) > 0 {
		fmt.Printf("%s %d branch(es) have conflicts:\n", style.Error.Render("✗"), len(conflicts))
		for _, b := range conflicts {
			fmt.Printf("    - %s\n", b)
		}
	}

	return nil
}

func runPRStackClear(cmd *cobra.Command, args []string) error {
	// Get current branch
	currentBranch, err := getCurrentBranchName()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Find rig context
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName, err := inferRigFromCwd(townRoot)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	orch := getDAGOrchestrator(r)
	if err := orch.LoadDAG(); err != nil {
		fmt.Printf("%s Branch %s is not in the stack\n", style.Dim.Render("ℹ"), currentBranch)
		return nil
	}

	if err := orch.UnregisterBranch(currentBranch); err != nil {
		return fmt.Errorf("unregistering branch: %w", err)
	}

	// Clear MR bead dependency
	_ = updateMRDependsOn(r, currentBranch, "")

	fmt.Printf("%s Removed %s from PR stack\n", style.Bold.Render("✓"), currentBranch)
	return nil
}

// Helper functions

func getCurrentBranchName() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getDAGOrchestrator(r *rig.Rig) *refinery.DAGOrchestrator {
	gitOps := refinery.NewRealGitOps(r.Path)
	return refinery.NewDAGOrchestrator(r.Path, "main", "upstream", "origin", gitOps, nil)
}

func updateMRDependsOn(r *rig.Rig, branch, dependsOn string) error {
	bd := beads.New(r.Path)

	// Find MR bead for this branch
	mr, err := bd.FindMRForBranch(branch)
	if err != nil {
		return err
	}
	if mr == nil {
		return fmt.Errorf("no MR found for branch %s", branch)
	}

	// Parse existing fields
	fields := beads.ParseMRFields(mr)
	if fields == nil {
		fields = &beads.MRFields{}
	}

	// Update depends_on
	fields.DependsOn = dependsOn

	// Update the issue description
	newDesc := beads.SetMRFields(mr, fields)
	return bd.Update(mr.ID, beads.UpdateOptions{Description: &newDesc})
}

func forcePushBranch(repoDir, remote, branch string) error {
	cmd := exec.Command("git", "push", "--force-with-lease", remote, branch)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
