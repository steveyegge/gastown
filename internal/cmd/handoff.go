package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
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

	t := tmux.NewTmux()

	// Verify we're in tmux
	if !tmux.IsInsideTmux() {
		return fmt.Errorf("not running in tmux - cannot hand off")
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return fmt.Errorf("TMUX_PANE not set - cannot hand off")
	}

	// Get current session name
	currentSession, err := getCurrentTmuxSession()
	if err != nil {
		return fmt.Errorf("getting session name: %w", err)
	}

	// Determine target session and check for bead hook
	targetSession := currentSession
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
			targetSession, err = resolveRoleToSession(arg)
			if err != nil {
				return fmt.Errorf("resolving role: %w", err)
			}
		}
	}

	// Build the restart command
	restartCmd, err := buildRestartCommand(targetSession)
	if err != nil {
		return err
	}

	// If handing off a different session, we need to find its pane and respawn there
	if targetSession != currentSession {
		return handoffRemoteSession(t, targetSession, restartCmd)
	}

	// Handing off ourselves - print feedback then respawn
	fmt.Printf("%s Handing off %s...\n", style.Bold.Render("🤝"), currentSession)

	// Log handoff event (both townlog and events feed)
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		agent := sessionToGTRole(currentSession)
		if agent == "" {
			agent = currentSession
		}
		_ = LogHandoff(townRoot, agent, handoffSubject)
		// Also log to activity feed
		_ = events.LogFeed(events.TypeHandoff, agent, events.HandoffPayload(handoffSubject, true))
	}

	// Dry run mode - show what would happen (BEFORE any side effects)
	if handoffDryRun {
		if handoffSubject != "" || handoffMessage != "" {
			fmt.Printf("Would send handoff mail: subject=%q (auto-hooked)\n", handoffSubject)
		}
		fmt.Printf("Would execute: tmux clear-history -t %s\n", pane)
		fmt.Printf("Would execute: tmux respawn-pane -k -t %s %s\n", pane, restartCmd)
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

	// NOTE: reportAgentState("stopped") removed (gt-zecmc)
	// Agent liveness is observable from tmux - no need to record it in bead.
	// "Discover, don't track" principle: reality is truth, state is derived.

	// Clear scrollback history before respawn (resets copy-mode from [0/N] to [0/0])
	if err := t.ClearHistory(pane); err != nil {
		// Non-fatal - continue with respawn even if clear fails
		style.PrintWarning("could not clear history: %v", err)
	}

	// Write handoff marker for successor detection (prevents handoff loop bug).
	// The marker is cleared by gt prime after it outputs the warning.
	// This tells the new session "you're post-handoff, don't re-run /handoff"
	if cwd, err := os.Getwd(); err == nil {
		runtimeDir := filepath.Join(cwd, constants.DirRuntime)
		_ = os.MkdirAll(runtimeDir, 0755)
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		_ = os.WriteFile(markerPath, []byte(currentSession), 0644)
	}

	// Use exec to respawn the pane - this kills us and restarts
	return t.RespawnPane(pane, restartCmd)
}

// getCurrentTmuxSession returns the current tmux session name.
func getCurrentTmuxSession() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveRoleToSession converts a role name or path to a tmux session name.
// Accepts:
//   - Role shortcuts: "crew", "witness", "refinery", "mayor", "deacon"
//   - Full paths: "<rig>/crew/<name>", "<rig>/witness", "<rig>/refinery"
//   - Direct session names (passed through)
//
// For role shortcuts that need context (crew, witness, refinery), it auto-detects from environment.
func resolveRoleToSession(role string) (string, error) {
	// First, check if it's a path format (contains /)
	if strings.Contains(role, "/") {
		return resolvePathToSession(role)
	}

	switch strings.ToLower(role) {
	case "mayor", "may":
		return getMayorSessionName(), nil

	case "deacon", "dea":
		return getDeaconSessionName(), nil

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
			return "", fmt.Errorf("cannot determine crew identity - run from crew directory or specify GT_RIG/GT_CREW")
		}
		return fmt.Sprintf("gt-%s-crew-%s", rig, crewName), nil

	case "witness", "wit":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return "", fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return fmt.Sprintf("gt-%s-witness", rig), nil

	case "refinery", "ref":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return "", fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return fmt.Sprintf("gt-%s-refinery", rig), nil

	default:
		// Assume it's a direct session name (e.g., gt-gastown-crew-max)
		return role, nil
	}
}

