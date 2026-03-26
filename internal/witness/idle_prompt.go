// Package witness provides idle prompt detection for polecat sessions.
// This file implements the two-phase watchdog that detects polecats stuck at
// the Claude Code idle ❯ prompt with unfinished hooked work (gas-xrp).
package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/tmux"
)

// idlePromptRecord tracks idle-at-prompt state for a single polecat session
// across patrol cycles.
type idlePromptRecord struct {
	// FirstSeenIdle is when we first observed the session at the idle prompt.
	FirstSeenIdle time.Time `json:"first_seen_idle"`

	// NudgeSent is true if we have already sent a "run gt done" nudge.
	NudgeSent bool `json:"nudge_sent"`

	// NudgeSentAt is when the nudge was sent (zero if not yet sent).
	NudgeSentAt time.Time `json:"nudge_sent_at,omitempty"`

	// HookBead is the bead that was on the hook when idle was first detected.
	HookBead string `json:"hook_bead"`
}

// idlePromptState persists idle-prompt detection records across patrol cycles.
type idlePromptState struct {
	// Sessions maps tmux session name → record.
	Sessions    map[string]*idlePromptRecord `json:"sessions"`
	LastUpdated time.Time                   `json:"last_updated"`
}

func idlePromptStateFile(townRoot string) string {
	return filepath.Join(townRoot, "witness", "idle-prompt-state.json")
}

func loadIdlePromptState(townRoot string) *idlePromptState {
	data, err := os.ReadFile(idlePromptStateFile(townRoot)) //nolint:gosec // G304: path from trusted townRoot
	if err != nil {
		return &idlePromptState{Sessions: make(map[string]*idlePromptRecord)}
	}
	var state idlePromptState
	if err := json.Unmarshal(data, &state); err != nil {
		return &idlePromptState{Sessions: make(map[string]*idlePromptRecord)}
	}
	if state.Sessions == nil {
		state.Sessions = make(map[string]*idlePromptRecord)
	}
	return &state
}

func saveIdlePromptState(townRoot string, state *idlePromptState) error {
	stateFile := idlePromptStateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating witness dir: %w", err)
	}
	state.LastUpdated = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling idle prompt state: %w", err)
	}
	return os.WriteFile(stateFile, data, 0600) //nolint:gosec // G306: state file with restricted permissions
}

// clearIdlePromptRecord removes the idle-prompt record for a session.
// Called when a session transitions out of idle (polecat resumed work).
func clearIdlePromptRecord(townRoot, sessionName string) {
	state := loadIdlePromptState(townRoot)
	if _, exists := state.Sessions[sessionName]; !exists {
		return
	}
	delete(state.Sessions, sessionName)
	_ = saveIdlePromptState(townRoot, state) // non-fatal: stale record is harmless
}

// polecatWorktreeStatus returns "clean", "dirty", or "unknown" for the polecat's
// git worktree. Used to include worktree context in the mayor notification.
func polecatWorktreeStatus(townRoot, rigName, polecatName string) string {
	// Try new worktree layout first: polecats/<name>/<rig>/
	polecatPath := filepath.Join(townRoot, rigName, "polecats", polecatName, rigName)
	if _, err := os.Stat(polecatPath); os.IsNotExist(err) {
		polecatPath = filepath.Join(townRoot, rigName, "polecats", polecatName)
	}
	if _, err := os.Stat(polecatPath); os.IsNotExist(err) {
		return "unknown"
	}

	g := git.NewGit(polecatPath)
	dirty, err := g.HasUncommittedChanges()
	if err != nil {
		return "unknown"
	}
	if dirty {
		return "dirty"
	}
	return "clean"
}

