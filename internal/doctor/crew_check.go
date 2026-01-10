package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CrewStateCheck validates crew worker state.json files for completeness.
// Empty or incomplete state.json files cause "can't find pane/session" errors.
type CrewStateCheck struct {
	FixableCheck
	invalidCrews []invalidCrew // Cached during Run for use in Fix
}

type invalidCrew struct {
	path      string
	stateFile string
	rigName   string
	crewName  string
	issue     string
}

// NewCrewStateCheck creates a new crew state check.
func NewCrewStateCheck() *CrewStateCheck {
	return &CrewStateCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "crew-state",
				CheckDescription: "Validate crew worker state.json files",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks all crew state.json files for completeness.
func (c *CrewStateCheck) Run(ctx *CheckContext) *CheckResult {
	c.invalidCrews = nil

	crewDirs := c.findAllCrewDirs(ctx.TownRoot)
	if len(crewDirs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No crew workspaces found",
		}
	}

	var validCount int
	var details []string

	for _, cd := range crewDirs {
		stateFile := filepath.Join(cd.path, "state.json")

		// Check if state.json exists
		data, err := os.ReadFile(stateFile)
		if err != nil {
			if os.IsNotExist(err) {
				// Missing state file is OK - code will use defaults
				validCount++
				continue
			}
			// Other errors are problems
			issue := fmt.Sprintf("cannot read state.json: %v", err)
			c.invalidCrews = append(c.invalidCrews, invalidCrew{
				path:      cd.path,
				stateFile: stateFile,
				rigName:   cd.rigName,
				crewName:  cd.crewName,
				issue:     issue,
			})
			details = append(details, fmt.Sprintf("%s/%s: %s", cd.rigName, cd.crewName, issue))
			continue
		}

		// Parse state.json
		var state struct {
			Name      string `json:"name"`
			Rig       string `json:"rig"`
			ClonePath string `json:"clone_path"`
		}
		if err := json.Unmarshal(data, &state); err != nil {
			issue := "invalid JSON in state.json"
			c.invalidCrews = append(c.invalidCrews, invalidCrew{
				path:      cd.path,
				stateFile: stateFile,
				rigName:   cd.rigName,
				crewName:  cd.crewName,
				issue:     issue,
			})
			details = append(details, fmt.Sprintf("%s/%s: %s", cd.rigName, cd.crewName, issue))
			continue
		}

		// Check for empty/incomplete state
		var issues []string
		if state.Name == "" {
			issues = append(issues, "missing name")
		}
		if state.Rig == "" {
			issues = append(issues, "missing rig")
		}
		if state.ClonePath == "" {
			issues = append(issues, "missing clone_path")
		}

		if len(issues) > 0 {
			issue := strings.Join(issues, ", ")
			c.invalidCrews = append(c.invalidCrews, invalidCrew{
				path:      cd.path,
				stateFile: stateFile,
				rigName:   cd.rigName,
				crewName:  cd.crewName,
				issue:     issue,
			})
			details = append(details, fmt.Sprintf("%s/%s: %s", cd.rigName, cd.crewName, issue))
		} else {
			validCount++
		}
	}

	if len(c.invalidCrews) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d crew state files valid", validCount),
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d crew workspace(s) with invalid state.json", len(c.invalidCrews)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to regenerate state files",
	}
}

// Fix regenerates invalid state.json files with correct values.
func (c *CrewStateCheck) Fix(ctx *CheckContext) error {
	if len(c.invalidCrews) == 0 {
		return nil
	}

	var lastErr error
	for _, ic := range c.invalidCrews {
		state := map[string]interface{}{
			"name":       ic.crewName,
			"rig":        ic.rigName,
			"clone_path": ic.path,
			"branch":     "main",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		}

		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			lastErr = fmt.Errorf("%s/%s: %w", ic.rigName, ic.crewName, err)
			continue
		}

		if err := os.WriteFile(ic.stateFile, data, 0644); err != nil {
			lastErr = fmt.Errorf("%s/%s: %w", ic.rigName, ic.crewName, err)
			continue
		}
	}

	return lastErr
}

type crewDir struct {
	path     string
	rigName  string
	crewName string
}

