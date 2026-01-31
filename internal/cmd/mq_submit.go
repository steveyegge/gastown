package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// branchInfo holds parsed branch information.
type branchInfo struct {
	Branch string // Full branch name
	Issue  string // Issue ID extracted from branch
	Worker string // Worker name (polecat name)
}

// issuePattern matches issue IDs in branch names (e.g., "gt-xyz" or "gt-abc.1")
var issuePattern = regexp.MustCompile(`([a-z]+-[a-z0-9]+(?:\.[0-9]+)?)`)

// parseBranchName extracts issue ID and worker from a branch name.
// Supports formats:
//   - polecat/<worker>/<issue>  → issue=<issue>, worker=<worker>
//   - polecat/<worker>-<timestamp>  → issue="", worker=<worker> (modern polecat branches)
//   - <issue>                   → issue=<issue>, worker=""
func parseBranchName(branch string) branchInfo {
	info := branchInfo{Branch: branch}

	// Try polecat/<worker>/<issue> or polecat/<worker>/<issue>@<timestamp> format
	if strings.HasPrefix(branch, constants.BranchPolecatPrefix) {
		parts := strings.SplitN(branch, "/", 3)
		if len(parts) == 3 {
			info.Worker = parts[1]
			// Strip @timestamp suffix if present (e.g., "gt-abc@mk123" -> "gt-abc")
			issue := parts[2]
			if atIdx := strings.Index(issue, "@"); atIdx > 0 {
				issue = issue[:atIdx]
			}
			info.Issue = issue
			return info
		}
		// Modern polecat branch format: polecat/<worker>-<timestamp>
		// The second part is "worker-timestamp", not an issue ID.
		// Don't try to extract an issue ID - gt done will use hook_bead fallback.
		if len(parts) == 2 {
			// Extract worker name from "worker-timestamp" format
			workerPart := parts[1]
			if dashIdx := strings.LastIndex(workerPart, "-"); dashIdx > 0 {
				info.Worker = workerPart[:dashIdx]
			} else {
				info.Worker = workerPart
			}
			// Explicitly don't set info.Issue - let hook_bead fallback handle it
			return info
		}
	}

	// Try to find an issue ID pattern in the branch name
	// Common patterns: prefix-xxx, prefix-xxx.n (subtask)
	if matches := issuePattern.FindStringSubmatch(branch); len(matches) > 1 {
		info.Issue = matches[1]
	}

	return info
}

func runMqSubmit(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	rigName, r, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Initialize git for the current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	g := git.NewGit(cwd)

	// Get current branch
	branch := mqSubmitBranch
	if branch == "" {
		branch, err = g.CurrentBranch()
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
	}

	// Get configured default branch for validation (can't submit default branch itself)
	defaultBranch := r.DefaultBranch()

	if branch == defaultBranch || branch == "master" {
		return fmt.Errorf("cannot submit %s/master branch to merge queue", defaultBranch)
	}

	// Parse branch info
	info := parseBranchName(branch)

	// Override with explicit flags
	issueID := mqSubmitIssue
	if issueID == "" {
		issueID = info.Issue
	}
	worker := info.Worker

	if issueID == "" {
		return fmt.Errorf("cannot determine source issue from branch '%s'; use --issue to specify", branch)
	}

	// Initialize beads for looking up source issue
	bd := beads.New(cwd)

	// Determine target branch
	// Use rig's TargetBranch (may be a release branch if configured)
	target := r.TargetBranch()
	if mqSubmitEpic != "" {
		// Explicit --epic flag takes precedence
		target = "integration/" + mqSubmitEpic
	} else {
		// Auto-detect: check if source issue has a parent epic with an integration branch
		autoTarget, err := detectIntegrationBranch(bd, g, issueID)
		if err != nil {
			// Non-fatal: log and continue with target branch as default
			fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("(note: %v)", err)))
		} else if autoTarget != "" {
			target = autoTarget
		}
	}

	// Get source issue for priority inheritance
	var priority int
	if mqSubmitPriority >= 0 {
		priority = mqSubmitPriority
	} else {
		// Try to inherit from source issue
		sourceIssue, err := bd.Show(issueID)
		if err != nil {
			// Issue not found, use default priority
			priority = 2
		} else {
			priority = sourceIssue.Priority
		}
	}

	// Build MR bead title and description
	title := fmt.Sprintf("Merge: %s", issueID)
	description := fmt.Sprintf("branch: %s\ntarget: %s\nsource_issue: %s\nrig: %s",
		branch, target, issueID, rigName)
	if worker != "" {
		description += fmt.Sprintf("\nworker: %s", worker)
	}

	// Check if MR bead already exists for this branch (idempotency)
	var mrIssue *beads.Issue
	existingMR, err := bd.FindMRForBranch(branch)
	if err != nil {
		style.PrintWarning("could not check for existing MR: %v", err)
		// Continue with creation attempt - Create will fail if duplicate
	} else if existingMR != nil {
		mrIssue = existingMR
		fmt.Printf("%s MR already exists (idempotent)\n", style.Bold.Render("✓"))
	} else {
		// Create MR bead (ephemeral wisp - will be cleaned up after merge)
		mrIssue, err = bd.Create(beads.CreateOptions{
			Title:       title,
			Type:        "merge-request",
			Priority:    priority,
			Description: description,
			Ephemeral:   true,
		})
		if err != nil {
			return fmt.Errorf("creating merge request bead: %w", err)
		}
	}

	// Success output
	fmt.Printf("%s Submitted to merge queue\n", style.Bold.Render("✓"))
	fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrIssue.ID))
	fmt.Printf("  Source: %s\n", branch)
	fmt.Printf("  Target: %s\n", target)
	fmt.Printf("  Issue: %s\n", issueID)
	if worker != "" {
		fmt.Printf("  Worker: %s\n", worker)
	}
	fmt.Printf("  Priority: P%d\n", priority)

	// Auto-cleanup for polecats: if this is a polecat branch and cleanup not disabled,
	// send lifecycle request and wait for termination
	if worker != "" && !mqSubmitNoCleanup {
		fmt.Println()
		fmt.Printf("%s Auto-cleanup: polecat work submitted\n", style.Bold.Render("✓"))
		if err := polecatCleanup(rigName, worker, townRoot); err != nil {
			// Non-fatal: warn but return success (MR was created)
			style.PrintWarning("Could not auto-cleanup: %v", err)
			fmt.Println(style.Dim.Render("  You may need to run 'gt handoff --shutdown' manually"))
			return nil
		}
		// polecatCleanup may timeout while waiting, but MR was already created
	}

	return nil
}

