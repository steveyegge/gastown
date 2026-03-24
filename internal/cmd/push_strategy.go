package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

// PushStrategy abstracts the branch push and merge submission mechanism.
// Implementations differ in how they push (origin vs fork remote) and how
// they submit work for merge (MR bead vs GitHub PR).
//
// The strategy is selected per-rig based on rig config:
//   - DefaultPushStrategy: push to origin, submit via MR bead (Refinery workflow)
//   - ForkPushStrategy: push to fork remote, submit via GitHub PR (fork workflow)
type PushStrategy interface {
	// Name returns a human-readable identifier for logging and diagnostics.
	Name() string
	// IsFork returns true if this strategy uses a fork/PR workflow.
	// Used by tap guards to allow or block gh pr create operations.
	IsFork() bool
	// Push pushes the branch to the appropriate remote. Returns an error if all
	// push attempts fail. The townRoot and rigName are used for fallback paths.
	Push(g *git.Git, branch, townRoot, rigName string) error
	// Submit creates the merge request: an MR bead (DefaultPushStrategy) or
	// a GitHub PR (ForkPushStrategy). Returns the resulting MR/PR identifier.
	Submit(params StrategySubmitParams) (string, error)
}

// StrategySubmitParams holds the parameters needed for Submit().
type StrategySubmitParams struct {
	G           *git.Git
	BD          *beads.Beads
	Branch      string
	Target      string
	IssueID     string
	Priority    int
	CommitSHA   string
	Worker      string
	AgentBeadID string
	PreVerified bool
	RigName     string
}

// selectPushStrategy returns the PushStrategy for the given rig config.
// Uses ForkPushStrategy when the rig has a PushURL (indicating a fork
// workflow), DefaultPushStrategy otherwise.
func selectPushStrategy(rigCfg *rig.RigConfig) PushStrategy {
	if rigCfg != nil && rigCfg.PushURL != "" {
		return &ForkPushStrategy{pushURL: rigCfg.PushURL, upstreamURL: rigCfg.UpstreamURL}
	}
	return &DefaultPushStrategy{}
}

// ─── DefaultPushStrategy ─────────────────────────────────────────────────────

// DefaultPushStrategy pushes to origin and submits via MR bead.
// This is the standard Gas Town workflow where the Refinery merges the work.
type DefaultPushStrategy struct{}

func (s *DefaultPushStrategy) Name() string  { return "default" }
func (s *DefaultPushStrategy) IsFork() bool  { return false }

// Push pushes the branch to origin with fallback to bare repo and mayor/rig.
func (s *DefaultPushStrategy) Push(g *git.Git, branch, townRoot, rigName string) error {
	refspec := branch + ":" + branch
	err := g.Push("origin", refspec, false)
	if err == nil {
		return nil
	}

	// Primary push failed — try fallback from the bare repo (GH #1348).
	style.PrintWarning("primary push failed: %v — trying bare repo fallback...", err)
	rigPath := filepath.Join(townRoot, rigName)
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if _, statErr := os.Stat(bareRepoPath); statErr == nil {
		bareGit := git.NewGitWithDir(bareRepoPath, "")
		if bareErr := bareGit.Push("origin", refspec, false); bareErr != nil {
			style.PrintWarning("bare repo push also failed: %v", bareErr)
		} else {
			fmt.Printf("%s Branch pushed via bare repo fallback\n", style.Bold.Render("✓"))
			return nil
		}
	} else {
		// No bare repo — try mayor/rig as last resort.
		mayorPath := filepath.Join(rigPath, "mayor", "rig")
		if _, statErr := os.Stat(mayorPath); statErr == nil {
			mayorGit := git.NewGit(mayorPath)
			if mayorErr := mayorGit.Push("origin", refspec, false); mayorErr != nil {
				style.PrintWarning("mayor/rig push also failed: %v", mayorErr)
			} else {
				fmt.Printf("%s Branch pushed via mayor/rig fallback\n", style.Bold.Render("✓"))
				return nil
			}
		}
	}

	return fmt.Errorf("push failed for branch '%s': %w", branch, err)
}

