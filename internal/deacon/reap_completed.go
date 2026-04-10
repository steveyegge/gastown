// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Default parameters for completed polecat reaping.
const (
	// DefaultIdleThreshold is how long a polecat must be idle (no output) before reaping.
	DefaultIdleThreshold = 60 * time.Second
)

// ReapConfig holds configurable parameters for completed polecat reaping.
type ReapConfig struct {
	// IdleThreshold is how long a polecat session must be idle before being reaped.
	IdleThreshold time.Duration `json:"idle_threshold"`
	// DryRun if true, only reports what would be done without making changes.
	DryRun bool `json:"dry_run"`
}

// DefaultReapConfig returns the default reap config.
func DefaultReapConfig() *ReapConfig {
	return &ReapConfig{
		IdleThreshold: DefaultIdleThreshold,
		DryRun:        false,
	}
}

// ReapResult represents the result of processing a single completed polecat.
type ReapResult struct {
	Rig             string `json:"rig"`
	Polecat         string `json:"polecat"`
	SessionName     string `json:"session_name"`
	BeadID          string `json:"bead_id,omitempty"`
	BeadStatus      string `json:"bead_status,omitempty"`
	SessionKilled   bool   `json:"session_killed"`
	WorktreeRemoved bool   `json:"worktree_removed"`
	PartialWork     bool   `json:"partial_work,omitempty"`
	WorktreeDirty   bool   `json:"worktree_dirty,omitempty"`
	UnpushedCount   int    `json:"unpushed_count,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ReapScanResult contains the full results of a completed polecat scan.
type ReapScanResult struct {
	ScannedAt     time.Time     `json:"scanned_at"`
	TotalPolecats int           `json:"total_polecats"`
	Completed     int           `json:"completed"`
	Reaped        int           `json:"reaped"`
	Results       []*ReapResult `json:"results"`
}

// polecatDir holds information about a discovered polecat directory.
type polecatDir struct {
	Rig     string
	Polecat string
	Path    string // path to polecats/<name>/ directory
}

// ScanCompletedPolecats finds polecats that have completed work (bead closed,
// session idle) and reaps them: kills tmux session, removes worktree, logs event.
//
// A polecat is considered completed when:
//  1. A tmux session exists for the polecat
//  2. The polecat has no hooked bead, OR its hooked bead is closed
//  3. The agent process is not running (session is idle)
//
// This is the counterpart to gt sling --create: sling creates, deacon reaps.
func ScanCompletedPolecats(townRoot string, cfg *ReapConfig) (*ReapScanResult, error) {
	if cfg == nil {
		cfg = DefaultReapConfig()
	}

	result := &ReapScanResult{
		ScannedAt: time.Now().UTC(),
		Results:   make([]*ReapResult, 0),
	}

	// Discover all polecat directories across rigs
	dirs := listPolecatDirs(townRoot)
	result.TotalPolecats = len(dirs)

	t := tmux.NewTmux()

	for _, dir := range dirs {
		prefix := session.PrefixFor(dir.Rig)
		sessionName := session.PolecatSessionName(prefix, dir.Polecat)

		// 1. Check if tmux session exists
		alive, _ := t.HasSession(sessionName)
		if !alive {
			// No session — nothing to reap. Worktree-only cleanup is out of scope
			// (that's polecat nuke territory).
			continue
		}

		// 2. Check if agent is still running (not just a shell)
		if t.IsAgentAlive(sessionName) {
			continue // Agent actively working — don't reap
		}

		// 3. Check bead status — is the work done?
		beadID, beadStatus := getPolecatBeadStatus(townRoot, dir.Rig, dir.Polecat)
		if beadID != "" && !isClosedStatus(beadStatus) {
			continue // Bead still open — work not done
		}

		// Polecat is completed: session exists, agent not running, bead closed or absent
		result.Completed++

		reapResult := &ReapResult{
			Rig:         dir.Rig,
			Polecat:     dir.Polecat,
			SessionName: sessionName,
			BeadID:      beadID,
			BeadStatus:  beadStatus,
		}

		// Check worktree state before cleanup
		worktreePath := polecatWorktreePath(townRoot, dir.Rig, dir.Polecat)
		if worktreePath != "" {
			checkReapWorktreeState(worktreePath, reapResult)
		}

		if cfg.DryRun {
			result.Results = append(result.Results, reapResult)
			continue
		}

		// Kill tmux session
		if err := t.KillSessionWithProcesses(sessionName); err != nil {
			reapResult.Error = fmt.Sprintf("killing session: %v", err)
			result.Results = append(result.Results, reapResult)
			continue
		}
		reapResult.SessionKilled = true

		// Remove worktree directory (polecats/<name>/)
		polecatDirPath := filepath.Join(townRoot, dir.Rig, "polecats", dir.Polecat)
		if reapResult.PartialWork {
			// Don't remove worktree with uncommitted/unpushed work — log and skip
			reapResult.WorktreeRemoved = false
		} else if worktreePath != "" {
			if err := removePolecatWorktree(worktreePath, polecatDirPath); err != nil {
				reapResult.Error = fmt.Sprintf("removing worktree: %v", err)
			} else {
				reapResult.WorktreeRemoved = true
			}
		}

		// Log reap event
		_ = events.LogFeed(events.TypeReap, "deacon", events.ReapPayload(
			dir.Rig, dir.Polecat, beadID, reapResult.SessionKilled, reapResult.WorktreeRemoved,
		))

		result.Reaped++
		result.Results = append(result.Results, reapResult)
	}

	return result, nil
}

// listPolecatDirs discovers all polecat directories across rigs in the town.
// It scans townRoot for directories that contain a polecats/ subdirectory.
func listPolecatDirs(townRoot string) []polecatDir {
	var dirs []polecatDir

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		polecatsPath := filepath.Join(townRoot, rigName, "polecats")

		polecatEntries, err := os.ReadDir(polecatsPath)
		if err != nil {
			continue // No polecats directory — not a rig or no polecats
		}

		for _, pe := range polecatEntries {
			if !pe.IsDir() || strings.HasPrefix(pe.Name(), ".") {
				continue
			}
			dirs = append(dirs, polecatDir{
				Rig:     rigName,
				Polecat: pe.Name(),
				Path:    filepath.Join(polecatsPath, pe.Name()),
			})
		}
	}

	return dirs
}

// polecatWorktreePath resolves the git worktree path for a polecat.
// Supports both new structure (rig/polecats/<name>/<rigname>/) and
// old structure (rig/polecats/<name>/).
// Returns "" if no worktree is found.
func polecatWorktreePath(townRoot, rigName, polecatName string) string {
	// New structure: rig/polecats/<name>/<rigname>/
	newPath := filepath.Join(townRoot, rigName, "polecats", polecatName, rigName)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(newPath, ".git")); err == nil {
			return newPath
		}
	}

	// Old structure: rig/polecats/<name>/
	oldPath := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(oldPath, ".git")); err == nil {
			return oldPath
		}
	}

	return ""
}

// getPolecatBeadStatus queries the agent bead for a polecat to get its hook bead status.
// Returns (beadID, status) where beadID is the hooked bead and status is its current state.
// Returns ("", "") if no bead info is available.
func getPolecatBeadStatus(townRoot, rigName, polecatName string) (string, string) {
	// Query bd for the agent bead's hook_bead field
	assignee := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	cmd := exec.Command("bd", "list", "--assignee="+assignee, "--json", "--flat", "--limit=1")
	cmd.Dir = townRoot
	util.SetDetachedProcessGroup(cmd)

	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 || string(trimmed) == "null" || (trimmed[0] != '[' && trimmed[0] != '{') {
		return "", ""
	}

	var beads []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(trimmed, &beads); err != nil || len(beads) == 0 {
		return "", ""
	}

	return beads[0].ID, beads[0].Status
}

// isClosedStatus returns true if the bead status indicates the work is complete.
func isClosedStatus(status string) bool {
	switch status {
	case "closed", "done", "merged":
		return true
	default:
		return false
	}
}

// checkReapWorktreeState checks a worktree for uncommitted/unpushed work
// and populates the result fields. Best-effort — errors don't prevent reaping.
func checkReapWorktreeState(worktreePath string, result *ReapResult) {
	g := git.NewGit(worktreePath)
	workStatus, err := g.CheckUncommittedWork()
	if err != nil {
		return
	}

	if !workStatus.CleanExcludingBeads() {
		result.PartialWork = true
		result.WorktreeDirty = workStatus.HasUncommittedChanges
		result.UnpushedCount = workStatus.UnpushedCommits
	}
}

// removePolecatWorktree removes the git worktree and its parent polecat directory.
// Handles both new structure (polecats/<name>/<rigname>/) and old structure
// (polecats/<name>/) by removing the worktree first, then cleaning up the
// parent directory if empty.
func removePolecatWorktree(worktreePath, polecatDirPath string) error {
	// Try git worktree remove first for clean cleanup
	g := git.NewGit(worktreePath)
	if err := g.WorktreeRemove(worktreePath, true); err != nil {
		// Fallback: force remove the directory
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("removing worktree directory: %w", err)
		}
	}

	// Clean up the polecat directory if it's now empty
	// (new structure: polecats/<name>/ may now be empty after removing polecats/<name>/<rig>/)
	if polecatDirPath != worktreePath {
		entries, err := os.ReadDir(polecatDirPath)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(polecatDirPath) // Best-effort cleanup of empty dir
		}
	} else {
		// Old structure: worktree IS the polecat dir, already removed
	}

	return nil
}
