package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BranchCheck detects persistent roles (crew, witness, refinery) that are
// not on the main branch. Long-lived roles should work directly on main
// to avoid orphaned work and branch decay.
type BranchCheck struct {
	FixableCheck
	offMainDirs []string // Cached during Run for use in Fix
}

// NewBranchCheck creates a new branch check.
func NewBranchCheck() *BranchCheck {
	return &BranchCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "persistent-role-branches",
				CheckDescription: "Detect persistent roles not on main branch",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks if persistent role directories are on main branch.
func (c *BranchCheck) Run(ctx *CheckContext) *CheckResult {
	var offMain []string
	var onMain int

	// Find all persistent role directories
	dirs := c.findPersistentRoleDirs(ctx.TownRoot)

	for _, dir := range dirs {
		branch, err := c.getCurrentBranch(dir)
		if err != nil {
			// Skip directories that aren't git repos
			continue
		}

		if branch == "main" || branch == "master" {
			onMain++
		} else {
			offMain = append(offMain, fmt.Sprintf("%s (on %s)", c.relativePath(ctx.TownRoot, dir), branch))
		}
	}

	// Cache for Fix
	c.offMainDirs = nil
	for _, dir := range dirs {
		branch, err := c.getCurrentBranch(dir)
		if err != nil {
			continue
		}
		if branch != "main" && branch != "master" {
			c.offMainDirs = append(c.offMainDirs, dir)
		}
	}

	if len(offMain) == 0 {
		if onMain == 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "No persistent role directories found",
			}
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d persistent roles on main branch", onMain),
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d persistent role(s) not on main branch", len(offMain)),
		Details: offMain,
		FixHint: "Run 'gt doctor --fix' to switch to main, or manually: git checkout main && git pull",
	}
}

// Fix switches all off-main directories to main branch.
func (c *BranchCheck) Fix(ctx *CheckContext) error {
	if len(c.offMainDirs) == 0 {
		return nil
	}

	var lastErr error
	for _, dir := range c.offMainDirs {
		// git checkout main
		cmd := exec.Command("git", "checkout", "main")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("%s: %w", dir, err)
			continue
		}

		// git pull --rebase
		cmd = exec.Command("git", "pull", "--rebase")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			// Pull failure is not fatal, just warn
			continue
		}
	}

	return lastErr
}

// findPersistentRoleDirs finds all directories that should be on main:
// - <rig>/crew/*
// - <rig>/witness/rig (if exists)
// - <rig>/refinery/rig (if exists)
func (c *BranchCheck) findPersistentRoleDirs(townRoot string) []string {
	var dirs []string

	// Find all rigs
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return dirs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip non-rig directories
		name := entry.Name()
		if name == "mayor" || name == ".beads" || strings.HasPrefix(name, ".") {
			continue
		}

		rigPath := filepath.Join(townRoot, name)

		// Check if this looks like a rig (has crew/, polecats/, witness/, or refinery/)
		if !c.isRig(rigPath) {
			continue
		}

		// Add crew members
		crewPath := filepath.Join(rigPath, "crew")
		if crewEntries, err := os.ReadDir(crewPath); err == nil {
			for _, crew := range crewEntries {
				if crew.IsDir() && !strings.HasPrefix(crew.Name(), ".") {
					dirs = append(dirs, filepath.Join(crewPath, crew.Name()))
				}
			}
		}

		// Add witness/rig if exists
		witnessRig := filepath.Join(rigPath, "witness", "rig")
		if _, err := os.Stat(witnessRig); err == nil {
			dirs = append(dirs, witnessRig)
		}

		// Add refinery/rig if exists
		refineryRig := filepath.Join(rigPath, "refinery", "rig")
		if _, err := os.Stat(refineryRig); err == nil {
			dirs = append(dirs, refineryRig)
		}
	}

	return dirs
}

