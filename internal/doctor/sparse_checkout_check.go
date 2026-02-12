package doctor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/git"
)

// SparseCheckoutCheck detects legacy sparse checkout configurations that should be removed.
// Sparse checkout was previously used to exclude .claude/ from source repos, but this
// prevented valid .claude/ files in rigged repos from being used. Now that gastown's
// repo no longer has .claude/ files, sparse checkout is no longer needed.
type SparseCheckoutCheck struct {
	FixableCheck
	rigPath       string
	affectedRepos []string // repos with legacy sparse checkout that should be removed
}

// NewSparseCheckoutCheck creates a new sparse checkout check.
func NewSparseCheckoutCheck() *SparseCheckoutCheck {
	return &SparseCheckoutCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "sparse-checkout",
				CheckDescription: "Check for legacy sparse checkout configuration that should be removed",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run checks if any git repos have legacy sparse checkout configured.
func (c *SparseCheckoutCheck) Run(ctx *CheckContext) *CheckResult {
	c.rigPath = ctx.RigPath()
	if c.rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	c.affectedRepos = nil

	// Check all git repo locations
	repoPaths := []string{
		filepath.Join(c.rigPath, "mayor", "rig"),
		filepath.Join(c.rigPath, "refinery", "rig"),
	}

	// Add crew clones
	crewDir := filepath.Join(c.rigPath, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() != "README.md" {
				repoPaths = append(repoPaths, filepath.Join(crewDir, entry.Name()))
			}
		}
	}

	// Add polecat worktrees (nested structure: polecats/<name>/<rigname>/)
	polecatDir := filepath.Join(c.rigPath, "polecats")
	if entries, err := os.ReadDir(polecatDir); err == nil {
		rigName := filepath.Base(c.rigPath)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// The actual worktree is at polecats/<name>/<rigname>/
			worktreePath := filepath.Join(polecatDir, entry.Name(), rigName)
			if _, err := os.Stat(worktreePath); err == nil {
				repoPaths = append(repoPaths, worktreePath)
			} else {
				// Fallback: legacy flat layout polecats/<name>/
				repoPaths = append(repoPaths, filepath.Join(polecatDir, entry.Name()))
			}
		}
	}

	for _, repoPath := range repoPaths {
		// Skip if not a git repo
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
			continue
		}

		// Check if sparse checkout is configured (legacy configuration to remove)
		if git.IsSparseCheckoutConfigured(repoPath) {
			c.affectedRepos = append(c.affectedRepos, repoPath)
		}
	}

	if len(c.affectedRepos) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No legacy sparse checkout configurations found",
		}
	}

	// Build details with relative paths
	var details []string
	for _, repoPath := range c.affectedRepos {
		relPath, _ := filepath.Rel(c.rigPath, repoPath)
		if relPath == "" {
			relPath = repoPath
		}
		details = append(details, relPath)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d repo(s) have legacy sparse checkout that should be removed", len(c.affectedRepos)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove sparse checkout and restore .claude/ files",
	}
}

// Fix removes sparse checkout configuration from affected repos.
func (c *SparseCheckoutCheck) Fix(ctx *CheckContext) error {
	for _, repoPath := range c.affectedRepos {
		if err := git.RemoveSparseCheckout(repoPath); err != nil {
			relPath, _ := filepath.Rel(c.rigPath, repoPath)
			return fmt.Errorf("failed to remove sparse checkout for %s: %w", relPath, err)
		}
	}
	return nil
}
