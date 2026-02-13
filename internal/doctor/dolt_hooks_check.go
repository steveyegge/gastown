package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// DoltHooksCheck detects stale JSONL-sync git hooks when the Dolt backend is active.
// After migrating to Dolt via "bd migrate --to-dolt", old inline hooks that call
// "bd sync --flush-only" (pre-commit) or "bd import -i" (post-merge) are pointless
// and cause confusing errors. Shim hooks (containing "# bd-shim") are fine because
// they delegate to "bd hooks run" which handles Dolt internally.
//
// Mirrors bd doctor's CheckGitHooksDoltCompatibility but covers both the town root
// and all registered rigs.
type DoltHooksCheck struct {
	FixableCheck
	stalePaths []string // directories with stale hooks, cached for Fix
}

// NewDoltHooksCheck creates a new Dolt hooks compatibility check.
func NewDoltHooksCheck() *DoltHooksCheck {
	return &DoltHooksCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "dolt-hooks-compat",
				CheckDescription: "Detect stale JSONL-sync hooks when Dolt backend is active",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Markers matching beads/cmd/bd/doctor/git.go and beads/cmd/bd/init_git_hooks.go
const (
	bdShimMarker       = "# bd-shim"
	bdInlineHookMarker = "# bd (beads)"
)

// beadsMetadata is a minimal struct for reading the backend field from metadata.json.
type beadsMetadata struct {
	Backend string `json:"backend"`
}

// readBeadsBackend reads metadata.json from beadsDir and returns the backend field.
// Returns "" on any error (missing file, bad JSON, etc.).
func readBeadsBackend(beadsDir string) string {
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json")) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return ""
	}
	var meta beadsMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.Backend
}

// isHookStaleDolt checks whether the post-merge hook in hooksDir is a stale
// JSONL-sync hook that predates Dolt migration. Returns true if the hook should
// be upgraded.
//
// Decision tree (mirrors bd doctor logic):
//  1. No hook file → not stale
//  2. Contains "# bd-shim" → OK (shim delegates to bd hooks run)
//  3. Does NOT contain "# bd (beads)" AND does NOT contain "bd" → OK (not a bd hook)
//  4. Contains both "backend" and "dolt" → OK (already has Dolt skip logic)
//  5. Otherwise → stale
func isHookStaleDolt(hooksDir string) bool {
	content, err := os.ReadFile(filepath.Join(hooksDir, "post-merge")) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return false // no hook file
	}
	s := string(content)

	if strings.Contains(s, bdShimMarker) {
		return false // shim hooks handle Dolt internally
	}
	if !strings.Contains(s, bdInlineHookMarker) && !strings.Contains(s, "bd") {
		return false // not a bd hook at all
	}
	if strings.Contains(s, "backend") && strings.Contains(s, "dolt") {
		return false // already has Dolt-aware logic
	}
	return true
}

// Run checks the town root and all registered rigs for stale JSONL-sync hooks.
func (c *DoltHooksCheck) Run(ctx *CheckContext) *CheckResult {
	c.stalePaths = nil
	var details []string

	// 1. Check town root
	townBeadsDir := beads.ResolveBeadsDir(ctx.TownRoot)
	if readBeadsBackend(townBeadsDir) == "dolt" {
		hooksDir := filepath.Join(ctx.TownRoot, ".git", "hooks")
		if isHookStaleDolt(hooksDir) {
			c.stalePaths = append(c.stalePaths, ctx.TownRoot)
			details = append(details, "Town root has stale JSONL-sync hooks")
		}
	}

	// 2. Check each registered rig
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsCfg, err := loadRigsConfig(rigsPath)
	if err == nil {
		for rigName := range rigsCfg.Rigs {
			rigPath := filepath.Join(ctx.TownRoot, rigName)
			rigBeadsDir := beads.ResolveBeadsDir(rigPath)
			if readBeadsBackend(rigBeadsDir) != "dolt" {
				continue
			}
			hooksDir := filepath.Join(rigPath, ".git", "hooks")
			if isHookStaleDolt(hooksDir) {
				c.stalePaths = append(c.stalePaths, rigPath)
				details = append(details, fmt.Sprintf("Rig %q has stale JSONL-sync hooks", rigName))
			}
		}
	}

	if len(c.stalePaths) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale JSONL-sync hooks found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d location(s) have stale JSONL-sync hooks with Dolt backend", len(c.stalePaths)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' or 'bd hooks install --force' in each affected directory",
	}
}

// Fix runs "bd hooks install --force" in each directory that was flagged during Run.
func (c *DoltHooksCheck) Fix(ctx *CheckContext) error {
	var errors []string
	for _, dir := range c.stalePaths {
		cmd := exec.Command("bd", "hooks", "install", "--force") // #nosec G204 - fixed command
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %s (%v)", dir, strings.TrimSpace(string(output)), err))
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("failed to fix hooks: %s", strings.Join(errors, "; "))
	}
	return nil
}
