package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/telemetry"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	nudgeMessageFlag  string
	nudgeForceFlag    bool
	nudgeStdinFlag    bool
	nudgeIfFreshFlag  bool
	nudgeModeFlag     string
	nudgePriorityFlag string
)

// Nudge delivery modes.
const (
	// NudgeModeImmediate sends directly via tmux send-keys (current behavior).
	// This interrupts in-flight work but guarantees immediate delivery.
	NudgeModeImmediate = "immediate"
	// NudgeModeQueue writes to a file queue; agent picks up via hook at next
	// turn boundary. Zero interruption but delivery depends on agent turn frequency.
	NudgeModeQueue = "queue"
	// NudgeModeWaitIdle waits for the agent to become idle (prompt visible),
	// then delivers directly. Falls back to queue on timeout. Best of both worlds.
	NudgeModeWaitIdle = "wait-idle"
)

func init() {
	rootCmd.AddCommand(nudgeCmd)
	nudgeCmd.Flags().StringVarP(&nudgeMessageFlag, "message", "m", "", "Message to send")
	nudgeCmd.Flags().BoolVarP(&nudgeForceFlag, "force", "f", false, "Send even if target has DND enabled")
	nudgeCmd.Flags().BoolVar(&nudgeStdinFlag, "stdin", false, "Read message from stdin (avoids shell quoting issues)")
	nudgeCmd.Flags().BoolVar(&nudgeIfFreshFlag, "if-fresh", false, "Only send if caller's tmux session is <60s old (suppresses compaction nudges)")
	nudgeCmd.Flags().StringVar(&nudgeModeFlag, "mode", NudgeModeImmediate, "Delivery mode: immediate (default), queue, or wait-idle")
	nudgeCmd.Flags().StringVar(&nudgePriorityFlag, "priority", nudge.PriorityNormal, "Queue priority: normal (default) or urgent")
}

var nudgeCmd = &cobra.Command{
	Use:     "nudge <target> [message]",
	GroupID: GroupComm,
	Short:   "Send a synchronous message to any Gas Town worker",
	Long: `Universal messaging API for Gas Town worker-to-worker communication.

Delivers a message to any worker's Claude Code session: polecats, crew,
witness, refinery, mayor, or deacon.

Delivery modes (--mode):
  immediate  Send directly via tmux send-keys (default). Interrupts in-flight
             work but guarantees immediate delivery.
  queue      Write to a file queue; agent picks up via hook at next turn
             boundary. Zero interruption. Use for non-urgent coordination.
  wait-idle  Wait for agent to become idle (prompt visible), then deliver
             directly. Falls back to queue on timeout. If both idle-wait and
             queue fail, falls back to immediate delivery as a last resort.

Queue and wait-idle modes require the target agent to support hooks
(UserPromptSubmit) for drain. Agents without hook support should use immediate.

The default is immediate for backward compatibility. For non-urgent messages
where you don't want to interrupt the agent's current work, use --mode=queue.

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
  gt nudge channel:workers "New priority work available"

  # Use --stdin for messages with special characters or formatting:
  gt nudge gastown/alpha --stdin <<'EOF'
  Status update:
  - Task 1: complete
  - Task 2: in progress
  EOF`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runNudge,
}

// ifFreshMaxAge is the maximum session age for --if-fresh to allow a nudge.
// Sessions older than this are considered compaction/clear restarts, not new sessions.
const ifFreshMaxAge = 60 * time.Second

// waitIdleTimeout is how long --mode=wait-idle will poll before falling back to queue.
// This is a var (not const) so tests can override it to avoid 15s waits.
var waitIdleTimeout = 15 * time.Second

