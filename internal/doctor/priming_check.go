package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
)

// PrimingCheck verifies the priming subsystem is correctly configured.
// This ensures agents receive proper context on startup via the gt prime chain.
type PrimingCheck struct {
	FixableCheck
	issues []primingIssue
}

type primingIssue struct {
	location    string // e.g., "mayor", "gastown/crew/max", "gastown/witness"
	issueType   string // e.g., "no_hook", "no_prime", "large_claude_md", "missing_prime_md"
	description string
	fixable     bool
}

// NewPrimingCheck creates a new priming subsystem check.
func NewPrimingCheck() *PrimingCheck {
	return &PrimingCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "priming",
				CheckDescription: "Verify priming subsystem is correctly configured",
			},
		},
	}
}

// Run checks the priming configuration across all agent locations.
func (c *PrimingCheck) Run(ctx *CheckContext) *CheckResult {
	c.issues = nil

	var details []string

	// Check 1: gt binary in PATH
	if err := exec.Command("which", "gt").Run(); err != nil {
		c.issues = append(c.issues, primingIssue{
			location:    "system",
			issueType:   "gt_not_in_path",
			description: "gt binary not found in PATH",
			fixable:     false,
		})
		details = append(details, "gt binary not found in PATH")
	}

	// Check 2: Mayor priming (town-level)
	mayorIssues := c.checkAgentPriming(ctx.TownRoot, "mayor", "mayor")
	for _, issue := range mayorIssues {
		details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
	}
	c.issues = append(c.issues, mayorIssues...)

	// Check 3: Deacon priming
	deaconPath := filepath.Join(ctx.TownRoot, "deacon")
	if dirExists(deaconPath) {
		deaconIssues := c.checkAgentPriming(ctx.TownRoot, "deacon", "deacon")
		for _, issue := range deaconIssues {
			details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
		}
		c.issues = append(c.issues, deaconIssues...)
	}

	// Check 4: Rig-level agents (witness, refinery, crew, polecats)
	rigIssues := c.checkRigPriming(ctx.TownRoot)
	for _, issue := range rigIssues {
		details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
	}
	c.issues = append(c.issues, rigIssues...)

	if len(c.issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Priming subsystem is correctly configured",
		}
	}

	// Count fixable issues
	fixableCount := 0
	for _, issue := range c.issues {
		if issue.fixable {
			fixableCount++
		}
	}

	fixHint := ""
	if fixableCount > 0 {
		fixHint = fmt.Sprintf("Run 'gt doctor --fix' to fix %d issue(s)", fixableCount)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d priming issue(s)", len(c.issues)),
		Details: details,
		FixHint: fixHint,
	}
}

// checkAgentPriming checks priming configuration for a specific agent.
func (c *PrimingCheck) checkAgentPriming(townRoot, agentDir, _ string) []primingIssue {
	var issues []primingIssue

	agentPath := filepath.Join(townRoot, agentDir)
	settingsPath := filepath.Join(agentPath, ".claude", "settings.json")

	// Check for SessionStart hook with gt prime
	if fileExists(settingsPath) {
		data, err := os.ReadFile(settingsPath)
		if err == nil {
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err == nil {
				if !c.hasGtPrimeHook(settings) {
					issues = append(issues, primingIssue{
						location:    agentDir,
						issueType:   "no_prime_hook",
						description: "SessionStart hook missing 'gt prime'",
						fixable:     false, // Requires template regeneration
					})
				}
			}
		}
	}

	// Check CLAUDE.md is minimal (bootstrap pointer, not full context)
	claudeMdPath := filepath.Join(agentPath, "CLAUDE.md")
	if fileExists(claudeMdPath) {
		lines := c.countLines(claudeMdPath)
		if lines > 30 {
			issues = append(issues, primingIssue{
				location:    agentDir,
				issueType:   "large_claude_md",
				description: fmt.Sprintf("CLAUDE.md has %d lines (should be <30 for bootstrap pointer)", lines),
				fixable:     false, // Requires manual review
			})
		}
	}

	// Check AGENTS.md is minimal (bootstrap pointer, not full context)
	agentsMdPath := filepath.Join(agentPath, "AGENTS.md")
	if fileExists(agentsMdPath) {
		lines := c.countLines(agentsMdPath)
		if lines > 20 {
			issues = append(issues, primingIssue{
				location:    agentDir,
				issueType:   "large_agents_md",
				description: fmt.Sprintf("AGENTS.md has %d lines (should be <20 for bootstrap pointer)", lines),
				fixable:     false, // Full context should come from gt prime templates
			})
		}
	}

	return issues
}

