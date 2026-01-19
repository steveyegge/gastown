package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var handoffCmd = &cobra.Command{
	Use:     "handoff [bead-or-role]",
	GroupID: GroupWork,
	Short:   "Hand off to a fresh session, work continues from hook",
	Long: `End watch. Hand off to a fresh agent session.

This is the canonical way to end any agent session. It handles all roles:

  - Mayor, Crew, Witness, Refinery, Deacon: Respawns with fresh Claude instance
  - Polecats: Calls 'gt done --status DEFERRED' (Witness handles lifecycle)

When run without arguments, hands off the current session.
When given a bead ID (gt-xxx, hq-xxx), hooks that work first, then restarts.
When given a role name, hands off that role's session (and switches to it).

Examples:
  gt handoff                          # Hand off current session
  gt handoff gt-abc                   # Hook bead, then restart
  gt handoff gt-abc -s "Fix it"       # Hook with context, then restart
  gt handoff -s "Context" -m "Notes"  # Hand off with custom message
  gt handoff -c                       # Collect state into handoff message
  gt handoff crew                     # Hand off crew session
  gt handoff mayor                    # Hand off mayor session

The --collect (-c) flag gathers current state (hooked work, inbox, ready beads,
in-progress items) and includes it in the handoff mail. This provides context
for the next session without manual summarization.

Any molecule on the hook will be auto-continued by the new session.
The SessionStart hook runs 'gt prime' to restore context.`,
	RunE: runHandoff,
}

var (
	handoffWatch   bool
	handoffDryRun  bool
	handoffSubject string
	handoffMessage string
	handoffCollect bool
)

func init() {
	handoffCmd.Flags().BoolVarP(&handoffWatch, "watch", "w", true, "Switch to new session (for remote handoff)")
	handoffCmd.Flags().BoolVarP(&handoffDryRun, "dry-run", "n", false, "Show what would be done without executing")
	handoffCmd.Flags().StringVarP(&handoffSubject, "subject", "s", "", "Subject for handoff mail (optional)")
	handoffCmd.Flags().StringVarP(&handoffMessage, "message", "m", "", "Message body for handoff mail (optional)")
	handoffCmd.Flags().BoolVarP(&handoffCollect, "collect", "c", false, "Auto-collect state (status, inbox, beads) into handoff message")
	rootCmd.AddCommand(handoffCmd)
}

func runHandoff(cmd *cobra.Command, args []string) error {
	// Check if we're a polecat - polecats use gt done instead
	// GT_POLECAT is set by the session manager when starting polecat sessions
	if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
		fmt.Printf("%s Polecat detected (%s) - using gt done for handoff\n",
			style.Bold.Render("🐾"), polecatName)
		// Polecats don't respawn themselves - Witness handles lifecycle
		// Call gt done with DEFERRED exit type to preserve work state
		doneCmd := exec.Command("gt", "done", "--exit", "DEFERRED")
		doneCmd.Stdout = os.Stdout
		doneCmd.Stderr = os.Stderr
		return doneCmd.Run()
	}

	// If --collect flag is set, auto-collect state into the message
	if handoffCollect {
		collected := collectHandoffState()
		if handoffMessage == "" {
			handoffMessage = collected
		} else {
			handoffMessage = handoffMessage + "\n\n---\n" + collected
		}
		if handoffSubject == "" {
			handoffSubject = "Session handoff with context"
		}
	}

	// Get current agent ID from environment
	currentID, err := agent.Self()
	if err != nil {
		return fmt.Errorf("identifying current agent: %w", err)
	}

	// Determine target agent and check for bead hook
	targetID := currentID
	if len(args) > 0 {
		arg := args[0]

		// Check if arg is a bead ID (gt-xxx, hq-xxx, bd-xxx, etc.)
		if looksLikeBeadID(arg) {
			// Hook the bead first
			if err := hookBeadForHandoff(arg); err != nil {
				return fmt.Errorf("hooking bead: %w", err)
			}
			// Update subject if not set
			if handoffSubject == "" {
				handoffSubject = fmt.Sprintf("🪝 HOOKED: %s", arg)
			}
		} else {
			// User specified a role to hand off
			targetID, err = resolveRoleToAgentID(arg)
			if err != nil {
				return fmt.Errorf("resolving role: %w", err)
			}
		}
	}

	// Get town root for agents and logging
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Create agents manager for respawn
	agents := agent.Default()

	// If handing off a different agent, use remote handoff flow
	if targetID != currentID {
		return handoffRemoteAgent(agents, targetID, townRoot)
	}

	// Handing off ourselves - print feedback then respawn
	fmt.Printf("%s Handing off %s...\n", style.Bold.Render("🤝"), targetID)

	// Log handoff event (both townlog and events feed)
	_ = LogHandoff(townRoot, targetID.String(), handoffSubject)
	_ = events.LogFeed(events.TypeHandoff, targetID.String(), events.HandoffPayload(handoffSubject, true))

	// Dry run mode - show what would happen (BEFORE any side effects)
	if handoffDryRun {
		if handoffSubject != "" || handoffMessage != "" {
			fmt.Printf("Would send handoff mail: subject=%q (auto-hooked)\n", handoffSubject)
		}
		fmt.Printf("Would respawn agent %s (reusing original command)\n", targetID)
		return nil
	}

	// If subject/message provided, send handoff mail to self first
	// The mail is auto-hooked so the next session picks it up
	if handoffSubject != "" || handoffMessage != "" {
		beadID, err := sendHandoffMail(handoffSubject, handoffMessage)
		if err != nil {
			style.PrintWarning("could not send handoff mail: %v", err)
			// Continue anyway - the respawn is more important
		} else {
			fmt.Printf("%s Sent handoff mail %s (auto-hooked)\n", style.Bold.Render("📬"), beadID)
		}
	}

	// Write handoff marker for successor detection (prevents handoff loop bug).
	// The marker is cleared by gt prime after it outputs the warning.
	// This tells the new session "you're post-handoff, don't re-run /handoff"
	if cwd, err := os.Getwd(); err == nil {
		runtimeDir := filepath.Join(cwd, constants.DirRuntime)
		_ = os.MkdirAll(runtimeDir, 0755)
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		_ = os.WriteFile(markerPath, []byte(targetID.String()), 0644)
	}

	// Respawn the agent - for self-handoff, this terminates the current process
	// Original command is reused; new session discovers handoff mail via hooks
	return agents.Respawn(targetID)
}

