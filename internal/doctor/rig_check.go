package doctor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RigIsGitRepoCheck verifies the rig has a valid mayor/rig git clone.
// Note: The rig directory itself is not a git repo - it contains clones.
type RigIsGitRepoCheck struct {
	BaseCheck
}

// NewRigIsGitRepoCheck creates a new rig git repo check.
func NewRigIsGitRepoCheck() *RigIsGitRepoCheck {
	return &RigIsGitRepoCheck{
		BaseCheck: BaseCheck{
			CheckName:        "rig-is-git-repo",
			CheckDescription: "Verify rig has a valid mayor/rig git clone",
		},
	}
}

// Run checks if the rig has a valid mayor/rig git clone.
func (c *RigIsGitRepoCheck) Run(ctx *CheckContext) *CheckResult {
	rigPath := ctx.RigPath()
	if rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	// Check mayor/rig/ which is the authoritative clone for the rig
	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
	gitPath := filepath.Join(mayorRigPath, ".git")
	info, err := os.Stat(gitPath)
	if os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No mayor/rig clone found",
			Details: []string{fmt.Sprintf("Missing: %s", gitPath)},
			FixHint: "Clone the repository to mayor/rig/",
		}
	}
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot access mayor/rig/.git: %v", err),
		}
	}

	// Verify git status works
	cmd := exec.Command("git", "-C", mayorRigPath, "status", "--porcelain")
	if err := cmd.Run(); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "git status failed on mayor/rig",
			Details: []string{fmt.Sprintf("Error: %v", err)},
			FixHint: "Check git configuration and repository integrity",
		}
	}

	gitType := "clone"
	if info.Mode().IsRegular() {
		gitType = "worktree"
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("Valid mayor/rig %s", gitType),
	}
}

// GitExcludeConfiguredCheck verifies .git/info/exclude has Gas Town directories.
type GitExcludeConfiguredCheck struct {
	FixableCheck
	missingEntries []string
	excludePath    string
}

// NewGitExcludeConfiguredCheck creates a new git exclude check.
func NewGitExcludeConfiguredCheck() *GitExcludeConfiguredCheck {
	return &GitExcludeConfiguredCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "git-exclude-configured",
				CheckDescription: "Check .git/info/exclude has Gas Town directories",
			},
		},
	}
}

// requiredExcludes returns the directories that should be excluded.
func (c *GitExcludeConfiguredCheck) requiredExcludes() []string {
	return []string{"polecats/", "witness/", "refinery/", "mayor/"}
}

// Run checks if .git/info/exclude contains required entries.
func (c *GitExcludeConfiguredCheck) Run(ctx *CheckContext) *CheckResult {
	rigPath := ctx.RigPath()
	if rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	// Check mayor/rig/ which is the authoritative clone
	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
	gitDir := filepath.Join(mayorRigPath, ".git")
	info, err := os.Stat(gitDir)
	if os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No mayor/rig clone found",
			FixHint: "Run rig-is-git-repo check first",
		}
	}

	// If .git is a file (worktree), read the actual git dir
	if info.Mode().IsRegular() {
		content, err := os.ReadFile(gitDir)
		if err != nil {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusError,
				Message: fmt.Sprintf("Cannot read .git file: %v", err),
			}
		}
		// Format: "gitdir: /path/to/actual/git/dir"
		line := strings.TrimSpace(string(content))
		if strings.HasPrefix(line, "gitdir: ") {
			gitDir = strings.TrimPrefix(line, "gitdir: ")
			// Resolve relative paths
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(rigPath, gitDir)
			}
		}
	}

	c.excludePath = filepath.Join(gitDir, "info", "exclude")

	// Read existing excludes
	existing := make(map[string]bool)
	if file, err := os.Open(c.excludePath); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				existing[line] = true
			}
		}
		_ = file.Close() //nolint:gosec // G104: best-effort close
	}

	// Check for missing entries
	c.missingEntries = nil
	for _, required := range c.requiredExcludes() {
		if !existing[required] {
			c.missingEntries = append(c.missingEntries, required)
		}
	}

	if len(c.missingEntries) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Git exclude properly configured",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d Gas Town directories not excluded", len(c.missingEntries)),
		Details: []string{fmt.Sprintf("Missing: %s", strings.Join(c.missingEntries, ", "))},
		FixHint: "Run 'gt doctor --fix' to add missing entries",
	}
}

