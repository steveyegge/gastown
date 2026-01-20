package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/ids"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var nudgeMessageFlag string
var nudgeForceFlag bool

func init() {
	rootCmd.AddCommand(nudgeCmd)
	nudgeCmd.Flags().StringVarP(&nudgeMessageFlag, "message", "m", "", "Message to send")
	nudgeCmd.Flags().BoolVarP(&nudgeForceFlag, "force", "f", false, "Send even if target has DND enabled")
}

var nudgeCmd = &cobra.Command{
	Use:     "nudge <target> [message]",
	GroupID: GroupComm,
	Short:   "Send a synchronous message to any Gas Town worker",
	Long: `Universal synchronous messaging API for Gas Town worker-to-worker communication.

Delivers a message directly to any worker's Claude Code session: polecats, crew,
witness, refinery, mayor, or deacon. Use this for real-time coordination when
you need immediate attention from another worker.

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
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" && !nudgeForceFlag && !strings.HasPrefix(target, "channel:") {
		shouldSend, level, _ := shouldNudgeTarget(townRoot, target, nudgeForceFlag)
		if !shouldSend {
			fmt.Printf("%s Target has DND enabled (%s) - nudge skipped\n", style.Dim.Render("○"), level)
			fmt.Printf("  Use %s to override\n", style.Bold.Render("--force"))
			return nil
		}
	}

	agents := agent.Default()

	// Expand role shortcuts to agent IDs
	// These shortcuts let users type "mayor" instead of full addresses
	var agentID agent.AgentID
	switch target {
	case "mayor":
		agentID = agent.MayorAddress
	case "deacon":
		agentID = agent.DeaconAddress
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
			agentID = agent.WitnessAddress(roleInfo.Rig)
		} else {
			agentID = agent.RefineryAddress(roleInfo.Rig)
		}
	}

	// Handle direct role shortcuts (mayor, deacon, witness, refinery)
	if agentID.Role != "" {
		// Check if agent is running
		if !agents.Exists(agentID) {
			fmt.Printf("%s %s not running, nudge skipped\n", style.Dim.Render("○"), target)
			return nil
		}

		if err := agents.Nudge(agentID, message); err != nil {
			return fmt.Errorf("nudging %s: %w", target, err)
		}

		fmt.Printf("%s Nudged %s\n", style.Bold.Render("✓"), target)

		// Log nudge event
		if townRoot != "" {
			_ = LogNudge(townRoot, target, message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload("", target, message))
		return nil
	}

	// Check if target is rig/polecat format or raw session name
	if strings.Contains(target, "/") {
		// Parse rig/polecat format
		rigName, polecatName, err := parseAddress(target)
		if err != nil {
			return err
		}

		var targetID agent.AgentID

		// Check if this is a crew address (polecatName starts with "crew/")
		if strings.HasPrefix(polecatName, "crew/") {
			// Extract crew name
			crewName := strings.TrimPrefix(polecatName, "crew/")
			targetID = agent.CrewAddress(rigName, crewName)
		} else {
			// Regular polecat
			targetID = agent.PolecatAddress(rigName, polecatName)
		}

		// Send nudge using the Agents abstraction
		if err := agents.Nudge(targetID, message); err != nil {
			return fmt.Errorf("nudging agent: %w", err)
		}

		fmt.Printf("%s Nudged %s/%s\n", style.Bold.Render("✓"), rigName, polecatName)

		// Log nudge event
		if townRoot != "" {
			_ = LogNudge(townRoot, target, message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload(rigName, target, message))
	} else {
		// Raw session name (legacy)
		targetID := ids.ParseSessionName(target)
		if targetID.Role == "" {
			return fmt.Errorf("invalid session name %q", target)
		}
		if !agents.Exists(targetID) {
			return fmt.Errorf("agent %q not found", target)
		}

		if err := agents.Nudge(targetID, message); err != nil {
			return fmt.Errorf("nudging agent: %w", err)
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
	agentSessions, err := getAgentSessions(townRoot, true)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Resolve patterns to session names
	var targets []string
	seenTargets := make(map[string]bool)

	for _, pattern := range patterns {
		resolved := resolveNudgePattern(pattern, agentSessions)
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
	agentsAPI := agent.Default()
	var succeeded, failed int
	var failures []string

	fmt.Printf("Nudging channel %q (%d target(s))...\n\n", channelName, len(targets))

	for i, agentAddr := range targets {
		agentID := ids.ParseAddress(agentAddr)
		if agentID.Role == "" {
			failed++
			failures = append(failures, fmt.Sprintf("%s: invalid agent address", agentAddr))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentAddr)
			continue
		}
		if err := agentsAPI.Nudge(agentID, prefixedMessage); err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s: %v", agentAddr, err))
			fmt.Printf("  %s %s\n", style.ErrorPrefix, agentAddr)
		} else {
			succeeded++
			fmt.Printf("  %s %s\n", style.SuccessPrefix, agentAddr)
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

// resolveNudgePattern resolves a nudge channel pattern to agent addresses.
// Patterns can be:
//   - Literal: "gastown/witness" → gastown/witness
//   - Wildcard: "gastown/polecats/*" → all polecat addresses in gastown
//   - Role: "*/witness" → all witness addresses
//   - Special: "mayor", "deacon" → mayor, deacon
//
// Returns agent addresses (not session names).
func resolveNudgePattern(pattern string, agentSessions []*AgentSession) []string {
	var results []string

	// Handle special cases
	switch pattern {
	case "mayor":
		return []string{agent.MayorAddress.String()}
	case "deacon":
		return []string{agent.DeaconAddress.String()}
	}

	// Parse pattern
	if !strings.Contains(pattern, "/") {
		// Unknown pattern format
		return nil
	}

	parts := strings.SplitN(pattern, "/", 2)
	rigPattern := parts[0]
	targetPattern := parts[1]

	for _, as := range agentSessions {
		// Match rig pattern
		if rigPattern != "*" && rigPattern != as.Rig {
			continue
		}

		// Match target pattern and construct address
		var addr string
		if strings.HasPrefix(targetPattern, "polecats/") {
			// polecats/* or polecats/<name>
			if as.Type != AgentPolecat {
				continue
			}
			suffix := strings.TrimPrefix(targetPattern, "polecats/")
			if suffix != "*" && suffix != as.AgentName {
				continue
			}
			addr = agent.PolecatAddress(as.Rig, as.AgentName).String()
		} else if strings.HasPrefix(targetPattern, "crew/") {
			// crew/* or crew/<name>
			if as.Type != AgentCrew {
				continue
			}
			suffix := strings.TrimPrefix(targetPattern, "crew/")
			if suffix != "*" && suffix != as.AgentName {
				continue
			}
			addr = agent.CrewAddress(as.Rig, as.AgentName).String()
		} else if targetPattern == "witness" {
			if as.Type != AgentWitness {
				continue
			}
			addr = agent.WitnessAddress(as.Rig).String()
		} else if targetPattern == "refinery" {
			if as.Type != AgentRefinery {
				continue
			}
			addr = agent.RefineryAddress(as.Rig).String()
		} else {
			// Assume it's a polecat name (legacy short format)
			if as.Type != AgentPolecat || as.AgentName != targetPattern {
				continue
			}
			addr = agent.PolecatAddress(as.Rig, as.AgentName).String()
		}

		results = append(results, addr)
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
// Bead IDs use the session name format:
//   - "mayor" -> "hq-mayor"
//   - "deacon" -> "hq-deacon"
//   - "gastown/witness" -> "gt-gastown-witness"
//   - "gastown/alpha" -> "gt-gastown-polecat-alpha"
//
// Returns empty string if the address cannot be converted.
func addressToAgentBeadID(address string) string {
	// Handle special cases - town-level agents use hq- prefix
	switch address {
	case "mayor":
		return "hq-mayor"
	case "deacon":
		return "hq-deacon"
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