// getCurrentTmuxSession returns the current agent's session-style identifier.
// Used by cycle commands for tmux session navigation.
func getCurrentTmuxSession() (string, error) {
	id, err := agent.Self()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// resolveRoleToAgentID converts a role name or path to an AgentID.
// Accepts:
//   - Role shortcuts: "crew", "witness", "refinery", "mayor", "deacon"
//   - Full paths: "<rig>/crew/<name>", "<rig>/witness", "<rig>/refinery"
//
// For role shortcuts that need context (crew, witness, refinery), it auto-detects from environment.
func resolveRoleToAgentID(role string) (agent.AgentID, error) {
	// First, check if it's a path format (contains /)
	if strings.Contains(role, "/") {
		return resolvePathToAgentID(role)
	}

	switch strings.ToLower(role) {
	case "mayor", "may":
		return agent.MayorAddress, nil

	case "deacon", "dea":
		return agent.DeaconAddress, nil

	case "crew":
		// Try to get rig and crew name from environment or cwd
		rig := os.Getenv("GT_RIG")
		crewName := os.Getenv("GT_CREW")
		if rig == "" || crewName == "" {
			// Try to detect from cwd
			detected, err := detectCrewFromCwd()
			if err == nil {
				rig = detected.rigName
				crewName = detected.crewName
			}
		}
		if rig == "" || crewName == "" {
			return agent.AgentID{}, fmt.Errorf("cannot determine crew identity - run from crew directory or specify GT_RIG/GT_CREW")
		}
		return agent.CrewAddress(rig, crewName), nil

	case "witness", "wit":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return agent.AgentID{}, fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return agent.WitnessAddress(rig), nil

	case "refinery", "ref":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return agent.AgentID{}, fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return agent.RefineryAddress(rig), nil

	default:
		return agent.AgentID{}, fmt.Errorf("unknown role: %s", role)
	}
}

// resolvePathToAgentID converts a path like "<rig>/crew/<name>" to an AgentID.
// Supported formats:
//   - <rig>/crew/<name> -> rig/crew/name
//   - <rig>/witness -> rig/witness
//   - <rig>/refinery -> rig/refinery
//   - <rig>/polecats/<name> -> rig/polecat/name
//   - <rig>/polecat/<name> -> rig/polecat/name
func resolvePathToAgentID(path string) (agent.AgentID, error) {
	parts := strings.Split(path, "/")

	// Handle <rig>/crew/<name> format
	if len(parts) == 3 && parts[1] == "crew" {
		return agent.CrewAddress(parts[0], parts[2]), nil
	}

	// Handle <rig>/polecats/<name> or <rig>/polecat/<name> format
	if len(parts) == 3 && (parts[1] == "polecats" || parts[1] == "polecat") {
		return agent.PolecatAddress(parts[0], parts[2]), nil
	}

	// Handle <rig>/<role> format
	if len(parts) == 2 {
		rig := parts[0]
		second := strings.ToLower(parts[1])

		switch second {
		case "witness":
			return agent.WitnessAddress(rig), nil
		case "refinery":
			return agent.RefineryAddress(rig), nil
		case "crew":
			return agent.AgentID{}, fmt.Errorf("crew path requires name: %s/crew/<name>", rig)
		case "polecats", "polecat":
			return agent.AgentID{}, fmt.Errorf("polecat path requires name: %s/polecat/<name>", rig)
		default:
			// Check if it's a crew member before assuming polecat
			townRoot := detectTownRootFromCwd()
			if townRoot != "" {
				crewPath := filepath.Join(townRoot, rig, "crew", parts[1])
				if info, err := os.Stat(crewPath); err == nil && info.IsDir() {
					return agent.CrewAddress(rig, parts[1]), nil
				}
			}
			// Assume polecat
			return agent.PolecatAddress(rig, parts[1]), nil
		}
	}

	return agent.AgentID{}, fmt.Errorf("cannot parse path '%s' - expected <rig>/<polecat>, <rig>/crew/<name>, <rig>/witness, or <rig>/refinery", path)
}

// handoffRemoteAgent respawns a different agent and optionally switches to its session.
func handoffRemoteAgent(agents agent.Agents, targetID agent.AgentID, _ string) error {
	// Check if target agent exists
	if !agents.Exists(targetID) {
		return fmt.Errorf("agent '%s' not found - is it running?", targetID)
	}

	fmt.Printf("%s Handing off %s...\n", style.Bold.Render("🤝"), targetID)

	// Dry run mode
	if handoffDryRun {
		fmt.Printf("Would respawn agent %s (reusing original command)\n", targetID)
		if handoffWatch {
			fmt.Printf("Would switch to session: %s\n", targetID)
		}
		return nil
	}

	// Respawn the remote agent - original command is reused
	// The new session discovers work via hooks
	if err := agents.Respawn(targetID); err != nil {
		return fmt.Errorf("respawning agent: %w", err)
	}

	// If --watch, attach/switch to the new session
	// Smart Attach handles context: switch if in tmux, attach if outside
	if handoffWatch {
		fmt.Printf("Switching to %s...\n", targetID)
		if err := agents.Attach(targetID); err != nil {
			fmt.Printf("Note: Could not auto-switch (use: tmux switch-client -t %s)\n", targetID)
		}
	}

	return nil
}

// detectTownRootFromCwd walks up from the current directory to find the town root.
func detectTownRootFromCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := cwd
	for {
		// Check for primary marker (mayor/town.json)
		markerPath := filepath.Join(dir, "mayor", "town.json")
		if _, err := os.Stat(markerPath); err == nil {
			return dir
		}

		// Move up
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// sendHandoffMail sends a handoff mail to self and auto-hooks it.
// Returns the created bead ID and any error.
func sendHandoffMail(subject, message string) (string, error) {
	// Build subject with handoff prefix if not already present
	if subject == "" {
		subject = "🤝 HANDOFF: Session cycling"
	} else if !strings.Contains(subject, "HANDOFF") {
		subject = "🤝 HANDOFF: " + subject
	}

	// Default message if not provided
	if message == "" {
		message = "Context cycling. Check bd ready for pending work."
	}

	// Detect agent identity for self-mail
	agentID, err := resolveSelfTarget()
	if err != nil {
		return "", fmt.Errorf("detecting agent identity: %w", err)
	}

	// Detect town root for beads location
	townRoot := detectTownRootFromCwd()
	if townRoot == "" {
		return "", fmt.Errorf("cannot detect town root")
	}

	// Build labels for mail metadata (matches mail router format)
	labels := fmt.Sprintf("from:%s", agentID)

	// Create mail bead directly using bd create with --silent to get the ID
	// Mail goes to town-level beads (hq- prefix)
	args := []string{
		"create", subject,
		"--type", "message",
		"--assignee", agentID,
		"-d", message,
		"--priority", "2",
		"--labels", labels,
		"--actor", agentID,
		"--ephemeral", // Handoff mail is ephemeral
		"--silent",    // Output only the bead ID
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = townRoot // Run from town root for town-level beads
	cmd.Env = append(os.Environ(), "BEADS_DIR="+filepath.Join(townRoot, ".beads"))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("creating handoff mail: %s", errMsg)
		}
		return "", fmt.Errorf("creating handoff mail: %w", err)
	}

	beadID := strings.TrimSpace(stdout.String())
	if beadID == "" {
		return "", fmt.Errorf("bd create did not return bead ID")
	}

	// Auto-hook the created mail bead
	hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+agentID)
	hookCmd.Dir = townRoot
	hookCmd.Env = append(os.Environ(), "BEADS_DIR="+filepath.Join(townRoot, ".beads"))
	hookCmd.Stderr = os.Stderr

	if err := hookCmd.Run(); err != nil {
		// Non-fatal: mail was created, just couldn't hook
		style.PrintWarning("created mail %s but failed to auto-hook: %v", beadID, err)
		return beadID, nil
	}

	return beadID, nil
}