// Fix appends missing entries to .git/info/exclude.
func (c *GitExcludeConfiguredCheck) Fix(ctx *CheckContext) error {
	if len(c.missingEntries) == 0 {
		return nil
	}

	// Ensure info directory exists
	infoDir := filepath.Dir(c.excludePath)
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return fmt.Errorf("failed to create info directory: %w", err)
	}

	// Append missing entries
	f, err := os.OpenFile(c.excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open exclude file: %w", err)
	}
	defer f.Close()

	// Add a header comment if file is empty or new
	info, _ := f.Stat()
	if info.Size() == 0 {
		if _, err := f.WriteString("# Gas Town directories\n"); err != nil {
			return err
		}
	} else {
		// Add newline before new entries
		if _, err := f.WriteString("\n# Gas Town directories\n"); err != nil {
			return err
		}
	}

	for _, entry := range c.missingEntries {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// WitnessExistsCheck verifies the witness directory structure exists.
type WitnessExistsCheck struct {
	FixableCheck
	rigPath     string
	needsCreate bool
	needsClone  bool
	needsMail   bool
}

// NewWitnessExistsCheck creates a new witness exists check.
func NewWitnessExistsCheck() *WitnessExistsCheck {
	return &WitnessExistsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "witness-exists",
				CheckDescription: "Verify witness/ directory structure exists",
			},
		},
	}
}

// Run checks if the witness directory structure exists.
func (c *WitnessExistsCheck) Run(ctx *CheckContext) *CheckResult {
	c.rigPath = ctx.RigPath()
	if c.rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	witnessDir := filepath.Join(c.rigPath, "witness")
	rigClone := filepath.Join(witnessDir, "rig")
	mailInbox := filepath.Join(witnessDir, "mail", "inbox.jsonl")

	var issues []string
	c.needsCreate = false
	c.needsClone = false
	c.needsMail = false

	// Check witness/ directory
	if _, err := os.Stat(witnessDir); os.IsNotExist(err) {
		issues = append(issues, "Missing: witness/")
		c.needsCreate = true
	} else {
		// Check witness/rig/ clone
		rigGit := filepath.Join(rigClone, ".git")
		if _, err := os.Stat(rigGit); os.IsNotExist(err) {
			issues = append(issues, "Missing: witness/rig/ (git clone)")
			c.needsClone = true
		}

		// Check witness/mail/inbox.jsonl
		if _, err := os.Stat(mailInbox); os.IsNotExist(err) {
			issues = append(issues, "Missing: witness/mail/inbox.jsonl")
			c.needsMail = true
		}
	}

	if len(issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Witness structure exists",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Witness structure incomplete",
		Details: issues,
		FixHint: "Run 'gt doctor --fix' to create missing structure",
	}
}

// Fix creates missing witness structure.
func (c *WitnessExistsCheck) Fix(ctx *CheckContext) error {
	witnessDir := filepath.Join(c.rigPath, "witness")

	if c.needsCreate {
		if err := os.MkdirAll(witnessDir, 0755); err != nil {
			return fmt.Errorf("failed to create witness/: %w", err)
		}
	}

	if c.needsMail {
		mailDir := filepath.Join(witnessDir, "mail")
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			return fmt.Errorf("failed to create witness/mail/: %w", err)
		}
		inboxPath := filepath.Join(mailDir, "inbox.jsonl")
		if err := os.WriteFile(inboxPath, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create inbox.jsonl: %w", err)
		}
	}

	// Note: Cannot auto-fix clone without knowing the repo URL
	if c.needsClone {
		return fmt.Errorf("cannot auto-create witness/rig/ clone (requires repo URL)")
	}

	return nil
}

// RefineryExistsCheck verifies the refinery directory structure exists.
type RefineryExistsCheck struct {
	FixableCheck
	rigPath     string
	needsCreate bool
	needsClone  bool
	needsMail   bool
}

// NewRefineryExistsCheck creates a new refinery exists check.
func NewRefineryExistsCheck() *RefineryExistsCheck {
	return &RefineryExistsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "refinery-exists",
				CheckDescription: "Verify refinery/ directory structure exists",
			},
		},
	}
}

