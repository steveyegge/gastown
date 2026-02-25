// Package polecat provides polecat workspace and session management.
package polecat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// debugSession logs non-fatal errors during session startup when GT_DEBUG_SESSION=1.
func debugSession(context string, err error) {
	if os.Getenv("GT_DEBUG_SESSION") != "" && err != nil {
		fmt.Fprintf(os.Stderr, "[session-debug] %s: %v\n", context, err)
	}
}


// Session errors
var (
	ErrSessionRunning  = errors.New("session already running")
	ErrSessionNotFound = errors.New("session not found")
	ErrIssueInvalid    = errors.New("issue not found or tombstoned")
)

// SessionManager handles polecat session lifecycle.
type SessionManager struct {
	tmux *tmux.Tmux
	rig  *rig.Rig
}

// NewSessionManager creates a new polecat session manager for a rig.
func NewSessionManager(t *tmux.Tmux, r *rig.Rig) *SessionManager {
	return &SessionManager{
		tmux: t,
		rig:  r,
	}
}

// SessionStartOptions configures polecat session startup.
type SessionStartOptions struct {
	// WorkDir overrides the default working directory (polecat clone dir).
	WorkDir string

	// Issue is an optional issue ID to work on.
	Issue string

	// Command overrides the default "claude" command.
	Command string

	// Account specifies the account handle to use (overrides default).
	Account string

	// RuntimeConfigDir is resolved config directory for the runtime account.
	// If set, this is injected as an environment variable.
	RuntimeConfigDir string

	// Agent is the agent override for this polecat session (e.g., "codex", "gemini").
	// If set, GT_AGENT is written to the tmux session environment table so that
	// IsAgentAlive and waitForPolecatReady read the correct process names.
	Agent string
}

// SessionInfo contains information about a running polecat session.
type SessionInfo struct {
	// Polecat is the polecat name.
	Polecat string `json:"polecat"`

	// SessionID is the tmux session identifier.
	SessionID string `json:"session_id"`

	// Running indicates if the session is currently active.
	Running bool `json:"running"`

	// RigName is the rig this session belongs to.
	RigName string `json:"rig_name"`

	// Attached indicates if someone is attached to the session.
	Attached bool `json:"attached,omitempty"`

	// Created is when the session was created.
	Created time.Time `json:"created,omitempty"`

	// Windows is the number of tmux windows.
	Windows int `json:"windows,omitempty"`

	// LastActivity is when the session last had activity.
	LastActivity time.Time `json:"last_activity,omitempty"`
}

// SessionName generates the tmux session name for a polecat.
// Validates that the polecat name doesn't contain the rig prefix to prevent
// double-prefix bugs (e.g., "gt-gastown_manager-gastown_manager-142").
func (m *SessionManager) SessionName(polecat string) string {
	sessionName := session.PolecatSessionName(session.PrefixFor(m.rig.Name), polecat)

	// Validate session name format to detect double-prefix bugs
	if err := validateSessionName(sessionName, m.rig.Name); err != nil {
		// Log warning but don't fail - allow the session to be created
		// so we can track and clean up malformed sessions later
		fmt.Fprintf(os.Stderr, "Warning: malformed session name: %v\n", err)
	}

	return sessionName
}

// validateSessionName checks for double-prefix session names.
// Returns an error if the session name has the rig prefix duplicated.
// Example bad name: "gt-gastown_manager-gastown_manager-142"
func validateSessionName(sessionName, rigName string) error {
	// Expected format: gt-<rig>-<name>
	// Check if the name part starts with the rig prefix (indicates double-prefix bug)
	prefix := session.PrefixFor(rigName) + "-"
	if !strings.HasPrefix(sessionName, prefix) {
		return nil // Not our rig, can't validate
	}

	namePart := strings.TrimPrefix(sessionName, prefix)

	// Check if name part starts with rig name followed by hyphen
	// This indicates overflow name included rig prefix: gt-<rig>-<rig>-N
	if strings.HasPrefix(namePart, rigName+"-") {
		return fmt.Errorf("double-prefix detected: %s (expected format: gt-%s-<name>)",
			sessionName, rigName)
	}

	return nil
}

