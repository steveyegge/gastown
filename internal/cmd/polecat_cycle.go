package cmd

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// cyclePolecatSession switches to the next or previous polecat session in the same rig.
// direction: 1 for next, -1 for previous
// sessionOverride: if non-empty, use this instead of detecting current session
func cyclePolecatSession(direction int, sessionOverride string) error {
	var currentSession string
	var err error

	if sessionOverride != "" {
		currentSession = sessionOverride
	} else {
		currentSession, err = getCurrentTmuxSession()
		if err != nil {
			return fmt.Errorf("not in a tmux session: %w", err)
		}
		if currentSession == "" {
			return fmt.Errorf("not in a tmux session")
		}
	}

	// Parse rig name from current session
	rigName, _, ok := parsePolecatSessionName(currentSession)
	if !ok {
		// Not a polecat session - no cycling
		return nil
	}

	// Find all polecat sessions for this rig
	sessions, err := findRigPolecatSessions(rigName)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil // No polecat sessions
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
		return nil
	}

	// Calculate target index (with wrapping)
	targetIdx := (currentIdx + direction + len(sessions)) % len(sessions)

	if targetIdx == currentIdx {
		// Only one session, nothing to switch to
		return nil
	}

	targetSession := sessions[targetIdx]

	// Switch to target session
	cmd := exec.Command("tmux", "switch-client", "-t", targetSession)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("switching to %s: %w", targetSession, err)
	}

	return nil
}

// parsePolecatSessionName extracts rig and polecat name from a tmux session name.
// Format: gt-<rig>-<name> where name is NOT crew-*, witness, or refinery.
// Returns empty strings and false if the format doesn't match.
func parsePolecatSessionName(sessionName string) (rigName, polecatName string, ok bool) { //nolint:unparam // polecatName kept for API consistency
	// Must start with "gt-"
	if !strings.HasPrefix(sessionName, "gt-") {
		return "", "", false
	}

	// Exclude town-level sessions by exact match
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()
	if sessionName == mayorSession || sessionName == deaconSession {
		return "", "", false
	}

	// Also exclude by suffix pattern (gt-{town}-mayor, gt-{town}-deacon)
	// This handles cases where town config isn't available
	if strings.HasSuffix(sessionName, "-mayor") || strings.HasSuffix(sessionName, "-deacon") {
		return "", "", false
	}

	// Remove "gt-" prefix
	rest := sessionName[3:]

	// Must have at least one hyphen (rig-name)
	idx := strings.Index(rest, "-")
	if idx == -1 {
		return "", "", false
	}

	rigName = rest[:idx]
	polecatName = rest[idx+1:]

	if rigName == "" || polecatName == "" {
		return "", "", false
	}

	// Exclude crew sessions (contain "crew-" prefix in the name part)
	if strings.HasPrefix(polecatName, "crew-") {
		return "", "", false
	}

	// Exclude rig infra sessions
	if polecatName == "witness" || polecatName == "refinery" {
		return "", "", false
	}

	return rigName, polecatName, true
}

// findRigPolecatSessions returns all polecat sessions for a given rig.
// Uses tmux list-sessions to find sessions matching gt-<rig>-<name> pattern,
// excluding crew, witness, and refinery sessions.
func findRigPolecatSessions(rigName string) ([]string, error) { //nolint:unparam // error return kept for future use
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No tmux server or no sessions
		return nil, nil
	}

	prefix := fmt.Sprintf("gt-%s-", rigName)
	var sessions []string

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		// Verify this is actually a polecat session
		_, _, ok := parsePolecatSessionName(line)
		if ok {
			sessions = append(sessions, line)
		}
	}

	return sessions, nil
}
