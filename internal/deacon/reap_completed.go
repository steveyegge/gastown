// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	// beadStatusTimeout is the maximum time to wait for a bd subprocess call.
	// If bd hangs (e.g., waiting for a Dolt lock), this prevents the daemon's
	// main goroutine from blocking forever and halting all event processing.
	beadStatusTimeout = 30 * time.Second
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
	BeadsSynced     bool   `json:"beads_synced,omitempty"`
	WorktreeRemoved bool   `json:"worktree_removed"`
	PartialWork     bool   `json:"partial_work,omitempty"`
	WorktreeDirty   bool   `json:"worktree_dirty,omitempty"`
	UnpushedCount   int    `json:"unpushed_count,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ReapDecision captures the reaper's decision for a single polecat.
// Every scanned polecat gets exactly one decision entry, regardless of outcome.
// This provides the visibility needed to diagnose why polecats are or aren't reaped.
type ReapDecision struct {
	Rig        string `json:"rig"`
	Polecat    string `json:"polecat"`
	Eligible   bool   `json:"eligible"`
	Reason     string `json:"reason"`      // no_session, agent_alive, bead_open, bead_query_error, bead_closed, no_bead
	HasSession bool   `json:"has_session"`
	AgentAlive bool   `json:"agent_alive,omitempty"`
	BeadID     string `json:"bead_id,omitempty"`
	BeadStatus string `json:"bead_status,omitempty"`
}

// ReapScanResult contains the full results of a completed polecat scan.
type ReapScanResult struct {
	ScannedAt     time.Time       `json:"scanned_at"`
	TotalPolecats int             `json:"total_polecats"`
	Completed     int             `json:"completed"`
	Reaped        int             `json:"reaped"`
	Results       []*ReapResult   `json:"results"`
	Decisions     []*ReapDecision `json:"decisions"`
}

// polecatDir holds information about a discovered polecat directory.
type polecatDir struct {
	Rig     string
	Polecat string
	Path    string // path to polecats/<name>/ directory
}

// ScanCompletedPolecats finds polecats that have completed work (bead closed,
// session idle) and reaps them: kills tmux session, removes worktree, logs event.
// Uses a background context with default timeout for subprocess calls.
func ScanCompletedPolecats(townRoot string, cfg *ReapConfig) (*ReapScanResult, error) {
	return ScanCompletedPolecatsCtx(context.Background(), townRoot, cfg)
}

// ScanCompletedPolecatsCtx is the context-aware variant of ScanCompletedPolecats.
// The context is used for subprocess timeouts (bd queries, git operations).
//
// A polecat is considered completed when:
//  1. A tmux session exists for the polecat
//  2. The polecat has no hooked bead, OR its hooked bead is closed
//  3. The agent process is not running (session is idle)
//
// This is the counterpart to gt sling --create: sling creates, deacon reaps.
func ScanCompletedPolecatsCtx(ctx context.Context, townRoot string, cfg *ReapConfig) (*ReapScanResult, error) {
	if cfg == nil {
		cfg = DefaultReapConfig()
	}

	result := &ReapScanResult{
		ScannedAt: time.Now().UTC(),
		Results:   make([]*ReapResult, 0),
		Decisions: make([]*ReapDecision, 0),
	}

	// Discover all polecat directories across rigs
	dirs := listPolecatDirs(townRoot)
	result.TotalPolecats = len(dirs)

	t := tmux.NewTmux()

	for _, dir := range dirs {
		// Check context before each polecat to bail early on shutdown/timeout
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		prefix := session.PrefixFor(dir.Rig)
		sessionName := session.PolecatSessionName(prefix, dir.Polecat)

		// 1. Check if tmux session exists
		alive, _ := t.HasSession(sessionName)
		if !alive {
			// No session — polecat self-exited. Check for a bead to trigger
			// archivist extraction and worktree cleanup.
			beadID, beadStatus, beadErr := getPolecatBeadStatusCtx(ctx, townRoot, dir.Rig, dir.Polecat)
			if beadErr == nil && beadID != "" {
				// Polecat had a bead (open or closed). Mark as completed so the
				// daemon can trigger the archivist for knowledge extraction.
				reason := "no_session_bead_open"
				if isClosedStatus(beadStatus) {
					reason = "no_session_bead_closed"
				}
				result.Decisions = append(result.Decisions, &ReapDecision{
					Rig: dir.Rig, Polecat: dir.Polecat,
					Eligible: true,
					BeadID: beadID, BeadStatus: beadStatus,
					Reason: reason,
				})
				result.Completed++
				reapResult := &ReapResult{
					Rig:           dir.Rig,
					Polecat:       dir.Polecat,
					SessionName:   sessionName,
					BeadID:        beadID,
					BeadStatus:    beadStatus,
					SessionKilled: false, // Session was already gone
				}
				// Clean up worktree if no partial work
				worktreePath := polecatWorktreePath(townRoot, dir.Rig, dir.Polecat)
				if worktreePath != "" {
					checkReapWorktreeState(worktreePath, reapResult)
				}
				if !cfg.DryRun && !reapResult.PartialWork && worktreePath != "" {
					polecatDirPath := filepath.Join(townRoot, dir.Rig, "polecats", dir.Polecat)
					if err := removePolecatWorktree(worktreePath, polecatDirPath); err != nil {
						reapResult.Error = fmt.Sprintf("removing worktree: %v", err)
					} else {
						reapResult.WorktreeRemoved = true
					}
					_ = events.LogFeed(events.TypeReap, "deacon", events.ReapPayload(
						dir.Rig, dir.Polecat, beadID, false, reapResult.WorktreeRemoved,
					))
					result.Reaped++
				}
				result.Results = append(result.Results, reapResult)
				continue
			}
			result.Decisions = append(result.Decisions, &ReapDecision{
				Rig: dir.Rig, Polecat: dir.Polecat,
				Reason: "no_session",
			})
			continue
		}

		// 2. Check if agent is still running (not just a shell)
		agentAlive := t.IsAgentAlive(sessionName)

		// 3. Check bead status — is the work done?
		beadID, beadStatus, beadErr := getPolecatBeadStatusCtx(ctx, townRoot, dir.Rig, dir.Polecat)
		if beadErr != nil {
			// Can't determine bead status — don't reap (could be a transient failure).
			// Previously this silently returned ("","") which treated bd failures as
			// "no bead" and incorrectly made the polecat eligible for reaping.
			result.Decisions = append(result.Decisions, &ReapDecision{
				Rig: dir.Rig, Polecat: dir.Polecat,
				HasSession: true,
				Reason: "bead_query_error",
			})
			result.Results = append(result.Results, &ReapResult{
				Rig:     dir.Rig,
				Polecat: dir.Polecat,
				Error:   fmt.Sprintf("bead query failed: %v", beadErr),
			})
			continue
		}
		if beadID != "" && !isClosedStatus(beadStatus) {
			// Bead still open — if agent is alive, it's actively working
			if agentAlive {
				result.Decisions = append(result.Decisions, &ReapDecision{
					Rig: dir.Rig, Polecat: dir.Polecat,
					HasSession: true, AgentAlive: true,
					BeadID: beadID, BeadStatus: beadStatus,
					Reason: "agent_alive",
				})
				continue
			}
			result.Decisions = append(result.Decisions, &ReapDecision{
				Rig: dir.Rig, Polecat: dir.Polecat,
				HasSession: true, BeadID: beadID, BeadStatus: beadStatus,
				Reason: "bead_open",
			})
			continue // Bead still open — work not done
		}

		// Bead is closed or absent. If agent is alive, only reap if idle at prompt.
		// This handles the dispatch-and-kill case: polecat closes its bead but
		// gt done fails to terminate the session (timeout, merge queue error, etc.).
		if agentAlive && !t.IsIdle(sessionName) {
			result.Decisions = append(result.Decisions, &ReapDecision{
				Rig: dir.Rig, Polecat: dir.Polecat,
				HasSession: true, AgentAlive: true,
				BeadID: beadID, BeadStatus: beadStatus,
				Reason: "agent_alive_bead_closed",
			})
			continue // Agent is doing something despite bead being closed — let it finish
		}

		// Polecat is completed: bead closed/absent, agent not running or idle at prompt
		decisionReason := "no_bead"
		if beadID != "" {
			decisionReason = "bead_closed"
			if agentAlive {
				decisionReason = "bead_closed_agent_idle"
			}
		}
		result.Decisions = append(result.Decisions, &ReapDecision{
			Rig: dir.Rig, Polecat: dir.Polecat,
			Eligible: true, HasSession: true,
			AgentAlive: agentAlive,
			BeadID: beadID, BeadStatus: beadStatus,
			Reason: decisionReason,
		})
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

		// Sync beads from rig to town root before removing the worktree.
		if err := syncBeadsToTown(townRoot, dir.Rig); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: beads sync failed for %s/%s: %v\n", dir.Rig, dir.Polecat, err)
		} else {
			reapResult.BeadsSynced = true
		}

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
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Use os.Stat (not entry.IsDir) to follow symlinks. In production,
		// rig directories at the town root are symlinks — entry.IsDir()
		// returns false for symlinks, which caused the reaper to skip all rigs.
		entryPath := filepath.Join(townRoot, entry.Name())
		info, err := os.Stat(entryPath)
		if err != nil || !info.IsDir() {
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
// Uses a default timeout to prevent blocking the daemon indefinitely.
func getPolecatBeadStatus(townRoot, rigName, polecatName string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), beadStatusTimeout)
	defer cancel()
	return getPolecatBeadStatusCtx(ctx, townRoot, rigName, polecatName)
}

// getPolecatBeadStatusCtx queries the agent bead for a polecat to get its hook bead status.
// Returns (beadID, status, err) where beadID is the hooked bead and status is its current state.
// Returns ("", "", nil) if no bead is assigned. Returns non-nil error if the query itself failed.
// The context controls the subprocess timeout — if bd hangs (e.g., waiting for a Dolt lock),
// the context deadline prevents the caller from blocking forever.
func getPolecatBeadStatusCtx(ctx context.Context, townRoot, rigName, polecatName string) (string, string, error) {
	// Query bd for the agent bead's hook_bead field
	assignee := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	cmd := exec.CommandContext(ctx, "bd", "list", "--assignee="+assignee, "--all", "--json", "--flat", "--limit=1")
	cmd.Dir = townRoot
	// Set BEADS_DIR explicitly so bd finds the correct database regardless of
	// the daemon's inherited environment. Matches pattern in plugin/recording.go.
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(townRoot))
	util.SetDetachedProcessGroup(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("bd list --assignee=%s: %w (stderr: %s)", assignee, err, strings.TrimSpace(stderr.String()))
	}

	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 || string(trimmed) == "null" || (trimmed[0] != '[' && trimmed[0] != '{') {
		return "", "", nil
	}

	var beadResults []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(trimmed, &beadResults); err != nil {
		return "", "", fmt.Errorf("parsing bd output: %w", err)
	}
	if len(beadResults) == 0 {
		return "", "", nil
	}

	return beadResults[0].ID, beadResults[0].Status, nil
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

	if !workStatus.CleanExcludingRuntime() {
		result.PartialWork = true
		result.WorktreeDirty = workStatus.HasUncommittedChanges
		result.UnpushedCount = workStatus.UnpushedCommits
	}
}

// removePolecatWorktree removes the git worktree and its parent polecat directory.
// syncBeadsToTown runs `bd repo sync` at the town root to pull rig-level bead
// changes into the town-root .beads/ database. This is the town-root sync step
// of the per-rig beads architecture: polecats work in rig-local .beads/ and the
// reaper syncs to the mayor's global view on completion.
func syncBeadsToTown(townRoot, rigName string) error {
	rigBeadsDir := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		return nil // No rig-level beads database — nothing to sync
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); os.IsNotExist(err) {
		return nil // No town-root beads database — nothing to sync into
	}

	cmd := exec.Command("bd", "repo", "sync")
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+townBeadsDir)
	util.SetDetachedProcessGroup(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd repo sync: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

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
