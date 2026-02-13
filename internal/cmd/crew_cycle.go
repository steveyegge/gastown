package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// crewCycleSession is the --session flag for crew next/prev commands.
// When run via tmux key binding (run-shell), the session context may not be
// correct, so we pass the session name explicitly via #{session_name} expansion.
var crewCycleSession string

// cycleCrewSession switches to the next or previous crew session in the same rig.
// direction: 1 for next, -1 for previous
// sessionOverride: if non-empty, use this instead of detecting current session
func cycleCrewSession(direction int, sessionOverride string) error {
	var currentSession string
	var err error

	if sessionOverride != "" {
		// Use the provided session name (from tmux key binding)
		currentSession = sessionOverride
	} else {
		// Get current session (uses existing function from handoff.go)
		currentSession, err = getCurrentTmuxSession()
		if err != nil {
			return fmt.Errorf("not in a tmux session: %w", err)
		}
		if currentSession == "" {
			return fmt.Errorf("not in a tmux session")
		}
	}

	// Parse rig name from current session
	rigName, _, ok := parseCrewSessionName(currentSession)
	if !ok {
		// Not a crew session (e.g., Mayor, Witness, Refinery) - no cycling, just stay put
		return nil
	}

	// Find all crew sessions for this rig
	sessions, err := findRigCrewSessions(rigName)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no crew sessions found for rig %s", rigName)
	}

	// Sort for consistent ordering
	sort.Strings(sessions)

	// Find current position
	currentIdx := -1
	for i, s := range sessions {
		if s == currentSession {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		// Current session not in list (shouldn't happen)
		return fmt.Errorf("current session not found in crew list")
	}

	// Calculate target index (with wrapping)
	targetIdx := (currentIdx + direction + len(sessions)) % len(sessions)

	if targetIdx == currentIdx {
		// Only one session, nothing to switch to
		return nil
	}

	targetSession := sessions[targetIdx]

	// SwitchClient is a tmux-only UI operation (no equivalent in K8s/Coop)
	return fmt.Errorf("session cycling requires tmux (not available in K8s): target %s", targetSession)
}

func runCrewNext(cmd *cobra.Command, args []string) error {
	return cycleCrewSession(1, crewCycleSession)
}

func runCrewPrev(cmd *cobra.Command, args []string) error {
	return cycleCrewSession(-1, crewCycleSession)
}