// resolvePathToSession converts a path like "<rig>/crew/<name>" to a session name.
// Supported formats:
//   - <rig>/crew/<name> -> gt-<rig>-crew-<name>
//   - <rig>/witness -> gt-<rig>-witness
//   - <rig>/refinery -> gt-<rig>-refinery
//   - <rig>/polecats/<name> -> gt-<rig>-<name> (explicit polecat)
//   - <rig>/<name> -> gt-<rig>-<name> (polecat shorthand, if name isn't a known role)
func resolvePathToSession(path string) (string, error) {
	parts := strings.Split(path, "/")

	// Handle <rig>/crew/<name> format
	if len(parts) == 3 && parts[1] == "crew" {
		rig := parts[0]
		name := parts[2]
		return fmt.Sprintf("gt-%s-crew-%s", rig, name), nil
	}

	// Handle <rig>/polecats/<name> format (explicit polecat path)
	if len(parts) == 3 && parts[1] == "polecats" {
		rig := parts[0]
		name := strings.ToLower(parts[2]) // normalize polecat name
		return fmt.Sprintf("gt-%s-%s", rig, name), nil
	}

	// Handle <rig>/<role-or-polecat> format
	if len(parts) == 2 {
		rig := parts[0]
		second := parts[1]
		secondLower := strings.ToLower(second)

		// Check for known roles first
		switch secondLower {
		case "witness":
			return fmt.Sprintf("gt-%s-witness", rig), nil
		case "refinery":
			return fmt.Sprintf("gt-%s-refinery", rig), nil
		case "crew":
			// Just "<rig>/crew" without a name - need more info
			return "", fmt.Errorf("crew path requires name: %s/crew/<name>", rig)
		case "polecats":
			// Just "<rig>/polecats" without a name - need more info
			return "", fmt.Errorf("polecats path requires name: %s/polecats/<name>", rig)
		default:
			// Not a known role - treat as polecat name (e.g., gastown/nux)
			return fmt.Sprintf("gt-%s-%s", rig, secondLower), nil
		}
	}

	return "", fmt.Errorf("cannot parse path '%s' - expected <rig>/<polecat>, <rig>/crew/<name>, <rig>/witness, or <rig>/refinery", path)
}

// claudeEnvVars lists the Claude-related environment variables to propagate
// during handoff. These vars aren't inherited by tmux respawn-pane's fresh shell.
var claudeEnvVars = []string{
	// Claude API and config
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_USE_BEDROCK",
	// AWS vars for Bedrock
	"AWS_PROFILE",
	"AWS_REGION",
}

// buildRestartCommand creates the command to run when respawning a session's pane.
// This needs to be the actual command to execute (e.g., claude), not a session attach command.
// The command includes a cd to the correct working directory for the role.
func buildRestartCommand(sessionName string) (string, error) {
	// Detect town root from current directory
	townRoot := detectTownRootFromCwd()
	if townRoot == "" {
		return "", fmt.Errorf("cannot detect town root - run from within a Gas Town workspace")
	}

	// Determine the working directory for this session type
	workDir, err := sessionWorkDir(sessionName, townRoot)
	if err != nil {
		return "", err
	}

	// Parse the session name to get the identity (used for GT_ROLE and beacon)
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		return "", fmt.Errorf("cannot parse session name %q: %w", sessionName, err)
	}
	gtRole := identity.GTRole()

	// Build startup beacon for predecessor discovery via /resume
	// Use FormatStartupNudge instead of bare "gt prime" which confuses agents
	// The SessionStart hook handles context injection (gt prime --hook)
	beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
		Recipient: identity.Address(),
		Sender:    "self",
		Topic:     "handoff",
	})

	// For respawn-pane, we:
	// 1. cd to the right directory (role's canonical home)
	// 2. export GT_ROLE and BD_ACTOR so role detection works correctly
	// 3. export Claude-related env vars (not inherited by fresh shell)
	// 4. run claude with the startup beacon (triggers immediate context loading)
	// Use exec to ensure clean process replacement.
	runtimeCmd := config.GetRuntimeCommandWithPrompt("", beacon)

	// Build environment exports - role vars first, then Claude vars
	var exports []string
	if gtRole != "" {
		runtimeConfig := config.LoadRuntimeConfig("")
		exports = append(exports, "GT_ROLE="+gtRole)
		exports = append(exports, "BD_ACTOR="+gtRole)
		exports = append(exports, "GIT_AUTHOR_NAME="+gtRole)
		if runtimeConfig.Session != nil && runtimeConfig.Session.SessionIDEnv != "" {
			exports = append(exports, "GT_SESSION_ID_ENV="+runtimeConfig.Session.SessionIDEnv)
		}
	}

	// Add Claude-related env vars from current environment
	for _, name := range claudeEnvVars {
		if val := os.Getenv(name); val != "" {
			// Shell-escape the value in case it contains special chars
			exports = append(exports, fmt.Sprintf("%s=%q", name, val))
		}
	}

	if len(exports) > 0 {
		return fmt.Sprintf("cd %s && export %s && exec %s", workDir, strings.Join(exports, " "), runtimeCmd), nil
	}
	return fmt.Sprintf("cd %s && exec %s", workDir, runtimeCmd), nil
}

