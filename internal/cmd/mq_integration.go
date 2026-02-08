package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// defaultIntegrationBranchTemplate is kept for local backward compat references.
var defaultIntegrationBranchTemplate = beads.DefaultIntegrationBranchTemplate

// invalidBranchCharsRegex matches characters that are invalid in git branch names.
// Git branch names cannot contain: ~ ^ : \ space, .., @{, or end with .lock
var invalidBranchCharsRegex = regexp.MustCompile(`[~^:\s\\?*\[]|\.\.|\.\.|@\{`)

// buildIntegrationBranchName wraps beads.BuildIntegrationBranchName for local callers.
func buildIntegrationBranchName(template, epicID string) string {
	return beads.BuildIntegrationBranchName(template, epicID)
}

// extractEpicPrefix wraps beads.ExtractEpicPrefix for local callers.
func extractEpicPrefix(epicID string) string {
	return beads.ExtractEpicPrefix(epicID)
}

// validateBranchName checks if a branch name is valid for git.
// Returns an error if the branch name contains invalid characters.
func validateBranchName(branchName string) error {
	if branchName == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check for invalid characters
	if invalidBranchCharsRegex.MatchString(branchName) {
		return fmt.Errorf("branch name %q contains invalid characters (~ ^ : \\ space, .., or @{)", branchName)
	}

	// Check for .lock suffix
	if strings.HasSuffix(branchName, ".lock") {
		return fmt.Errorf("branch name %q cannot end with .lock", branchName)
	}

	// Check for leading/trailing slashes or dots
	if strings.HasPrefix(branchName, "/") || strings.HasSuffix(branchName, "/") {
		return fmt.Errorf("branch name %q cannot start or end with /", branchName)
	}
	if strings.HasPrefix(branchName, ".") || strings.HasSuffix(branchName, ".") {
		return fmt.Errorf("branch name %q cannot start or end with .", branchName)
	}

	// Check for consecutive slashes
	if strings.Contains(branchName, "//") {
		return fmt.Errorf("branch name %q cannot contain consecutive slashes", branchName)
	}

	return nil
}

// getIntegrationBranchField wraps beads.GetIntegrationBranchField for local callers.
func getIntegrationBranchField(description string) string {
	return beads.GetIntegrationBranchField(description)
}

// getRigGit returns a Git object for the rig's repository.
// Prefers .repo.git (bare repo) if it exists, falls back to mayor/rig.
func getRigGit(rigPath string) (*git.Git, error) {
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if info, err := os.Stat(bareRepoPath); err == nil && info.IsDir() {
		return git.NewGitWithDir(bareRepoPath, ""), nil
	}
	mayorPath := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(mayorPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no repo base found (neither .repo.git nor mayor/rig exists)")
	}
	return git.NewGit(mayorPath), nil
}

// createLandWorktree creates a temporary worktree from .repo.git for land operations.
// This avoids disrupting running agents (refinery, mayor) by operating in an isolated worktree.
// The caller MUST call the returned cleanup function when done (typically via defer).
// The worktree is checked out to startBranch (e.g., "main").
func createLandWorktree(rigPath, startBranch string) (*git.Git, func(), error) {
	landPath := filepath.Join(rigPath, ".land-worktree")
	noop := func() {}

	// Get bare repo for worktree creation
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if _, err := os.Stat(bareRepoPath); err != nil {
		return nil, noop, fmt.Errorf("bare repo not found at %s: %w", bareRepoPath, err)
	}
	bareGit := git.NewGitWithDir(bareRepoPath, "")

	// Clean up any stale worktree from a previous failed run
	if _, err := os.Stat(landPath); err == nil {
		_ = bareGit.WorktreeRemove(landPath, true)
		_ = os.RemoveAll(landPath)
	}

	// Create worktree checked out to the target branch.
	// Use --force because the branch may already be checked out in refinery/rig.
	// Use NoSparse variant since land worktrees are temporary and don't need .claude/ exclusion.
	if err := bareGit.WorktreeAddExistingForceNoSparse(landPath, startBranch); err != nil {
		return nil, noop, fmt.Errorf("creating land worktree: %w", err)
	}

	cleanup := func() {
		_ = bareGit.WorktreeRemove(landPath, true)
		_ = os.RemoveAll(landPath)
	}

	return git.NewGit(landPath), cleanup, nil
}