// polecatDir returns the parent directory for a polecat.
// This is polecats/<name>/ - the polecat's home directory.
func (m *SessionManager) polecatDir(polecat string) string {
	return filepath.Join(m.rig.Path, "polecats", polecat)
}

// clonePath returns the path where the git worktree lives.
// New structure: polecats/<name>/<rigname>/ - gives LLMs recognizable repo context.
// Falls back to old structure: polecats/<name>/ for backward compatibility.
func (m *SessionManager) clonePath(polecat string) string {
	// New structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(m.rig.Path, "polecats", polecat, m.rig.Name)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		return newPath
	}

	// Old structure: polecats/<name>/ (backward compat)
	oldPath := filepath.Join(m.rig.Path, "polecats", polecat)
	if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
		// Check if this is actually a git worktree (has .git file or dir)
		gitPath := filepath.Join(oldPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return oldPath
		}
	}

	// Default to new structure for new polecats
	return newPath
}

// hasPolecat checks if the polecat exists in this rig.
func (m *SessionManager) hasPolecat(polecat string) bool {
	polecatPath := m.polecatDir(polecat)
	info, err := os.Stat(polecatPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Start creates and starts a new session for a polecat.
func (m *SessionManager) Start(polecat string, opts SessionStartOptions) error {
	if !m.hasPolecat(polecat) {
		return fmt.Errorf("%w: %s", ErrPolecatNotFound, polecat)
	}

	sessionID := m.SessionName(polecat)

	// Check if session already exists.
	// If an existing session's pane process has died, kill the stale session
	// and proceed rather than returning ErrSessionRunning (gt-jn40ft).
	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		if m.isSessionStale(sessionID) {
			if err := m.tmux.KillSessionWithProcesses(sessionID); err != nil {
				return fmt.Errorf("killing stale session %s: %w", sessionID, err)
			}
		} else {
			return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
		}
	}

	// Determine working directory
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = m.clonePath(polecat)
	}

	// Validate issue exists and isn't tombstoned BEFORE creating session.
	// This prevents CPU spin loops from agents retrying work on invalid issues.
	if opts.Issue != "" {
		if err := m.validateIssue(opts.Issue, workDir); err != nil {
			return err
		}
	}

	// Resolve runtime config for the agent that will actually run in this session.
	// When an explicit --agent override is provided (e.g., "codex"), use it to resolve
	// the correct agent config. Without this, ResolveRoleAgentConfig returns the default
	// role agent (usually Claude), causing WaitForRuntimeReady to poll for the wrong
	// prompt prefix and all fallback/nudge logic to use incorrect agent capabilities.
	// This was the root cause of gt-1j3m: Codex polecats sat idle because the startup
	// sequence used Claude's ReadyPromptPrefix ("❯ ") to detect readiness in a Codex
	// session, timing out instead of using Codex's delay-based readiness.
	townRoot := filepath.Dir(m.rig.Path)
	var runtimeConfig *config.RuntimeConfig
	if opts.Agent != "" {
		rc, _, err := config.ResolveAgentConfigWithOverride(townRoot, m.rig.Path, opts.Agent)
		if err != nil {
			return fmt.Errorf("resolving agent config for %s: %w", opts.Agent, err)
		}
		runtimeConfig = rc
	} else {
		runtimeConfig = config.ResolveRoleAgentConfig("polecat", townRoot, m.rig.Path)
	}

	// Ensure runtime settings exist in the shared polecats parent directory.
	// Settings are passed to Claude Code via --settings flag.
	polecatSettingsDir := config.RoleSettingsDir("polecat", m.rig.Path)
	if err := runtime.EnsureSettingsForRole(polecatSettingsDir, workDir, "polecat", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Get fallback info to determine beacon content based on agent capabilities.
	// Non-hook agents need "Run gt prime" in beacon; work instructions come as delayed nudge.
	fallbackInfo := runtime.GetStartupFallbackInfo(runtimeConfig)

	// Build startup command with beacon for predecessor discovery.
	// Configure beacon based on agent's hook/prompt capabilities.
	address := session.BeaconRecipient("polecat", polecat, m.rig.Name)
	beaconConfig := session.BeaconConfig{
		Recipient:               address,
		Sender:                  "witness",
		Topic:                   "assigned",
		MolID:                   opts.Issue,
		IncludePrimeInstruction: fallbackInfo.IncludePrimeInBeacon,
		ExcludeWorkInstructions: fallbackInfo.SendStartupNudge,
	}
	beacon := session.FormatStartupBeacon(beaconConfig)

	command := opts.Command
	if command == "" {
		var err error
		command, err = config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
			Role:        "polecat",
			Rig:         m.rig.Name,
			AgentName:   polecat,
			TownRoot:    townRoot,
			Prompt:      beacon,
			Issue:       opts.Issue,
			Topic:       "assigned",
			SessionName: sessionID,
		}, m.rig.Path, beacon, "")
		if err != nil {
			return fmt.Errorf("building startup command: %w", err)
		}
	}
	// Prepend runtime config dir env if needed
	if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && opts.RuntimeConfigDir != "" {
		command = config.PrependEnv(command, map[string]string{runtimeConfig.Session.ConfigDirEnv: opts.RuntimeConfigDir})
	}

	// Disable Dolt auto-commit for polecats to prevent manifest contention
	// under concurrent load (gt-5cc2p). Changes merge at gt done time.
	command = config.PrependEnv(command, map[string]string{"BD_DOLT_AUTO_COMMIT": "off"})

	// FIX (ga-6s284): Prepend GT_RIG, GT_POLECAT, GT_ROLE to startup command
	// so they're inherited by Kimi and other agents. Setting via tmux.SetEnvironment
	// after session creation doesn't work for all agent types.
	//
	// GT_BRANCH and GT_POLECAT_PATH are critical for gt done's nuked-worktree fallback:
	// when the polecat's cwd is deleted before gt done finishes, these env vars allow
	// branch detection and path resolution without a working directory.
	polecatGitBranch := ""
	if g := git.NewGit(workDir); g != nil {
		if b, err := g.CurrentBranch(); err == nil {
			polecatGitBranch = b
		}
	}
	envVarsToInject := map[string]string{
		"GT_RIG":          m.rig.Name,
		"GT_POLECAT":      polecat,
		"GT_ROLE":         fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat),
		"GT_POLECAT_PATH": workDir,
		"GT_TOWN_ROOT":    townRoot,
	}
	if polecatGitBranch != "" {
		envVarsToInject["GT_BRANCH"] = polecatGitBranch
	}
	command = config.PrependEnv(command, envVarsToInject)

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	// Note: townRoot already defined above for ResolveRoleAgentConfig
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             "polecat",
		Rig:              m.rig.Name,
		AgentName:        polecat,
		TownRoot:         townRoot,
		RuntimeConfigDir: opts.RuntimeConfigDir,
		Agent:            opts.Agent,
	})
	for k, v := range envVars {
		debugSession("SetEnvironment "+k, m.tmux.SetEnvironment(sessionID, k, v))
	}

	// Fallback: set GT_AGENT from resolved config when no explicit --agent override.
	// AgentEnv only emits GT_AGENT when opts.Agent is non-empty (explicit override).
	// Without this fallback, the default path (no --agent flag) leaves GT_AGENT
	// unset in the tmux session table, causing the validation below to fail and
	// kill the session. BuildStartupCommand sets GT_AGENT in process env via
	// exec env, but tmux show-environment reads the session table, not process env.
	// This mirrors the daemon's compensating logic (daemon.go ~line 1593-1595).
	if _, hasGTAgent := envVars["GT_AGENT"]; !hasGTAgent && runtimeConfig.ResolvedAgent != "" {
		debugSession("SetEnvironment GT_AGENT (resolved)", m.tmux.SetEnvironment(sessionID, "GT_AGENT", runtimeConfig.ResolvedAgent))
	}

	// Set GT_BRANCH and GT_POLECAT_PATH in tmux session environment.
	// This ensures respawned processes also inherit these for gt done fallback.
	if polecatGitBranch != "" {
		debugSession("SetEnvironment GT_BRANCH", m.tmux.SetEnvironment(sessionID, "GT_BRANCH", polecatGitBranch))
	}
	debugSession("SetEnvironment GT_POLECAT_PATH", m.tmux.SetEnvironment(sessionID, "GT_POLECAT_PATH", workDir))
	debugSession("SetEnvironment GT_TOWN_ROOT", m.tmux.SetEnvironment(sessionID, "GT_TOWN_ROOT", townRoot))

	// Disable Dolt auto-commit in tmux session environment (gt-5cc2p).
	// This ensures respawned processes also inherit the setting.
	debugSession("SetEnvironment BD_DOLT_AUTO_COMMIT", m.tmux.SetEnvironment(sessionID, "BD_DOLT_AUTO_COMMIT", "off"))

	// Set GT_PROCESS_NAMES for accurate liveness detection. Custom agents may
	// shadow built-in preset names (e.g., custom "codex" running "opencode"),
	// so we resolve process names from both agent name and actual command.
	processNames := config.ResolveProcessNames(runtimeConfig.ResolvedAgent, runtimeConfig.Command)
	debugSession("SetEnvironment GT_PROCESS_NAMES", m.tmux.SetEnvironment(sessionID, "GT_PROCESS_NAMES", strings.Join(processNames, ",")))
	// Hook the issue to the polecat if provided via --issue flag
	if opts.Issue != "" {
		agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
		if err := m.hookIssue(opts.Issue, agentID, workDir); err != nil {
			style.PrintWarning("could not hook issue %s: %v", opts.Issue, err)
		}
	}

	// Apply theme (non-fatal)
	theme := tmux.AssignTheme(m.rig.Name)
	debugSession("ConfigureGasTownSession", m.tmux.ConfigureGasTownSession(sessionID, theme, m.rig.Name, polecat, "polecat"))

	// Set pane-died hook for crash detection (non-fatal)
	agentID := fmt.Sprintf("%s/%s", m.rig.Name, polecat)
	debugSession("SetPaneDiedHook", m.tmux.SetPaneDiedHook(sessionID, agentID))

	// Wait for Claude to start (non-fatal)
	debugSession("WaitForCommand", m.tmux.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout))

	// Accept startup dialogs (workspace trust + bypass permissions) if they appear
	debugSession("AcceptStartupDialogs", m.tmux.AcceptStartupDialogs(sessionID))

	// Wait for runtime to be fully ready at the prompt (not just started).
	// Uses prompt-based polling for agents with ReadyPromptPrefix (e.g., Claude "❯ "),
	// falling back to ReadyDelayMs sleep for agents without prompt detection.
	debugSession("WaitForRuntimeReady", m.tmux.WaitForRuntimeReady(sessionID, runtimeConfig, constants.ClaudeStartTimeout))

	// Handle fallback nudges for non-hook agents.
	// See StartupFallbackInfo in runtime package for the fallback matrix.
	if fallbackInfo.SendBeaconNudge && fallbackInfo.SendStartupNudge && fallbackInfo.StartupNudgeDelayMs == 0 {
		// Hooks + no prompt: Single combined nudge (hook already ran gt prime synchronously)
		combined := beacon + "\n\n" + runtime.StartupNudgeContent()
		debugSession("SendCombinedNudge", m.tmux.NudgeSession(sessionID, combined))
	} else {
		if fallbackInfo.SendBeaconNudge {
			// Agent doesn't support CLI prompt - send beacon via nudge
			debugSession("SendBeaconNudge", m.tmux.NudgeSession(sessionID, beacon))
		}

		if fallbackInfo.StartupNudgeDelayMs > 0 {
			// Wait for agent to finish processing beacon + gt prime before sending work instructions.
			// Uses prompt-based detection where available; falls back to max(ReadyDelayMs, StartupNudgeDelayMs).
			primeWaitRC := runtime.RuntimeConfigWithMinDelay(runtimeConfig, fallbackInfo.StartupNudgeDelayMs)
			debugSession("WaitForPrimeReady", m.tmux.WaitForRuntimeReady(sessionID, primeWaitRC, constants.ClaudeStartTimeout))
		}

		if fallbackInfo.SendStartupNudge {
			// Send work instructions via nudge
			debugSession("SendStartupNudge", m.tmux.NudgeSession(sessionID, runtime.StartupNudgeContent()))
		}
	}

	// Verify startup nudge was delivered: poll for idle prompt and retry if lost.
	// This fixes the Mode B race where the nudge arrives before Claude Code is ready,
	// causing the polecat to sit idle at an empty prompt. See GH#1379.
	if fallbackInfo.SendStartupNudge {
		m.verifyStartupNudgeDelivery(sessionID, runtimeConfig)
	}

	// Legacy fallback for other startup paths (non-fatal)
	_ = runtime.RunStartupFallback(m.tmux, sessionID, "polecat", runtimeConfig)

	// Verify session survived startup - if the command crashed, the session may have died.
	// Without this check, Start() would return success even if the pane died during initialization.
	running, err = m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("verifying session: %w", err)
	}
	if !running {
		return fmt.Errorf("session %s died during startup (agent command may have failed)", sessionID)
	}

	// Validate GT_AGENT is set. Without GT_AGENT, IsAgentAlive falls back to
	// ["node", "claude"] process detection and witness patrol will auto-nuke
	// polecats running non-Claude agents (e.g., opencode). Fail fast.
	gtAgent, _ := m.tmux.GetEnvironment(sessionID, "GT_AGENT")
	if gtAgent == "" {
		_ = m.tmux.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("GT_AGENT not set in session %s (command=%q); "+
			"witness patrol will misidentify this polecat as a zombie and auto-nuke it. "+
			"Ensure RuntimeConfig.ResolvedAgent is set during agent config resolution",
			sessionID, runtimeConfig.Command)
	}

	// Track PID for defense-in-depth orphan cleanup (non-fatal)
	_ = session.TrackSessionPID(townRoot, sessionID, m.tmux)

	return nil
}

