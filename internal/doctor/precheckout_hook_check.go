package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PreCheckoutHookCheck verifies that the pre-checkout hook is installed in the
// town root to prevent accidental branch switches.
type PreCheckoutHookCheck struct {
	FixableCheck
	hookMissing bool // Cached during Run for use in Fix
}

// NewPreCheckoutHookCheck creates a new pre-checkout hook check.
func NewPreCheckoutHookCheck() *PreCheckoutHookCheck {
	return &PreCheckoutHookCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "pre-checkout-hook",
				CheckDescription: "Verify pre-checkout hook prevents branch switches",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// PreCheckoutHookScript is the expected content marker for our hook.
const preCheckoutHookMarker = "Gas Town pre-checkout hook"

// preCheckoutHookScript is the full hook script content.
// This matches the script in cmd/gitinit.go.
const preCheckoutHookScript = `#!/bin/bash
# Gas Town pre-checkout hook
# Prevents accidental branch switches in the town root (HQ).
# The town root must stay on main to avoid breaking gt commands.

# Only check branch checkouts (not file checkouts)
# $3 is 1 for file checkout, 0 for branch checkout
if [ "$3" = "1" ]; then
    exit 0
fi

# Get the target branch name
TARGET_BRANCH=$(git rev-parse --abbrev-ref "$2" 2>/dev/null)

# Allow checkout to main or master
if [ "$TARGET_BRANCH" = "main" ] || [ "$TARGET_BRANCH" = "master" ]; then
    exit 0
fi

# Get current branch
CURRENT_BRANCH=$(git branch --show-current)

# If already not on main, allow (might be fixing the situation)
if [ "$CURRENT_BRANCH" != "main" ] && [ "$CURRENT_BRANCH" != "master" ]; then
    exit 0
fi

# Block the checkout with a warning
echo ""
echo "⚠️  BLOCKED: Town root must stay on main branch"
echo ""
echo "   You're trying to switch from '$CURRENT_BRANCH' to '$TARGET_BRANCH'"
echo "   in the Gas Town HQ directory."
echo ""
echo "   The town root (~/gt) should always be on main. Switching branches"
echo "   can break gt commands (missing rigs.json, wrong configs, etc.)."
echo ""
echo "   If you really need to switch branches, you can:"
echo "   1. Temporarily rename .git/hooks/pre-checkout"
echo "   2. Do your work"
echo "   3. Switch back to main"
echo "   4. Restore the hook"
echo ""
exit 1
`

// Run checks if the pre-checkout hook is installed.
func (c *PreCheckoutHookCheck) Run(ctx *CheckContext) *CheckResult {
	gitDir := filepath.Join(ctx.TownRoot, ".git")

	// Check if town root is a git repo
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Town root is not a git repository (skipped)",
		}
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-checkout")

	// Check if hook exists
	content, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		c.hookMissing = true
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Pre-checkout hook not installed",
			Details: []string{
				"The pre-checkout hook prevents accidental branch switches in the town root",
				"Without it, a git checkout in ~/gt could switch to a polecat branch",
				"This can break gt commands (missing rigs.json, wrong configs)",
			},
			FixHint: "Run 'gt doctor --fix' or 'gt git-init' to install the hook",
		}
	}

	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Failed to read pre-checkout hook: %v", err),
		}
	}

	// Check if it's our hook
	if !strings.Contains(string(content), preCheckoutHookMarker) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Pre-checkout hook exists but is not Gas Town's",
			Details: []string{
				"A pre-checkout hook exists but doesn't contain the Gas Town marker",
				"Consider adding branch protection manually or replacing it",
			},
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Pre-checkout hook installed",
	}
}

// Fix installs the pre-checkout hook.
func (c *PreCheckoutHookCheck) Fix(ctx *CheckContext) error {
	if !c.hookMissing {
		return nil
	}

	hooksDir := filepath.Join(ctx.TownRoot, ".git", "hooks")

	// Ensure hooks directory exists
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-checkout")

	// Install the hook
	if err := os.WriteFile(hookPath, []byte(preCheckoutHookScript), 0755); err != nil {
		return fmt.Errorf("writing hook: %w", err)
	}

	return nil
}