// getIntegrationBranchTemplate returns the integration branch template to use.
// Priority: CLI flag > rig config > default
func getIntegrationBranchTemplate(rigPath, cliOverride string) string {
	if cliOverride != "" {
		return cliOverride
	}

	// Try to load rig settings
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return defaultIntegrationBranchTemplate
	}

	if settings.MergeQueue != nil && settings.MergeQueue.IntegrationBranchTemplate != "" {
		return settings.MergeQueue.IntegrationBranchTemplate
	}

	return defaultIntegrationBranchTemplate
}

// IntegrationStatusOutput is the JSON output structure for integration status.
type IntegrationStatusOutput struct {
	Epic            string                       `json:"epic"`
	Branch          string                       `json:"branch"`
	Created         string                       `json:"created,omitempty"`
	AheadOfMain     int                          `json:"ahead_of_main"`
	MergedMRs       []IntegrationStatusMRSummary `json:"merged_mrs"`
	PendingMRs      []IntegrationStatusMRSummary `json:"pending_mrs"`
	ReadyToLand     bool                         `json:"ready_to_land"`
	AutoLandEnabled bool                         `json:"auto_land_enabled"`
	ChildrenTotal   int                          `json:"children_total"`
	ChildrenClosed  int                          `json:"children_closed"`
}

// IntegrationStatusMRSummary represents a merge request in the integration status output.
type IntegrationStatusMRSummary struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

// runMqIntegrationCreate creates an integration branch for an epic.
func runMqIntegrationCreate(cmd *cobra.Command, args []string) error {
	epicID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	_, r, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Initialize beads for the rig
	bd := beads.New(r.Path)

	// 1. Verify epic exists
	epic, err := bd.Show(epicID)
	if err != nil {
		if err == beads.ErrNotFound {
			return fmt.Errorf("epic '%s' not found", epicID)
		}
		return fmt.Errorf("fetching epic: %w", err)
	}

	// Verify it's actually an epic
	if epic.Type != "epic" {
		return fmt.Errorf("'%s' is a %s, not an epic", epicID, epic.Type)
	}

	// Build integration branch name from template
	template := getIntegrationBranchTemplate(r.Path, mqIntegrationCreateBranch)
	branchName := buildIntegrationBranchName(template, epicID)

	// Validate the branch name
	if err := validateBranchName(branchName); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	// Initialize git for the rig
	g, err := getRigGit(r.Path)
	if err != nil {
		return fmt.Errorf("initializing git: %w", err)
	}

	// Check if integration branch already exists locally
	exists, err := g.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("checking branch existence: %w", err)
	}
	if exists {
		return fmt.Errorf("integration branch '%s' already exists locally", branchName)
	}

	// Check if branch exists on remote
	remoteExists, err := g.RemoteBranchExists("origin", branchName)
	if err != nil {
		// Log warning but continue - remote check isn't critical
		fmt.Printf("  %s\n", style.Dim.Render("(could not check remote, continuing)"))
	}
	if remoteExists {
		return fmt.Errorf("integration branch '%s' already exists on origin", branchName)
	}

	// Ensure we have latest refs
	fmt.Printf("Fetching latest from origin...\n")
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching from origin: %w", err)
	}

	// 2. Create branch from base (default: origin/main)
	baseBranch := "origin/main"
	baseBranchDisplay := "main"
	if mqIntegrationCreateBaseBranch != "" {
		baseBranch = "origin/" + strings.TrimPrefix(mqIntegrationCreateBaseBranch, "origin/")
		baseBranchDisplay = strings.TrimPrefix(baseBranch, "origin/")
	}
	fmt.Printf("Creating branch '%s' from %s...\n", branchName, baseBranchDisplay)
	if err := g.CreateBranchFrom(branchName, baseBranch); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// 3. Push to origin
	fmt.Printf("Pushing to origin...\n")
	if err := g.Push("origin", branchName, false); err != nil {
		// Clean up local branch on push failure (best-effort cleanup)
		_ = g.DeleteBranch(branchName, true)
		return fmt.Errorf("pushing to origin: %w", err)
	}

	// 4. Store integration branch info in epic metadata
	// Update the epic's description to include the integration branch info
	newDesc := addIntegrationBranchField(epic.Description, branchName)
	// Also store base_branch if non-main was used (for land to know where to merge back)
	if mqIntegrationCreateBaseBranch != "" {
		newDesc = beads.AddBaseBranchField(newDesc, baseBranchDisplay)
	}
	if newDesc != epic.Description {
		if err := bd.Update(epicID, beads.UpdateOptions{Description: &newDesc}); err != nil {
			// Non-fatal - branch was created, just metadata update failed
			fmt.Printf("  %s\n", style.Dim.Render("(warning: could not update epic metadata)"))
		}
	}

	// Success output
	fmt.Printf("\n%s Created integration branch\n", style.Bold.Render("✓"))
	fmt.Printf("  Epic:   %s\n", epicID)
	fmt.Printf("  Branch: %s\n", branchName)
	fmt.Printf("  From:   %s\n", baseBranchDisplay)
	fmt.Printf("\n  Future MRs for this epic's children can target:\n")
	fmt.Printf("    gt mq submit --epic %s\n", epicID)

	return nil
}