// isSessionStale checks if a tmux session's pane process has died.
// A stale session exists in tmux but its main process (the agent) is no longer running.
// This happens when the agent crashes during startup but tmux keeps the dead pane.
// Delegates to isSessionProcessDead to avoid duplicating process-check logic (gt-qgzj1h).
func (m *SessionManager) isSessionStale(sessionID string) bool {
	return isSessionProcessDead(m.tmux, sessionID)
}

// Stop terminates a polecat session.
func (m *SessionManager) Stop(polecat string, force bool) error {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	// Try graceful shutdown first
	if !force {
		_ = m.tmux.SendKeysRaw(sessionID, "C-c")
		session.WaitForSessionExit(m.tmux, sessionID, constants.GracefulShutdownTimeout)
	}

	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	// This prevents orphan bash processes from Claude's Bash tool surviving session termination.
	if err := m.tmux.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if a polecat session is active and healthy.
// Checks both tmux session existence AND agent process liveness to avoid
// reporting zombie sessions (tmux alive but Claude dead) as "running".
func (m *SessionManager) IsRunning(polecat string) (bool, error) {
	sessionID := m.SessionName(polecat)
	status := m.tmux.CheckSessionHealth(sessionID, 0)
	return status == tmux.SessionHealthy, nil
}

// Status returns detailed status for a polecat session.
func (m *SessionManager) Status(polecat string) (*SessionInfo, error) {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}

	info := &SessionInfo{
		Polecat:   polecat,
		SessionID: sessionID,
		Running:   running,
		RigName:   m.rig.Name,
	}

	if !running {
		return info, nil
	}

	tmuxInfo, err := m.tmux.GetSessionInfo(sessionID)
	if err != nil {
		return info, nil
	}

	info.Attached = tmuxInfo.Attached
	info.Windows = tmuxInfo.Windows

	if tmuxInfo.Created != "" {
		formats := []string{
			"2006-01-02 15:04:05",
			"Mon Jan 2 15:04:05 2006",
			"Mon Jan _2 15:04:05 2006",
			time.ANSIC,
			time.UnixDate,
		}
		for _, format := range formats {
			if t, err := time.Parse(format, tmuxInfo.Created); err == nil {
				info.Created = t
				break
			}
		}
	}

	if tmuxInfo.Activity != "" {
		var activityUnix int64
		if _, err := fmt.Sscanf(tmuxInfo.Activity, "%d", &activityUnix); err == nil && activityUnix > 0 {
			info.LastActivity = time.Unix(activityUnix, 0)
		}
	}

	return info, nil
}

