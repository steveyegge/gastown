package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeGitdirCheck validates that worktree .git files reference existing
// gitdir paths. When a worktree's .git file contains "gitdir: /path/to/.repo.git/worktrees/X"
// but the referenced path doesn't exist, all git operations in that worktree fail.
//
// This detects the scenario from gt-fmnml: a rig's .repo.git was missing, causing
// refinery/rig and polecat worktrees to break with "fatal: not a git repository".
type WorktreeGitdirCheck struct {
	FixableCheck
	brokenWorktrees []brokenWorktree
}

type brokenWorktree struct {
	worktreePath string // e.g., /Users/stevey/gt/wyvern/refinery/rig
	gitdirTarget string // e.g., /Users/stevey/gt/wyvern/.repo.git/worktrees/rig
	rigPath      string // e.g., /Users/stevey/gt/wyvern
	bareRepoPath string // e.g., /Users/stevey/gt/wyvern/.repo.git
	reason       string // what's broken
}

// NewWorktreeGitdirCheck creates a new worktree gitdir validity check.
func NewWorktreeGitdirCheck() *WorktreeGitdirCheck {
	return &WorktreeGitdirCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "worktree-gitdir-valid",
				CheckDescription: "Verify worktree .git files reference existing gitdir paths",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run scans all rigs for worktrees with broken gitdir references.
func (c *WorktreeGitdirCheck) Run(ctx *CheckContext) *CheckResult {
	c.brokenWorktrees = nil

	entries, err := os.ReadDir(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot read town root: %v", err),
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigPath := filepath.Join(ctx.TownRoot, entry.Name())

		// Skip non-rig directories
		if !isRigDir(rigPath) {
			continue
		}

		// If --rig is specified, only check that rig
		if ctx.RigName != "" && entry.Name() != ctx.RigName {
			continue
		}

		c.checkRigWorktrees(rigPath, entry.Name())
	}

	if len(c.brokenWorktrees) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All worktree gitdir references are valid",
		}
	}

	var details []string
	for _, bw := range c.brokenWorktrees {
		relPath, _ := filepath.Rel(ctx.TownRoot, bw.worktreePath)
		if relPath == "" {
			relPath = bw.worktreePath
		}
		details = append(details, fmt.Sprintf("%s: %s", relPath, bw.reason))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("%d worktree(s) with broken gitdir references", len(c.brokenWorktrees)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to re-create broken worktrees from .repo.git",
	}
}

// checkRigWorktrees checks all worktrees within a single rig.
func (c *WorktreeGitdirCheck) checkRigWorktrees(rigPath, rigName string) {
	// Check refinery/rig
	refineryRig := filepath.Join(rigPath, "refinery", "rig")
	c.checkWorktree(refineryRig, rigPath)

	// Check polecats (both structures: polecats/<name>/<rigname>/ and polecats/<name>/)
	polecatsDir := filepath.Join(rigPath, "polecats")
	polecatEntries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return
	}

	for _, entry := range polecatEntries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Try new structure first: polecats/<name>/<rigname>/
		newPath := filepath.Join(polecatsDir, entry.Name(), rigName)
		if c.hasGitFile(newPath) {
			c.checkWorktree(newPath, rigPath)
			continue
		}

		// Fall back to old structure: polecats/<name>/
		oldPath := filepath.Join(polecatsDir, entry.Name())
		if c.hasGitFile(oldPath) {
			c.checkWorktree(oldPath, rigPath)
		}
	}

	// Check witness/rig
	witnessRig := filepath.Join(rigPath, "witness", "rig")
	if c.hasGitFile(witnessRig) {
		c.checkWorktree(witnessRig, rigPath)
	}
}