// Run checks if the refinery directory structure exists.
func (c *RefineryExistsCheck) Run(ctx *CheckContext) *CheckResult {
	c.rigPath = ctx.RigPath()
	if c.rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	refineryDir := filepath.Join(c.rigPath, "refinery")
	rigClone := filepath.Join(refineryDir, "rig")
	mailInbox := filepath.Join(refineryDir, "mail", "inbox.jsonl")

	var issues []string
	c.needsCreate = false
	c.needsClone = false
	c.needsMail = false

	// Check refinery/ directory
	if _, err := os.Stat(refineryDir); os.IsNotExist(err) {
		issues = append(issues, "Missing: refinery/")
		c.needsCreate = true
	} else {
		// Check refinery/rig/ clone
		rigGit := filepath.Join(rigClone, ".git")
		if _, err := os.Stat(rigGit); os.IsNotExist(err) {
			issues = append(issues, "Missing: refinery/rig/ (git clone)")
			c.needsClone = true
		}

		// Check refinery/mail/inbox.jsonl
		if _, err := os.Stat(mailInbox); os.IsNotExist(err) {
			issues = append(issues, "Missing: refinery/mail/inbox.jsonl")
			c.needsMail = true
		}
	}

	if len(issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Refinery structure exists",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Refinery structure incomplete",
		Details: issues,
		FixHint: "Run 'gt doctor --fix' to create missing structure",
	}
}

// Fix creates missing refinery structure.
func (c *RefineryExistsCheck) Fix(ctx *CheckContext) error {
	refineryDir := filepath.Join(c.rigPath, "refinery")

	if c.needsCreate {
		if err := os.MkdirAll(refineryDir, 0755); err != nil {
			return fmt.Errorf("failed to create refinery/: %w", err)
		}
	}

	if c.needsMail {
		mailDir := filepath.Join(refineryDir, "mail")
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			return fmt.Errorf("failed to create refinery/mail/: %w", err)
		}
		inboxPath := filepath.Join(mailDir, "inbox.jsonl")
		if err := os.WriteFile(inboxPath, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create inbox.jsonl: %w", err)
		}
	}

	// Note: Cannot auto-fix clone without knowing the repo URL
	if c.needsClone {
		return fmt.Errorf("cannot auto-create refinery/rig/ clone (requires repo URL)")
	}

	return nil
}

// MayorCloneExistsCheck verifies the mayor/rig clone exists.
type MayorCloneExistsCheck struct {
	FixableCheck
	rigPath     string
	needsCreate bool
	needsClone  bool
}

// NewMayorCloneExistsCheck creates a new mayor clone check.
func NewMayorCloneExistsCheck() *MayorCloneExistsCheck {
	return &MayorCloneExistsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "mayor-clone-exists",
				CheckDescription: "Verify mayor/rig/ git clone exists",
			},
		},
	}
}

// Run checks if the mayor/rig clone exists.
func (c *MayorCloneExistsCheck) Run(ctx *CheckContext) *CheckResult {
	c.rigPath = ctx.RigPath()
	if c.rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	mayorDir := filepath.Join(c.rigPath, "mayor")
	rigClone := filepath.Join(mayorDir, "rig")

	var issues []string
	c.needsCreate = false
	c.needsClone = false

	// Check mayor/ directory
	if _, err := os.Stat(mayorDir); os.IsNotExist(err) {
		issues = append(issues, "Missing: mayor/")
		c.needsCreate = true
	} else {
		// Check mayor/rig/ clone
		rigGit := filepath.Join(rigClone, ".git")
		if _, err := os.Stat(rigGit); os.IsNotExist(err) {
			issues = append(issues, "Missing: mayor/rig/ (git clone)")
			c.needsClone = true
		}
	}

	if len(issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Mayor clone exists",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Mayor structure incomplete",
		Details: issues,
		FixHint: "Run 'gt doctor --fix' to create structure (clone requires repo URL)",
	}
}

// Fix creates missing mayor structure.
func (c *MayorCloneExistsCheck) Fix(ctx *CheckContext) error {
	mayorDir := filepath.Join(c.rigPath, "mayor")

	if c.needsCreate {
		if err := os.MkdirAll(mayorDir, 0755); err != nil {
			return fmt.Errorf("failed to create mayor/: %w", err)
		}
	}

	// Note: Cannot auto-fix clone without knowing the repo URL
	if c.needsClone {
		return fmt.Errorf("cannot auto-create mayor/rig/ clone (requires repo URL)")
	}

	return nil
}

// PolecatClonesValidCheck verifies each polecat directory is a valid clone.
type PolecatClonesValidCheck struct {
	BaseCheck
}

// NewPolecatClonesValidCheck creates a new polecat clones check.
func NewPolecatClonesValidCheck() *PolecatClonesValidCheck {
	return &PolecatClonesValidCheck{
		BaseCheck: BaseCheck{
			CheckName:        "polecat-clones-valid",
			CheckDescription: "Verify polecat directories are valid git clones",
		},
	}
}

