// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/tmux"
)

// TownSession represents a town-level tmux session.
type TownSession struct {
	Name      string // Display name (e.g., "Mayor")
	SessionID string // Tmux session ID (e.g., "gt-mayor")
}

// TownSessions returns the list of town-level sessions in shutdown order.
// Order matters: Boot (Deacon's watchdog) must be stopped before Deacon,
// otherwise Boot will try to restart Deacon.
func TownSessions() []TownSession {
	return []TownSession{
		{"Mayor", MayorSessionName()},
		{"Boot", boot.SessionName},
		{"Deacon", DeaconSessionName()},
	}
}

// StopTownSession stops a single town-level tmux session.
// If force is true, skips graceful shutdown (Ctrl-C) and kills immediately.
// Returns true if the session was running and stopped, false if not running.
func StopTownSession(t *tmux.Tmux, ts TownSession, force bool) (bool, error) {
	running, err := t.HasSession(ts.SessionID)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}

	// Try graceful shutdown first (unless forced)
	if !force {
		_ = t.SendKeysRaw(ts.SessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Kill the session
	if err := t.KillSession(ts.SessionID); err != nil {
		return false, fmt.Errorf("killing %s session: %w", ts.Name, err)
	}

	return true, nil
}