// isRig checks if a directory looks like a rig.
func (c *BranchCheck) isRig(path string) bool {
	markers := []string{"crew", "polecats", "witness", "refinery"}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

// getCurrentBranch returns the current git branch for a directory.
func (c *BranchCheck) getCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// relativePath returns path relative to base, or the full path if that fails.
func (c *BranchCheck) relativePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// BeadsSyncOrphanCheck detects code changes on beads-sync branch that weren't
// merged to main. This catches cases where merges lose code changes.
type BeadsSyncOrphanCheck struct {
	BaseCheck
}

// NewBeadsSyncOrphanCheck creates a new beads-sync orphan check.
func NewBeadsSyncOrphanCheck() *BeadsSyncOrphanCheck {
	return &BeadsSyncOrphanCheck{
		BaseCheck: BaseCheck{
			CheckName:        "beads-sync-orphans",
			CheckDescription: "Detect orphaned code on beads-sync branch",
			CheckCategory:    CategoryCleanup,
		},
	}
}

// Run checks for code differences between main and beads-sync.
func (c *BeadsSyncOrphanCheck) Run(ctx *CheckContext) *CheckResult {
	// Find the first rig with a crew member (that has beads-sync branch)
	crewDirs := c.findCrewDirs(ctx.TownRoot)
	if len(crewDirs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No crew directories found",
		}
	}

	// Use first crew dir to check beads-sync
	crewDir := crewDirs[0]

	// Check if beads-sync branch exists
	cmd := exec.Command("git", "rev-parse", "--verify", "beads-sync")
	cmd.Dir = crewDir
	if err := cmd.Run(); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No beads-sync branch (single-clone setup)",
		}
	}

	// Get diff between main and beads-sync, excluding .beads/
	cmd = exec.Command("git", "diff", "--name-only", "main..beads-sync", "--", ".", ":(exclude).beads")
	cmd.Dir = crewDir
	out, err := cmd.Output()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not diff main..beads-sync",
			Details: []string{err.Error()},
		}
	}

	files := strings.TrimSpace(string(out))
	if files == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No orphaned code on beads-sync",
		}
	}

	// Filter to code files only
	var codeFiles []string
	for _, f := range strings.Split(files, "\n") {
		if f == "" {
			continue
		}
		// Check if it's a code file
		if strings.HasSuffix(f, ".go") || strings.HasSuffix(f, ".md") ||
			strings.HasSuffix(f, ".toml") || strings.HasSuffix(f, ".json") ||
			strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml") ||
			strings.HasSuffix(f, ".tmpl") {
			codeFiles = append(codeFiles, f)
		}
	}

	if len(codeFiles) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No orphaned code on beads-sync (only non-code files differ)",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d file(s) on beads-sync not in main", len(codeFiles)),
		Details: codeFiles,
		FixHint: "Review with: git diff main..beads-sync -- <file>",
	}
}

// findCrewDirs returns crew directories that might have beads-sync.
func (c *BeadsSyncOrphanCheck) findCrewDirs(townRoot string) []string {
	var dirs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return dirs
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" {
			continue
		}

		crewPath := filepath.Join(townRoot, entry.Name(), "crew")
		if crewEntries, err := os.ReadDir(crewPath); err == nil {
			for _, crew := range crewEntries {
				if crew.IsDir() && !strings.HasPrefix(crew.Name(), ".") {
					dirs = append(dirs, filepath.Join(crewPath, crew.Name()))
				}
			}
		}
	}

	return dirs
}

// CloneDivergenceCheck detects when git clones have drifted significantly apart.
// This is an emergency condition - all clones should be tracking origin/main
// and staying reasonably in sync. Divergence here is different from beads-sync
// divergence, which is expected.
type CloneDivergenceCheck struct {
	BaseCheck
}

// NewCloneDivergenceCheck creates a new clone divergence check.
func NewCloneDivergenceCheck() *CloneDivergenceCheck {
	return &CloneDivergenceCheck{
		BaseCheck: BaseCheck{
			CheckName:        "clone-divergence",
			CheckDescription: "Detect emergency divergence between git clones",
			CheckCategory:    CategoryCleanup,
		},
	}
}

// cloneInfo holds information about a single clone.
type cloneInfo struct {
	path     string
	branch   string
	headSHA  string
	behindBy int // commits behind origin/main
}

