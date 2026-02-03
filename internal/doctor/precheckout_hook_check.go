package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BranchProtectionCheck verifies that the post-checkout hook includes branch
// protection to auto-revert accidental branch switches in the town root.
//
// NOTE: Git does NOT support "pre-checkout" hooks. We use post-checkout to
// detect and auto-revert bad checkouts immediately after they happen.
type BranchProtectionCheck struct {
	FixableCheck
	needsUpdate bool // Cached during Run for use in Fix
}

// NewBranchProtectionCheck creates a new branch protection check.
func NewBranchProtectionCheck() *BranchProtectionCheck {
	return &BranchProtectionCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "branch-protection",
				CheckDescription: "Verify post-checkout hook protects town root branch",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Backwards compatibility alias
func NewPreCheckoutHookCheck() *BranchProtectionCheck {
	return NewBranchProtectionCheck()
}

// branchProtectionMarker identifies our branch protection code in post-checkout.
const branchProtectionMarker = "Gas Town branch protection"

// branchProtectionScript is the code to prepend to post-checkout hook.
// It auto-reverts to main if a non-main branch was checked out in the town root.
const branchProtectionScript = `# Gas Town branch protection
# Auto-reverts to main if a non-main branch is checked out in the town root.
# The town root must stay on main to avoid breaking gt commands.
# NOTE: Git does NOT support pre-checkout hooks, so we auto-revert after.

# Only check branch checkouts (not file checkouts)
# $3 is 1 for branch checkout, 0 for file checkout
if [ "$3" = "1" ]; then
    # Get current branch after checkout
    CURRENT_BRANCH=$(git branch --show-current 2>/dev/null)

    # If on main or master, all good
    if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
        : # OK, continue with rest of hook
    elif [ -n "$CURRENT_BRANCH" ]; then
        # Non-main branch detected - auto-revert!
        echo "" >&2
        echo "⚠️  AUTO-REVERTING: Town root must stay on main branch" >&2
        echo "" >&2
        echo "   Detected checkout to '$CURRENT_BRANCH' in the Gas Town HQ directory." >&2
        echo "   The town root should always be on main. Switching back..." >&2
        echo "" >&2

        # Revert to main
        if git checkout main >/dev/null 2>&1; then
            echo "   ✓ Reverted to main branch" >&2
        elif git checkout master >/dev/null 2>&1; then
            echo "   ✓ Reverted to master branch" >&2
        else
            echo "   ✗ Failed to revert - please run: git checkout main" >&2
        fi
        echo "" >&2
    fi
fi

`

// Run checks if branch protection is in the post-checkout hook.
func (c *BranchProtectionCheck) Run(ctx *CheckContext) *CheckResult {
	gitDir := filepath.Join(ctx.TownRoot, ".git")

	// Check if town root is a git repo
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Town root is not a git repository (skipped)",
		}
	}

	// Also check for and warn about the useless pre-checkout hook
	preCheckoutPath := filepath.Join(gitDir, "hooks", "pre-checkout")
	if content, err := os.ReadFile(preCheckoutPath); err == nil {
		if strings.Contains(string(content), "Gas Town pre-checkout hook") {
			// Old useless hook exists - needs migration
			c.needsUpdate = true
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusWarning,
				Message: "Obsolete pre-checkout hook found (git doesn't support pre-checkout)",
				Details: []string{
					"The pre-checkout hook was installed but git doesn't support it",
					"Branch protection needs to be in post-checkout instead",
				},
				FixHint: "Run 'gt doctor --fix' to migrate to post-checkout",
			}
		}
	}

	hookPath := filepath.Join(gitDir, "hooks", "post-checkout")

	// Check if post-checkout hook exists
	content, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		c.needsUpdate = true
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Post-checkout hook not installed",
			Details: []string{
				"Branch protection prevents accidental branch switches in the town root",
				"Without it, a git checkout in ~/gt could switch to a polecat branch",
				"This can break gt commands (missing rigs.json, wrong configs)",
			},
			FixHint: "Run 'gt doctor --fix' to install branch protection",
		}
	}

	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Failed to read post-checkout hook: %v", err),
		}
	}

	// Check if branch protection is present
	if !strings.Contains(string(content), branchProtectionMarker) {
		c.needsUpdate = true
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Post-checkout hook missing branch protection",
			Details: []string{
				"Post-checkout hook exists but doesn't include branch protection",
				"Branch protection auto-reverts if non-main branch is checked out",
			},
			FixHint: "Run 'gt doctor --fix' to add branch protection",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Branch protection installed in post-checkout hook",
	}
}

// Fix adds branch protection to the post-checkout hook.
func (c *BranchProtectionCheck) Fix(ctx *CheckContext) error {
	if !c.needsUpdate {
		return nil
	}

	hooksDir := filepath.Join(ctx.TownRoot, ".git", "hooks")

	// Ensure hooks directory exists
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	// Remove obsolete pre-checkout hook if it's ours
	preCheckoutPath := filepath.Join(hooksDir, "pre-checkout")
	if content, err := os.ReadFile(preCheckoutPath); err == nil {
		if strings.Contains(string(content), "Gas Town pre-checkout hook") {
			_ = os.Remove(preCheckoutPath) // Best effort removal
		}
	}

	hookPath := filepath.Join(hooksDir, "post-checkout")

	// Read existing hook content (if any)
	existingContent, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading existing hook: %w", err)
	}

	var newContent string
	if len(existingContent) == 0 {
		// No existing hook - create new one with shebang
		newContent = "#!/bin/sh\n" + branchProtectionScript
	} else if strings.Contains(string(existingContent), branchProtectionMarker) {
		// Already has branch protection
		return nil
	} else {
		// Prepend branch protection after shebang
		content := string(existingContent)
		if strings.HasPrefix(content, "#!") {
			// Find end of shebang line
			idx := strings.Index(content, "\n")
			if idx != -1 {
				newContent = content[:idx+1] + branchProtectionScript + content[idx+1:]
			} else {
				newContent = content + "\n" + branchProtectionScript
			}
		} else {
			newContent = "#!/bin/sh\n" + branchProtectionScript + content
		}
	}

	// Write the hook
	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("writing hook: %w", err)
	}

	return nil
}

// Legacy type alias for backwards compatibility
type PreCheckoutHookCheck = BranchProtectionCheck