// detectIdlePrompt checks whether a polecat with a live session and live agent
// process is stuck at the Claude Code idle ❯ prompt with unfinished hooked work.
//
// Two-phase algorithm:
//
//	Phase 1 (first detection): session is at idle prompt with hook_bead set.
//	  Records first-seen time; if grace period elapsed, nudges polecat.
//	Phase 2 (confirmed stuck): polecat is still idle after IdlePromptThreshold
//	  from when the nudge was sent. Returns ZombieAtIdlePrompt for notification.
//
// Returns (ZombieResult, true) if the polecat needs attention, (ZombieResult{}, false)
// if everything looks healthy or we're still within the wait windows.
func detectIdlePrompt(workDir, townRoot, rigName, polecatName, sessionName, hookBead string,
	t *tmux.Tmux, witCfg *config.WitnessThresholds) (ZombieResult, bool) {

	if hookBead == "" {
		return ZombieResult{}, false
	}

	if !t.IsIdle(sessionName) {
		// Polecat is actively working — clear any stale idle record.
		clearIdlePromptRecord(townRoot, sessionName)
		return ZombieResult{}, false
	}

	// Polecat is at idle prompt with a hooked bead. Apply two-phase detection.
	state := loadIdlePromptState(townRoot)
	rec, exists := state.Sessions[sessionName]
	now := time.Now().UTC()

	grace := witCfg.IdlePromptGraceD()
	threshold := witCfg.IdlePromptThresholdD()

	if !exists {
		// Phase 1 first time: record idle start. Don't nudge yet — wait for grace.
		rec = &idlePromptRecord{
			FirstSeenIdle: now,
			HookBead:      hookBead,
		}
		state.Sessions[sessionName] = rec
		_ = saveIdlePromptState(townRoot, state)
		// Nothing to report yet — just tracking.
		return ZombieResult{}, false
	}

	idleAge := now.Sub(rec.FirstSeenIdle)

	if idleAge < grace {
		// Still within grace window. Do nothing.
		return ZombieResult{}, false
	}

	// Grace period has elapsed. If we haven't nudged yet, do so now.
	if !rec.NudgeSent {
		nudgeMsg := fmt.Sprintf(
			"Your session appears idle at the ❯ prompt with work on hook (%s). "+
				"If you have finished your work, run `gt done` to submit it. "+
				"(idle for %v)",
			hookBead, idleAge.Round(time.Second))

		if nudgeErr := t.NudgeSession(sessionName, nudgeMsg); nudgeErr == nil {
			rec.NudgeSent = true
			rec.NudgeSentAt = now
		}
		_ = saveIdlePromptState(townRoot, state)

		return ZombieResult{
			PolecatName:    polecatName,
			AgentState:     "active",
			Classification: ZombieAtIdlePromptNudged,
			HookBead:       hookBead,
			WasActive:      true,
			Action: fmt.Sprintf("nudged-idle-polecat (idle=%v, hook=%s)",
				idleAge.Round(time.Second), hookBead),
		}, true
	}

	// Nudge was already sent. Check whether IdlePromptThreshold has elapsed.
	timeSinceNudge := now.Sub(rec.NudgeSentAt)
	if timeSinceNudge < threshold {
		// Still waiting for nudge to take effect.
		return ZombieResult{}, false
	}

	// Confirmed stuck: polecat was nudged but is still at idle prompt.
	// Report to patrol so --notify can alert the mayor.
	worktreeStatus := polecatWorktreeStatus(townRoot, rigName, polecatName)
	action := fmt.Sprintf(
		"confirmed-idle-prompt-stuck (idle=%v, nudge-age=%v, worktree=%s, hook=%s)",
		idleAge.Round(time.Second), timeSinceNudge.Round(time.Second),
		worktreeStatus, hookBead)

	// Clear state so we don't fire repeatedly. Next patrol will re-detect
	// if the polecat is still stuck.
	delete(state.Sessions, sessionName)
	_ = saveIdlePromptState(townRoot, state)

	return ZombieResult{
		PolecatName:    polecatName,
		AgentState:     "active",
		Classification: ZombieAtIdlePrompt,
		HookBead:       hookBead,
		WasActive:      true,
		Action:         action,
	}, true
}
