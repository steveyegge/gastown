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

// Integration branch template constants
const defaultIntegrationBranchTemplate = "integration/{epic}"

// invalidBranchCharsRegex matches characters that are invalid in git branch names.
// Git branch names cannot contain: ~ ^ : \ space, .., @{, or end with .lock
var invalidBranchCharsRegex = regexp.MustCompile(`[~^:\s\\]|\.\.|\.\.|@\{`)

// buildIntegrationBranchName expands an integration branch template with variables.
// Variables supported:
//   - {epic}: Full epic ID (e.g., "RA-123")
//   - {prefix}: Epic prefix before first hyphen (e.g., "RA")
//   - {user}: Git user.name (e.g., "klauern")
//
// If template is empty, uses defaultIntegrationBranchTemplate.
func buildIntegrationBranchName(template, epicID string) string {
	if template == "" {
		template = defaultIntegrationBranchTemplate
	}

	result := template
	result = strings.ReplaceAll(result, "{epic}", epicID)
	result = strings.ReplaceAll(result, "{prefix}", extractEpicPrefix(epicID))

	// Git user (optional - leaves placeholder if not available)
	if user := getGitUserName(); user != "" {
		result = strings.ReplaceAll(result, "{user}", user)
	}

	return result
}

// extractEpicPrefix extracts the prefix from an epic ID (before the first hyphen).
// Examples: "RA-123" -> "RA", "PROJ-456" -> "PROJ", "abc" -> "abc"
func extractEpicPrefix(epicID string) string {
	if idx := strings.Index(epicID, "-"); idx > 0 {
		return epicID[:idx]
	}
	return epicID
}