// detectIntegrationBranch checks if an issue is a descendant of an epic that has an integration branch.
// Traverses up the parent chain until it finds an epic or runs out of parents.
// Returns the integration branch target (e.g., "integration/gt-epic") if found, or "" if not.
func detectIntegrationBranch(bd *beads.Beads, g *git.Git, issueID string) (string, error) {
	// Traverse up the parent chain looking for an epic with an integration branch
	// Limit depth to prevent infinite loops in case of circular references
	const maxDepth = 10
	currentID := issueID

	for depth := 0; depth < maxDepth; depth++ {
		// Get the current issue
		issue, err := bd.Show(currentID)
		if err != nil {
			return "", fmt.Errorf("looking up issue %s: %w", currentID, err)
		}

		// Check if this issue is an epic
		if issue.Type == "epic" {
			// Found an epic - check if it has an integration branch
			integrationBranch := "integration/" + issue.ID

			// Check local first (faster)
			exists, err := g.BranchExists(integrationBranch)
			if err != nil {
				return "", fmt.Errorf("checking local branch: %w", err)
			}
			if exists {
				return integrationBranch, nil
			}

			// Check remote
			exists, err = g.RemoteBranchExists("origin", integrationBranch)
			if err != nil {
				// Remote check failure is non-fatal, continue to parent
			} else if exists {
				return integrationBranch, nil
			}
			// Epic found but no integration branch - continue checking parents
			// in case there's a higher-level epic with an integration branch
		}

		// Move to parent
		if issue.Parent == "" {
			return "", nil // No more parents, no integration branch found
		}
		currentID = issue.Parent
	}

	return "", nil // Max depth reached, no integration branch found
}

// polecatCleanup sends a lifecycle shutdown request to the witness and waits for termination.
// This is called after a polecat successfully submits an MR.
func polecatCleanup(rigName, worker, townRoot string) error {
	// Send lifecycle request to witness
	manager := rigName + "/witness"
	subject := fmt.Sprintf("LIFECYCLE: polecat-%s requesting shutdown", worker)
	body := fmt.Sprintf(`Lifecycle request from polecat %s.

Action: shutdown
Reason: MR submitted to merge queue
Time: %s

Please verify state and execute lifecycle action.
`, worker, time.Now().Format(time.RFC3339))

	// Send via gt mail
	cmd := exec.Command("gt", "mail", "send", manager,
		"-s", subject,
		"-m", body,
	)
	cmd.Dir = townRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sending lifecycle request: %w: %s", err, string(out))
	}
	fmt.Printf("%s Sent shutdown request to %s\n", style.Bold.Render("✓"), manager)

	// Wait for retirement with periodic status
	fmt.Println()
	fmt.Printf("%s Waiting for retirement...\n", style.Dim.Render("◌"))
	fmt.Println(style.Dim.Render("(Witness will terminate this session)"))

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Timeout after 5 minutes to prevent indefinite blocking
	const maxCleanupWait = 5 * time.Minute
	timeout := time.After(maxCleanupWait)

	waitStart := time.Now()
	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(waitStart).Round(time.Second)
			fmt.Printf("%s Still waiting (%v elapsed)...\n", style.Dim.Render("◌"), elapsed)
			if elapsed >= 2*time.Minute {
				fmt.Println(style.Dim.Render("  Hint: If witness isn't responding, you may need to:"))
				fmt.Println(style.Dim.Render("  - Check if witness is running: gt rig status"))
				fmt.Println(style.Dim.Render("  - Use Ctrl+C to abort and manually exit"))
			}
		case <-timeout:
			fmt.Printf("%s Timeout waiting for polecat retirement\n", style.WarningPrefix)
			fmt.Println(style.Dim.Render("  The polecat may have already terminated, or witness is unresponsive."))
			fmt.Println(style.Dim.Render("  You can verify with: gt polecat status"))
			return nil // Don't fail the MR submission just because cleanup timed out
		}
	}
}