// sessionWorkDir returns the correct working directory for a session.
// This is the canonical home for each role type.
func sessionWorkDir(sessionName, townRoot string) (string, error) {
	// Get session names for comparison
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()

	switch {
	case sessionName == mayorSession:
		return townRoot, nil

	case sessionName == deaconSession:
		return townRoot + "/deacon", nil

	case strings.Contains(sessionName, "-crew-"):
		// gt-<rig>-crew-<name> -> <townRoot>/<rig>/crew/<name>
		parts := strings.Split(sessionName, "-")
		if len(parts) < 4 {
			return "", fmt.Errorf("invalid crew session name: %s", sessionName)
		}
		// Find the index of "crew" to split rig name (may contain dashes)
		for i, p := range parts {
			if p == "crew" && i > 1 && i < len(parts)-1 {
				rig := strings.Join(parts[1:i], "-")
				name := strings.Join(parts[i+1:], "-")
				return fmt.Sprintf("%s/%s/crew/%s", townRoot, rig, name), nil
			}
		}
		return "", fmt.Errorf("cannot parse crew session name: %s", sessionName)

	case strings.HasSuffix(sessionName, "-witness"):
		// gt-<rig>-witness -> <townRoot>/<rig>/witness/rig
		rig := strings.TrimPrefix(sessionName, "gt-")
		rig = strings.TrimSuffix(rig, "-witness")
		return fmt.Sprintf("%s/%s/witness/rig", townRoot, rig), nil

	case strings.HasSuffix(sessionName, "-refinery"):
		// gt-<rig>-refinery -> <townRoot>/<rig>/refinery/rig
		rig := strings.TrimPrefix(sessionName, "gt-")
		rig = strings.TrimSuffix(rig, "-refinery")
		return fmt.Sprintf("%s/%s/refinery/rig", townRoot, rig), nil

	default:
		// Assume polecat: gt-<rig>-<name> -> <townRoot>/<rig>/polecats/<name>
		// Use session.ParseSessionName to determine rig and name
		identity, err := session.ParseSessionName(sessionName)
		if err != nil {
			return "", fmt.Errorf("unknown session type: %s (%w)", sessionName, err)
		}
		if identity.Role != session.RolePolecat {
			return "", fmt.Errorf("unknown session type: %s (role %s, try specifying role explicitly)", sessionName, identity.Role)
		}
		return fmt.Sprintf("%s/%s/polecats/%s", townRoot, identity.Rig, identity.Name), nil
	}
}

// sessionToGTRole converts a session name to a GT_ROLE value.
// Uses session.ParseSessionName for consistent parsing across the codebase.
func sessionToGTRole(sessionName string) string {
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		return ""
	}
	return identity.GTRole()
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

// handoffRemoteSession respawns a different session and optionally switches to it.
func handoffRemoteSession(t *tmux.Tmux, targetSession, restartCmd string) error {
	// Check if target session exists
	exists, err := t.HasSession(targetSession)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' not found - is the agent running?", targetSession)
	}

	// Get the pane ID for the target session
	targetPane, err := getSessionPane(targetSession)
	if err != nil {
		return fmt.Errorf("getting target pane: %w", err)
	}

	fmt.Printf("%s Handing off %s...\n", style.Bold.Render("🤝"), targetSession)

	// Dry run mode
	if handoffDryRun {
		fmt.Printf("Would execute: tmux clear-history -t %s\n", targetPane)
		fmt.Printf("Would execute: tmux respawn-pane -k -t %s %s\n", targetPane, restartCmd)
		if handoffWatch {
			fmt.Printf("Would execute: tmux switch-client -t %s\n", targetSession)
		}
		return nil
	}

	// Clear scrollback history before respawn (resets copy-mode from [0/N] to [0/0])
	if err := t.ClearHistory(targetPane); err != nil {
		// Non-fatal - continue with respawn even if clear fails
		style.PrintWarning("could not clear history: %v", err)
	}

	// Respawn the remote session's pane
	if err := t.RespawnPane(targetPane, restartCmd); err != nil {
		return fmt.Errorf("respawning pane: %w", err)
	}

	// If --watch, switch to that session
	if handoffWatch {
		fmt.Printf("Switching to %s...\n", targetSession)
		// Use tmux switch-client to move our view to the target session
		if err := exec.Command("tmux", "switch-client", "-t", targetSession).Run(); err != nil {
			// Non-fatal - they can manually switch
			fmt.Printf("Note: Could not auto-switch (use: tmux switch-client -t %s)\n", targetSession)
		}
	}

	return nil
}

// getSessionPane returns the pane identifier for a session's main pane.
func getSessionPane(sessionName string) (string, error) {
	// Get the pane ID for the first pane in the session
	out, err := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no panes found in session")
	}
	return lines[0], nil
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
	agentID, _, _, err := resolveSelfTarget()
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
	agentID, _, _, err := resolveSelfTarget()
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