// addIntegrationBranchField wraps beads.AddIntegrationBranchField for local callers.
func addIntegrationBranchField(description, branchName string) string {
	return beads.AddIntegrationBranchField(description, branchName)
}

// runMqIntegrationLand merges an integration branch to main.
func runMqIntegrationLand(cmd *cobra.Command, args []string) error {
	epicID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	_, r, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Initialize beads and git for the rig
	// Use getRigGit for early ref-only checks (branch exists, fetch).
	// Work-tree operations (checkout, merge, push) use a temporary worktree created later.
	bd := beads.New(r.Path)
	g, err := getRigGit(r.Path)
	if err != nil {
		return fmt.Errorf("initializing git: %w", err)
	}

	// Show what we're about to do
	if mqIntegrationLandDryRun {
		fmt.Printf("%s Dry run - no changes will be made\n\n", style.Bold.Render("🔍"))
	}

	// 1. Verify epic exists
	epic, err := bd.Show(epicID)
	if err != nil {
		if err == beads.ErrNotFound {
			return fmt.Errorf("epic '%s' not found", epicID)
		}
		return fmt.Errorf("fetching epic: %w", err)
	}

	if epic.Type != "epic" {
		return fmt.Errorf("'%s' is a %s, not an epic", epicID, epic.Type)
	}

	// Get integration branch name from epic metadata (stored at create time)
	// Fall back to default template for backward compatibility with old epics
	branchName := getIntegrationBranchField(epic.Description)
	if branchName == "" {
		branchName = buildIntegrationBranchName(defaultIntegrationBranchTemplate, epicID)
	}

	// Read base_branch from epic metadata (where to merge back)
	// Default to "main" if not stored (backward compat with pre-base-branch epics)
	targetBranch := beads.GetBaseBranchField(epic.Description)
	if targetBranch == "" {
		targetBranch = "main"
	}

	fmt.Printf("Landing integration branch for epic: %s\n", epicID)
	fmt.Printf("  Title: %s\n\n", epic.Title)

	// 2. Verify integration branch exists
	fmt.Printf("Checking integration branch...\n")
	exists, err := g.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("checking branch existence: %w", err)
	}

	// Also check remote if local doesn't exist
	if !exists {
		remoteExists, err := g.RemoteBranchExists("origin", branchName)
		if err != nil {
			return fmt.Errorf("checking remote branch: %w", err)
		}
		if !remoteExists {
			return fmt.Errorf("integration branch '%s' does not exist (locally or on origin)", branchName)
		}
		// Fetch and create local tracking branch
		fmt.Printf("Fetching integration branch from origin...\n")
		if err := g.FetchBranch("origin", branchName); err != nil {
			return fmt.Errorf("fetching branch: %w", err)
		}
	}
	fmt.Printf("  %s Branch exists\n", style.Bold.Render("✓"))

	// 3. Verify all MRs targeting this integration branch are merged
	fmt.Printf("Checking open merge requests...\n")
	openMRs, err := findOpenMRsForIntegration(bd, branchName)
	if err != nil {
		return fmt.Errorf("checking open MRs: %w", err)
	}

	if len(openMRs) > 0 {
		fmt.Printf("\n  %s Open merge requests targeting %s:\n", style.Bold.Render("⚠"), branchName)
		for _, mr := range openMRs {
			fmt.Printf("    - %s: %s\n", mr.ID, mr.Title)
		}
		fmt.Println()

		if !mqIntegrationLandForce {
			return fmt.Errorf("cannot land: %d open MRs (use --force to override)", len(openMRs))
		}
		fmt.Printf("  %s Proceeding anyway (--force)\n", style.Dim.Render("⚠"))
	} else {
		fmt.Printf("  %s No open MRs targeting integration branch\n", style.Bold.Render("✓"))
	}

	// Dry run stops here
	if mqIntegrationLandDryRun {
		fmt.Printf("\n%s Dry run complete. Would perform:\n", style.Bold.Render("🔍"))
		fmt.Printf("  1. Merge %s to %s (--no-ff)\n", branchName, targetBranch)
		if !mqIntegrationLandSkipTests {
			fmt.Printf("  2. Run tests on %s\n", targetBranch)
		}
		fmt.Printf("  3. Push %s to origin\n", targetBranch)
		fmt.Printf("  4. Delete integration branch (local and remote)\n")
		fmt.Printf("  5. Update epic status to closed\n")
		return nil
	}

	// Fetch latest before creating worktree (ensures refs are up to date)
	fmt.Printf("Fetching latest from origin...\n")
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching from origin: %w", err)
	}

	// Create a temporary worktree for the merge operation.
	// This avoids disrupting running agents (refinery, mayor) whose worktrees
	// would be corrupted by checkout/merge operations.
	fmt.Printf("Creating temporary worktree for merge...\n")
	landGit, cleanup, err := createLandWorktree(r.Path, targetBranch)
	if err != nil {
		return fmt.Errorf("creating land worktree: %w", err)
	}
	defer cleanup()

	// Pull latest target branch into the worktree
	if err := landGit.Pull("origin", targetBranch); err != nil {
		// Non-fatal if pull fails (e.g., first time)
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(pull from origin/%s skipped)", targetBranch)))
	}

	// 4. Merge integration branch into target
	fmt.Printf("Merging %s to %s...\n", branchName, targetBranch)
	mergeMsg := fmt.Sprintf("Merge %s: %s\n\nEpic: %s", branchName, epic.Title, epicID)
	if err := landGit.MergeNoFF("origin/"+branchName, mergeMsg); err != nil {
		// Abort merge on failure (cleanup handles worktree removal)
		_ = landGit.AbortMerge()
		return fmt.Errorf("merge failed: %w", err)
	}
	fmt.Printf("  %s Merged successfully\n", style.Bold.Render("✓"))

	// 5. Run tests (if configured and not skipped)
	if !mqIntegrationLandSkipTests {
		testCmd := getTestCommand(r.Path)
		if testCmd != "" {
			fmt.Printf("Running tests: %s\n", testCmd)
			if err := runTestCommand(landGit.WorkDir(), testCmd); err != nil {
				// Tests failed - no need to reset, worktree is temporary
				fmt.Printf("  %s Tests failed\n", style.Bold.Render("✗"))
				return fmt.Errorf("tests failed: %w", err)
			}
			fmt.Printf("  %s Tests passed\n", style.Bold.Render("✓"))
		} else {
			fmt.Printf("  %s\n", style.Dim.Render("(no test command configured)"))
		}
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("(tests skipped)"))
	}

	// Verify the merge actually brought changes (guard against empty merges).
	// An empty merge means conflict resolution discarded all integration branch work,
	// which would silently lose data if we proceed to delete the branch.
	verifyCmd := exec.Command("git", "diff", "--stat", "HEAD~1..HEAD")
	verifyCmd.Dir = landGit.WorkDir()
	diffOutput, verifyErr := verifyCmd.Output()
	if verifyErr == nil && len(strings.TrimSpace(string(diffOutput))) == 0 {
		return fmt.Errorf("merge produced no file changes — integration branch work may have been discarded during conflict resolution\n"+
			"  Integration branch '%s' has NOT been deleted.\n"+
			"  Inspect manually: git diff %s...origin/%s", branchName, targetBranch, branchName)
	}

	// 6. Push to origin
	fmt.Printf("Pushing %s to origin...\n", targetBranch)
	if err := landGit.Push("origin", targetBranch, false); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	fmt.Printf("  %s Pushed to origin\n", style.Bold.Render("✓"))

	// 7. Delete integration branch (use bare repo git — ref-only operations)
	fmt.Printf("Deleting integration branch...\n")
	// Delete remote first
	if err := g.DeleteRemoteBranch("origin", branchName); err != nil {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(could not delete remote branch: %v)", err)))
	} else {
		fmt.Printf("  %s Deleted from origin\n", style.Bold.Render("✓"))
	}
	// Delete local
	if err := g.DeleteBranch(branchName, true); err != nil {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(could not delete local branch: %v)", err)))
	} else {
		fmt.Printf("  %s Deleted locally\n", style.Bold.Render("✓"))
	}

	// 8. Update epic status
	fmt.Printf("Updating epic status...\n")
	if err := bd.Close(epicID); err != nil {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(could not close epic: %v)", err)))
	} else {
		fmt.Printf("  %s Epic closed\n", style.Bold.Render("✓"))
	}

	// Success output
	fmt.Printf("\n%s Successfully landed integration branch\n", style.Bold.Render("✓"))
	fmt.Printf("  Epic:   %s\n", epicID)
	fmt.Printf("  Branch: %s → %s\n", branchName, targetBranch)

	return nil
}

