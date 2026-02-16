package doctor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LandWorktreeGitignoreCheck ensures .land-worktree/ is in each rig's .gitignore.
// This entry is added during rig init, but existing rigs created before the
// integration-branch feature may not have it.
type LandWorktreeGitignoreCheck struct {
	FixableCheck
	// affectedRigs caches rig paths missing the entry (populated by Run, consumed by Fix)
	affectedRigs []string
}

// NewLandWorktreeGitignoreCheck creates a new land-worktree gitignore check.
func NewLandWorktreeGitignoreCheck() *LandWorktreeGitignoreCheck {
	return &LandWorktreeGitignoreCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "land-worktree-gitignore",
				CheckDescription: "Check that .land-worktree/ is gitignored in all rigs",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks each rig's .gitignore for the .land-worktree/ entry.
func (c *LandWorktreeGitignoreCheck) Run(ctx *CheckContext) *CheckResult {
	rigs := findAllRigs(ctx.TownRoot)
	if len(rigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs found",
		}
	}

	c.affectedRigs = nil
	var details []string

	for _, rigPath := range rigs {
		gitignorePath := filepath.Join(rigPath, ".gitignore")
		if !hasGitignoreEntry(gitignorePath, ".land-worktree/") {
			c.affectedRigs = append(c.affectedRigs, rigPath)
			rigName := filepath.Base(rigPath)
			details = append(details, fmt.Sprintf("%s: .gitignore missing .land-worktree/", rigName))
		}
	}

	if len(c.affectedRigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf(".land-worktree/ gitignored in %d rig(s)", len(rigs)),
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d rig(s) missing .land-worktree/ in .gitignore", len(c.affectedRigs)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to add .land-worktree/ to .gitignore",
	}
}

// Fix adds .land-worktree/ to .gitignore in all affected rigs.
func (c *LandWorktreeGitignoreCheck) Fix(ctx *CheckContext) error {
	for _, rigPath := range c.affectedRigs {
		gitignorePath := filepath.Join(rigPath, ".gitignore")
		if err := appendGitignoreEntry(gitignorePath, ".land-worktree/"); err != nil {
			return fmt.Errorf("fixing %s: %w", gitignorePath, err)
		}
	}
	c.affectedRigs = nil
	return nil
}

// hasGitignoreEntry checks if a .gitignore file contains the given entry.
func hasGitignoreEntry(gitignorePath, entry string) bool {
	file, err := os.Open(gitignorePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == entry {
			return true
		}
	}
	return false
}

// appendGitignoreEntry adds an entry to a .gitignore file, creating it if needed.
func appendGitignoreEntry(gitignorePath, entry string) error {
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: .gitignore should be readable
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry + "\n")
	return err
}