// getGitUserName returns the git user.name config value, or empty if not set.
func getGitUserName() string {
	cmd := exec.Command("git", "config", "user.name")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

// getIntegrationBranchField extracts the integration_branch field from an epic's description.
// Returns empty string if the field is not found.
func getIntegrationBranchField(description string) string {
	if description == "" {
		return ""
	}

	lines := strings.Split(description, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "integration_branch:") {
			value := strings.TrimPrefix(trimmed, "integration_branch:")
			value = strings.TrimPrefix(value, "Integration_branch:")
			value = strings.TrimPrefix(value, "INTEGRATION_BRANCH:")
			// Handle case variations
			for _, prefix := range []string{"integration_branch:", "Integration_branch:", "INTEGRATION_BRANCH:"} {
				if strings.HasPrefix(trimmed, prefix) {
					value = strings.TrimPrefix(trimmed, prefix)
					break
				}
			}
			// Re-parse properly - the prefix removal above is messy
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
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
	Epic        string                       `json:"epic"`
	Branch      string                       `json:"branch"`
	Created     string                       `json:"created,omitempty"`
	AheadOfMain int                          `json:"ahead_of_main"`
	MergedMRs   []IntegrationStatusMRSummary `json:"merged_mrs"`
	PendingMRs  []IntegrationStatusMRSummary `json:"pending_mrs"`
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
	g := git.NewGit(r.Path)

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

	// Ensure we have latest main
	fmt.Printf("Fetching latest from origin...\n")
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching from origin: %w", err)
	}

	// 2. Create branch from origin/main
	fmt.Printf("Creating branch '%s' from main...\n", branchName)
	if err := g.CreateBranchFrom(branchName, "origin/main"); err != nil {
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
	fmt.Printf("  From:   main\n")
	fmt.Printf("\n  Future MRs for this epic's children can target:\n")
	fmt.Printf("    gt mq submit --epic %s\n", epicID)

	return nil
}

// addIntegrationBranchField adds or updates the integration_branch field in a description.
func addIntegrationBranchField(description, branchName string) string {
	fieldLine := "integration_branch: " + branchName

	// If description is empty, just return the field
	if description == "" {
		return fieldLine
	}

	// Check if integration_branch field already exists
	lines := strings.Split(description, "\n")
	var newLines []string
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "integration_branch:") {
			// Replace existing field
			newLines = append(newLines, fieldLine)
			found = true
		} else {
			newLines = append(newLines, line)
		}
	}

	if !found {
		// Add field at the beginning
		newLines = append([]string{fieldLine}, newLines...)
	}

	return strings.Join(newLines, "\n")
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
	bd := beads.New(r.Path)
	g := git.NewGit(r.Path)

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
		fmt.Printf("  1. Merge %s to main (--no-ff)\n", branchName)
		if !mqIntegrationLandSkipTests {
			fmt.Printf("  2. Run tests on main\n")
		}
		fmt.Printf("  3. Push main to origin\n")
		fmt.Printf("  4. Delete integration branch (local and remote)\n")
		fmt.Printf("  5. Update epic status to closed\n")
		return nil
	}

	// Ensure working directory is clean
	status, err := g.Status()
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}
	if !status.Clean {
		return fmt.Errorf("working directory is not clean; please commit or stash changes")
	}

	// Fetch latest
	fmt.Printf("Fetching latest from origin...\n")
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching from origin: %w", err)
	}

	// 4. Checkout main and merge integration branch
	fmt.Printf("Checking out main...\n")
	if err := g.Checkout("main"); err != nil {
		return fmt.Errorf("checking out main: %w", err)
	}

	// Pull latest main
	if err := g.Pull("origin", "main"); err != nil {
		// Non-fatal if pull fails (e.g., first time)
		fmt.Printf("  %s\n", style.Dim.Render("(pull from origin/main skipped)"))
	}

	// Merge with --no-ff
	fmt.Printf("Merging %s to main...\n", branchName)
	mergeMsg := fmt.Sprintf("Merge %s: %s\n\nEpic: %s", branchName, epic.Title, epicID)
	if err := g.MergeNoFF("origin/"+branchName, mergeMsg); err != nil {
		// Abort merge on failure (best-effort cleanup)
		_ = g.AbortMerge()
		return fmt.Errorf("merge failed: %w", err)
	}
	fmt.Printf("  %s Merged successfully\n", style.Bold.Render("✓"))

	// 5. Run tests (if configured and not skipped)
	if !mqIntegrationLandSkipTests {
		testCmd := getTestCommand(r.Path)
		if testCmd != "" {
			fmt.Printf("Running tests: %s\n", testCmd)
			if err := runTestCommand(r.Path, testCmd); err != nil {
				// Tests failed - reset main
				fmt.Printf("  %s Tests failed, resetting main...\n", style.Bold.Render("✗"))
				_ = g.Checkout("main") // best-effort: need to be on main to reset
				resetErr := resetHard(g, "HEAD~1")
				if resetErr != nil {
					return fmt.Errorf("tests failed and could not reset: %w (test error: %v)", resetErr, err)
				}
				return fmt.Errorf("tests failed: %w", err)
			}
			fmt.Printf("  %s Tests passed\n", style.Bold.Render("✓"))
		} else {
			fmt.Printf("  %s\n", style.Dim.Render("(no test command configured)"))
		}
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("(tests skipped)"))
	}

	// 6. Push to origin
	fmt.Printf("Pushing main to origin...\n")
	if err := g.Push("origin", "main", false); err != nil {
		// Reset on push failure
		resetErr := resetHard(g, "HEAD~1")
		if resetErr != nil {
			return fmt.Errorf("push failed and could not reset: %w (push error: %v)", resetErr, err)
		}
		return fmt.Errorf("push failed: %w", err)
	}
	fmt.Printf("  %s Pushed to origin\n", style.Bold.Render("✓"))

	// 7. Delete integration branch
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
	fmt.Printf("  Branch: %s → main\n", branchName)

	return nil
}

// findOpenMRsForIntegration finds all open merge requests targeting an integration branch.
func findOpenMRsForIntegration(bd *beads.Beads, targetBranch string) ([]*beads.Issue, error) {
	// List all open MRs by label (bd-3q6.10: MRs now use type=task)
	opts := beads.ListOptions{
		Label:  "gt:merge-request",
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
	g := git.NewGit(r.Path)

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

	// Get all MRs by label (bd-3q6.10: MRs now use type=task)
	allMRs, err := bd.List(beads.ListOptions{
		Label:  "gt:merge-request",
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

	// Build output structure
	output := IntegrationStatusOutput{
		Epic:        epicID,
		Branch:      branchName,
		Created:     createdDate,
		AheadOfMain: aheadCount,
		MergedMRs:   make([]IntegrationStatusMRSummary, 0, len(mergedMRs)),
		PendingMRs:  make([]IntegrationStatusMRSummary, 0, len(pendingMRs)),
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

// printIntegrationStatus prints the integration status in human-readable format.
func printIntegrationStatus(output *IntegrationStatusOutput) error {
	fmt.Printf("Integration: %s\n", style.Bold.Render(output.Branch))
	if output.Created != "" {
		fmt.Printf("Created: %s\n", output.Created)
	}
	fmt.Printf("Ahead of main: %d commits\n", output.AheadOfMain)

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

	return nil
}
