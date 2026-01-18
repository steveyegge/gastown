package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// StaleBeadsRedirectCheck detects .beads directories that have both a redirect
// file AND stale data files. This can happen when:
// - A rig is added from a repo that already has .beads/ tracked in git
// - Crew workspaces are cloned from repos with existing .beads/ files
// - SetupRedirect failed or was run before cleanup logic was added
//
// Additionally, this check verifies redirect topology:
// - Worktrees (crew, polecats, refinery) should have redirects
// - Redirects should point to the correct canonical location
// - Redirect targets should exist
//
// When both redirect and data files exist, bd commands may use stale data
// instead of following the redirect.
type StaleBeadsRedirectCheck struct {
	FixableCheck
	staleLocations   []string        // Cached for Fix - dirs with stale files
	missingRedirects []redirectIssue // Cached for Fix - worktrees missing redirects
	incorrectRedirects []redirectIssue // Cached for Fix - worktrees with wrong redirect target
}

// redirectIssue represents a missing or incorrect redirect.
type redirectIssue struct {
	worktreePath   string // Full path to the worktree (e.g., <rig>/crew/max)
	townRoot       string // Town root for SetupRedirect
	currentTarget  string // Current redirect target (empty if missing)
	expectedTarget string // Expected redirect target
}

// NewStaleBeadsRedirectCheck creates a new stale beads redirect check.
func NewStaleBeadsRedirectCheck() *StaleBeadsRedirectCheck {
	return &StaleBeadsRedirectCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-beads-redirect",
				CheckDescription: "Check for stale files in .beads directories with redirects",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// staleFilePatterns are runtime files that should NOT exist alongside a redirect.
// These are gitignored runtime files that would conflict with redirected data.
// Note: config.yaml is NOT included because it may be tracked in git.
var staleFilePatterns = []string{
	// SQLite databases
	"*.db",
	"*.db-*",
	"*.db?*",
	// JSONL data files (tracked but stale in redirect locations)
	"issues.jsonl",
	"interactions.jsonl",
	// Sync and metadata
	"metadata.json",
	"sync-state.json",
	"last-touched",
	".local_version",
	// Daemon runtime files
	"daemon.lock",
	"daemon.log",
	"daemon.pid",
	"bd.sock",
}

// Run checks for stale files in .beads directories that have redirects,
// and verifies redirect topology for all worktrees.
func (c *StaleBeadsRedirectCheck) Run(ctx *CheckContext) *CheckResult {
	var staleLocations []string
	var missingRedirects []redirectIssue
	var incorrectRedirects []redirectIssue

	// Get list of rigs to scan
	rigDirs, err := findRigDirs(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not scan rigs: %v", err),
		}
	}

	// For each rig, check all potential .beads locations
	for _, rigDir := range rigDirs {
		locations := getBeadsDirsToCheck(rigDir)
		for _, beadsDir := range locations {
			if hasRedirectWithStaleFiles(beadsDir) {
				// Make path relative to town root for readability
				relPath, _ := filepath.Rel(ctx.TownRoot, beadsDir)
				if relPath == "" {
					relPath = beadsDir
				}
				staleLocations = append(staleLocations, relPath)
			}
		}

		// Verify redirect topology for this rig
		missing, incorrect := c.verifyRedirectTopology(ctx.TownRoot, rigDir)
		missingRedirects = append(missingRedirects, missing...)
		incorrectRedirects = append(incorrectRedirects, incorrect...)
	}

	// Cache for Fix
	c.staleLocations = staleLocations
	c.missingRedirects = missingRedirects
	c.incorrectRedirects = incorrectRedirects

	// Build result
	var details []string
	totalIssues := len(staleLocations) + len(missingRedirects) + len(incorrectRedirects)

	if totalIssues == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale beads files or redirect issues found",
		}
	}

	// Add details for each issue type
	for _, loc := range staleLocations {
		details = append(details, fmt.Sprintf("stale files: %s", loc))
	}
	for _, issue := range missingRedirects {
		relPath, _ := filepath.Rel(ctx.TownRoot, issue.worktreePath)
		details = append(details, fmt.Sprintf("missing redirect: %s", relPath))
	}
	for _, issue := range incorrectRedirects {
		relPath, _ := filepath.Rel(ctx.TownRoot, issue.worktreePath)
		details = append(details, fmt.Sprintf("incorrect redirect: %s (has %q, expected %q)",
			relPath, issue.currentTarget, issue.expectedTarget))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d beads redirect issue(s) found", totalIssues),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to repair redirects and remove stale files",
	}
}