// checkRigPriming checks priming for all rigs.
func (c *PrimingCheck) checkRigPriming(townRoot string) []primingIssue {
	var issues []primingIssue

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return issues
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip non-rig directories
		if rigName == "mayor" || rigName == "deacon" || rigName == "daemon" ||
			rigName == "docs" || rigName[0] == '.' {
			continue
		}

		// Check if this is actually a rig (has .beads directory)
		if !dirExists(filepath.Join(rigPath, ".beads")) {
			continue
		}

		// Check PRIME.md exists at rig level
		primeMdPath := filepath.Join(rigPath, ".beads", "PRIME.md")
		if !fileExists(primeMdPath) {
			issues = append(issues, primingIssue{
				location:    rigName,
				issueType:   "missing_prime_md",
				description: "Missing .beads/PRIME.md (Gas Town context fallback)",
				fixable:     true,
			})
		}

		// Check AGENTS.md is minimal at rig level (bootstrap pointer, not full context)
		agentsMdPath := filepath.Join(rigPath, "AGENTS.md")
		if fileExists(agentsMdPath) {
			lines := c.countLines(agentsMdPath)
			if lines > 20 {
				issues = append(issues, primingIssue{
					location:    rigName,
					issueType:   "large_agents_md",
					description: fmt.Sprintf("AGENTS.md has %d lines (should be <20 for bootstrap pointer)", lines),
					fixable:     false, // Requires manual review
				})
			}
		}

		// Check for unexpected CLAUDE.md inside mayor/rig (pollutes user's repo)
		// Agent-level CLAUDE.md belongs at mayor/, not mayor/rig/
		mayorRigClaudeMd := filepath.Join(rigPath, "mayor", "rig", "CLAUDE.md")
		if fileExists(mayorRigClaudeMd) {
			issues = append(issues, primingIssue{
				location:    fmt.Sprintf("%s/mayor/rig", rigName),
				issueType:   "unexpected_claude_md",
				description: "CLAUDE.md in source tree is unsupported and may conflict with Gas Town agent configuration",
				fixable:     true,
			})
		}

		// Check for unexpected CLAUDE.md inside refinery/rig (pollutes user's repo)
		// Agent-level CLAUDE.md belongs at refinery/, not refinery/rig/
		refineryRigClaudeMd := filepath.Join(rigPath, "refinery", "rig", "CLAUDE.md")
		if fileExists(refineryRigClaudeMd) {
			issues = append(issues, primingIssue{
				location:    fmt.Sprintf("%s/refinery/rig", rigName),
				issueType:   "unexpected_claude_md",
				description: "CLAUDE.md in source tree is unsupported and may conflict with Gas Town agent configuration",
				fixable:     true,
			})
		}

		// Check for missing CLAUDE.md/AGENTS.md at agent level directories.
		// These are bootstrap pointers that tell agents to run `gt prime` for full context.
		// Per-rig mayor is just a source clone - town-level mayor gets files from gt install.
		for _, role := range []string{"refinery", "witness", "crew", "polecats"} {
			agentPath := filepath.Join(rigPath, role)
			if dirExists(agentPath) {
				if issue := c.checkAgentBootstrapFiles(agentPath, rigName, role); issue != nil {
					issues = append(issues, *issue)
				}
			}
		}

		// Check witness priming
		witnessPath := filepath.Join(rigPath, "witness")
		if dirExists(witnessPath) {
			witnessIssues := c.checkAgentPriming(townRoot, filepath.Join(rigName, "witness"), "witness")
			issues = append(issues, witnessIssues...)
		}

		// Check refinery priming
		refineryPath := filepath.Join(rigPath, "refinery")
		if dirExists(refineryPath) {
			refineryIssues := c.checkAgentPriming(townRoot, filepath.Join(rigName, "refinery"), "refinery")
			issues = append(issues, refineryIssues...)
		}

		// Check crew PRIME.md (shared settings, individual worktrees)
		crewDir := filepath.Join(rigPath, "crew")
		if dirExists(crewDir) {
			crewEntries, _ := os.ReadDir(crewDir)
			for _, crewEntry := range crewEntries {
				if !crewEntry.IsDir() || crewEntry.Name() == ".claude" {
					continue
				}
				crewPath := filepath.Join(crewDir, crewEntry.Name())

				// Check for unexpected CLAUDE.md inside crew worktree (pollutes user's repo)
				crewClaudeMd := filepath.Join(crewPath, "CLAUDE.md")
				if fileExists(crewClaudeMd) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/crew/%s", rigName, crewEntry.Name()),
						issueType:   "unexpected_claude_md",
						description: "CLAUDE.md in source tree is unsupported and may conflict with Gas Town agent configuration",
						fixable:     true,
					})
				}

				// Check if beads redirect is set up (crew should redirect to rig)
				beadsDir := beads.ResolveBeadsDir(crewPath)
				primeMdPath := filepath.Join(beadsDir, "PRIME.md")
				if !fileExists(primeMdPath) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/crew/%s", rigName, crewEntry.Name()),
						issueType:   "missing_prime_md",
						description: "Missing PRIME.md (Gas Town context fallback)",
						fixable:     true,
					})
				}
			}
		}

		// Check polecat PRIME.md
		// Polecat structure: polecats/<name>/<rigname>/ (worktree is nested inside polecatDir)
		polecatsDir := filepath.Join(rigPath, "polecats")
		if dirExists(polecatsDir) {
			pcEntries, _ := os.ReadDir(polecatsDir)
			for _, pcEntry := range pcEntries {
				if !pcEntry.IsDir() || pcEntry.Name() == ".claude" {
					continue
				}
				polecatDir := filepath.Join(polecatsDir, pcEntry.Name())

				// Check for orphaned .beads at polecatDir level (bug created these)
				// The .beads should only exist at worktree level: polecats/<name>/<rigname>/.beads
				orphanedBeads := filepath.Join(polecatDir, ".beads")
				if dirExists(orphanedBeads) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/polecats/%s", rigName, pcEntry.Name()),
						issueType:   "orphaned_beads_dir",
						description: "Orphaned .beads directory at wrong level (should be in worktree)",
						fixable:     true,
					})
				}

				// The actual worktree is at polecats/<name>/<rigname>/
				polecatWorktree := filepath.Join(polecatDir, rigName)
				if !dirExists(polecatWorktree) {
					// No worktree yet - skip (polecat may not be fully set up)
					continue
				}

				// Check for unexpected CLAUDE.md inside polecat worktree (pollutes user's repo)
				polecatClaudeMd := filepath.Join(polecatWorktree, "CLAUDE.md")
				if fileExists(polecatClaudeMd) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/polecats/%s/%s", rigName, pcEntry.Name(), rigName),
						issueType:   "unexpected_claude_md",
						description: "CLAUDE.md in source tree is unsupported and may conflict with Gas Town agent configuration",
						fixable:     true,
					})
				}

				// Check if beads redirect is set up in the worktree
				beadsDir := beads.ResolveBeadsDir(polecatWorktree)
				primeMdPath := filepath.Join(beadsDir, "PRIME.md")
				if !fileExists(primeMdPath) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/polecats/%s/%s", rigName, pcEntry.Name(), rigName),
						issueType:   "missing_prime_md",
						description: "Missing PRIME.md (Gas Town context fallback)",
						fixable:     true,
					})
				}
			}
		}
	}

	return issues
}

