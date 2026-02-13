package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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

	// When running outside K8s, use kubectl port-forward to reach agent pods.
	// ResolveBackend returns coop_url with internal pod IPs (e.g., 10.x.x.x)
	// which are unreachable from a developer workstation.
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		return peekViaPortForward(address, lines)
	}

	// Inside K8s: use ResolveBackend which can hit pod IPs directly.
	identity, err := session.ParseAddress(address)
	if err == nil {
		beadID := identity.BeadID()
		backend := terminal.ResolveBackend(beadID)
		switch backend.(type) {
		case *terminal.CoopBackend:
			return peekViaBackend(backend, "claude", lines)
		}
	}

	// Fallback: resolve session name for backend lookup.
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

	resolvedBackend, sessionKey := resolveBackendForSession(sessionName)
	return peekViaBackend(resolvedBackend, sessionKey, lines)
}

// peekViaPortForward uses kubectl port-forward to reach an agent pod's coop API.
// This is the path used when running outside K8s (developer workstation).
func peekViaPortForward(address string, lines int) error {
	podName, ns := resolveCoopTarget(address)
	if podName == "" {
		// resolveCoopTarget needs GT_K8S_NAMESPACE. Try bead metadata as fallback.
		podInfo, err := terminal.ResolveAgentPodInfo(address)
		if err != nil {
			return fmt.Errorf("cannot find pod for %q: set GT_K8S_NAMESPACE or ensure agent bead has pod metadata", address)
		}
		podName = podInfo.PodName
		ns = podInfo.Namespace
	}

	conn := terminal.NewCoopPodConnection(terminal.CoopPodConnectionConfig{
		PodName:   podName,
		Namespace: ns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := conn.Open(ctx); err != nil {
		return fmt.Errorf("connecting to pod %s: %w", podName, err)
	}
	defer conn.Close()

	// Create a CoopBackend pointing at the port-forwarded local URL.
	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	backend.AddSession("claude", conn.LocalURL())
	return peekViaBackend(backend, "claude", lines)
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