// Fix removes stale files from .beads directories that have redirects,
// and creates/repairs missing or incorrect redirects.
func (c *StaleBeadsRedirectCheck) Fix(ctx *CheckContext) error {
	// Remove stale files
	for _, relPath := range c.staleLocations {
		beadsDir := filepath.Join(ctx.TownRoot, relPath)
		if err := cleanStaleBeadsFiles(beadsDir); err != nil {
			return fmt.Errorf("cleaning %s: %w", relPath, err)
		}
	}

	// Create missing redirects
	for _, issue := range c.missingRedirects {
		if err := beads.SetupRedirect(issue.townRoot, issue.worktreePath); err != nil {
			relPath, _ := filepath.Rel(ctx.TownRoot, issue.worktreePath)
			return fmt.Errorf("creating redirect for %s: %w", relPath, err)
		}
	}

	// Fix incorrect redirects (same as creating - SetupRedirect overwrites)
	for _, issue := range c.incorrectRedirects {
		if err := beads.SetupRedirect(issue.townRoot, issue.worktreePath); err != nil {
			relPath, _ := filepath.Rel(ctx.TownRoot, issue.worktreePath)
			return fmt.Errorf("fixing redirect for %s: %w", relPath, err)
		}
	}

	return nil
}

// findRigDirs returns all rig directories in the town.
func findRigDirs(townRoot string) ([]string, error) {
	var rigs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip hidden dirs, mayor, docs
		if strings.HasPrefix(name, ".") || name == "mayor" || name == "docs" {
			continue
		}

		rigPath := filepath.Join(townRoot, name)

		// A rig should have at least a .git directory (be a git repo)
		// or have a mayor/rig subdirectory
		if isLikelyRig(rigPath) {
			rigs = append(rigs, rigPath)
		}
	}

	return rigs, nil
}

// isLikelyRig checks if a directory looks like a rig.
func isLikelyRig(path string) bool {
	// Check for .git (it's a git repo)
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	// Check for mayor/rig (has the standard rig structure)
	if _, err := os.Stat(filepath.Join(path, "mayor", "rig")); err == nil {
		return true
	}
	// Check for .beads directory (has beads configured)
	if _, err := os.Stat(filepath.Join(path, ".beads")); err == nil {
		return true
	}
	return false
}

// getBeadsDirsToCheck returns all .beads directories to check for a rig.
func getBeadsDirsToCheck(rigDir string) []string {
	var dirs []string

	// Rig root .beads
	rigBeads := filepath.Join(rigDir, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		dirs = append(dirs, rigBeads)
	}

	// Crew .beads directories: <rig>/crew/*/.beads
	crewDir := filepath.Join(rigDir, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				beadsDir := filepath.Join(crewDir, entry.Name(), ".beads")
				if _, err := os.Stat(beadsDir); err == nil {
					dirs = append(dirs, beadsDir)
				}
			}
		}
	}

	// Refinery .beads: <rig>/refinery/rig/.beads
	refineryBeads := filepath.Join(rigDir, "refinery", "rig", ".beads")
	if _, err := os.Stat(refineryBeads); err == nil {
		dirs = append(dirs, refineryBeads)
	}

	// Polecats .beads directories: <rig>/polecats/*/.beads
	polecatsDir := filepath.Join(rigDir, "polecats")
	if entries, err := os.ReadDir(polecatsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				beadsDir := filepath.Join(polecatsDir, entry.Name(), ".beads")
				if _, err := os.Stat(beadsDir); err == nil {
					dirs = append(dirs, beadsDir)
				}
			}
		}
	}

	return dirs
}

