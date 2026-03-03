package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TestutilSymlinkCheck verifies that crew and refinery/rig internal/testutil/
// directories are symlinks to the canonical mayor/rig/internal/testutil/.
// This prevents identical-copies drift across rig clones.
type TestutilSymlinkCheck struct {
	FixableCheck
	issues []symlinkIssue
}

type symlinkIssue struct {
	dir     string // directory containing internal/testutil
	path    string // full path to the testutil dir/symlink
	problem string // description of the issue
}

// NewTestutilSymlinkCheck creates a new testutil symlink check.
func NewTestutilSymlinkCheck() *TestutilSymlinkCheck {
	return &TestutilSymlinkCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "testutil-symlink",
				CheckDescription: "Verify testutil dirs are symlinks to mayor/rig canonical copy",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// canonicalTestutilPath returns the path to the canonical testutil directory.
func canonicalTestutilPath(rigPath string) string {
	return filepath.Join(rigPath, "mayor", "rig", "internal", "testutil")
}

// Run checks if crew and refinery/rig internal/testutil are proper symlinks.
func (c *TestutilSymlinkCheck) Run(ctx *CheckContext) *CheckResult {
	rigPath := ctx.RigPath()
	if rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	// Verify canonical copy exists
	canonical := canonicalTestutilPath(rigPath)
	if _, err := os.Stat(canonical); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No mayor/rig/internal/testutil/ found (canonical source missing)",
			FixHint: "Ensure mayor/rig clone is set up with internal/testutil/",
		}
	}

	canonicalResolved, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot resolve canonical testutil path: %v", err),
		}
	}

	c.issues = nil
	var checked int

	// Check crew workers: crew/<name>/internal/testutil
	crewDir := filepath.Join(rigPath, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			testutilPath := filepath.Join(crewDir, entry.Name(), "internal", "testutil")
			c.checkSymlink(testutilPath, canonicalResolved, fmt.Sprintf("crew/%s", entry.Name()))
			checked++
		}
	}

	// Check refinery/rig/internal/testutil
	refineryTestutil := filepath.Join(rigPath, "refinery", "rig", "internal", "testutil")
	if _, err := os.Stat(filepath.Join(rigPath, "refinery", "rig")); err == nil {
		c.checkSymlink(refineryTestutil, canonicalResolved, "refinery/rig")
		checked++
	}

	if checked == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No crew or refinery clones to check",
		}
	}

	if len(c.issues) > 0 {
		details := make([]string, len(c.issues))
		for i, issue := range c.issues {
			details[i] = fmt.Sprintf("%s: %s", issue.dir, issue.problem)
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d testutil dir(s) not symlinked to canonical copy", len(c.issues)),
			Details: details,
			FixHint: "Run 'gt doctor --fix --rig <rig>' to replace with symlinks",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d testutil symlink(s) verified", checked),
	}
}

// checkSymlink verifies a single testutil path is a proper symlink to the canonical copy.
func (c *TestutilSymlinkCheck) checkSymlink(testutilPath, canonicalResolved, label string) {
	info, err := os.Lstat(testutilPath)
	if os.IsNotExist(err) {
		// No testutil dir at all — might not have internal/ yet
		return
	}
	if err != nil {
		c.issues = append(c.issues, symlinkIssue{
			dir:     label,
			path:    testutilPath,
			problem: fmt.Sprintf("cannot stat: %v", err),
		})
		return
	}

	if info.Mode()&os.ModeSymlink == 0 {
		// Not a symlink — it's a real directory (the drift problem)
		c.issues = append(c.issues, symlinkIssue{
			dir:     label,
			path:    testutilPath,
			problem: "real directory (should be symlink to mayor/rig canonical copy)",
		})
		return
	}

	// It is a symlink — verify it resolves
	target, err := os.Readlink(testutilPath)
	if err != nil {
		c.issues = append(c.issues, symlinkIssue{
			dir:     label,
			path:    testutilPath,
			problem: fmt.Sprintf("cannot read symlink: %v", err),
		})
		return
	}

	// Resolve the symlink target to absolute path for comparison
	resolvedTarget := target
	if !filepath.IsAbs(target) {
		resolvedTarget = filepath.Join(filepath.Dir(testutilPath), target)
	}
	resolvedTarget, err = filepath.EvalSymlinks(resolvedTarget)
	if err != nil {
		c.issues = append(c.issues, symlinkIssue{
			dir:     label,
			path:    testutilPath,
			problem: fmt.Sprintf("symlink target does not resolve: %s", target),
		})
		return
	}

	// Verify it points to the canonical copy
	if resolvedTarget != canonicalResolved {
		c.issues = append(c.issues, symlinkIssue{
			dir:     label,
			path:    testutilPath,
			problem: fmt.Sprintf("symlink points to %s (not canonical copy)", target),
		})
	}
}

// Fix replaces real testutil directories with symlinks to the canonical copy.
func (c *TestutilSymlinkCheck) Fix(ctx *CheckContext) error {
	rigPath := ctx.RigPath()
	canonical := canonicalTestutilPath(rigPath)

	// Verify canonical still exists
	if _, err := os.Stat(canonical); err != nil {
		return fmt.Errorf("canonical testutil not found at %s: %w", canonical, err)
	}

	for _, issue := range c.issues {
		// Compute relative symlink target from the symlink's parent to the canonical dir
		symlinkParent := filepath.Dir(issue.path)
		relTarget, err := filepath.Rel(symlinkParent, canonical)
		if err != nil {
			return fmt.Errorf("cannot compute relative path for %s: %w", issue.dir, err)
		}

		// Remove existing dir/symlink
		if err := os.RemoveAll(issue.path); err != nil {
			return fmt.Errorf("cannot remove %s: %w", issue.path, err)
		}

		// Create symlink
		if err := os.Symlink(relTarget, issue.path); err != nil {
			return fmt.Errorf("cannot create symlink at %s: %w", issue.path, err)
		}
	}

	return nil
}
