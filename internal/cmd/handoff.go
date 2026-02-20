package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
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

The --cycle flag triggers automatic session cycling (used by PreCompact hooks).
Unlike --auto (state only) or normal handoff (polecat→gt-done redirect), --cycle
always does a full respawn regardless of role. This enables crew workers and
polecats to get a fresh context window when the current one fills up.

Any molecule on the hook will be auto-continued by the new session.
The SessionStart hook runs 'gt prime' to restore context.`,
	RunE: runHandoff,
}

var (
	handoffWatch      bool
	handoffDryRun     bool
	handoffSubject    string
	handoffMessage    string
	handoffCollect    bool
	handoffStdin      bool
	handoffAuto       bool
	handoffCycle      bool
	handoffReason     string
	handoffNoGitCheck bool
)

func init() {
	handoffCmd.Flags().BoolVarP(&handoffWatch, "watch", "w", true, "Switch to new session (for remote handoff)")
	handoffCmd.Flags().BoolVarP(&handoffDryRun, "dry-run", "n", false, "Show what would be done without executing")
	handoffCmd.Flags().StringVarP(&handoffSubject, "subject", "s", "", "Subject for handoff mail (optional)")
	handoffCmd.Flags().StringVarP(&handoffMessage, "message", "m", "", "Message body for handoff mail (optional)")
	handoffCmd.Flags().BoolVarP(&handoffCollect, "collect", "c", false, "Auto-collect state (status, inbox, beads) into handoff message")
	handoffCmd.Flags().BoolVar(&handoffStdin, "stdin", false, "Read message body from stdin (avoids shell quoting issues)")
	handoffCmd.Flags().BoolVar(&handoffAuto, "auto", false, "Save state only, no session cycling (for PreCompact hooks)")
	handoffCmd.Flags().BoolVar(&handoffCycle, "cycle", false, "Auto-cycle session (for PreCompact hooks that want full session replacement)")
	handoffCmd.Flags().StringVar(&handoffReason, "reason", "", "Reason for handoff (e.g., 'compaction', 'idle')")
	handoffCmd.Flags().BoolVar(&handoffNoGitCheck, "no-git-check", false, "Skip git workspace cleanliness check")
	rootCmd.AddCommand(handoffCmd)
}

func runHandoff(cmd *cobra.Command, args []string) error {
	// Handle --stdin: read message body from stdin (avoids shell quoting issues)
	if handoffStdin {
		if handoffMessage != "" {
			return fmt.Errorf("cannot use --stdin with --message/-m")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		handoffMessage = strings.TrimRight(string(data), "\n")
	}

	// --auto mode: save state only, no session cycling.
	// Used by PreCompact hook to preserve state before compaction.
	// Note: auto-mode exits here, before the git-status warning check below.
	// This is intentional — auto-handoffs are triggered by hooks and should not
	// spam warnings. The --no-git-check flag has no effect in auto mode.
	if handoffAuto {
		return runHandoffAuto()
	}

	// --cycle mode: full session cycling, triggered by PreCompact hook.
	// Unlike --auto (state only), this replaces the current session with a fresh one.
	// Unlike normal handoff, this skips the polecat→gt-done redirect because
	// cycling preserves work state (the hook stays attached).
	//
	// Flow: collect state → send handoff mail → respawn pane (fresh Claude instance)
	// The successor session picks up hooked work via SessionStart hook (gt prime --hook).
	if handoffCycle {
		return runHandoffCycle()
	}

	// Check if we're a polecat - polecats use gt done instead.
	// Check GT_ROLE first: coordinators (mayor, witness, etc.) may have a stale
	// GT_POLECAT in their environment from spawning polecats. Only block if the
	// parsed role is actually polecat (handles compound forms like
	// "gastown/polecats/Toast"). If GT_ROLE is unset, fall back to GT_POLECAT.
	isPolecat := false
	polecatName := ""
	if role := os.Getenv("GT_ROLE"); role != "" {
		parsedRole, _, name := parseRoleString(role)
		if parsedRole == RolePolecat {
			isPolecat = true
			polecatName = name
			// Bare "polecat" role yields empty name; fall back to GT_POLECAT.
			if polecatName == "" {
				polecatName = os.Getenv("GT_POLECAT")
			}
		}
	} else if name := os.Getenv("GT_POLECAT"); name != "" {
		isPolecat = true
		polecatName = name
	}
	if isPolecat {
		fmt.Printf("%s Polecat detected (%s) - using gt done for handoff\n",
			style.Bold.Render("🐾"), polecatName)
		// Polecats don't respawn themselves - Witness handles lifecycle
		// Call gt done with DEFERRED status to preserve work state
		doneCmd := exec.Command("gt", "done", "--status", "DEFERRED")
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

	// Warn if workspace has uncommitted or unpushed work (wa-7967c).
	// Note: this checks the caller's cwd, not the target session's workdir.
	// For remote handoff (gt handoff <role>), the warning reflects the caller's
	// workspace state. Checking the target session's workdir would require tmux
	// pane introspection and is deferred to a future enhancement.
	if !handoffNoGitCheck {
		warnHandoffGitStatus()
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
		// Update tmux session env before respawn (not during dry-run — see below)
		updateSessionEnvForHandoff(t, targetSession, "")
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

	// Update tmux session environment for liveness detection.
	// IsAgentAlive reads GT_PROCESS_NAMES via tmux show-environment (session env),
	// not from shell exports. The restart command sets shell exports for the child
	// process, but we must also update the session env so liveness checks work.
	// Placed after the dry-run guard to avoid mutating session state during dry-run.
	updateSessionEnvForHandoff(t, currentSession, "")

	// Send handoff mail to self (defaults applied inside sendHandoffMail).
	// The mail is auto-hooked so the next session picks it up.
	beadID, err := sendHandoffMail(handoffSubject, handoffMessage)
	if err != nil {
		style.PrintWarning("could not send handoff mail: %v", err)
		// Continue anyway - the respawn is more important
	} else {
		fmt.Printf("%s Sent handoff mail %s (auto-hooked)\n", style.Bold.Render("📬"), beadID)
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

	// Set remain-on-exit so the pane survives process death during handoff.
	// Without this, killing processes causes tmux to destroy the pane before
	// we can respawn it. This is essential for tmux session reuse.
	if err := t.SetRemainOnExit(pane, true); err != nil {
		style.PrintWarning("could not set remain-on-exit: %v", err)
	}

	// NOTE: For self-handoff, we do NOT call KillPaneProcesses here.
	// That would kill the gt handoff process itself before it can call RespawnPane,
	// leaving the pane dead with no respawn. RespawnPane's -k flag handles killing
	// atomically - tmux kills the old process and spawns the new one together.
	// See: https://github.com/steveyegge/gastown/issues/859 (pane is dead bug)
	//
	// For orphan prevention, we rely on respawn-pane -k which sends SIGHUP/SIGTERM.
	// If orphans still occur, the solution is to adjust the restart command to
	// kill orphans at startup, not to kill ourselves before respawning.

	// Check if pane's working directory exists (may have been deleted)
	paneWorkDir, _ := t.GetPaneWorkDir(currentSession)
	if paneWorkDir != "" {
		if _, err := os.Stat(paneWorkDir); err != nil {
			if townRoot := detectTownRootFromCwd(); townRoot != "" {
				style.PrintWarning("pane working directory deleted, using town root")
				return t.RespawnPaneWithWorkDir(pane, townRoot, restartCmd)
			}
		}
	}

	// Use respawn-pane -k to atomically kill current process and start new one
	// Note: respawn-pane automatically resets remain-on-exit to off
	return t.RespawnPane(pane, restartCmd)
}

// runHandoffAuto saves state without cycling the session.
// Used by the PreCompact hook to preserve context before compaction.
// No tmux required — just collects state, sends handoff mail, and writes marker.
func runHandoffAuto() error {
	// Build subject
	subject := handoffSubject
	if subject == "" {
		reason := handoffReason
		if reason == "" {
			reason = "auto"
		}
		subject = fmt.Sprintf("🤝 HANDOFF: %s", reason)
	}

	// Auto-collect state if no explicit message
	message := handoffMessage
	if message == "" {
		message = collectHandoffState()
	}

	if handoffDryRun {
		fmt.Printf("[auto-handoff] Would send mail: subject=%q\n", subject)
		fmt.Printf("[auto-handoff] Would write handoff marker\n")
		return nil
	}

	// Send handoff mail to self
	beadID, err := sendHandoffMail(subject, message)
	if err != nil {
		// Non-fatal — log and continue
		fmt.Fprintf(os.Stderr, "auto-handoff: could not send mail: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "auto-handoff: saved state to %s\n", beadID)
	}

	// Write handoff marker so post-compact prime knows it's post-handoff
	if cwd, err := os.Getwd(); err == nil {
		runtimeDir := filepath.Join(cwd, constants.DirRuntime)
		_ = os.MkdirAll(runtimeDir, 0755)
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		sessionName := "auto-handoff"
		if tmux.IsInsideTmux() {
			if name, err := getCurrentTmuxSession(); err == nil {
				sessionName = name
			}
		}
		_ = os.WriteFile(markerPath, []byte(sessionName), 0644)
	}

	// Log handoff event
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		agent := detectSender()
		if agent == "" || agent == "overseer" {
			agent = "unknown"
		}
		_ = events.LogFeed(events.TypeHandoff, agent, events.HandoffPayload(subject, false))
	}

	return nil
}

// runHandoffCycle performs a full session cycle — save state AND respawn.
// This is the PreCompact-triggered session succession mechanism (gt-op78).
//
// Unlike --auto (state only) or normal handoff (polecat→gt-done redirect),
// --cycle always does a full respawn regardless of role. This enables
// crew workers (and polecats) to get a fresh context window when the
// current one fills up.
//
// The flow:
//  1. Auto-collect state (inbox, ready beads, hooked work)
//  2. Send handoff mail to self (auto-hooked for successor)
//  3. Write handoff marker (prevents handoff loop)
//  4. Respawn the tmux pane with a fresh Claude instance
//
// The successor session starts via SessionStart hook (gt prime --hook),
// finds the hooked work, and continues from where we left off.
func runHandoffCycle() error {
	// Build subject
	subject := handoffSubject
	if subject == "" {
		reason := handoffReason
		if reason == "" {
			reason = "context-cycle"
		}
		subject = fmt.Sprintf("🤝 HANDOFF: %s", reason)
	}

	// Auto-collect state if no explicit message
	message := handoffMessage
	if message == "" {
		message = collectHandoffState()
	}

	// Must be in tmux to respawn
	if !tmux.IsInsideTmux() {
		// Fall back to auto mode (save state only) if not in tmux
		fmt.Fprintf(os.Stderr, "handoff --cycle: not in tmux, falling back to state-save only\n")
		handoffMessage = message
		handoffSubject = subject
		return runHandoffAuto()
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		fmt.Fprintf(os.Stderr, "handoff --cycle: TMUX_PANE not set, falling back to state-save only\n")
		handoffMessage = message
		handoffSubject = subject
		return runHandoffAuto()
	}

	currentSession, err := getCurrentTmuxSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "handoff --cycle: could not get session: %v, falling back to state-save only\n", err)
		handoffMessage = message
		handoffSubject = subject
		return runHandoffAuto()
	}

	t := tmux.NewTmux()

	if handoffDryRun {
		fmt.Printf("[cycle] Would send handoff mail: subject=%q\n", subject)
		fmt.Printf("[cycle] Would write handoff marker\n")
		fmt.Printf("[cycle] Would execute: tmux clear-history -t %s\n", pane)
		fmt.Printf("[cycle] Would execute: tmux respawn-pane -k -t %s <restart-cmd>\n", pane)
		return nil
	}

	// Send handoff mail to self (auto-hooked for successor)
	beadID, err := sendHandoffMail(subject, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handoff --cycle: could not send mail: %v\n", err)
		// Continue — respawn is more important than mail
	} else {
		fmt.Fprintf(os.Stderr, "handoff --cycle: saved state to %s\n", beadID)
	}

	// Write handoff marker so post-cycle prime knows it's post-handoff
	if cwd, err := os.Getwd(); err == nil {
		runtimeDir := filepath.Join(cwd, constants.DirRuntime)
		_ = os.MkdirAll(runtimeDir, 0755)
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		_ = os.WriteFile(markerPath, []byte(currentSession), 0644)
	}

	// Log cycle event
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		agent := sessionToGTRole(currentSession)
		if agent == "" {
			agent = currentSession
		}
		_ = LogHandoff(townRoot, agent, subject)
		_ = events.LogFeed(events.TypeHandoff, agent, events.HandoffPayload(subject, true))
	}

	// Build restart command for fresh session
	restartCmd, err := buildRestartCommand(currentSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handoff --cycle: could not build restart command: %v\n", err)
		return err
	}

	fmt.Fprintf(os.Stderr, "handoff --cycle: cycling session %s\n", currentSession)

	// Set remain-on-exit so the pane survives process death during handoff
	if err := t.SetRemainOnExit(pane, true); err != nil {
		style.PrintWarning("could not set remain-on-exit: %v", err)
	}

	// Clear scrollback history before respawn
	if err := t.ClearHistory(pane); err != nil {
		style.PrintWarning("could not clear history: %v", err)
	}

	// Check if pane's working directory exists (may have been deleted)
	paneWorkDir, _ := t.GetPaneWorkDir(currentSession)
	if paneWorkDir != "" {
		if _, err := os.Stat(paneWorkDir); err != nil {
			if townRoot := detectTownRootFromCwd(); townRoot != "" {
				return t.RespawnPaneWithWorkDir(pane, townRoot, restartCmd)
			}
		}
	}

	// Respawn pane — this atomically kills current process and starts fresh
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
		return session.CrewSessionName(session.PrefixFor(rig), crewName), nil

	case "witness", "wit":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return "", fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return session.WitnessSessionName(session.PrefixFor(rig)), nil

	case "refinery", "ref":
		rig := os.Getenv("GT_RIG")
		if rig == "" {
			return "", fmt.Errorf("cannot determine rig - set GT_RIG or run from rig context")
		}
		return session.RefinerySessionName(session.PrefixFor(rig)), nil

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
		return session.CrewSessionName(session.PrefixFor(rig), name), nil
	}

	// Handle <rig>/polecats/<name> format (explicit polecat path)
	if len(parts) == 3 && parts[1] == "polecats" {
		rig := parts[0]
		name := strings.ToLower(parts[2]) // normalize polecat name
		return session.PolecatSessionName(session.PrefixFor(rig), name), nil
	}

	// Handle <rig>/<role-or-polecat> format
	if len(parts) == 2 {
		rig := parts[0]
		second := parts[1]
		secondLower := strings.ToLower(second)

		// Check for known roles first
		switch secondLower {
		case "witness":
			return session.WitnessSessionName(session.PrefixFor(rig)), nil
		case "refinery":
			return session.RefinerySessionName(session.PrefixFor(rig)), nil
		case "crew":
			// Just "<rig>/crew" without a name - need more info
			return "", fmt.Errorf("crew path requires name: %s/crew/<name>", rig)
		case "polecats":
			// Just "<rig>/polecats" without a name - need more info
			return "", fmt.Errorf("polecats path requires name: %s/polecats/<name>", rig)
		default:
			// Not a known role - check if it's a crew member before assuming polecat.
			// Crew members exist at <townRoot>/<rig>/crew/<name>.
			// This fixes: gt sling gt-375 gastown/max failing because max is crew, not polecat.
			townRoot := detectTownRootFromCwd()
			if townRoot != "" {
				crewPath := filepath.Join(townRoot, rig, "crew", second)
				if info, err := os.Stat(crewPath); err == nil && info.IsDir() {
					return session.CrewSessionName(session.PrefixFor(rig), second), nil
				}
			}
			// Not a crew member - treat as polecat name (e.g., gastown/nux)
			return session.PolecatSessionName(session.PrefixFor(rig), secondLower), nil
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

	// Derive rigPath from session identity for --settings flag resolution
	rigPath := ""
	if identity.Rig != "" {
		rigPath = filepath.Join(townRoot, identity.Rig)
	}

	// Build startup beacon for predecessor discovery via /resume
	// Use FormatStartupBeacon instead of bare "gt prime" which confuses agents
	// The SessionStart hook handles context injection (gt prime --hook)
	beacon := session.FormatStartupBeacon(session.BeaconConfig{
		Recipient: identity.BeaconAddress(),
		Sender:    "self",
		Topic:     "handoff",
	})

	// For respawn-pane, we:
	// 1. cd to the right directory (role's canonical home)
	// 2. export GT_ROLE and BD_ACTOR so role detection works correctly
	// 3. export Claude-related env vars (not inherited by fresh shell)
	// 4. run claude with the startup beacon (triggers immediate context loading)
	// Use exec to ensure clean process replacement.
	//
	// Check if current session is using a non-default agent (GT_AGENT env var).
	// If so, preserve it across handoff by using the override variant.
	// Fall back to tmux session environment if process env doesn't have it,
	// since exec env vars may not propagate through all agent runtimes.
	currentAgent := os.Getenv("GT_AGENT")
	if currentAgent == "" {
		t := tmux.NewTmux()
		if val, err := t.GetEnvironment(sessionName, "GT_AGENT"); err == nil && val != "" {
			currentAgent = val
		}
	}
	var runtimeCmd string
	if currentAgent != "" {
		var err error
		runtimeCmd, err = config.GetRuntimeCommandWithPromptAndAgentOverride(rigPath, beacon, currentAgent)
		if err != nil {
			return "", fmt.Errorf("resolving agent config: %w", err)
		}
	} else {
		runtimeCmd = config.GetRuntimeCommandWithPrompt(rigPath, beacon)
	}

	// Build environment exports - role vars first, then Claude vars
	var exports []string
	var agentEnv map[string]string // agent config Env (rc.toml [agents.X.env])
	if gtRole != "" {
		simpleRole := config.ExtractSimpleRole(gtRole)
		// When GT_AGENT is set, resolve config with the override so we pick up
		// the active agent's env (e.g., NODE_OPTIONS from [agents.X.env]).
		// Otherwise, fall back to role-based resolution.
		var runtimeConfig *config.RuntimeConfig
		if currentAgent != "" {
			rc, _, err := config.ResolveAgentConfigWithOverride(townRoot, rigPath, currentAgent)
			if err == nil {
				runtimeConfig = rc
			} else {
				runtimeConfig = config.ResolveRoleAgentConfig(simpleRole, townRoot, rigPath)
			}
		} else {
			runtimeConfig = config.ResolveRoleAgentConfig(simpleRole, townRoot, rigPath)
		}
		agentEnv = runtimeConfig.Env
		exports = append(exports, "GT_ROLE="+gtRole)
		exports = append(exports, "BD_ACTOR="+gtRole)
		exports = append(exports, "GIT_AUTHOR_NAME="+gtRole)
		if runtimeConfig.Session != nil && runtimeConfig.Session.SessionIDEnv != "" {
			exports = append(exports, "GT_SESSION_ID_ENV="+runtimeConfig.Session.SessionIDEnv)
		}
	}

	// Propagate GT_ROOT so subsequent handoffs can use it as fallback
	// when cwd-based detection fails (broken state recovery)
	exports = append(exports, "GT_ROOT="+townRoot)

	// Preserve GT_AGENT across handoff so agent override persists
	if currentAgent != "" {
		exports = append(exports, "GT_AGENT="+currentAgent)
	}

	// Preserve GT_PROCESS_NAMES across handoff for accurate liveness detection.
	// Without this, custom agents that shadow built-in presets (e.g., custom
	// "codex" running "opencode") would revert to GT_AGENT-based lookup after
	// handoff, causing false liveness failures.
	if processNames := os.Getenv("GT_PROCESS_NAMES"); processNames != "" {
		// Preserve existing process names from environment
		exports = append(exports, "GT_PROCESS_NAMES="+processNames)
	} else if currentAgent != "" {
		// First boot or missing GT_PROCESS_NAMES — compute from agent config
		resolved := config.ResolveProcessNames(currentAgent, "")
		exports = append(exports, "GT_PROCESS_NAMES="+strings.Join(resolved, ","))
	}

	// Add Claude-related env vars from current environment
	for _, name := range claudeEnvVars {
		if val := os.Getenv(name); val != "" {
			// Shell-escape the value in case it contains special chars
			exports = append(exports, fmt.Sprintf("%s=%q", name, val))
		}
	}

	// Clear NODE_OPTIONS to prevent debugger flags (e.g., --inspect from VSCode)
	// from being inherited through tmux into Claude's Node.js runtime.
	// When the agent's runtime config explicitly sets NODE_OPTIONS (e.g., for
	// memory tuning via --max-old-space-size in rc.toml [agents.X.env]), export
	// that value so it survives handoff. Otherwise clear it.
	// Note: agentEnv is intentionally nil when gtRole is empty (non-role handoffs),
	// which causes the nil map lookup to return ("", false) — clearing NODE_OPTIONS.
	if val, hasNodeOpts := agentEnv["NODE_OPTIONS"]; hasNodeOpts {
		exports = append(exports, fmt.Sprintf("NODE_OPTIONS=%q", val))
	} else {
		exports = append(exports, "NODE_OPTIONS=")
	}

	if len(exports) > 0 {
		return fmt.Sprintf("cd %s && export %s && exec %s", workDir, strings.Join(exports, " "), runtimeCmd), nil
	}
	return fmt.Sprintf("cd %s && exec %s", workDir, runtimeCmd), nil
}

// updateSessionEnvForHandoff updates the tmux session environment with the
// agent name and process names for liveness detection. IsAgentAlive reads
// GT_PROCESS_NAMES from the tmux session env (via tmux show-environment), not
// from shell exports in the pane. Without this, post-handoff liveness checks
// would use stale values from the previous agent.
func updateSessionEnvForHandoff(t *tmux.Tmux, sessionName, agentOverride string) {
	// Resolve current agent using the same priority as buildRestartCommandWithAgent
	var currentAgent string
	if agentOverride != "" {
		currentAgent = agentOverride
	} else {
		currentAgent = os.Getenv("GT_AGENT")
		if currentAgent == "" {
			if val, err := t.GetEnvironment(sessionName, "GT_AGENT"); err == nil && val != "" {
				currentAgent = val
			}
		}
	}

	if currentAgent == "" {
		return
	}

	// Update GT_AGENT in session env
	_ = t.SetEnvironment(sessionName, "GT_AGENT", currentAgent)

	// Resolve and update GT_PROCESS_NAMES in session env
	// When switching agents, recompute from config. When preserving, use env value.
	var processNames string
	if agentOverride != "" {
		// Agent is changing — resolve config to get the command for process name resolution
		townRoot := detectTownRootFromCwd()
		if townRoot != "" {
			identity, err := session.ParseSessionName(sessionName)
			rigPath := ""
			if err == nil && identity.Rig != "" {
				rigPath = filepath.Join(townRoot, identity.Rig)
			}
			rc, _, err := config.ResolveAgentConfigWithOverride(townRoot, rigPath, currentAgent)
			if err == nil {
				resolved := config.ResolveProcessNames(currentAgent, rc.Command)
				processNames = strings.Join(resolved, ",")
			}
		}
	}
	if processNames == "" {
		// Preserve existing value or compute from current agent
		if pn := os.Getenv("GT_PROCESS_NAMES"); pn != "" {
			processNames = pn
		} else {
			resolved := config.ResolveProcessNames(currentAgent, "")
			processNames = strings.Join(resolved, ",")
		}
	}

	_ = t.SetEnvironment(sessionName, "GT_PROCESS_NAMES", processNames)
}

// sessionWorkDir returns the correct working directory for a session.
// This is the canonical home for each role type.
func sessionWorkDir(sessionName, townRoot string) (string, error) {
	// Get session names for comparison
	mayorSession := getMayorSessionName()
	deaconSession := getDeaconSessionName()

	switch {
	case sessionName == mayorSession:
		// Mayor runs from ~/gt/mayor/, not town root.
		// Tools use workspace.FindFromCwd() which walks UP to find town root.
		return townRoot + "/mayor", nil

	case sessionName == deaconSession:
		return townRoot + "/deacon", nil

	case strings.Contains(sessionName, "-crew-"):
		// gt-<rig>-crew-<name> -> <townRoot>/<rig>/crew/<name>
		rig, name, _, ok := parseCrewSessionName(sessionName)
		if !ok {
			return "", fmt.Errorf("cannot parse crew session name: %s", sessionName)
		}
		return fmt.Sprintf("%s/%s/crew/%s", townRoot, rig, name), nil

	default:
		// Parse session name to determine role and resolve paths
		identity, err := session.ParseSessionName(sessionName)
		if err != nil {
			return "", fmt.Errorf("unknown session type: %s (%w)", sessionName, err)
		}
		switch identity.Role {
		case session.RoleWitness:
			return fmt.Sprintf("%s/%s/witness", townRoot, identity.Rig), nil
		case session.RoleRefinery:
			return fmt.Sprintf("%s/%s/refinery/rig", townRoot, identity.Rig), nil
		case session.RolePolecat:
			return fmt.Sprintf("%s/%s/polecats/%s", townRoot, identity.Rig, identity.Name), nil
		default:
			return "", fmt.Errorf("unknown session type: %s (role %s, try specifying role explicitly)", sessionName, identity.Role)
		}
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
// Falls back to GT_TOWN_ROOT or GT_ROOT env vars if cwd detection fails (broken state recovery).
func detectTownRootFromCwd() string {
	// Use workspace.FindFromCwd which handles both primary (mayor/town.json)
	// and secondary (mayor/ directory) markers
	townRoot, err := workspace.FindFromCwd()
	if err == nil && townRoot != "" {
		return townRoot
	}

	// Fallback: try environment variables for town root
	// GT_TOWN_ROOT is set by shell integration, GT_ROOT is set by session manager
	// This enables handoff to work even when cwd detection fails due to
	// detached HEAD, wrong branch, deleted worktree, etc.
	for _, envName := range []string{"GT_TOWN_ROOT", "GT_ROOT"} {
		if envRoot := os.Getenv(envName); envRoot != "" {
			// Verify it's actually a workspace
			if _, statErr := os.Stat(filepath.Join(envRoot, workspace.PrimaryMarker)); statErr == nil {
				return envRoot
			}
			// Try secondary marker too
			if info, statErr := os.Stat(filepath.Join(envRoot, workspace.SecondaryMarker)); statErr == nil && info.IsDir() {
				return envRoot
			}
		}
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

	// Set remain-on-exit so the pane survives process death during handoff.
	// Without this, killing processes causes tmux to destroy the pane before
	// we can respawn it. This is essential for tmux session reuse.
	if err := t.SetRemainOnExit(targetPane, true); err != nil {
		style.PrintWarning("could not set remain-on-exit: %v", err)
	}

	// Kill all processes in the pane before respawning to prevent orphan leaks
	// RespawnPane's -k flag only sends SIGHUP which Claude/Node may ignore
	if err := t.KillPaneProcesses(targetPane); err != nil {
		// Non-fatal but log the warning
		style.PrintWarning("could not kill pane processes: %v", err)
	}

	// Clear scrollback history before respawn (resets copy-mode from [0/N] to [0/0])
	if err := t.ClearHistory(targetPane); err != nil {
		// Non-fatal - continue with respawn even if clear fails
		style.PrintWarning("could not clear history: %v", err)
	}

	// Respawn the remote session's pane, handling deleted working directories
	respawnErr := func() error {
		paneWorkDir, _ := t.GetPaneWorkDir(targetSession)
		if paneWorkDir != "" {
			if _, statErr := os.Stat(paneWorkDir); statErr != nil {
				if townRoot := detectTownRootFromCwd(); townRoot != "" {
					style.PrintWarning("pane working directory deleted, using town root")
					return t.RespawnPaneWithWorkDir(targetPane, townRoot, restartCmd)
				}
			}
		}
		return t.RespawnPane(targetPane, restartCmd)
	}()
	if respawnErr != nil {
		return fmt.Errorf("respawning pane: %w", respawnErr)
	}

	// If --watch, switch to that session
	if handoffWatch {
		fmt.Printf("Switching to %s...\n", targetSession)
		// Use tmux switch-client to move our view to the target session
		if err := exec.Command("tmux", "-u", "switch-client", "-t", targetSession).Run(); err != nil {
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

	// Normalize identity to match mailbox query format
	agentID = mail.AddressToIdentity(agentID)

	// Detect town root for beads location
	townRoot := detectTownRootFromCwd()
	if townRoot == "" {
		return "", fmt.Errorf("cannot detect town root")
	}

	// Build labels for mail metadata (matches mail router format)
	labels := fmt.Sprintf("from:%s", agentID)

	// Create mail bead directly using bd create with --silent to get the ID
	// Mail goes to town-level beads (hq- prefix)
	// Flags go first, then -- to end flag parsing, then the positional subject.
	// This prevents subjects like "--help" from being parsed as flags.
	args := []string{
		"create",
		"--assignee", agentID,
		"-d", message,
		"--priority", "2",
		"--labels", labels + ",gt:message",
		"--actor", agentID,
		"--ephemeral", // Handoff mail is ephemeral
		"--silent",    // Output only the bead ID
		"--", subject,
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

// warnHandoffGitStatus checks the current workspace for uncommitted or unpushed
// work and prints a warning if found. Non-blocking — handoff continues regardless.
// Skips .beads/ changes since those are managed by Dolt and not a concern.
func warnHandoffGitStatus() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	g := git.NewGit(cwd)
	if !g.IsRepo() {
		return
	}
	status, err := g.CheckUncommittedWork()
	if err != nil || status.CleanExcludingBeads() {
		return
	}
	style.PrintWarning("workspace has uncommitted work: %s", status.String())
	if len(status.ModifiedFiles) > 0 {
		style.PrintWarning("  modified: %s", strings.Join(status.ModifiedFiles, ", "))
	}
	if len(status.UntrackedFiles) > 0 {
		style.PrintWarning("  untracked: %s", strings.Join(status.UntrackedFiles, ", "))
	}
	if status.UnpushedCommits > 0 {
		style.PrintWarning("  %d unpushed commit(s) — run 'git push' before handoff", status.UnpushedCommits)
	}
	fmt.Println("  (use --no-git-check to suppress this warning)")
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