// looksLikeBeadID checks if a string looks like a bead ID.
// Bead IDs have format: prefix-xxxx where prefix is 1-5 lowercase letters and xxxx is alphanumeric.
// Examples: "gt-abc123", "bd-ka761", "hq-cv-abc", "beads-xyz", "ap-qtsup.16"
func looksLikeBeadID(s string) bool {
	// Find the first hyphen
	idx := strings.Index(s, "-")
	if idx < 1 || idx > 5 {
		// No hyphen, or prefix is empty/too long
		return false
	}

	// Check prefix is all lowercase letters
	prefix := s[:idx]
	for _, c := range prefix {
		if c < 'a' || c > 'z' {
			return false
		}
	}

	// Check there's something after the hyphen
	rest := s[idx+1:]
	if len(rest) == 0 {
		return false
	}

	// Check rest starts with alphanumeric and contains only alphanumeric, dots, hyphens
	first := rest[0]
	if !((first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		return false
	}

	return true
}

// hookBeadForHandoff attaches a bead to the current agent's hook.
func hookBeadForHandoff(beadID string) error {
	// Verify the bead exists first
	verifyCmd := exec.Command("bd", "show", beadID, "--json")
	if err := verifyCmd.Run(); err != nil {
		return fmt.Errorf("bead '%s' not found", beadID)
	}

	// Determine agent identity
	agentID, err := resolveSelfTarget()
	if err != nil {
		return fmt.Errorf("detecting agent identity: %w", err)
	}

	fmt.Printf("%s Hooking %s...\n", style.Bold.Render("🪝"), beadID)

	if handoffDryRun {
		fmt.Printf("Would run: bd update %s --status=pinned --assignee=%s\n", beadID, agentID)
		return nil
	}

	// Pin the bead using bd update (discovery-based approach)
	pinCmd := exec.Command("bd", "update", beadID, "--status=pinned", "--assignee="+agentID)
	pinCmd.Stderr = os.Stderr
	if err := pinCmd.Run(); err != nil {
		return fmt.Errorf("pinning bead: %w", err)
	}

	fmt.Printf("%s Work attached to hook (pinned bead)\n", style.Bold.Render("✓"))
	return nil
}

// collectHandoffState gathers current state for handoff context.
// Collects: inbox summary, ready beads, hooked work.
func collectHandoffState() string {
	var parts []string

	// Get hooked work
	hookOutput, err := exec.Command("gt", "hook").Output()
	if err == nil {
		hookStr := strings.TrimSpace(string(hookOutput))
		if hookStr != "" && !strings.Contains(hookStr, "Nothing on hook") {
			parts = append(parts, "## Hooked Work\n"+hookStr)
		}
	}

	// Get inbox summary (first few messages)
	inboxOutput, err := exec.Command("gt", "mail", "inbox").Output()
	if err == nil {
		inboxStr := strings.TrimSpace(string(inboxOutput))
		if inboxStr != "" && !strings.Contains(inboxStr, "Inbox empty") {
			// Limit to first 10 lines for brevity
			lines := strings.Split(inboxStr, "\n")
			if len(lines) > 10 {
				lines = append(lines[:10], "... (more messages)")
			}
			parts = append(parts, "## Inbox\n"+strings.Join(lines, "\n"))
		}
	}

	// Get ready beads
	readyOutput, err := exec.Command("bd", "ready").Output()
	if err == nil {
		readyStr := strings.TrimSpace(string(readyOutput))
		if readyStr != "" && !strings.Contains(readyStr, "No issues ready") {
			// Limit to first 10 lines
			lines := strings.Split(readyStr, "\n")
			if len(lines) > 10 {
				lines = append(lines[:10], "... (more issues)")
			}
			parts = append(parts, "## Ready Work\n"+strings.Join(lines, "\n"))
		}
	}

	// Get in-progress beads
	inProgressOutput, err := exec.Command("bd", "list", "--status=in_progress").Output()
	if err == nil {
		ipStr := strings.TrimSpace(string(inProgressOutput))
		if ipStr != "" && !strings.Contains(ipStr, "No issues") {
			lines := strings.Split(ipStr, "\n")
			if len(lines) > 5 {
				lines = append(lines[:5], "... (more)")
			}
			parts = append(parts, "## In Progress\n"+strings.Join(lines, "\n"))
		}
	}

	if len(parts) == 0 {
		return "No active state to report."
	}

	return strings.Join(parts, "\n\n")
}
