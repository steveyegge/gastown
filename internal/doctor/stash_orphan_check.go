package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StashOrphanCheck scans every git working tree under each rig (root, crew clones,
// polecat worktrees) for stashes older than a threshold. It is purely observational —
// it never drops, pops, or otherwise mutates stashes. Recovery is the operator's call.
//
// Background: agents have been observed running `git stash` to clean their working
// tree before rebase/checkout, then dying before `git stash pop`. Orphaned stashes
// accumulate in `.git/refs/stash` indefinitely with no canonical visibility. The
// `gt-pvx` safety net in `gt done` recovers stashes at session-end, but only for the
// session that runs `gt done` — stashes from sessions that died abruptly stay hidden.
//
// This check surfaces those stashes so an operator can decide whether to pop, drop,
// or leave them.
type StashOrphanCheck struct {
	BaseCheck
	threshold time.Duration
}

// NewStashOrphanCheck creates a stash-orphan check with a 24h threshold.
func NewStashOrphanCheck() *StashOrphanCheck {
	return &StashOrphanCheck{
		BaseCheck: BaseCheck{
			CheckName:        "stash-orphan",
			CheckDescription: "Detect git stashes older than 24h in rigs and crew clones",
			CheckCategory:    CategoryCleanup,
		},
		threshold: 24 * time.Hour,
	}
}

// Run scans every git working tree under each rig and reports stashes older than
// the threshold. Returns Warning if any are found, OK otherwise.
func (c *StashOrphanCheck) Run(ctx *CheckContext) *CheckResult {
	rigs, err := discoverRigs(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Failed to discover rigs",
			Details: []string{err.Error()},
		}
	}

	if len(rigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs configured",
		}
	}

	var details []string
	totalOrphans := 0

	for _, rigName := range rigs {
		rigRoot := filepath.Join(ctx.TownRoot, rigName)
		// Discover every git working tree owned by this rig:
		//   <rigRoot>/                       (rig root — polecat territory + canonical ref)
		//   <rigRoot>/crew/<name>/           (crew clones)
		//   <rigRoot>/polecats/<name>/<rig>/ (polecat worktrees, if any persist)
		workdirs := discoverGitWorkdirs(rigRoot)
		for _, wd := range workdirs {
			orphans := countOrphanStashes(wd, c.threshold)
			if orphans > 0 {
				rel, err := filepath.Rel(ctx.TownRoot, wd)
				if err != nil {
					rel = wd
				}
				details = append(details, fmt.Sprintf("%s: %d stash(es) >%s old", rel, orphans, c.threshold))
				totalOrphans += orphans
			}
		}
	}

	if totalOrphans > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d orphan stash(es) found across rigs (>%s old)", totalOrphans, c.threshold),
			Details: details,
			FixHint: "Inspect with 'cd <path> && git stash list', then 'git stash pop' or 'git stash drop'. " +
				"This check is observational — automatic drop is unsafe.",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No orphan stashes found",
	}
}

// discoverGitWorkdirs returns every directory under rigRoot that has a .git
// directory or file (i.e. is a git working tree). Probes the rig root itself
// plus immediate children of crew/ and polecats/. Anything deeper is ignored
// (we don't walk the entire tree — performance + intent: only sanctioned
// per-agent paths).
func discoverGitWorkdirs(rigRoot string) []string {
	var dirs []string

	// Rig root itself
	if isGitWorkdir(rigRoot) {
		dirs = append(dirs, rigRoot)
	}

	// Crew clones at <rigRoot>/crew/<name>/
	for _, sub := range []string{"crew", "polecats"} {
		parent := filepath.Join(rigRoot, sub)
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(parent, e.Name())
			// For polecats, the working tree is one level deeper:
			//   polecats/<name>/<rig-name>/
			if sub == "polecats" {
				inner, err := os.ReadDir(candidate)
				if err != nil {
					continue
				}
				for _, ie := range inner {
					if !ie.IsDir() {
						continue
					}
					path := filepath.Join(candidate, ie.Name())
					if isGitWorkdir(path) {
						dirs = append(dirs, path)
					}
				}
				continue
			}
			if isGitWorkdir(candidate) {
				dirs = append(dirs, candidate)
			}
		}
	}

	return dirs
}

// isGitWorkdir reports whether dir contains a .git directory or file.
func isGitWorkdir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// countOrphanStashes returns the number of stash entries in dir whose author
// timestamp is older than the threshold. Returns 0 on any error (best-effort).
func countOrphanStashes(dir string, threshold time.Duration) int {
	// Use the stash reflog with a parseable format: "<ref>|<author-date-iso>".
	// %gd = stash ref (e.g. stash@{0}), %ai = author date (ISO 8601 with TZ).
	cmd := exec.Command("git", "stash", "list", "--format=%gd|%ai")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return 0
	}

	cutoff := time.Now().Add(-threshold)
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		ts, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			count++
		}
	}
	return count
}