// hasRedirectWithStaleFiles checks if a .beads directory has both a redirect
// file and stale data files.
func hasRedirectWithStaleFiles(beadsDir string) bool {
	// Must have redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
		return false
	}

	// Check for any stale files
	for _, pattern := range staleFilePatterns {
		matches, err := filepath.Glob(filepath.Join(beadsDir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			return true
		}
	}

	return false
}

// cleanStaleBeadsFiles removes stale files from a .beads directory,
// preserving the redirect file and .gitignore.
func cleanStaleBeadsFiles(beadsDir string) error {
	// Verify redirect exists before cleaning
	redirectPath := filepath.Join(beadsDir, "redirect")
	if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
		return fmt.Errorf("no redirect file found - refusing to clean")
	}

	// Remove files matching stale patterns
	for _, pattern := range staleFilePatterns {
		matches, err := filepath.Glob(filepath.Join(beadsDir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			if err := os.RemoveAll(match); err != nil {
				return fmt.Errorf("removing %s: %w", filepath.Base(match), err)
			}
		}
	}

	// Also remove mq directory if it exists
	mqDir := filepath.Join(beadsDir, "mq")
	if _, err := os.Stat(mqDir); err == nil {
		if err := os.RemoveAll(mqDir); err != nil {
			return fmt.Errorf("removing mq: %w", err)
		}
	}

	return nil
}

// verifyRedirectTopology checks that all worktrees in a rig have correct redirects.
// Returns lists of missing and incorrect redirect issues.
func (c *StaleBeadsRedirectCheck) verifyRedirectTopology(townRoot, rigDir string) (missing, incorrect []redirectIssue) {
	// Determine architecture: check if rig has local beads or uses tracked beads (mayor/rig)
	rigBeadsPath := filepath.Join(rigDir, ".beads")
	mayorBeadsPath := filepath.Join(rigDir, "mayor", "rig", ".beads")

	// Determine if this rig uses tracked beads (mayor/rig/.beads is canonical)
	usesTrackedBeads := isCanonicalBeadsLocation(mayorBeadsPath)

	// If neither location has beads, skip this rig (not configured)
	if !beadsDirExists(rigBeadsPath) && !beadsDirExists(mayorBeadsPath) {
		return nil, nil
	}

	// Get all worktrees that should have redirects
	worktrees := getWorktreePaths(rigDir)

	for _, worktreePath := range worktrees {
		// Skip if worktree doesn't exist
		if !beadsDirExists(worktreePath) {
			continue
		}

		expected := expectedRedirectTarget(rigDir, worktreePath, usesTrackedBeads)
		actual := readRedirectTarget(worktreePath)

		if actual == "" {
			// Missing redirect
			missing = append(missing, redirectIssue{
				worktreePath:   worktreePath,
				townRoot:       townRoot,
				expectedTarget: expected,
			})
		} else if normalizeRedirectPath(actual) != normalizeRedirectPath(expected) {
			// Incorrect redirect - but verify it's actually wrong
			// The redirect might point to a valid intermediate location
			if !isValidRedirect(worktreePath, actual, rigDir, usesTrackedBeads) {
				incorrect = append(incorrect, redirectIssue{
					worktreePath:   worktreePath,
					townRoot:       townRoot,
					currentTarget:  actual,
					expectedTarget: expected,
				})
			}
		}
	}

	return missing, incorrect
}

// isCanonicalBeadsLocation checks if a .beads directory is a canonical location
// (has actual beads data, not just a redirect).
func isCanonicalBeadsLocation(beadsDir string) bool {
	if !beadsDirExists(beadsDir) {
		return false
	}
	// Canonical location should NOT have a redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	_, err := os.Stat(redirectPath)
	return os.IsNotExist(err)
}