// Submit creates an MR bead in the shared beads store for the Refinery to process.
func (s *DefaultPushStrategy) Submit(params StrategySubmitParams) (string, error) {
	if params.BD == nil {
		return "", fmt.Errorf("beads instance required for default push strategy submit")
	}

	// Check for an existing MR bead (idempotency).
	var (
		existingMR *beads.Issue
		err        error
	)
	if params.CommitSHA != "" {
		existingMR, err = params.BD.FindMRForBranchAndSHA(params.Branch, params.CommitSHA)
	} else {
		existingMR, err = params.BD.FindMRForBranch(params.Branch)
	}
	if err != nil {
		style.PrintWarning("could not check for existing MR: %v", err)
	}

	if existingMR != nil {
		mrID := existingMR.ID
		fmt.Printf("%s MR already exists (idempotent)\n", style.Bold.Render("✓"))
		fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))
		return mrID, nil
	}

	// Build MR bead description.
	description := fmt.Sprintf("branch: %s\ntarget: %s\nsource_issue: %s\nrig: %s",
		params.Branch, params.Target, params.IssueID, params.RigName)
	if params.CommitSHA != "" {
		description += fmt.Sprintf("\ncommit_sha: %s", params.CommitSHA)
	}
	if params.Worker != "" {
		description += fmt.Sprintf("\nworker: %s", params.Worker)
	}
	if params.AgentBeadID != "" {
		description += fmt.Sprintf("\nagent_bead: %s", params.AgentBeadID)
	}
	description += "\nretry_count: 0"
	description += "\nlast_conflict_sha: null"
	description += "\nconflict_task_id: null"

	if params.PreVerified {
		description += "\npre_verified: true"
		description += fmt.Sprintf("\npre_verified_at: %s", time.Now().UTC().Format(time.RFC3339))
		if verifiedBase, baseErr := params.G.Rev("origin/" + params.Target); baseErr == nil {
			description += fmt.Sprintf("\npre_verified_base: %s", verifiedBase)
		} else {
			style.PrintWarning("could not resolve origin/%s for pre-verified base: %v", params.Target, baseErr)
		}
	}

	mrIssue, err := params.BD.Create(beads.CreateOptions{
		Title:       fmt.Sprintf("Merge: %s", params.IssueID),
		Labels:      []string{"gt:merge-request"},
		Priority:    params.Priority,
		Description: description,
		Ephemeral:   true,
	})
	if err != nil {
		return "", fmt.Errorf("MR bead creation failed: %w", err)
	}
	mrID := mrIssue.ID
	if mrID == "" {
		return "", fmt.Errorf("MR bead creation returned empty ID")
	}

	// Verify MR bead is readable (GH#1945).
	if verified, verifyErr := params.BD.Show(mrID); verifyErr != nil || verified == nil {
		return "", fmt.Errorf("MR bead created but verification read-back failed (id=%s): %w", mrID, verifyErr)
	}

	// Supersede older open MRs for the same source issue (GH#3032).
	if params.IssueID != "" {
		if oldMRs, findErr := params.BD.FindOpenMRsForIssue(params.IssueID); findErr == nil {
			for _, old := range oldMRs {
				if old.ID == mrID {
					continue
				}
				reason := fmt.Sprintf("superseded by %s", mrID)
				if closeErr := params.BD.CloseWithReason(reason, old.ID); closeErr != nil {
					style.PrintWarning("could not supersede old MR %s: %v", old.ID, closeErr)
					continue
				}
				fmt.Printf("  %s Superseded old MR: %s\n", style.Dim.Render("○"), old.ID)
			}
		}
	}

	// Update agent bead with active_mr reference.
	if params.AgentBeadID != "" {
		if err := params.BD.UpdateAgentActiveMR(params.AgentBeadID, mrID); err != nil {
			style.PrintWarning("could not update agent bead with active_mr: %v", err)
		}
	}

	// Back-link source issue to MR bead (GH#2599).
	if params.IssueID != "" {
		comment := fmt.Sprintf("MR created: %s", mrID)
		if _, err := params.BD.Run("comments", "add", params.IssueID, comment); err != nil {
			style.PrintWarning("could not back-link source issue %s to MR %s: %v", params.IssueID, mrID, err)
		}
	}

	fmt.Printf("%s Work submitted to merge queue (verified)\n", style.Bold.Render("✓"))
	fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))

	return mrID, nil
}

// ─── ForkPushStrategy ────────────────────────────────────────────────────────

// ForkPushStrategy pushes to the configured fork remote and submits via GitHub PR.
// Used for rigs where origin is read-only (e.g., upstream maintainer repos) and
// contributions go through a fork + pull request workflow.
type ForkPushStrategy struct {
	pushURL     string // fork remote URL (rig.PushURL)
	upstreamURL string // upstream URL (rig.UpstreamURL), used for PR base
}