// findOpenMRsForIntegration finds all open merge requests targeting an integration branch.
func findOpenMRsForIntegration(bd *beads.Beads, targetBranch string) ([]*beads.Issue, error) {
	// List all open merge requests
	opts := beads.ListOptions{
		Type:   "merge-request",
		Status: "open",
	}
	allMRs, err := bd.List(opts)
	if err != nil {
		return nil, err
	}

	return filterMRsByTarget(allMRs, targetBranch), nil
}

// filterMRsByTarget filters merge requests to those targeting a specific branch.
func filterMRsByTarget(mrs []*beads.Issue, targetBranch string) []*beads.Issue {
	var result []*beads.Issue
	for _, mr := range mrs {
		fields := beads.ParseMRFields(mr)
		if fields != nil && fields.Target == targetBranch {
			result = append(result, mr)
		}
	}
	return result
}

// getTestCommand returns the test command from rig settings.
func getTestCommand(rigPath string) string {
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return ""
	}
	if settings.MergeQueue != nil && settings.MergeQueue.TestCommand != "" {
		return settings.MergeQueue.TestCommand
	}
	return ""
}

// runTestCommand executes a test command in the given directory.
func runTestCommand(workDir, testCmd string) error {
	parts := strings.Fields(testCmd)
	if len(parts) == 0 {
		return nil
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// resetHard performs a git reset --hard to the given ref.
func resetHard(g *git.Git, ref string) error {
	// We need to use the git package, but it doesn't have a Reset method
	// For now, use the internal run method via Checkout workaround
	// This is a bit of a hack but works for now
	cmd := exec.Command("git", "reset", "--hard", ref)
	cmd.Dir = g.WorkDir()
	return cmd.Run()
}

// runMqIntegrationStatus shows the status of an integration branch for an epic.
func runMqIntegrationStatus(cmd *cobra.Command, args []string) error {
	epicID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	_, r, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Initialize beads for the rig
	bd := beads.New(r.Path)

	// Fetch epic to get stored branch name
	epic, err := bd.Show(epicID)
	if err != nil {
		if err == beads.ErrNotFound {
			return fmt.Errorf("epic '%s' not found", epicID)
		}
		return fmt.Errorf("fetching epic: %w", err)
	}

	// Get integration branch name from epic metadata (stored at create time)
	// Fall back to default template for backward compatibility with old epics
	branchName := getIntegrationBranchField(epic.Description)
	if branchName == "" {
		branchName = buildIntegrationBranchName(defaultIntegrationBranchTemplate, epicID)
	}

	// Initialize git for the rig
	g, err := getRigGit(r.Path)
	if err != nil {
		return fmt.Errorf("initializing git: %w", err)
	}

	// Fetch from origin to ensure we have latest refs
	if err := g.Fetch("origin"); err != nil {
		// Non-fatal, continue with local data
	}

	// Check if integration branch exists (locally or remotely)
	localExists, _ := g.BranchExists(branchName)
	remoteExists, _ := g.RemoteBranchExists("origin", branchName)

	if !localExists && !remoteExists {
		return fmt.Errorf("integration branch '%s' does not exist", branchName)
	}

	// Determine which ref to use for comparison
	ref := branchName
	if !localExists && remoteExists {
		ref = "origin/" + branchName
	}

	// Get branch creation date
	createdDate, err := g.BranchCreatedDate(ref)
	if err != nil {
		createdDate = "" // Non-fatal
	}

	// Get commits ahead of main
	aheadCount, err := g.CommitsAhead("main", ref)
	if err != nil {
		aheadCount = 0 // Non-fatal
	}

	// Query for MRs targeting this integration branch (use resolved name)
	targetBranch := branchName

	// Get all merge-request issues
	allMRs, err := bd.List(beads.ListOptions{
		Type:   "merge-request",
		Status: "", // all statuses
	})
	if err != nil {
		return fmt.Errorf("querying merge requests: %w", err)
	}

	// Filter by target branch and separate into merged/pending
	var mergedMRs, pendingMRs []*beads.Issue
	for _, mr := range allMRs {
		fields := beads.ParseMRFields(mr)
		if fields == nil || fields.Target != targetBranch {
			continue
		}

		if mr.Status == "closed" {
			mergedMRs = append(mergedMRs, mr)
		} else {
			pendingMRs = append(pendingMRs, mr)
		}
	}

	// Check if auto-land is enabled in settings
	settingsPath := filepath.Join(r.Path, "settings", "config.json")
	settings, _ := config.LoadRigSettings(settingsPath) // Ignore error, use defaults
	autoLandEnabled := false
	if settings != nil && settings.MergeQueue != nil {
		autoLandEnabled = settings.MergeQueue.IsIntegrationBranchAutoLandEnabled()
	}

	// Query children of the epic to determine if ready to land
	// Use status "all" to include both open and closed children
	// Use Priority -1 to disable priority filtering
	children, err := bd.List(beads.ListOptions{
		Parent:   epicID,
		Status:   "all",
		Priority: -1,
	})
	childrenTotal := 0
	childrenClosed := 0
	if err == nil {
		for _, child := range children {
			childrenTotal++
			if child.Status == "closed" {
				childrenClosed++
			}
		}
	}

	readyToLand := isReadyToLand(aheadCount, childrenTotal, childrenClosed, len(pendingMRs))

	// Build output structure
	output := IntegrationStatusOutput{
		Epic:            epicID,
		Branch:          branchName,
		Created:         createdDate,
		AheadOfMain:     aheadCount,
		MergedMRs:       make([]IntegrationStatusMRSummary, 0, len(mergedMRs)),
		PendingMRs:      make([]IntegrationStatusMRSummary, 0, len(pendingMRs)),
		ReadyToLand:     readyToLand,
		AutoLandEnabled: autoLandEnabled,
		ChildrenTotal:   childrenTotal,
		ChildrenClosed:  childrenClosed,
	}

	for _, mr := range mergedMRs {
		// Extract the title without "Merge: " prefix for cleaner display
		title := strings.TrimPrefix(mr.Title, "Merge: ")
		output.MergedMRs = append(output.MergedMRs, IntegrationStatusMRSummary{
			ID:    mr.ID,
			Title: title,
		})
	}

	for _, mr := range pendingMRs {
		title := strings.TrimPrefix(mr.Title, "Merge: ")
		output.PendingMRs = append(output.PendingMRs, IntegrationStatusMRSummary{
			ID:     mr.ID,
			Title:  title,
			Status: mr.Status,
		})
	}

	// JSON output
	if mqIntegrationStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Human-readable output
	return printIntegrationStatus(&output)
}

// isReadyToLand determines if an integration branch is ready to land.
// Ready when: has commits ahead of main, has children, all children closed, no pending MRs.
func isReadyToLand(aheadCount, childrenTotal, childrenClosed, pendingMRCount int) bool {
	return aheadCount > 0 &&
		childrenTotal > 0 &&
		childrenTotal == childrenClosed &&
		pendingMRCount == 0
}

// printIntegrationStatus prints the integration status in human-readable format.
func printIntegrationStatus(output *IntegrationStatusOutput) error {
	fmt.Printf("Integration: %s\n", style.Bold.Render(output.Branch))
	if output.Created != "" {
		fmt.Printf("Created: %s\n", output.Created)
	}
	fmt.Printf("Ahead of main: %d commits\n", output.AheadOfMain)
	fmt.Printf("Epic children: %d/%d closed\n", output.ChildrenClosed, output.ChildrenTotal)

	// Merged MRs
	fmt.Printf("\nMerged MRs (%d):\n", len(output.MergedMRs))
	if len(output.MergedMRs) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(none)"))
	} else {
		for _, mr := range output.MergedMRs {
			fmt.Printf("  %-12s  %s\n", mr.ID, mr.Title)
		}
	}

	// Pending MRs
	fmt.Printf("\nPending MRs (%d):\n", len(output.PendingMRs))
	if len(output.PendingMRs) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(none)"))
	} else {
		for _, mr := range output.PendingMRs {
			statusInfo := ""
			if mr.Status != "" && mr.Status != "open" {
				statusInfo = fmt.Sprintf(" (%s)", mr.Status)
			}
			fmt.Printf("  %-12s  %s%s\n", mr.ID, mr.Title, style.Dim.Render(statusInfo))
		}
	}

	// Landing status
	fmt.Println()
	if output.ReadyToLand {
		fmt.Printf("%s Integration branch is ready to land.\n", style.Bold.Render("✓"))
		if output.AutoLandEnabled {
			fmt.Printf("  Auto-land: %s\n", style.Bold.Render("enabled"))
		} else {
			fmt.Printf("  Auto-land: %s\n", style.Dim.Render("disabled"))
			fmt.Printf("  Run: gt mq integration land %s\n", output.Epic)
		}
	} else {
		if output.ChildrenTotal == 0 {
			fmt.Printf("%s Epic has no children yet.\n", style.Dim.Render("○"))
		} else if output.ChildrenClosed < output.ChildrenTotal {
			fmt.Printf("%s Waiting for %d/%d children to close.\n",
				style.Dim.Render("○"), output.ChildrenTotal-output.ChildrenClosed, output.ChildrenTotal)
		} else if len(output.PendingMRs) > 0 {
			fmt.Printf("%s Waiting for %d pending MRs to merge.\n",
				style.Dim.Render("○"), len(output.PendingMRs))
		} else if output.AheadOfMain == 0 {
			fmt.Printf("%s No commits ahead of main.\n", style.Dim.Render("○"))
		}
		// Show auto-land status even when not ready
		if output.AutoLandEnabled {
			fmt.Printf("  Auto-land: %s (will land when ready)\n", style.Bold.Render("enabled"))
		} else {
			fmt.Printf("  Auto-land: %s\n", style.Dim.Render("disabled"))
		}
	}

	return nil
}