// beadsDirExists checks if a directory exists.
func beadsDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// getWorktreePaths returns all worktree paths that should have redirects.
func getWorktreePaths(rigDir string) []string {
	var paths []string

	// Crew workspaces: <rig>/crew/*
	crewDir := filepath.Join(rigDir, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			// Skip hidden directories (like .claude)
			if entry.IsDir() && !strings.HasPrefix(name, ".") {
				paths = append(paths, filepath.Join(crewDir, name))
			}
		}
	}

	// Polecats: <rig>/polecats/*
	polecatsDir := filepath.Join(rigDir, "polecats")
	if entries, err := os.ReadDir(polecatsDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			// Skip hidden directories
			if entry.IsDir() && !strings.HasPrefix(name, ".") {
				paths = append(paths, filepath.Join(polecatsDir, name))
			}
		}
	}

	// Refinery: <rig>/refinery/rig
	refineryPath := filepath.Join(rigDir, "refinery", "rig")
	if beadsDirExists(refineryPath) {
		paths = append(paths, refineryPath)
	}

	return paths
}

// expectedRedirectTarget computes the expected redirect target for a worktree.
func expectedRedirectTarget(rigDir, worktreePath string, usesTrackedBeads bool) string {
	// Compute depth from worktree to rig root
	relPath, err := filepath.Rel(rigDir, worktreePath)
	if err != nil {
		return ""
	}

	// Count path components
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	depth := len(parts)

	upPath := strings.Repeat("../", depth)

	if usesTrackedBeads {
		// Direct path to mayor/rig/.beads (bypass any rig-level redirect)
		return upPath + "mayor/rig/.beads"
	}

	// Check if rig-level beads has a redirect (tracked beads case)
	rigBeadsPath := filepath.Join(rigDir, ".beads")
	rigRedirectPath := filepath.Join(rigBeadsPath, "redirect")
	if data, err := os.ReadFile(rigRedirectPath); err == nil {
		rigRedirectTarget := strings.TrimSpace(string(data))
		if rigRedirectTarget != "" {
			// Rig has redirect - worktree should redirect directly to final destination
			return upPath + rigRedirectTarget
		}
	}

	// Local beads - redirect to rig's .beads
	return upPath + ".beads"
}

// readRedirectTarget reads the redirect target from a worktree's .beads/redirect file.
// Returns empty string if no redirect exists.
func readRedirectTarget(worktreePath string) string {
	redirectPath := filepath.Join(worktreePath, ".beads", "redirect")
	data, err := os.ReadFile(redirectPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// normalizeRedirectPath normalizes a redirect path for comparison.
func normalizeRedirectPath(path string) string {
	// Remove trailing newlines/spaces and clean the path
	path = strings.TrimSpace(path)
	// Normalize slashes
	path = filepath.ToSlash(path)
	// Remove trailing slash
	path = strings.TrimSuffix(path, "/")
	return path
}

// isValidRedirect checks if a redirect is functionally valid even if not the "expected" path.
// This handles cases like redirecting to ../../.beads when rig/.beads itself has a redirect.
func isValidRedirect(worktreePath, redirectTarget, rigDir string, usesTrackedBeads bool) bool {
	// Resolve the redirect to an absolute path
	resolved := filepath.Clean(filepath.Join(worktreePath, redirectTarget))

	// Check if it resolves to a valid beads location
	if usesTrackedBeads {
		expectedCanonical := filepath.Join(rigDir, "mayor", "rig", ".beads")
		// Follow any chain to see if we eventually get to the canonical location
		finalDest := beads.ResolveBeadsDir(filepath.Dir(resolved))
		return filepath.Clean(finalDest) == filepath.Clean(expectedCanonical)
	}

	// For local beads, should resolve to rig's .beads
	expectedRigBeads := filepath.Join(rigDir, ".beads")
	return filepath.Clean(resolved) == filepath.Clean(expectedRigBeads)
}