// List returns information about all sessions for this rig.
// This includes polecats, witness, refinery, and crew sessions.
// Use ListPolecats() to get only polecat sessions.
func (m *SessionManager) List() ([]SessionInfo, error) {
	sessions, err := m.tmux.ListSessions()
	if err != nil {
		return nil, err
	}

	prefix := session.PrefixFor(m.rig.Name) + "-"
	var infos []SessionInfo

	for _, sessionID := range sessions {
		if !strings.HasPrefix(sessionID, prefix) {
			continue
		}

		polecat := strings.TrimPrefix(sessionID, prefix)
		infos = append(infos, SessionInfo{
			Polecat:   polecat,
			SessionID: sessionID,
			Running:   true,
			RigName:   m.rig.Name,
		})
	}

	return infos, nil
}

// ListPolecats returns information only about polecat sessions for this rig.
// Filters out witness, refinery, and crew sessions.
func (m *SessionManager) ListPolecats() ([]SessionInfo, error) {
	infos, err := m.List()
	if err != nil {
		return nil, err
	}

	var filtered []SessionInfo
	for _, info := range infos {
		// Skip non-polecat sessions
		if info.Polecat == "witness" || info.Polecat == "refinery" || strings.HasPrefix(info.Polecat, "crew-") {
			continue
		}
		filtered = append(filtered, info)
	}

	return filtered, nil
}

