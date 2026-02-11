package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/terminal"
)

// Peek command flags
var peekLines int

func init() {
	rootCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&peekLines, "lines", "n", 100, "Number of lines to capture")
}

var peekCmd = &cobra.Command{
	Use:     "peek <target> [count]",
	GroupID: GroupComm,
	Short:   "View recent output from an agent session",
	Long: `Capture and display recent terminal output from an agent session.

This is the ergonomic alias for 'gt session capture'. Use it to check
what an agent is currently doing or has recently output.

The nudge/peek pair provides the canonical interface for agent sessions:
  gt nudge - send messages TO a session (reliable delivery)
  gt peek  - read output FROM a session (capture-pane wrapper)

Supports town-level agents, polecats, and crew workers:
  - Town-level: mayor, deacon, boot (no rig prefix needed)
  - Polecats: rig/name format (e.g., greenplace/furiosa)
  - Crew: rig/crew/name format (e.g., beads/crew/dave)

Examples:
  gt peek mayor                      # Town-level: Mayor agent
  gt peek deacon                     # Town-level: Deacon agent
  gt peek boot                       # Town-level: Boot watchdog
  gt peek greenplace/furiosa         # Polecat: last 100 lines (default)
  gt peek greenplace/furiosa 50      # Polecat: last 50 lines
  gt peek beads/crew/dave            # Crew: last 100 lines
  gt peek beads/crew/dave -n 200     # Crew: last 200 lines`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPeek,
}

func runPeek(cmd *cobra.Command, args []string) error {
	address := args[0]

	// Handle optional positional count argument
	lines := peekLines
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid line count: %s", args[1])
		}
		lines = n
	}

	// Convert address to bead ID format for remote backend lookup.
	// ParseAddress handles rig/name, rig/crew/name, and role shortnames.
	identity, err := session.ParseAddress(address)
	if err == nil {
		beadID := identity.BeadID()
		backend := terminal.ResolveBackend(beadID)
		switch backend.(type) {
		case *terminal.CoopBackend, *terminal.SSHBackend:
			return peekViaBackend(backend, "claude", lines)
		}
	}

	// Resolve session name from address for local tmux lookup
	var sessionName string
	switch address {
	case "mayor":
		sessionName = session.MayorSessionName()
	case "deacon":
		sessionName = session.DeaconSessionName()
	case "boot":
		sessionName = "gt-boot"
	default:
		if identity != nil {
			sessionName = identity.SessionName()
		} else {
			// Fallback for unparseable addresses
			rigName, polecatName, parseErr := parseAddress(address)
			if parseErr != nil {
				return parseErr
			}
			if strings.HasPrefix(polecatName, "crew/") {
				crewName := strings.TrimPrefix(polecatName, "crew/")
				sessionName = session.CrewSessionName(rigName, crewName)
			} else {
				sessionName = fmt.Sprintf("gt-%s-%s", rigName, polecatName)
			}
		}
	}

	// Resolve backend for this session — routes to coop for remote agents, tmux for local
	resolvedBackend, sessionKey := resolveBackendForSession(sessionName)
	return peekViaBackend(resolvedBackend, sessionKey, lines)
}

// peekViaBackend captures terminal output using a terminal.Backend.
func peekViaBackend(backend terminal.Backend, sessionName string, lines int) error {
	exists, err := backend.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %q not running", sessionName)
	}
	output, err := backend.CapturePane(sessionName, lines)
	if err != nil {
		return fmt.Errorf("capturing output: %w", err)
	}
	fmt.Print(output)
	return nil
}