// findAllCrewDirs finds all crew directories in the workspace.
func (c *CrewStateCheck) findAllCrewDirs(townRoot string) []crewDir {
	var dirs []crewDir

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return dirs
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" {
			continue
		}

		rigName := entry.Name()
		crewPath := filepath.Join(townRoot, rigName, "crew")

		crewEntries, err := os.ReadDir(crewPath)
		if err != nil {
			continue
		}

		for _, crew := range crewEntries {
			if !crew.IsDir() || strings.HasPrefix(crew.Name(), ".") {
				continue
			}
			dirs = append(dirs, crewDir{
				path:     filepath.Join(crewPath, crew.Name()),
				rigName:  rigName,
				crewName: crew.Name(),
			})
		}
	}

	return dirs
}

// CrewWorktreeCheck detects stale cross-rig worktrees in crew directories.
// Cross-rig worktrees are created by `gt worktree <rig>` and live in crew/
// with names like `<source-rig>-<crewname>`. They should be cleaned up when
// no longer needed to avoid confusion with regular crew workspaces.
type CrewWorktreeCheck struct {
	FixableCheck
	staleWorktrees []staleWorktree
}

type staleWorktree struct {
	path      string
	rigName   string
	name      string
	sourceRig string
	crewName  string
}

// NewCrewWorktreeCheck creates a new crew worktree check.
func NewCrewWorktreeCheck() *CrewWorktreeCheck {
	return &CrewWorktreeCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "crew-worktrees",
				CheckDescription: "Detect stale cross-rig worktrees in crew directories",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks for cross-rig worktrees that may need cleanup.
func (c *CrewWorktreeCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleWorktrees = nil

	worktrees := c.findCrewWorktrees(ctx.TownRoot)
	if len(worktrees) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No cross-rig worktrees in crew directories",
		}
	}

	c.staleWorktrees = worktrees
	var details []string
	for _, wt := range worktrees {
		details = append(details, fmt.Sprintf("%s/crew/%s (from %s/crew/%s)",
			wt.rigName, wt.name, wt.sourceRig, wt.crewName))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d cross-rig worktree(s) in crew directories", len(worktrees)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove, or use 'gt crew remove <name> --purge'",
	}
}

// Fix removes stale cross-rig worktrees.
func (c *CrewWorktreeCheck) Fix(ctx *CheckContext) error {
	if len(c.staleWorktrees) == 0 {
		return nil
	}

	var lastErr error
	for _, wt := range c.staleWorktrees {
		// Use git worktree remove to properly clean up
		mayorRigPath := filepath.Join(ctx.TownRoot, wt.rigName, "mayor", "rig")
		removeCmd := exec.Command("git", "worktree", "remove", "--force", wt.path)
		removeCmd.Dir = mayorRigPath
		if output, err := removeCmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s/crew/%s: %v (%s)", wt.rigName, wt.name, err, strings.TrimSpace(string(output)))
		}
	}

	return lastErr
}

// findCrewWorktrees finds cross-rig worktrees in crew directories.
// These are worktrees with hyphenated names (e.g., "beads-dave") that
// indicate they were created via `gt worktree` for cross-rig work.
func (c *CrewWorktreeCheck) findCrewWorktrees(townRoot string) []staleWorktree {
	var worktrees []staleWorktree

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return worktrees
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" {
			continue
		}

		rigName := entry.Name()
		crewPath := filepath.Join(townRoot, rigName, "crew")

		crewEntries, err := os.ReadDir(crewPath)
		if err != nil {
			continue
		}

		for _, crew := range crewEntries {
			if !crew.IsDir() || strings.HasPrefix(crew.Name(), ".") {
				continue
			}

			name := crew.Name()
			path := filepath.Join(crewPath, name)

			// Check if it's a worktree (has .git file, not directory)
			gitPath := filepath.Join(path, ".git")
			info, err := os.Stat(gitPath)
			if err != nil || info.IsDir() {
				// Not a worktree (regular clone or error)
				continue
			}

			// Check for hyphenated name pattern: <source-rig>-<crewname>
			// This indicates a cross-rig worktree created by `gt worktree`
			parts := strings.SplitN(name, "-", 2)
			if len(parts) != 2 {
				// Not a cross-rig worktree pattern
				continue
			}

			sourceRig := parts[0]
			crewName := parts[1]

			// Verify the source rig exists (sanity check)
			sourceRigPath := filepath.Join(townRoot, sourceRig)
			if _, err := os.Stat(sourceRigPath); os.IsNotExist(err) {
				// Source rig doesn't exist - definitely stale
			}

			worktrees = append(worktrees, staleWorktree{
				path:      path,
				rigName:   rigName,
				name:      name,
				sourceRig: sourceRig,
				crewName:  crewName,
			})
		}
	}

	return worktrees
}
