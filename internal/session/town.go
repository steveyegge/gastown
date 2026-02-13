// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/terminal"
	"github.com/steveyegge/gastown/internal/tmux"
)

// TownSession represents a town-level tmux session.
type TownSession struct {
	Name      string // Display name (e.g., "Mayor")
	SessionID string // Tmux session ID (e.g., "hq-mayor")
}

// TownSessions returns the list of town-level sessions in shutdown order.
// Order matters: Boot (Deacon's watchdog) must be stopped before Deacon,
// otherwise Boot will try to restart Deacon.
func TownSessions() []TownSession {
	return []TownSession{
		{"Mayor", MayorSessionName()},
		{"Boot", BootSessionName()},
		{"Deacon", DeaconSessionName()},
	}
}

// StopTownSession stops a single town-level session.
// If force is true, skips graceful shutdown (Ctrl-C) and kills immediately.
// Returns true if the session was running and stopped, false if not running.
// Uses the provided backend for session liveness checks.
func StopTownSession(t *tmux.Tmux, ts TownSession, force bool, backend ...terminal.Backend) (bool, error) {
	var running bool
	var err error
	if len(backend) > 0 && backend[0] != nil {
		running, err = backend[0].HasSession(ts.SessionID)
	} else {
		running, err = t.HasSession(ts.SessionID)
	}
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	return stopTownSessionInternal(t, ts, force)
}

// StopTownSessionWithCache is like StopTownSession but uses a pre-fetched
// SessionSet for O(1) existence check instead of spawning a subprocess.
func StopTownSessionWithCache(t *tmux.Tmux, ts TownSession, force bool, cache *tmux.SessionSet) (bool, error) {
	if !cache.Has(ts.SessionID) {
		return false, nil
	}

	return stopTownSessionInternal(t, ts, force)
}

// stopTownSessionInternal performs the actual session stop.
func stopTownSessionInternal(t *tmux.Tmux, ts TownSession, force bool) (bool, error) {
	// Try graceful shutdown first (unless forced)
	if !force {
		_ = t.SendKeysRaw(ts.SessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Log pre-death event for crash investigation (before killing)
	reason := "user shutdown"
	if force {
		reason = "forced shutdown"
	}
	_ = events.LogFeed(events.TypeSessionDeath, ts.SessionID,
		events.SessionDeathPayload(ts.SessionID, ts.Name, reason, "gt down"))

	// Kill the session.
	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	if err := t.KillSessionWithProcesses(ts.SessionID); err != nil {
		return false, fmt.Errorf("killing %s session: %w", ts.Name, err)
	}

	return true, nil
}
