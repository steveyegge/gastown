package cmd

import (
	"fmt"
	"os/exec"
	"sort"

	"github.com/spf13/cobra"
)

// townCycleSession is the --session flag for town next/prev commands.
// When run via tmux key binding (run-shell), the session context may not be
// correct, so we pass the session name explicitly via #{session_name} expansion.
var townCycleSession string

// getTownLevelSessions returns the town-level session names for the current workspace.
func getTownLevelSessions() []string {
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()
	return []string{mayorSession, deaconSession}
}

// isTownLevelSession checks if the given session name is a town-level session.
// Town-level sessions (Mayor, Deacon) use the "hq-" prefix, so we can identify
// them by name alone without requiring workspace context. This is critical for
// tmux run-shell which may execute from outside the workspace directory.
func isTownLevelSession(sessionName string) bool {
	// Town-level sessions are identified by their fixed names
	mayorSession := getMayorSessionName()  // "hq-mayor"
	deaconSession := getDeaconSessionName() // "hq-deacon"
	return sessionName == mayorSession || sessionName == deaconSession
}

func init() {
	rootCmd.AddCommand(townCmd)
	townCmd.AddCommand(townNextCmd)
	townCmd.AddCommand(townPrevCmd)

	townNextCmd.Flags().StringVar(&townCycleSession, "session", "", "Override current session (used by tmux binding)")
	townPrevCmd.Flags().StringVar(&townCycleSession, "session", "", "Override current session (used by tmux binding)")
}

var townCmd = &cobra.Command{
	Use:   "town",
	Short: "Town-level operations",
	Long:  `Commands for town-level operations including session cycling.`,
}

var townNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Switch to next town session (mayor/deacon)",
	Long: `Switch to the next town-level session in the cycle order.
Town sessions cycle between Mayor and Deacon.

This command is typically invoked via the C-b n keybinding when in a
town-level session (Mayor or Deacon).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleTownSession(1, townCycleSession)
	},
}

var townPrevCmd = &cobra.Command{
	Use:   "prev",
	Short: "Switch to previous town session (mayor/deacon)",
	Long: `Switch to the previous town-level session in the cycle order.
Town sessions cycle between Mayor and Deacon.

This command is typically invoked via the C-b p keybinding when in a
town-level session (Mayor or Deacon).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleTownSession(-1, townCycleSession)
	},
}

// cycleTownSession switches to the next or previous town-level session.
// direction: 1 for next, -1 for previous
// sessionOverride: if non-empty, use this instead of detecting current session
func cycleTownSession(direction int, sessionOverride string) error {
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

	// Check if current session is a town-level session
	if !isTownLevelSession(currentSession) {
		// Not a town session - no cycling, just stay put
		return nil
	}

	// Find running town sessions
	sessions, err := findRunningTownSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no town sessions found")
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
		return fmt.Errorf("current session not found in town session list")
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

// findRunningTownSessions returns a list of currently running town-level sessions.
func findRunningTownSessions() ([]string, error) {
	// Get all tmux sessions
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing tmux sessions: %w", err)
	}

	// Get town-level session names
	townLevelSessions := getTownLevelSessions()
	if townLevelSessions == nil {
		return nil, fmt.Errorf("cannot determine town-level sessions")
	}

	var running []string
	for _, line := range splitLines(string(out)) {
		if line == "" {
			continue
		}
		// Check if this is a town-level session
		for _, townSession := range townLevelSessions {
			if line == townSession {
				running = append(running, line)
				break
			}
		}
	}

	return running, nil
}