func (s *ForkPushStrategy) Name() string  { return "fork" }
func (s *ForkPushStrategy) IsFork() bool  { return true }

// Push pushes the branch to the fork remote ("fork" by convention).
func (s *ForkPushStrategy) Push(g *git.Git, branch, townRoot, rigName string) error {
	forkRemote := "fork"
	refspec := branch + ":" + branch

	fmt.Printf("Pushing branch to fork remote (%s)...\n", forkRemote)
	if err := g.Push(forkRemote, refspec, false); err != nil {
		return fmt.Errorf("fork push failed for branch '%s': %w", branch, err)
	}
	fmt.Printf("%s Branch pushed to %s\n", style.Bold.Render("✓"), forkRemote)
	return nil
}

// Submit creates a GitHub PR from the fork branch to the upstream repo.
// Returns the PR URL as the submission identifier.
func (s *ForkPushStrategy) Submit(params StrategySubmitParams) (string, error) {
	// Determine the fork org from the push URL.
	forkOrg, err := extractGitHubOrg(s.pushURL)
	if err != nil {
		return "", fmt.Errorf("cannot determine fork org from push URL %q: %w", s.pushURL, err)
	}

	// Determine upstream repo path for --repo flag.
	upstreamRepo, err := extractGitHubRepo(s.upstreamURL)
	if err != nil {
		// Fallback: use origin remote URL from git config.
		if originURL, gErr := params.G.RemoteURL("origin"); gErr == nil {
			upstreamRepo, err = extractGitHubRepo(originURL)
		}
		if err != nil {
			return "", fmt.Errorf("cannot determine upstream repo: %w", err)
		}
	}

	title := fmt.Sprintf("Merge: %s", params.IssueID)
	body := fmt.Sprintf("source_issue: %s\nbranch: %s\ntarget: %s\nrig: %s\nstrategy: fork",
		params.IssueID, params.Branch, params.Target, params.RigName)
	if params.CommitSHA != "" {
		body += fmt.Sprintf("\ncommit_sha: %s", params.CommitSHA)
	}
	if params.Worker != "" {
		body += fmt.Sprintf("\nworker: %s", params.Worker)
	}

	// head is "forkOrg:branch" for cross-repo PRs.
	head := forkOrg + ":" + params.Branch

	fmt.Printf("Creating GitHub PR from %s to %s/%s...\n", head, upstreamRepo, params.Target)

	args := []string{
		"pr", "create",
		"--repo", upstreamRepo,
		"--head", head,
		"--base", params.Target,
		"--title", title,
		"--body", body,
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create failed: %w\nOutput: %s", err, strings.TrimSpace(string(out)))
	}

	prURL := strings.TrimSpace(string(out))
	fmt.Printf("%s Pull request created\n", style.Bold.Render("✓"))
	fmt.Printf("  PR: %s\n", style.Bold.Render(prURL))

	return prURL, nil
}

// extractGitHubOrg returns the org/user part from a GitHub remote URL.
// Supports both HTTPS and SSH URL formats.
func extractGitHubOrg(remoteURL string) (string, error) {
	// HTTPS: https://github.com/org/repo.git
	if strings.HasPrefix(remoteURL, "https://github.com/") {
		path := strings.TrimPrefix(remoteURL, "https://github.com/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) >= 1 && parts[0] != "" {
			return parts[0], nil
		}
	}
	// SSH: git@github.com:org/repo.git
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path := strings.TrimPrefix(remoteURL, "git@github.com:")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) >= 1 && parts[0] != "" {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("unsupported remote URL format (expected GitHub HTTPS or SSH): %q", remoteURL)
}

// extractGitHubRepo returns the "org/repo" path from a GitHub remote URL.
// Supports both HTTPS and SSH URL formats.
func extractGitHubRepo(remoteURL string) (string, error) {
	// HTTPS: https://github.com/org/repo.git
	if strings.HasPrefix(remoteURL, "https://github.com/") {
		path := strings.TrimPrefix(remoteURL, "https://github.com/")
		return strings.TrimSuffix(path, ".git"), nil
	}
	// SSH: git@github.com:org/repo.git
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path := strings.TrimPrefix(remoteURL, "git@github.com:")
		return strings.TrimSuffix(path, ".git"), nil
	}
	return "", fmt.Errorf("unsupported remote URL format (expected GitHub HTTPS or SSH): %q", remoteURL)
}