// hasGtPrimeHook checks if settings have a SessionStart hook that calls gt prime.
func (c *PrimingCheck) hasGtPrimeHook(settings map[string]any) bool {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	hookList, ok := hooks["SessionStart"].([]any)
	if !ok {
		return false
	}

	for _, hook := range hookList {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, inner := range innerHooks {
			innerMap, ok := inner.(map[string]any)
			if !ok {
				continue
			}
			cmd, ok := innerMap["command"].(string)
			if ok && strings.Contains(cmd, "gt prime") {
				return true
			}
		}
	}
	return false
}

// checkAgentBootstrapFiles checks if CLAUDE.md and AGENTS.md exist at agent level.
// Returns an issue if either file is missing.
func (c *PrimingCheck) checkAgentBootstrapFiles(agentPath, rigName, role string) *primingIssue {
	missingClaudeMd := !fileExists(filepath.Join(agentPath, "CLAUDE.md"))
	missingAgentsMd := !fileExists(filepath.Join(agentPath, "AGENTS.md"))
	if missingClaudeMd || missingAgentsMd {
		return &primingIssue{
			location:    fmt.Sprintf("%s/%s", rigName, role),
			issueType:   "missing_agent_bootstrap",
			description: "Missing bootstrap files (CLAUDE.md/AGENTS.md) at agent level",
			fixable:     true,
		}
	}
	return nil
}

