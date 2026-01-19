// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

// TownSessionInfo represents metadata about a town-level tmux session.
type TownSessionInfo struct {
	Name      string // Display name (e.g., "Mayor")
	SessionID string // Tmux session ID (e.g., "hq-mayor")
}

// TownSessionInfos returns the list of town-level sessions in shutdown order.
// Order matters: Boot (Deacon's watchdog) must be stopped before Deacon,
// otherwise Boot will try to restart Deacon.
func TownSessionInfos() []TownSessionInfo {
	return []TownSessionInfo{
		{"Mayor", MayorSessionName()},
		{"Boot", BootSessionName},
		{"Deacon", DeaconSessionName()},
	}
}

// StopTownSessionInfo stops a single town-level tmux session.
// If force is true, skips graceful shutdown (Ctrl-C) and kills immediately.
// Returns true if the session was running and stopped, false if not running.
func StopTownSessionInfo(sess Sessions, ts TownSessionInfo, force bool) (bool, error) {
	id := SessionID(ts.SessionID)
	running, err := sess.Exists(id)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	// Try graceful shutdown first (unless forced)
	if !force {
		_ = sess.SendControl(id, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Log pre-death event for crash investigation (before killing)
	reason := "user shutdown"
	if force {
		reason = "forced shutdown"
	}
	_ = events.LogFeed(events.TypeSessionDeath, ts.SessionID,
		events.SessionDeathPayload(ts.SessionID, ts.Name, reason, "gt down"))

	// Kill the session
	if err := sess.Stop(id); err != nil {
		return false, fmt.Errorf("killing %s session: %w", ts.Name, err)
	}

	return true, nil
}