// deliverNudge routes a nudge based on the --mode flag.
// For "immediate" mode: sends directly via tmux (current behavior).
// For "queue" mode: writes to the nudge queue for cooperative delivery.
// For "wait-idle" mode: waits for idle, then delivers or falls back to queue.
func deliverNudge(t *tmux.Tmux, sessionName, message, sender string) error {
	townRoot, _ := workspace.FindFromCwd()

	// For direct tmux delivery, prefix with sender attribution.
	// Queue-based delivery stores Sender as a separate field and
	// FormatForInjection adds the prefix, so we must NOT double-prefix.
	prefixedMessage := fmt.Sprintf("[from %s] %s", sender, message)

	switch nudgeModeFlag {
	case NudgeModeQueue:
		if townRoot == "" {
			return fmt.Errorf("--mode=queue requires a Gas Town workspace")
		}
		return nudge.Enqueue(townRoot, sessionName, nudge.QueuedNudge{
			Sender:   sender,
			Message:  message,
			Priority: nudgePriorityFlag,
		})

	case NudgeModeWaitIdle:
		if townRoot == "" {
			// wait-idle needs workspace for queue fallback — fail explicitly
			// rather than silently degrading to immediate (destructive) delivery.
			return fmt.Errorf("--mode=wait-idle requires a Gas Town workspace")
		}
		// Try to wait for idle
		err := t.WaitForIdle(sessionName, waitIdleTimeout)
		if err == nil {
			// Agent is idle — safe to deliver directly
			return t.NudgeSession(sessionName, prefixedMessage)
		}
		// Terminal errors (session gone, no server) — propagate, don't queue.
		// Queueing a nudge for a dead session means it will never be delivered.
		if errors.Is(err, tmux.ErrSessionNotFound) || errors.Is(err, tmux.ErrNoServer) {
			return fmt.Errorf("wait-idle: %w", err)
		}
		// Timeout (agent busy) — queue instead
		if qErr := nudge.Enqueue(townRoot, sessionName, nudge.QueuedNudge{
			Sender:   sender,
			Message:  message,
			Priority: nudgePriorityFlag,
		}); qErr != nil {
			// Queue failed — fall back to immediate as last resort.
			// Better to interrupt than lose the message entirely.
			fmt.Fprintf(os.Stderr, "Warning: queue fallback failed (%v), delivering immediately\n", qErr)
			return t.NudgeSession(sessionName, prefixedMessage)
		}
		return nil

	default: // NudgeModeImmediate
		return t.NudgeSession(sessionName, prefixedMessage)
	}
}

// validNudgeModes is the set of allowed --mode values.
var validNudgeModes = map[string]bool{
	NudgeModeImmediate: true,
	NudgeModeQueue:     true,
	NudgeModeWaitIdle:  true,
}

// validNudgePriorities is the set of allowed --priority values.
var validNudgePriorities = map[string]bool{
	nudge.PriorityNormal: true,
	nudge.PriorityUrgent: true,
}

func runNudge(cmd *cobra.Command, args []string) (retErr error) {
	defer func() {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		telemetry.RecordNudge(context.Background(), target, retErr)
	}()
	// Validate --mode and --priority before doing anything else.
	if !validNudgeModes[nudgeModeFlag] {
		return fmt.Errorf("invalid --mode %q: must be one of immediate, queue, wait-idle", nudgeModeFlag)
	}
	if !validNudgePriorities[nudgePriorityFlag] {
		return fmt.Errorf("invalid --priority %q: must be one of normal, urgent", nudgePriorityFlag)
	}

	// --if-fresh: skip nudge if the caller's tmux session is older than 60s.
	// This prevents compaction/clear SessionStart hooks from spamming the deacon.
	if nudgeIfFreshFlag {
		sessionName := tmux.CurrentSessionName()
		if sessionName != "" {
			t := tmux.NewTmux()
			created, err := t.GetSessionCreatedUnix(sessionName)
			if err == nil && created > 0 {
				age := time.Since(time.Unix(created, 0))
				if age > ifFreshMaxAge {
					// Session is old — this is a compaction/clear, not a new session
					return nil
				}
			}
		}
	}

	target := args[0]

	// Handle --stdin: read message from stdin (avoids shell quoting issues)
	if nudgeStdinFlag {
		if nudgeMessageFlag != "" {
			return fmt.Errorf("cannot use --stdin with --message/-m")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		nudgeMessageFlag = strings.TrimRight(string(data), "\n")
	}

	// Get message from -m flag or positional arg
	var message string
	if nudgeMessageFlag != "" {
		message = nudgeMessageFlag
	} else if len(args) >= 2 {
		message = args[1]
	} else {
		return fmt.Errorf("message required: use -m flag or provide as second argument")
	}

	// Identify sender for message prefix (needed before channel check)
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

	// Handle channel syntax: channel:<name>
	if strings.HasPrefix(target, "channel:") {
		channelName := strings.TrimPrefix(target, "channel:")
		return runNudgeChannel(channelName, message, sender)
	}

	// Check DND status for target (unless force flag or channel target)
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" && !nudgeForceFlag {
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
	case "mayor":
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
		rigPrefix := session.PrefixFor(roleInfo.Rig)
		if target == "witness" {
			target = session.WitnessSessionName(rigPrefix)
		} else {
			target = session.RefinerySessionName(rigPrefix)
		}
	}

	// Special case: "deacon" target maps to the Deacon session
	if target == "deacon" {
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

		if err := deliverNudge(t, deaconSession, message, sender); err != nil {
			return fmt.Errorf("nudging deacon: %w", err)
		}

		fmt.Printf("%s Nudged deacon (%s)\n", style.Bold.Render("✓"), nudgeModeFlag)

		// Log nudge event
		if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
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
		} else if strings.HasPrefix(polecatName, "polecats/") {
			// Explicit polecat address (e.g., "vastal/polecats/furiosa").
			// Bypasses crew-first resolution for short addresses.
			pcName := strings.TrimPrefix(polecatName, "polecats/")
			mgr, _, err := getSessionManager(rigName)
			if err != nil {
				return err
			}
			sessionName = mgr.SessionName(pcName)
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

		// For queue/wait-idle modes, verify session exists before enqueuing.
		// Without this, queue mode silently succeeds for nonexistent sessions —
		// the file is written but never drained.
		if nudgeModeFlag != NudgeModeImmediate {
			exists, err := t.HasSession(sessionName)
			if err != nil {
				return fmt.Errorf("checking session: %w", err)
			}
			if !exists {
				return fmt.Errorf("session %q not found (cannot queue nudge for nonexistent session)", sessionName)
			}
		}

		// Send nudge using the configured delivery mode
		if err := deliverNudge(t, sessionName, message, sender); err != nil {
			return fmt.Errorf("nudging session: %w", err)
		}

		fmt.Printf("%s Nudged %s/%s (%s)\n", style.Bold.Render("✓"), rigName, polecatName, nudgeModeFlag)

		// Log nudge event
		if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
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

		if err := deliverNudge(t, target, message, sender); err != nil {
			return fmt.Errorf("nudging session: %w", err)
		}

		fmt.Printf("✓ Nudged %s (%s)\n", target, nudgeModeFlag)

		// Log nudge event
		if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
			_ = LogNudge(townRoot, target, message)
		}
		_ = events.LogFeed(events.TypeNudge, sender, events.NudgePayload("", target, message))
	}

	return nil
}