// countLines counts the number of lines in a file.
func (c *PrimingCheck) countLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

// Fix attempts to fix priming issues.
func (c *PrimingCheck) Fix(ctx *CheckContext) error {
	var errors []string

	for _, issue := range c.issues {
		if !issue.fixable {
			continue
		}

		switch issue.issueType {
		case "orphaned_beads_dir":
			// Remove orphaned .beads directory at polecatDir level
			// These were incorrectly created by a bug that looked at polecats/<name>/
			// instead of polecats/<name>/<rigname>/
			orphanedPath := filepath.Join(ctx.TownRoot, issue.location, ".beads")
			if err := os.RemoveAll(orphanedPath); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to remove orphaned .beads: %v", issue.location, err))
			}

		case "missing_prime_md":
			// Provision PRIME.md at the appropriate location
			var targetPath string

			// Parse the location to determine where to provision
			if strings.Contains(issue.location, "/crew/") || strings.Contains(issue.location, "/polecats/") {
				// Worker location - use beads.ProvisionPrimeMDForWorktree
				worktreePath := filepath.Join(ctx.TownRoot, issue.location)
				if err := beads.ProvisionPrimeMDForWorktree(worktreePath); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", issue.location, err))
				}
			} else {
				// Rig location - provision directly
				targetPath = filepath.Join(ctx.TownRoot, issue.location, constants.DirBeads)
				if err := beads.ProvisionPrimeMD(targetPath); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", issue.location, err))
				}
			}

		case "unexpected_claude_md":
			// Remove unexpected CLAUDE.md from inside repo worktrees
			// Context should come from gt prime, not on-disk files
			claudeMdPath := filepath.Join(ctx.TownRoot, issue.location, "CLAUDE.md")
			if err := os.Remove(claudeMdPath); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("%s: failed to remove CLAUDE.md: %v", issue.location, err))
			}

		case "missing_agent_bootstrap":
			// Create CLAUDE.md and AGENTS.md bootstrap pointers at agent level
			agentPath := filepath.Join(ctx.TownRoot, issue.location)
			role := filepath.Base(issue.location)
			rigName := filepath.Base(filepath.Dir(issue.location))
			bootstrap := agent.GenerateBootstrap(role, rigName)

			// Create both files (idempotent - only writes if missing)
			for _, filename := range []string{"CLAUDE.md", "AGENTS.md"} {
				filePath := filepath.Join(agentPath, filename)
				if !fileExists(filePath) {
					if err := os.WriteFile(filePath, []byte(bootstrap), 0644); err != nil {
						errors = append(errors, fmt.Sprintf("%s: failed to create %s: %v", issue.location, filename, err))
					}
				}
			}

		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