// Run checks if each polecat directory is a valid git clone.
func (c *PolecatClonesValidCheck) Run(ctx *CheckContext) *CheckResult {
	rigPath := ctx.RigPath()
	if rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	polecatsDir := filepath.Join(rigPath, "polecats")
	entries, err := os.ReadDir(polecatsDir)
	if os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No polecats/ directory (none deployed)",
		}
	}
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot read polecats/: %v", err),
		}
	}

	var issues []string
	var warnings []string
	validCount := 0

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatPath := filepath.Join(polecatsDir, entry.Name())
		polecatName := entry.Name()

		// Check if it's a git clone
		gitPath := filepath.Join(polecatPath, ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			issues = append(issues, fmt.Sprintf("%s: not a git clone", polecatName))
			continue
		}

		// Verify git status works and check for uncommitted changes
		cmd := exec.Command("git", "-C", polecatPath, "status", "--porcelain")
		output, err := cmd.Output()
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: git status failed", polecatName))
			continue
		}

		if len(output) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: has uncommitted changes", polecatName))
		}

		// Check if on a polecat branch
		cmd = exec.Command("git", "-C", polecatPath, "branch", "--show-current")
		branchOutput, err := cmd.Output()
		if err == nil {
			branch := strings.TrimSpace(string(branchOutput))
			if !strings.HasPrefix(branch, "polecat/") {
				warnings = append(warnings, fmt.Sprintf("%s: on branch '%s' (expected polecat/*)", polecatName, branch))
			}
		}

		validCount++
	}

	if len(issues) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d polecat(s) invalid", len(issues)),
			Details: append(issues, warnings...),
			FixHint: "Cannot auto-fix (data loss risk)",
		}
	}

	if len(warnings) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d polecat(s) valid, %d warning(s)", validCount, len(warnings)),
			Details: warnings,
		}
	}

	if validCount == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No polecats deployed",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d polecat(s) valid", validCount),
	}
}

// BeadsConfigValidCheck verifies beads configuration if .beads/ exists.
type BeadsConfigValidCheck struct {
	FixableCheck
	rigPath   string
	needsSync bool
}

// NewBeadsConfigValidCheck creates a new beads config check.
func NewBeadsConfigValidCheck() *BeadsConfigValidCheck {
	return &BeadsConfigValidCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "beads-config-valid",
				CheckDescription: "Verify beads configuration if .beads/ exists",
			},
		},
	}
}

// Run checks if beads is properly configured.
func (c *BeadsConfigValidCheck) Run(ctx *CheckContext) *CheckResult {
	c.rigPath = ctx.RigPath()
	if c.rigPath == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "No rig specified",
		}
	}

	beadsDir := filepath.Join(c.rigPath, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No .beads/ directory (beads not configured)",
		}
	}

	// Check if bd command works
	cmd := exec.Command("bd", "stats", "--json")
	cmd.Dir = c.rigPath
	if err := cmd.Run(); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "bd command failed",
			Details: []string{fmt.Sprintf("Error: %v", err)},
			FixHint: "Check beads installation and .beads/ configuration",
		}
	}

	// Check sync status
	cmd = exec.Command("bd", "sync", "--status")
	cmd.Dir = c.rigPath
	output, err := cmd.CombinedOutput()
	c.needsSync = false
	if err != nil {
		// sync --status may exit non-zero if out of sync
		outputStr := string(output)
		if strings.Contains(outputStr, "out of sync") || strings.Contains(outputStr, "behind") {
			c.needsSync = true
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusWarning,
				Message: "Beads out of sync",
				Details: []string{strings.TrimSpace(outputStr)},
				FixHint: "Run 'gt doctor --fix' or 'bd sync' to synchronize",
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Beads configured and in sync",
	}
}

// Fix runs bd sync if needed.
func (c *BeadsConfigValidCheck) Fix(ctx *CheckContext) error {
	if !c.needsSync {
		return nil
	}

	cmd := exec.Command("bd", "sync")
	cmd.Dir = c.rigPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd sync failed: %s", string(output))
	}

	return nil
}

// RigChecks returns all rig-level health checks.
func RigChecks() []Check {
	return []Check{
		NewRigIsGitRepoCheck(),
		NewGitExcludeConfiguredCheck(),
		NewWitnessExistsCheck(),
		NewRefineryExistsCheck(),
		NewMayorCloneExistsCheck(),
		NewPolecatClonesValidCheck(),
		NewBeadsConfigValidCheck(),
	}
}