// runNudgeChannel nudges all members of a named channel.
// Routes each target through deliverNudge so --mode is respected.
func runNudgeChannel(channelName, message, sender string) error {
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

	// Send nudges via deliverNudge (respects --mode flag)
	t := tmux.NewTmux()
	var succeeded, failed, skipped int
	var failures []string

	fmt.Printf("Nudging channel %q (%d target(s), mode=%s)...\n\n", channelName, len(targets), nudgeModeFlag)

	for i, sessionName := range targets {
		// Check DND status before nudging each target
		// Convert session name back to address format for DND lookup
		targetAddr := sessionNameToAddress(sessionName)
		if targetAddr != "" {
			if shouldSend, level, _ := shouldNudgeTarget(townRoot, targetAddr, false); !shouldSend {
				skipped++
				fmt.Printf("  %s %s (DND: %s)\n", style.Dim.Render("○"), sessionName, level)
				continue
			}
		}

		if err := deliverNudge(t, sessionName, message, sender); err != nil {
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
		summary := fmt.Sprintf("Channel nudge complete: %d succeeded, %d failed", succeeded, failed)
		if skipped > 0 {
			summary += fmt.Sprintf(", %d skipped (DND)", skipped)
		}
		fmt.Printf("%s %s\n", style.WarningPrefix, summary)
		for _, f := range failures {
			fmt.Printf("  %s\n", style.Dim.Render(f))
		}
		return fmt.Errorf("%d nudge(s) failed", failed)
	}

	summary := fmt.Sprintf("Channel nudge complete: %d target(s) nudged", succeeded)
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped (DND)", skipped)
	}
	fmt.Printf("%s %s\n", style.SuccessPrefix, summary)
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

// sessionNameToAddress converts a tmux session name back to a mail address
// for DND lookup. Returns empty string if the format is unrecognized.
// Examples:
//   - "gt-gastown-crew-max" -> "gastown/crew/max"
//   - "gt-gastown-alpha" -> "gastown/alpha"
//   - "gt-gastown-witness" -> "gastown/witness"
//   - "hq-mayor" -> "mayor"
//   - "hq-deacon" -> "deacon"
func sessionNameToAddress(sessionName string) string {
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		return ""
	}

	// Use short address format: rig/name (not rig/polecats/name)
	switch identity.Role {
	case session.RoleMayor:
		return "mayor"
	case session.RoleDeacon:
		return "deacon"
	case session.RoleWitness:
		return fmt.Sprintf("%s/witness", identity.Rig)
	case session.RoleRefinery:
		return fmt.Sprintf("%s/refinery", identity.Rig)
	case session.RoleCrew:
		return fmt.Sprintf("%s/crew/%s", identity.Rig, identity.Name)
	case session.RolePolecat:
		return fmt.Sprintf("%s/%s", identity.Rig, identity.Name)
	default:
		return ""
	}
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
	case "mayor":
		return session.MayorSessionName()
	case "deacon":
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
		return session.WitnessSessionName(session.PrefixFor(rig))
	case "refinery":
		return session.RefinerySessionName(session.PrefixFor(rig))
	default:
		// Assume polecat
		if strings.HasPrefix(role, "crew/") {
			crewName := strings.TrimPrefix(role, "crew/")
			return session.CrewSessionName(session.PrefixFor(rig), crewName)
		}
		if strings.HasPrefix(role, "polecats/") {
			pcName := strings.TrimPrefix(role, "polecats/")
			return session.PolecatSessionName(session.PrefixFor(rig), pcName)
		}
		return session.PolecatSessionName(session.PrefixFor(rig), role)
	}
}
