package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/inject"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var nudgeMessageFlag string
var nudgeForceFlag bool
var nudgeDirectFlag bool // deprecated: direct is now default
var nudgeQueueFlag bool
var nudgeDrainQuiet bool

func init() {
	rootCmd.AddCommand(nudgeCmd)
	nudgeCmd.Flags().StringVarP(&nudgeMessageFlag, "message", "m", "", "Message to send")
	nudgeCmd.Flags().BoolVarP(&nudgeForceFlag, "force", "f", false, "Send even if target has DND enabled")
	nudgeCmd.Flags().BoolVar(&nudgeDirectFlag, "direct", false, "DEPRECATED: direct is now the default behavior")
	nudgeCmd.Flags().BoolVar(&nudgeQueueFlag, "queue", false, "Queue the nudge for later delivery (use when target may be busy processing tools)")

	// Add drain subcommand
	nudgeCmd.AddCommand(nudgeDrainCmd)
	nudgeDrainCmd.Flags().BoolVarP(&nudgeDrainQuiet, "quiet", "q", false, "Exit 0 even if queue is empty")
}

var nudgeCmd = &cobra.Command{
	Use:     "nudge <target> [message]",
	GroupID: GroupComm,
	Short:   "Send a synchronous message to any Gas Town worker",
	Long: `Universal synchronous messaging API for Gas Town worker-to-worker communication.

Delivers a message directly to any worker's Claude Code session: polecats, crew,
witness, refinery, mayor, or deacon. Use this for real-time coordination when
you need immediate attention from another worker.

DELIVERY MODES:
  Direct (default): Sends immediately via tmux. Use this to wake up idle agents
                    waiting for decisions, stale agents, or any agent you need to
                    respond immediately. May cause API 400 errors if the target is
                    actively processing tools (recoverable - message still arrives).

  Queued (--queue): Buffers the nudge for later delivery via the target's
                    PostToolUse hook. Use this when sending many nudges to a busy
                    agent to avoid API errors. WARNING: Queued nudges are NEVER
                    delivered to idle agents (no tools running = no hooks firing).

Uses a reliable delivery pattern:
1. Sends text in literal mode (-l flag)
2. Waits 500ms for paste to complete
3. Sends Enter as a separate command

This is the ONLY way to send messages to Claude sessions.
Do not use raw tmux send-keys elsewhere.

Role shortcuts (expand to session names):
  mayor     Maps to gt-mayor
  deacon    Maps to gt-deacon
  witness   Maps to gt-<rig>-witness (uses current rig)
  refinery  Maps to gt-<rig>-refinery (uses current rig)

Channel syntax:
  channel:<name>  Nudges all members of a named channel defined in
                  ~/gt/config/messaging.json under "nudge_channels".
                  Patterns like "gastown/polecats/*" are expanded.

DND (Do Not Disturb):
  If the target has DND enabled (gt dnd on), the nudge is skipped.
  Use --force to override DND and send anyway.

Examples:
  gt nudge greenplace/furiosa "Check your mail and start working"
  gt nudge greenplace/alpha -m "What's your status?"
  gt nudge mayor "Status update requested"
  gt nudge witness "Check polecat health"
  gt nudge deacon session-started
  gt nudge channel:workers "New priority work available"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runNudge,
}

func runNudge(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Get message from -m flag or positional arg
	var message string
	if nudgeMessageFlag != "" {
		message = nudgeMessageFlag
	} else if len(args) >= 2 {
		message = args[1]
	} else {
		return fmt.Errorf("message required: use -m flag or provide as second argument")
	}

	// Handle channel syntax: channel:<name>
	if strings.HasPrefix(target, "channel:") {
		channelName := strings.TrimPrefix(target, "channel:")
		return runNudgeChannel(channelName, message)
	}

	// Identify sender for message prefix
	sender := "unknown"
	if roleInfo, err := GetRole(); err == nil {
		switch roleInfo.Role {
		case RoleMayor:
			sender = "mayor"
		case RoleCrew:
			sender = fmt.Sprintf("%s/crew/%s", roleInfo.Rig, roleInfo.Polecat)
		case RolePolecat:
			sender = fmt.Sprintf("%s/%s", roleInfo.Rig, roleInfo.Polecat)
		case RoleWitness:
			sender = fmt.Sprintf("%s/witness", roleInfo.Rig)
		case RoleRefinery:
			sender = fmt.Sprintf("%s/refinery", roleInfo.Rig)
		case RoleDeacon:
			sender = "deacon"
		default:
			sender = string(roleInfo.Role)
		}
	}

	// Prefix message with sender
	message = fmt.Sprintf("[from %s] %s", sender, message)

	// Check DND status for target (unless force flag or channel target)
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		townRoot = "" // Not in a workspace, will use direct nudge
	}
	if townRoot != "" && !nudgeForceFlag && !strings.HasPrefix(target, "channel:") {
		shouldSend, level, _ := shouldNudgeTarget(townRoot, target, nudgeForceFlag)
		if !shouldSend {
			fmt.Printf("%s Target has DND enabled (%s) - nudge skipped\n", style.Dim.Render("○"), level)
			fmt.Printf("  Use %s to override\n", style.Bold.Render("--force"))
			return nil
		}
	}

	t := tmux.NewTmux()

	// Expand role shortcuts to session names
	// These shortcuts let users type "mayor" instead of "gt-mayor"
	switch target {
	case "mayor", "mayor/":
		target = session.MayorSessionName()
	case "witness", "refinery":
		// These need the current rig
		roleInfo, err := GetRole()
		if err != nil {
			return fmt.Errorf("cannot determine rig for %s shortcut: %w", target, err)
		}
		if roleInfo.Rig == "" {
			return fmt.Errorf("cannot determine rig for %s shortcut (not in a rig context)", target)
		}
		if target == "witness" {
			target = session.WitnessSessionName(roleInfo.Rig)
		} else {
			target = session.RefinerySessionName(roleInfo.Rig)
		}
	}

	// Special case: "deacon" target maps to the Deacon session
	if target == "deacon" || target == "deacon/" {
		deaconSession := session.DeaconSessionName()
		// Check if Deacon session exists
		exists, err := t.HasSession(deaconSession)
		if err != nil {
			return fmt.Errorf("checking deacon session: %w", err)
		}
		if !exists {
			// Deacon not running - this is not an error, just log and return
			fmt.Printf("%s Deacon not running, nudge skipped\n", style.Dim.Render("○"))
			return nil
		}

		if err := sendOrQueueNudge(t, townRoot, deaconSession, message); err != nil {
			return fmt.Errorf("nudging deacon: %w", err)
		}

		fmt.Printf("%s Nudged deacon\n", style.Bold.Render("✓"))

		// Log nudge event
		if townRoot != "" {
			_ = LogNudge(townRoot, "deacon", message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload("", "deacon", message))
		return nil
	}

	// Check if target is rig/polecat format or raw session name
	if strings.Contains(target, "/") {
		// Parse rig/polecat format
		rigName, polecatName, err := parseAddress(target)
		if err != nil {
			return err
		}

		var sessionName string

		// Check if this is a crew address (polecatName starts with "crew/")
		if strings.HasPrefix(polecatName, "crew/") {
			// Extract crew name and use crew session naming
			crewName := strings.TrimPrefix(polecatName, "crew/")
			sessionName = crewSessionName(rigName, crewName)
		} else {
			// Short address (e.g., "gastown/holden") - could be crew or polecat.
			// Try crew first (matches mail system's addressToSessionIDs pattern),
			// then fall back to polecat.
			crewSession := crewSessionName(rigName, polecatName)
			if exists, _ := t.HasSession(crewSession); exists {
				sessionName = crewSession
			} else {
				mgr, _, err := getSessionManager(rigName)
				if err != nil {
					return err
				}
				sessionName = mgr.SessionName(polecatName)
			}
		}

		// Send nudge using queue (safe) or direct (may cause API 400)
		if err := sendOrQueueNudge(t, townRoot, sessionName, message); err != nil {
			return fmt.Errorf("nudging session: %w", err)
		}

		fmt.Printf("%s Nudged %s/%s\n", style.Bold.Render("✓"), rigName, polecatName)

		// Log nudge event
		if townRoot != "" {
			_ = LogNudge(townRoot, target, message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload(rigName, target, message))
	} else {
		// Raw session name (legacy)
		exists, err := t.HasSession(target)
		if err != nil {
			return fmt.Errorf("checking session: %w", err)
		}
		if !exists {
			return fmt.Errorf("session %q not found", target)
		}

		if err := sendOrQueueNudge(t, townRoot, target, message); err != nil {
			return fmt.Errorf("nudging session: %w", err)
		}

		fmt.Printf("✓ Nudged %s\n", target)

		// Log nudge event
		if townRoot != "" {
			_ = LogNudge(townRoot, target, message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload("", target, message))
	}

	return nil
}

// runNudgeChannel nudges all members of a named channel.
func runNudgeChannel(channelName, message string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("cannot find town root: %w", err)
	}

	// Load messaging config
	msgConfigPath := config.MessagingConfigPath(townRoot)
	msgConfig, err := config.LoadMessagingConfig(msgConfigPath)
	if err != nil {
		return fmt.Errorf("loading messaging config: %w", err)
	}

	// Look up channel
	patterns, ok := msgConfig.NudgeChannels[channelName]
	if !ok {
		return fmt.Errorf("nudge channel %q not found in messaging config", channelName)
	}

	if len(patterns) == 0 {
		return fmt.Errorf("nudge channel %q has no members", channelName)
	}

	// Identify sender for message prefix
	sender := "unknown"
	if roleInfo, err := GetRole(); err == nil {
		switch roleInfo.Role {
		case RoleMayor:
			sender = "mayor"
		case RoleCrew:
			sender = fmt.Sprintf("%s/crew/%s", roleInfo.Rig, roleInfo.Polecat)
		case RolePolecat:
			sender = fmt.Sprintf("%s/%s", roleInfo.Rig, roleInfo.Polecat)
		case RoleWitness:
			sender = fmt.Sprintf("%s/witness", roleInfo.Rig)
		case RoleRefinery:
			sender = fmt.Sprintf("%s/refinery", roleInfo.Rig)
		case RoleDeacon:
			sender = "deacon"
		default:
			sender = string(roleInfo.Role)
		}
	}

	// Prefix message with sender
	prefixedMessage := fmt.Sprintf("[from %s] %s", sender, message)

	// Get all running sessions for pattern matching
	agents, err := getAgentSessions(true)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Resolve patterns to session names
	var targets []string
	seenTargets := make(map[string]bool)

	for _, pattern := range patterns {
		resolved := resolveNudgePattern(pattern, agents)
		for _, sessionName := range resolved {
			if !seenTargets[sessionName] {
				seenTargets[sessionName] = true
				targets = append(targets, sessionName)
			}
		}
	}

	if len(targets) == 0 {
		fmt.Printf("%s No sessions match channel %q patterns\n", style.WarningPrefix, channelName)
		return nil
	}

	// Send nudges
	t := tmux.NewTmux()
	var succeeded, failed int
	var failures []string

	fmt.Printf("Nudging channel %q (%d target(s))...\n\n", channelName, len(targets))

	for i, sessionName := range targets {
		if err := t.NudgeSession(sessionName, prefixedMessage); err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", sessionName, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, sessionName)
		} else {
			succeeded++
			fmt.Printf("  %s %s\n", style.SuccessPrefix, sessionName)
		}

		// Small delay between nudges
		if i < len(targets)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Println()

	// Log nudge event
	_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload("", "channel:"+channelName, message))

	if failed > 0 {
		fmt.Printf("%s Channel nudge complete: %d succeeded, %d failed\n",
			style.WarningPrefix, succeeded, failed)
		for _, f := range failures {
			fmt.Printf("  %s\n", style.Dim.Render(f))
		}
		return fmt.Errorf("%d nudge(s) failed", failed)
	}

	fmt.Printf("%s Channel nudge complete: %d target(s) nudged\n", style.SuccessPrefix, succeeded)
	return nil
}

// resolveNudgePattern resolves a nudge channel pattern to session names.
// Patterns can be:
//   - Literal: "gastown/witness" → gt-gastown-witness
//   - Wildcard: "gastown/polecats/*" → all polecat sessions in gastown
//   - Role: "*/witness" → all witness sessions
//   - Special: "mayor", "deacon" → gt-{town}-mayor, gt-{town}-deacon
// townName is used to generate the correct session names for mayor/deacon.
func resolveNudgePattern(pattern string, agents []*AgentSession) []string {
	var results []string

	// Handle special cases
	switch pattern {
	case "mayor":
		return []string{session.MayorSessionName()}
	case "deacon":
		return []string{session.DeaconSessionName()}
	}

	// Parse pattern
	if !strings.Contains(pattern, "/") {
		// Unknown pattern format
		return nil
	}

	parts := strings.SplitN(pattern, "/", 2)
	rigPattern := parts[0]
	targetPattern := parts[1]

	for _, agent := range agents {
		// Match rig pattern
		if rigPattern != "*" && rigPattern != agent.Rig {
			continue
		}

		// Match target pattern
		if strings.HasPrefix(targetPattern, "polecats/") {
			// polecats/* or polecats/<name>
			if agent.Type != AgentPolecat {
				continue
			}
			suffix := strings.TrimPrefix(targetPattern, "polecats/")
			if suffix != "*" && suffix != agent.AgentName {
				continue
			}
		} else if strings.HasPrefix(targetPattern, "crew/") {
			// crew/* or crew/<name>
			if agent.Type != AgentCrew {
				continue
			}
			suffix := strings.TrimPrefix(targetPattern, "crew/")
			if suffix != "*" && suffix != agent.AgentName {
				continue
			}
		} else if targetPattern == "witness" {
			if agent.Type != AgentWitness {
				continue
			}
		} else if targetPattern == "refinery" {
			if agent.Type != AgentRefinery {
				continue
			}
		} else {
			// Assume it's a polecat name (legacy short format)
			if agent.Type != AgentPolecat || agent.AgentName != targetPattern {
				continue
			}
		}

		results = append(results, agent.Name)
	}

	return results
}

// shouldNudgeTarget checks if a nudge should be sent based on the target's notification level.
// Returns (shouldSend bool, level string, err error).
// If force is true, always returns true.
// If the agent bead cannot be found, returns true (fail-open for backward compatibility).
func shouldNudgeTarget(townRoot, targetAddress string, force bool) (bool, string, error) { //nolint:unparam // error return kept for future use
	if force {
		return true, "", nil
	}

	// Try to determine agent bead ID from address
	agentBeadID := addressToAgentBeadID(targetAddress)
	if agentBeadID == "" {
		// Can't determine agent bead, allow the nudge
		return true, "", nil
	}

	bd := beads.New(townRoot)
	level, err := bd.GetAgentNotificationLevel(agentBeadID)
	if err != nil {
		// Agent bead might not exist, allow the nudge
		return true, "", nil
	}

	// Allow nudge if level is not muted
	return level != beads.NotifyMuted, level, nil
}

// addressToAgentBeadID converts a target address to an agent bead ID.
// Examples:
//   - "mayor" -> "gt-{town}-mayor"
//   - "deacon" -> "gt-{town}-deacon"
//   - "gastown/witness" -> "gt-gastown-witness"
//   - "gastown/alpha" -> "gt-gastown-polecat-alpha"
//
// Returns empty string if the address cannot be converted.
func addressToAgentBeadID(address string) string {
	// Handle special cases
	switch address {
	case "mayor", "mayor/":
		return session.MayorSessionName()
	case "deacon", "deacon/":
		return session.DeaconSessionName()
	}

	// Parse rig/role format
	if !strings.Contains(address, "/") {
		return ""
	}

	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 {
		return ""
	}

	rig := parts[0]
	role := parts[1]

	switch role {
	case "witness":
		return fmt.Sprintf("gt-%s-witness", rig)
	case "refinery":
		return fmt.Sprintf("gt-%s-refinery", rig)
	default:
		// Assume polecat
		if strings.HasPrefix(role, "crew/") {
			crewName := strings.TrimPrefix(role, "crew/")
			return fmt.Sprintf("gt-%s-crew-%s", rig, crewName)
		}
		return fmt.Sprintf("gt-%s-polecat-%s", rig, role)
	}
}

// sendOrQueueNudge sends a nudge either directly via tmux (default) or queued for later delivery.
// By default, nudges are sent directly to wake up idle agents immediately.
// Use --queue flag to buffer nudges when the target may be busy processing tools.
func sendOrQueueNudge(t *tmux.Tmux, townRoot, sessionName, message string) error {
	// If --queue flag is set and we're in a workspace, queue the nudge
	if nudgeQueueFlag && townRoot != "" {
		nq := inject.NewNudgeQueue(townRoot, sessionName)
		if err := nq.Enqueue(message); err != nil {
			// Fall back to direct send if queueing fails
			return t.NudgeSession(sessionName, message)
		}
		return nil
	}

	// Default: send directly via tmux to wake up the target immediately
	return t.NudgeSession(sessionName, message)
}

// nudgeDrainCmd drains the nudge queue for the current session.
var nudgeDrainCmd = &cobra.Command{
	Use:   "drain",
	Short: "Output and clear queued nudge messages",
	Long: `Drain the nudge queue, outputting all queued nudge messages.

This command should be called from a PostToolUse hook to safely
deliver nudge messages that were queued while tools were running.

This prevents API 400 errors that occur when nudges are sent
directly via tmux while Claude is processing a tool.

Exit codes:
  0 - Content was drained (or queue empty with --quiet)
  1 - Queue empty (normal mode)

Examples:
  gt nudge drain          # Output and clear queue
  gt nudge drain --quiet  # Silent if empty`,
	RunE:          runNudgeDrain,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runNudgeDrain(cmd *cobra.Command, args []string) error {
	// Get tmux session name via tmux display-message
	t := tmux.NewTmux()
	sessionName, err := t.GetCurrentSessionName()
	if err != nil {
		if nudgeDrainQuiet {
			return nil
		}
		return fmt.Errorf("cannot determine session name: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		if nudgeDrainQuiet {
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create nudge queue and drain
	nq := inject.NewNudgeQueue(townRoot, sessionName)
	entries, err := nq.Drain()
	if err != nil {
		if nudgeDrainQuiet {
			return nil
		}
		return fmt.Errorf("draining nudge queue: %w", err)
	}

	if len(entries) == 0 {
		if nudgeDrainQuiet {
			return nil
		}
		return NewSilentExit(1)
	}

	// Output each nudge as a system reminder
	for _, entry := range entries {
		fmt.Printf("<system-reminder>\n📬 Nudge received:\n%s\n</system-reminder>\n", entry.Content)
	}

	return nil
}
