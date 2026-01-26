package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BeadsSyncWorktreeCheck detects orphaned beads-sync worktrees in crew/polecat
// workspaces. These worktrees were incorrectly created by a bug in beads v0.47.1
// (fixed in v0.48.0) when bd sync was run from a workspace with a beads redirect.
//
// Workspaces using beads redirects should NOT have their own beads-sync worktrees;
// the worktree belongs in the redirected location (typically mayor/rig).
type BeadsSyncWorktreeCheck struct {
	FixableCheck
	orphanedWorktrees []string // Cached during Run for use in Fix
}

// NewBeadsSyncWorktreeCheck creates a new beads-sync worktree check.
func NewBeadsSyncWorktreeCheck() *BeadsSyncWorktreeCheck {
	return &BeadsSyncWorktreeCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "beads-sync-worktree",
				CheckDescription: "Detect orphaned beads-sync worktrees in redirected workspaces",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks for orphaned beads-sync worktrees.
func (c *BeadsSyncWorktreeCheck) Run(ctx *CheckContext) *CheckResult {
	var orphaned []string

	// Find all crew and polecat workspaces
	workspaces := c.findRedirectedWorkspaces(ctx.TownRoot)

	for _, ws := range workspaces {
		// Check if workspace has a beads redirect
		redirectPath := filepath.Join(ws, ".beads", "redirect")
		if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
			// No redirect, this workspace manages its own beads - skip
			continue
		}

		// Workspace uses redirect - check for orphaned worktree
		worktreePath := filepath.Join(ws, ".git", "beads-worktrees", "beads-sync")
		if _, err := os.Stat(worktreePath); err == nil {
			// Orphaned worktree found
			relPath := c.relativePath(ctx.TownRoot, worktreePath)
			orphaned = append(orphaned, relPath)
		}
	}

	// Cache for Fix
	c.orphanedWorktrees = nil
	for _, ws := range workspaces {
		redirectPath := filepath.Join(ws, ".beads", "redirect")
		if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
			continue
		}
		worktreePath := filepath.Join(ws, ".git", "beads-worktrees", "beads-sync")
		if _, err := os.Stat(worktreePath); err == nil {
			c.orphanedWorktrees = append(c.orphanedWorktrees, worktreePath)
		}
	}

	if len(orphaned) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No orphaned beads-sync worktrees found",
			Category: c.Category(),
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d orphaned beads-sync worktree(s) found", len(orphaned)),
		Details:  orphaned,
		FixHint:  "Run 'gt doctor --fix' to remove orphaned worktrees",
		Category: c.Category(),
	}
}

// Fix removes orphaned beads-sync worktrees.
func (c *BeadsSyncWorktreeCheck) Fix(ctx *CheckContext) error {
	for _, worktreePath := range c.orphanedWorktrees {
		// Remove the orphaned worktree directory
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("removing %s: %w", worktreePath, err)
		}

		// Also clean up the parent beads-worktrees dir if now empty
		parentDir := filepath.Dir(worktreePath)
		entries, err := os.ReadDir(parentDir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(parentDir) // Best effort, ignore errors
		}
	}
	return nil
}

// findRedirectedWorkspaces finds all crew and polecat workspaces that might
// have beads redirects (and thus might have orphaned worktrees).
func (c *BeadsSyncWorktreeCheck) findRedirectedWorkspaces(townRoot string) []string {
	var workspaces []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return workspaces
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" {
			continue
		}

		rigPath := filepath.Join(townRoot, entry.Name())

		// Check if this looks like a rig
		if !c.isRig(rigPath) {
			continue
		}

		// Add crew workspaces
		crewPath := filepath.Join(rigPath, "crew")
		if crewEntries, err := os.ReadDir(crewPath); err == nil {
			for _, crew := range crewEntries {
				if crew.IsDir() && !strings.HasPrefix(crew.Name(), ".") {
					workspaces = append(workspaces, filepath.Join(crewPath, crew.Name()))
				}
			}
		}

		// Add polecat workspaces (both old and new structure)
		rigName := entry.Name()
		polecatsPath := filepath.Join(rigPath, "polecats")
		if polecatEntries, err := os.ReadDir(polecatsPath); err == nil {
			for _, polecat := range polecatEntries {
				if polecat.IsDir() && !strings.HasPrefix(polecat.Name(), ".") {
					// Try new structure first: polecats/<name>/<rigname>/
					newPath := filepath.Join(polecatsPath, polecat.Name(), rigName)
					if c.isGitRepo(newPath) {
						workspaces = append(workspaces, newPath)
					} else {
						// Fall back to old structure: polecats/<name>/
						oldPath := filepath.Join(polecatsPath, polecat.Name())
						if c.isGitRepo(oldPath) {
							workspaces = append(workspaces, oldPath)
						}
					}
				}
			}
		}
	}

	return workspaces
}

// isRig checks if a directory looks like a rig.
func (c *BeadsSyncWorktreeCheck) isRig(path string) bool {
	markers := []string{"crew", "polecats", "witness", "refinery", "mayor"}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

// isGitRepo checks if a directory is a git repository.
func (c *BeadsSyncWorktreeCheck) isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	return false
}

// relativePath returns path relative to base.
func (c *BeadsSyncWorktreeCheck) relativePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