// Attach attaches to a polecat session.
func (m *SessionManager) Attach(polecat string) error {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	return m.tmux.AttachSession(sessionID)
}

// Capture returns the recent output from a polecat session.
func (m *SessionManager) Capture(polecat string, lines int) (string, error) {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	return m.tmux.CapturePane(sessionID, lines)
}

// CaptureSession returns the recent output from a session by raw session ID.
func (m *SessionManager) CaptureSession(sessionID string, lines int) (string, error) {
	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	return m.tmux.CapturePane(sessionID, lines)
}

// Inject sends a message to a polecat session.
func (m *SessionManager) Inject(polecat, message string) error {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	debounceMs := 200 + (len(message)/1024)*100
	if debounceMs > 1500 {
		debounceMs = 1500
	}

	return m.tmux.SendKeysDebounced(sessionID, message, debounceMs)
}

// StopAll terminates all polecat sessions for this rig.
func (m *SessionManager) StopAll(force bool) error {
	infos, err := m.ListPolecats()
	if err != nil {
		return err
	}

	var errs []error
	for _, info := range infos {
		if err := m.Stop(info.Polecat, force); err != nil {
			errs = append(errs, fmt.Errorf("stopping %s: %w", info.Polecat, err))
		}
	}

	return errors.Join(errs...)
}