// Run checks for significant divergence between clones.
func (c *CloneDivergenceCheck) Run(ctx *CheckContext) *CheckResult {
	clones := c.findAllClones(ctx.TownRoot)
	if len(clones) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No clones found",
		}
	}

	// Gather info about each clone
	var infos []cloneInfo
	for _, path := range clones {
		info, err := c.getCloneInfo(path)
		if err != nil {
			continue // Skip problematic clones
		}
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No valid git clones found",
		}
	}

	// Check for clones significantly behind origin/main
	var warnings []string
	var errors []string

	for _, info := range infos {
		relPath := c.relativePath(ctx.TownRoot, info.path)

		// Only check clones on main branch (others are caught by BranchCheck)
		if info.branch != "main" && info.branch != "master" {
			continue
		}

		if info.behindBy > 50 {
			errors = append(errors, fmt.Sprintf("%s: %d commits behind origin/main (EMERGENCY)", relPath, info.behindBy))
		} else if info.behindBy > 10 {
			warnings = append(warnings, fmt.Sprintf("%s: %d commits behind origin/main", relPath, info.behindBy))
		}
	}

	if len(errors) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d clone(s) critically diverged", len(errors)),
			Details: append(errors, warnings...),
			FixHint: "Run 'git pull --rebase' in affected directories",
		}
	}

	if len(warnings) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d clone(s) behind origin/main", len(warnings)),
			Details: warnings,
			FixHint: "Run 'git pull --rebase' in affected directories",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("All %d clones in sync with origin/main", len(infos)),
	}
}

// findAllClones finds all git clones in the workspace.
func (c *CloneDivergenceCheck) findAllClones(townRoot string) []string {
	var clones []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return clones
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" || entry.Name() == "docs" {
			continue
		}

		rigPath := filepath.Join(townRoot, entry.Name())

		// Check standard clone locations
		locations := []string{
			"mayor/rig",
			"witness/rig",
			"refinery/rig",
		}

		for _, loc := range locations {
			path := filepath.Join(rigPath, loc)
			if c.isGitRepo(path) {
				clones = append(clones, path)
			}
		}

		// Add crew members
		crewPath := filepath.Join(rigPath, "crew")
		if crewEntries, err := os.ReadDir(crewPath); err == nil {
			for _, crew := range crewEntries {
				if crew.IsDir() && !strings.HasPrefix(crew.Name(), ".") {
					path := filepath.Join(crewPath, crew.Name())
					if c.isGitRepo(path) {
						clones = append(clones, path)
					}
				}
			}
		}

		// Add polecats (handle both new and old structures)
		// New structure: polecats/<name>/<rigname>/
		// Old structure: polecats/<name>/
		rigName := entry.Name()
		polecatsPath := filepath.Join(rigPath, "polecats")
		if polecatEntries, err := os.ReadDir(polecatsPath); err == nil {
			for _, polecat := range polecatEntries {
				if polecat.IsDir() && !strings.HasPrefix(polecat.Name(), ".") {
					// Try new structure first
					path := filepath.Join(polecatsPath, polecat.Name(), rigName)
					if !c.isGitRepo(path) {
						// Fall back to old structure
						path = filepath.Join(polecatsPath, polecat.Name())
					}
					if c.isGitRepo(path) {
						clones = append(clones, path)
					}
				}
			}
		}
	}

	return clones
}

// isGitRepo checks if a directory is a git repository.
func (c *CloneDivergenceCheck) isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	return false
}

// getCloneInfo gathers information about a clone.
func (c *CloneDivergenceCheck) getCloneInfo(path string) (cloneInfo, error) {
	info := cloneInfo{path: path}

	// Get current branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return info, err
	}
	info.branch = strings.TrimSpace(string(out))

	// Get HEAD SHA
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err = cmd.Output()
	if err != nil {
		return info, err
	}
	info.headSHA = strings.TrimSpace(string(out))

	// Count commits behind origin/main (uses existing refs, may be stale)
	cmd = exec.Command("git", "rev-list", "--count", "HEAD..origin/main")
	cmd.Dir = path
	out, err = cmd.Output()
	if err != nil {
		// origin/main might not exist, treat as 0 behind
		info.behindBy = 0
		return info, nil
	}

	var behind int
	_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &behind)
	info.behindBy = behind

	return info, nil
}

// relativePath returns path relative to base.
func (c *CloneDivergenceCheck) relativePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
