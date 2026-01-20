package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
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

	// Handle town-level agents (no rig prefix needed)
	var sessionName string
	switch address {
	case "mayor":
		sessionName = session.MayorSessionName()
	case "deacon":
		sessionName = session.DeaconSessionName()
	case "boot":
		sessionName = "gt-boot" // Boot watchdog session
	}

	if sessionName != "" {
		// Town-level agent - capture directly via tmux
		t := tmux.NewTmux()
		exists, err := t.HasSession(sessionName)
		if err != nil {
			return fmt.Errorf("checking %s session: %w", address, err)
		}
		if !exists {
			return fmt.Errorf("%s session not running (expected %s)", address, sessionName)
		}
		output, err := t.CapturePane(sessionName, lines)
		if err != nil {
			return fmt.Errorf("capturing %s output: %w", address, err)
		}
		fmt.Print(output)
		return nil
	}

	// Standard rig/polecat format
	rigName, polecatName, err := parseAddress(address)
	if err != nil {
		return err
	}

	mgr, _, err := getSessionManager(rigName)
	if err != nil {
		return err
	}

	var output string

	// Handle crew/ prefix for cross-rig crew workers
	// e.g., "beads/crew/dave" -> session name "gt-beads-crew-dave"
	if strings.HasPrefix(polecatName, "crew/") {
		crewName := strings.TrimPrefix(polecatName, "crew/")
		sessionID := session.CrewSessionName(rigName, crewName)
		output, err = mgr.CaptureSession(sessionID, lines)
	} else {
		output, err = mgr.Capture(polecatName, lines)
	}

	if err != nil {
		return fmt.Errorf("capturing output: %w", err)
	}

	fmt.Print(output)
	return nil
}