// checkWorktree validates a single worktree's .git file reference.
func (c *WorktreeGitdirCheck) checkWorktree(worktreePath, rigPath string) {
	gitFile := filepath.Join(worktreePath, ".git")

	info, err := os.Stat(gitFile)
	if err != nil {
		return // No .git file, not a worktree
	}

	// Only check .git files (worktrees), not .git directories (clones)
	if info.IsDir() {
		return
	}

	content, err := os.ReadFile(gitFile)
	if err != nil {
		c.brokenWorktrees = append(c.brokenWorktrees, brokenWorktree{
			worktreePath: worktreePath,
			rigPath:      rigPath,
			reason:       fmt.Sprintf("cannot read .git file: %v", err),
		})
		return
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		c.brokenWorktrees = append(c.brokenWorktrees, brokenWorktree{
			worktreePath: worktreePath,
			rigPath:      rigPath,
			reason:       fmt.Sprintf("malformed .git file (no gitdir: prefix): %s", line),
		})
		return
	}

	gitdirTarget := strings.TrimPrefix(line, "gitdir: ")

	// Resolve relative paths
	if !filepath.IsAbs(gitdirTarget) {
		gitdirTarget = filepath.Join(worktreePath, gitdirTarget)
	}

	// Check if the gitdir target exists
	if _, err := os.Stat(gitdirTarget); os.IsNotExist(err) {
		// Determine what's missing: the .repo.git itself, or just the worktree entry
		bareRepoPath := ""
		if strings.Contains(gitdirTarget, ".repo.git") {
			parts := strings.SplitN(gitdirTarget, ".repo.git", 2)
			bareRepoPath = parts[0] + ".repo.git"
		}

		reason := fmt.Sprintf("gitdir target does not exist: %s", gitdirTarget)
		if bareRepoPath != "" {
			if _, err := os.Stat(bareRepoPath); os.IsNotExist(err) {
				reason = fmt.Sprintf(".repo.git missing (gitdir: %s)", gitdirTarget)
			} else {
				reason = fmt.Sprintf("worktree entry missing in .repo.git (gitdir: %s)", gitdirTarget)
			}
		}

		c.brokenWorktrees = append(c.brokenWorktrees, brokenWorktree{
			worktreePath: worktreePath,
			gitdirTarget: gitdirTarget,
			rigPath:      rigPath,
			bareRepoPath: bareRepoPath,
			reason:       reason,
		})
	}
}

// hasGitFile checks if a directory has a .git file (not directory).
func (c *WorktreeGitdirCheck) hasGitFile(path string) bool {
	gitFile := filepath.Join(path, ".git")
	info, err := os.Stat(gitFile)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// Fix attempts to re-create broken worktrees.
func (c *WorktreeGitdirCheck) Fix(ctx *CheckContext) error {
	var lastErr error

	for _, bw := range c.brokenWorktrees {
		if bw.bareRepoPath == "" {
			lastErr = fmt.Errorf("%s: cannot fix (not a .repo.git worktree)", bw.worktreePath)
			continue
		}

		// Check if .repo.git exists
		if _, err := os.Stat(bw.bareRepoPath); os.IsNotExist(err) {
			lastErr = fmt.Errorf("%s: cannot fix (.repo.git does not exist, needs re-clone via 'gt rig install')", bw.worktreePath)
			continue
		}

		// .repo.git exists but worktree entry is missing - re-create the worktree.
		// First remove the broken .git file so git worktree add can create a fresh one.
		gitFile := filepath.Join(bw.worktreePath, ".git")
		if err := os.Remove(gitFile); err != nil {
			lastErr = fmt.Errorf("%s: cannot remove broken .git file: %w", bw.worktreePath, err)
			continue
		}

		// Determine default branch from the bare repo
		cmd := exec.Command("git", "-C", bw.bareRepoPath, "symbolic-ref", "HEAD")
		out, err := cmd.Output()
		branch := "main" // fallback
		if err == nil {
			ref := strings.TrimSpace(string(out))
			branch = strings.TrimPrefix(ref, "refs/heads/")
		}

		// Re-create the worktree
		cmd = exec.Command("git", "-C", bw.bareRepoPath, "worktree", "add", bw.worktreePath, branch)
		if output, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s: failed to re-create worktree: %v (%s)",
				bw.worktreePath, err, strings.TrimSpace(string(output)))
		}
	}

	return lastErr
}

// isRigDir checks if a directory looks like a rig (has config.json or known subdirectories).
func isRigDir(path string) bool {
	// Check for config.json (most reliable indicator)
	if _, err := os.Stat(filepath.Join(path, "config.json")); err == nil {
		return true
	}
	// Check for known rig subdirectories
	markers := []string{"refinery", "witness", "polecats", "mayor"}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}
