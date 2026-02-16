package cmd

import (
	"os/exec"
	"sort"

	"github.com/spf13/cobra"
	sessionpkg "github.com/steveyegge/gastown/internal/session"
)

// cycleSession is the --session flag for cycle next/prev commands.
// When run via tmux key binding (run-shell), the session context may not be
// correct, so we pass the session name explicitly via #{session_name} expansion.
var cycleSession string

func init() {
	rootCmd.AddCommand(cycleCmd)
	cycleCmd.AddCommand(cycleNextCmd)
	cycleCmd.AddCommand(cyclePrevCmd)

	cycleNextCmd.Flags().StringVar(&cycleSession, "session", "", "Override current session (used by tmux binding)")
	cyclePrevCmd.Flags().StringVar(&cycleSession, "session", "", "Override current session (used by tmux binding)")
}

var cycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "Cycle between sessions in the same group",
	Long: `Cycle between related tmux sessions based on the current session type.

Session groups:
- Town sessions: Mayor ↔ Deacon
- Crew sessions: All crew members in the same rig (e.g., greenplace/crew/max ↔ greenplace/crew/joe)
- Rig infra sessions: Witness ↔ Refinery (per rig)
- Polecat sessions: All polecats in the same rig (e.g., greenplace/Toast ↔ greenplace/Nux)

The appropriate cycling is detected automatically from the session name.

Examples:
  gt cycle next    # Switch to next session in group
  gt cycle prev    # Switch to previous session in group`,
}

var cycleNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Switch to next session in group",
	Long: `Switch to the next session in the current group.

This command is typically invoked via the C-b n keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.

Examples:
  gt cycle next
  gt cycle next --session gt-gastown-witness  # Explicit session context`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleToSession(1, cycleSession)
	},
}

var cyclePrevCmd = &cobra.Command{
	Use:   "prev",
	Short: "Switch to previous session in group",
	Long: `Switch to the previous session in the current group.

This command is typically invoked via the C-b p keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.

Examples:
  gt cycle prev
  gt cycle prev --session gt-gastown-witness  # Explicit session context`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleToSession(-1, cycleSession)
	},
}

// cycleToSession dispatches to the appropriate cycling function based on session type.
// direction: 1 for next, -1 for previous
// sessionOverride: if non-empty, use this instead of detecting current session
func cycleToSession(direction int, sessionOverride string) error {
	session := sessionOverride
	if session == "" {
		var err error
		session, err = getCurrentTmuxSession()
		if err != nil {
			return nil // Not in tmux, nothing to do
		}
	}

	// Check if it's a town-level session
	townLevelSessions := getTownLevelSessions()
	if townLevelSessions != nil {
		for _, townSession := range townLevelSessions {
			if session == townSession {
				return cycleTownSession(direction, session)
			}
		}
	}

	// Check if it's a crew session (format: <prefix>-crew-<name>)
	if identity, err := sessionpkg.ParseSessionName(session); err == nil && identity.Role == sessionpkg.RoleCrew {
		return cycleCrewSession(direction, session)
	}

	// Check if it's a rig infra session (witness or refinery)
	if rig := parseRigInfraSession(session); rig != "" {
		return cycleRigInfraSession(direction, session, rig)
	}

	// Check if it's a polecat session (gt-<rig>-<name>, not crew/witness/refinery)
	if rig, _, ok := parsePolecatSessionName(session); ok && rig != "" {
		return cyclePolecatSession(direction, session)
	}

	// Unknown session type - do nothing
	return nil
}

// parseRigInfraSession extracts rig name if this is a witness or refinery session.
// Returns empty string if not a rig infra session.
// Format: <prefix>-witness or <prefix>-refinery
func parseRigInfraSession(sess string) string {
	identity, err := sessionpkg.ParseSessionName(sess)
	if err != nil {
		return ""
	}
	if identity.Role == sessionpkg.RoleWitness || identity.Role == sessionpkg.RoleRefinery {
		return identity.Rig
	}
	return ""
}

// cycleRigInfraSession cycles between witness and refinery sessions for a rig.
func cycleRigInfraSession(direction int, currentSession, rig string) error {
	// Find running infra sessions for this rig
	witnessSession := sessionpkg.WitnessSessionName(sessionpkg.PrefixFor(rig))
	refinerySession := sessionpkg.RefinerySessionName(sessionpkg.PrefixFor(rig))

	var sessions []string
	allSessions, err := listTmuxSessions()
	if err != nil {
		return err
	}

	for _, s := range allSessions {
		if s == witnessSession || s == refinerySession {
			sessions = append(sessions, s)
		}
	}

	if len(sessions) == 0 {
		return nil // No infra sessions running
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
		return nil // Current session not in list
	}

	// Calculate target index (with wrapping)
	targetIdx := (currentIdx + direction + len(sessions)) % len(sessions)

	if targetIdx == currentIdx {
		return nil // Only one session
	}

	// Switch to target session
	cmd := exec.Command("tmux", "-u", "switch-client", "-t", sessions[targetIdx])
	return cmd.Run()
}

// listTmuxSessions returns all tmux session names.
func listTmuxSessions() ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}
