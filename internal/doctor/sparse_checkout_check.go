package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
)

// SparseCheckoutCheck verifies that git clones/worktrees have sparse checkout configured
// to exclude Claude Code context files from source repos. This ensures source repo settings
// and instructions don't override Gas Town agent configuration.
// Excluded files: .claude/, CLAUDE.md, CLAUDE.local.md
// Note: .mcp.json is NOT excluded so worktrees inherit MCP server config.
type SparseCheckoutCheck struct {
	FixableCheck
	rigPath       string
	affectedRepos []string // repos missing sparse checkout configuration
}

// NewSparseCheckoutCheck creates a new sparse checkout check.
func NewSparseCheckoutCheck() *SparseCheckoutCheck {
	return &SparseCheckoutCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "sparse-checkout",
				CheckDescription: "Verify sparse checkout excludes Claude context files (.claude/, CLAUDE.md, etc.)",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run checks if sparse checkout is configured for all git repos in the rig.
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

	// Add polecat worktrees
	polecatDir := filepath.Join(c.rigPath, "polecats")
	if entries, err := os.ReadDir(polecatDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				repoPaths = append(repoPaths, filepath.Join(polecatDir, entry.Name()))
			}
		}
	}

	for _, repoPath := range repoPaths {
		// Skip if not a git repo
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
			continue
		}

		// Check if sparse checkout is configured (not just if .claude/ exists)
		if !git.IsSparseCheckoutConfigured(repoPath) {
			c.affectedRepos = append(c.affectedRepos, repoPath)
		}
	}

	if len(c.affectedRepos) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All repos have sparse checkout configured to exclude Claude context files",
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
		Status:  StatusError,
		Message: fmt.Sprintf("%d repo(s) missing sparse checkout configuration", len(c.affectedRepos)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to configure sparse checkout",
	}
}

// Fix configures sparse checkout for affected repos to exclude Claude context files.
func (c *SparseCheckoutCheck) Fix(ctx *CheckContext) error {
	for _, repoPath := range c.affectedRepos {
		if err := git.ConfigureSparseCheckout(repoPath); err != nil {
			relPath, _ := filepath.Rel(c.rigPath, repoPath)
			return fmt.Errorf("failed to configure sparse checkout for %s: %w", relPath, err)
		}

		// Check if any excluded files remain (untracked or modified files won't be removed by git read-tree)
		if remaining := git.CheckExcludedFilesExist(repoPath); len(remaining) > 0 {
			relPath, _ := filepath.Rel(c.rigPath, repoPath)
			return fmt.Errorf("sparse checkout configured for %s but these files still exist: %s\n"+
				"These files are untracked or modified and were not removed by git.\n"+
				"Please manually remove or revert these files in %s",
				relPath, strings.Join(remaining, ", "), repoPath)
		}
	}
	return nil
}
