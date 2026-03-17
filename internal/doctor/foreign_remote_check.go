package doctor

import (
	"fmt"
	"os/exec"
	"strings"
)

// ForeignRemoteCheck detects git remotes in the town repo that point to
// unrelated repositories. The town repo (~/gt, tracking stevey-gt) should
// only have its own origin remote. Remotes for rig repos (gastown, beads)
// pollute the ref space with unrelated history and cause confusion — agents
// comparing branches across unrelated remotes see phantom divergence.
type ForeignRemoteCheck struct {
	FixableCheck
	foreignRemotes []foreignRemote // Cached during Run for use in Fix
}

type foreignRemote struct {
	name string
	url  string
}

// NewForeignRemoteCheck creates a new foreign remote check.
func NewForeignRemoteCheck() *ForeignRemoteCheck {
	return &ForeignRemoteCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "foreign-remotes",
				CheckDescription: "Detect git remotes pointing to unrelated repositories",
				CheckCategory:    CategoryCore,
			},
		},
	}
}

// Run checks if the town repo has remotes beyond origin that point to
// unrelated repositories (no shared commit ancestry with origin/main).
func (c *ForeignRemoteCheck) Run(ctx *CheckContext) *CheckResult {
	c.foreignRemotes = nil

	// List all remotes
	cmd := exec.Command("git", "remote")
	cmd.Dir = ctx.TownRoot
	out, err := cmd.Output()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Not a git repository (skipped)",
		}
	}

	remotes := strings.Fields(strings.TrimSpace(string(out)))
	if len(remotes) <= 1 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No extra remotes configured",
		}
	}

	// For each non-origin remote, check if it shares history with origin
	for _, remote := range remotes {
		if remote == "origin" {
			continue
		}

		// Get remote URL for reporting
		urlCmd := exec.Command("git", "remote", "get-url", remote)
		urlCmd.Dir = ctx.TownRoot
		urlOut, _ := urlCmd.Output()
		url := strings.TrimSpace(string(urlOut))

		// Check if remote has a main/master branch we can compare
		refName := remote + "/main"
		refCmd := exec.Command("git", "rev-parse", "--verify", refName)
		refCmd.Dir = ctx.TownRoot
		if err := refCmd.Run(); err != nil {
			// Try master
			refName = remote + "/master"
			refCmd = exec.Command("git", "rev-parse", "--verify", refName)
			refCmd.Dir = ctx.TownRoot
			if err := refCmd.Run(); err != nil {
				// No main or master branch — can't verify, skip
				continue
			}
		}

		// Check for shared ancestry with origin/main
		mergeBaseCmd := exec.Command("git", "merge-base", "origin/main", refName)
		mergeBaseCmd.Dir = ctx.TownRoot
		if err := mergeBaseCmd.Run(); err != nil {
			// No common ancestor — this is a foreign remote
			c.foreignRemotes = append(c.foreignRemotes, foreignRemote{
				name: remote,
				url:  url,
			})
		}
	}

	if len(c.foreignRemotes) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All remotes share history with origin",
		}
	}

	details := []string{
		"The town repo has remotes pointing to unrelated repositories.",
		"These pollute the ref space and cause phantom divergence in git status.",
		"Rig remotes belong in their own clones (mayor/rig, crew worktrees), not here.",
	}
	for _, fr := range c.foreignRemotes {
		details = append(details, fmt.Sprintf("  %s → %s", fr.name, fr.url))
	}

	names := make([]string, len(c.foreignRemotes))
	for i, fr := range c.foreignRemotes {
		names[i] = fr.name
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d foreign remote(s): %s", len(c.foreignRemotes), strings.Join(names, ", ")),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove foreign remotes",
	}
}

// Fix removes foreign remotes from the town repo.
func (c *ForeignRemoteCheck) Fix(ctx *CheckContext) error {
	if len(c.foreignRemotes) == 0 {
		return nil
	}

	var errs []string
	for _, fr := range c.foreignRemotes {
		cmd := exec.Command("git", "remote", "remove", fr.name)
		cmd.Dir = ctx.TownRoot
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fr.name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove some remotes: %s", strings.Join(errs, "; "))
	}
	return nil
}