// resolveBeadsDir determines the correct working directory for bd commands
// on a given issue. This enables cross-rig beads resolution via routes.jsonl.
// This is the core fix for GitHub issue #1056.
func (m *SessionManager) resolveBeadsDir(issueID, fallbackDir string) string {
	townRoot := filepath.Dir(m.rig.Path)
	return beads.ResolveHookDir(townRoot, issueID, fallbackDir)
}

// validateIssue checks that an issue exists and is not tombstoned.
// This must be called before starting a session to avoid CPU spin loops
// from agents retrying work on invalid issues.
func (m *SessionManager) validateIssue(issueID, workDir string) error {
	bdWorkDir := m.resolveBeadsDir(issueID, workDir)

	ctx, cancel := context.WithTimeout(context.Background(), constants.BdCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "show", issueID, "--json") //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = bdWorkDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrIssueInvalid, issueID)
	}

	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return fmt.Errorf("parsing issue: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("%w: %s", ErrIssueInvalid, issueID)
	}
	if issues[0].Status == "tombstone" {
		return fmt.Errorf("%w: %s is tombstoned", ErrIssueInvalid, issueID)
	}
	return nil
}

// verifyStartupNudgeDelivery checks if the polecat started working after the
// startup nudge and retries the nudge if the agent is still idle at its prompt.
// This fixes the Mode B race condition (GH#1379) where the startup nudge arrives
// before Claude Code is ready, causing the polecat to sit idle.
//
// The approach models ensureAgentReady (sling_helpers.go): after the nudge, wait
// a verification delay, then check if the agent is at its idle prompt. If idle,
// re-send the nudge and check again, up to StartupNudgeMaxRetries times.
//
// Non-fatal: if verification fails or times out, the session is left running.
// The witness zombie patrol will eventually detect and handle truly idle polecats.
func (m *SessionManager) verifyStartupNudgeDelivery(sessionID string, rc *config.RuntimeConfig) {
	// Only verify for agents with prompt detection. Without ReadyPromptPrefix,
	// we can't distinguish "idle at prompt" from "busy processing".
	if rc == nil || rc.Tmux == nil || rc.Tmux.ReadyPromptPrefix == "" {
		return
	}

	nudgeContent := runtime.StartupNudgeContent()

	for attempt := 1; attempt <= constants.StartupNudgeMaxRetries; attempt++ {
		// Wait for the agent to process the nudge before checking.
		time.Sleep(constants.StartupNudgeVerifyDelay)

		// Check if session is still alive
		running, err := m.tmux.HasSession(sessionID)
		if err != nil || !running {
			return // Session died, nothing to verify
		}

		// If the agent is NOT at the prompt, it's working — nudge was received.
		if !m.tmux.IsAtPrompt(sessionID, rc) {
			return
		}

		// Agent is at the idle prompt — nudge was likely lost. Retry.
		fmt.Fprintf(os.Stderr, "[startup-nudge] attempt %d/%d: agent %s idle at prompt, retrying nudge\n",
			attempt, constants.StartupNudgeMaxRetries, sessionID)
		if err := m.tmux.NudgeSession(sessionID, nudgeContent); err != nil {
			fmt.Fprintf(os.Stderr, "[startup-nudge] retry nudge failed for %s: %v\n", sessionID, err)
			return
		}
	}

	// If we exhausted retries and the agent is still idle, log a warning.
	// The witness zombie patrol will handle this case.
	if m.tmux.IsAtPrompt(sessionID, rc) {
		fmt.Fprintf(os.Stderr, "[startup-nudge] WARNING: agent %s still idle after %d nudge retries\n",
			sessionID, constants.StartupNudgeMaxRetries)
	}
}

// hookIssue pins an issue to a polecat's hook using bd update.
func (m *SessionManager) hookIssue(issueID, agentID, workDir string) error {
	bdWorkDir := m.resolveBeadsDir(issueID, workDir)

	ctx, cancel := context.WithTimeout(context.Background(), constants.BdCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "update", issueID, "--status=hooked", "--assignee="+agentID) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = bdWorkDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd update failed: %w", err)
	}
	fmt.Printf("✓ Hooked issue %s to %s\n", issueID, agentID)
	return nil
}
