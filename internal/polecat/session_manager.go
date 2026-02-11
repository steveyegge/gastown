// Package polecat provides polecat workspace and session management.
package polecat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/ratelimit"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/terminal"
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
	ErrRateLimited     = errors.New("rate limited")
)

// SessionManager handles polecat session lifecycle.
type SessionManager struct {
	tmux    *tmux.Tmux
	rig     *rig.Rig
	backend terminal.Backend
}

// NewSessionManager creates a new polecat session manager for a rig.
// The backend defaults to local tmux; use SetBackend to override for coop/SSH.
func NewSessionManager(t *tmux.Tmux, r *rig.Rig) *SessionManager {
	var backend terminal.Backend
	if t != nil {
		backend = terminal.NewTmuxBackend(t)
	}
	return &SessionManager{
		tmux:    t,
		rig:     r,
		backend: backend,
	}
}

// SetBackend overrides the terminal backend used for session liveness checks.
// This enables coop or SSH backends for K8s-hosted agents.
func (m *SessionManager) SetBackend(b terminal.Backend) {
	m.backend = b
}

// hasSession checks if a terminal session exists, routing through the
// configured backend (coop, SSH, or local tmux).
func (m *SessionManager) hasSession(sessionID string) (bool, error) {
	if m.backend != nil {
		return m.backend.HasSession(sessionID)
	}
	// Fallback to direct tmux if no backend configured
	if m.tmux != nil {
		return m.tmux.HasSession(sessionID)
	}
	return false, fmt.Errorf("no terminal backend available")
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

	// AuthToken is an optional ANTHROPIC_AUTH_TOKEN for API authentication.
	// If set, this takes precedence over OAuth credentials.
	AuthToken string

	// BaseURL is an optional ANTHROPIC_BASE_URL for custom API endpoints.
	// Used with AuthToken for alternative API providers (e.g., LiteLLM).
	BaseURL string
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
func (m *SessionManager) SessionName(polecat string) string {
	return fmt.Sprintf("gt-%s-%s", m.rig.Name, polecat)
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

	// Check for rate limit backoff before starting
	tracker := ratelimit.NewTracker(m.rig.Path)
	if err := tracker.Load(); err == nil {
		if tracker.ShouldDefer() {
			waitTime := tracker.TimeUntilReady()
			return fmt.Errorf("%w: backoff active, retry in %v", ErrRateLimited, waitTime.Round(time.Second))
		}
	}

	sessionID := m.SessionName(polecat)

	// Check if session already exists
	// Note: Orphan sessions are cleaned up by ReconcilePool during AllocateName,
	// so by this point, any existing session should be legitimately in use.
	running, err := m.hasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
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

	runtimeConfig := config.LoadRuntimeConfig(m.rig.Path)

	// Ensure runtime settings exist in polecat's home directory (polecats/<name>/).
	// This keeps settings out of the git worktree while allowing runtime to find them
	// when walking up the tree from workDir (polecats/<name>/<rigname>/).
	// Each polecat gets isolated settings rather than sharing a single settings file.
	// Try to materialize Claude hooks from config beads first; fall back to templates.
	polecatHomeDir := m.polecatDir(polecat)
	if err := polecatMaterializeOrEnsureSettings(polecatHomeDir, "polecat", m.rig.Path, m.rig.Name, polecat, runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Get fallback info to determine beacon content based on agent capabilities.
	// Non-hook agents need "Run gt prime" in beacon; work instructions come as delayed nudge.
	fallbackInfo := runtime.GetStartupFallbackInfo(runtimeConfig)

	// Build startup command with beacon for predecessor discovery.
	// Configure beacon based on agent's hook/prompt capabilities.
	address := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
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
		command = config.BuildPolecatStartupCommand(m.rig.Name, polecat, m.rig.Path, beacon)
	}
	// Prepend account-related env vars if needed
	prependEnvVars := make(map[string]string)
	if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && opts.RuntimeConfigDir != "" {
		prependEnvVars[runtimeConfig.Session.ConfigDirEnv] = opts.RuntimeConfigDir
	}
	if opts.AuthToken != "" {
		prependEnvVars["ANTHROPIC_AUTH_TOKEN"] = opts.AuthToken
	}
	if opts.BaseURL != "" {
		prependEnvVars["ANTHROPIC_BASE_URL"] = opts.BaseURL
	}
	if len(prependEnvVars) > 0 {
		command = config.PrependEnv(command, prependEnvVars)
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	townRoot := filepath.Dir(m.rig.Path)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             "polecat",
		Rig:              m.rig.Name,
		AgentName:        polecat,
		TownRoot:         townRoot,
		RuntimeConfigDir: opts.RuntimeConfigDir,
		BeadsNoDaemon:    true,
		AuthToken:        opts.AuthToken,
		BaseURL:          opts.BaseURL,
		BDDaemonHost:     os.Getenv("BD_DAEMON_HOST"),
	})
	for k, v := range envVars {
		debugSession("SetEnvironment "+k, m.tmux.SetEnvironment(sessionID, k, v))
	}

	// Hook the issue to the polecat if provided via --issue flag
	if opts.Issue != "" {
		agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
		if err := m.hookIssue(opts.Issue, agentID, workDir); err != nil {
			fmt.Printf("Warning: could not hook issue %s: %v\n", opts.Issue, err)
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

	// Accept bypass permissions warning dialog if it appears
	debugSession("AcceptBypassPermissionsWarning", m.tmux.AcceptBypassPermissionsWarning(sessionID))

	// Wait for runtime to be fully ready at the prompt (not just started)
	runtime.SleepForReadyDelay(runtimeConfig)

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
			// Wait for agent to run gt prime before sending work instructions
			time.Sleep(time.Duration(fallbackInfo.StartupNudgeDelayMs) * time.Millisecond)
		}

		if fallbackInfo.SendStartupNudge {
			// Send work instructions via nudge
			debugSession("SendStartupNudge", m.tmux.NudgeSession(sessionID, runtime.StartupNudgeContent()))
		}
	}

	// Legacy fallback for other startup paths (non-fatal)
	_ = runtime.RunStartupFallback(m.tmux, sessionID, "polecat", runtimeConfig)

	// Verify session survived startup - if the command crashed, the session may have died.
	// Without this check, Start() would return success even if the pane died during initialization.
	//
	// With remain-on-exit enabled, if the command crashes the pane stays around, allowing
	// us to capture diagnostic output. We check for this "zombie pane" state specifically.
	running, err = m.hasSession(sessionID)
	if err != nil {
		return fmt.Errorf("verifying session: %w", err)
	}

	if running {
		// Session exists - check if pane is dead (command exited but pane preserved)
		dead, err := m.tmux.IsPaneDead(sessionID)
		if err == nil && dead {
			// Capture diagnostic output before cleanup
			diagnosticOutput := m.tmux.CaptureDeadPaneOutput(sessionID, 50)
			// Kill the zombie session
			_ = m.tmux.KillSession(sessionID)

			// Check for rate limit in diagnostic output
			if ratelimit.DetectRateLimit(diagnosticOutput) {
				tracker := ratelimit.NewTracker(m.rig.Path)
				_ = tracker.Load()
				tracker.RecordRateLimit(fmt.Sprintf("polecat:%s", polecat), opts.Account)
				_ = tracker.Save()
				return fmt.Errorf("%w: session %s died due to rate limiting. Diagnostic output:\n%s", ErrRateLimited, sessionID, diagnosticOutput)
			}

			// Return error with diagnostics
			if diagnosticOutput != "" {
				return fmt.Errorf("session %s died during startup. Diagnostic output:\n%s", sessionID, diagnosticOutput)
			}
			return fmt.Errorf("session %s died during startup (pane dead, no diagnostic output - check agent binary and credentials)", sessionID)
		}
		// Session is alive and pane is not dead - success
		return nil
	}

	// Session doesn't exist at all (remain-on-exit may not have taken effect)
	// This is a different failure mode than pane death - the session was destroyed entirely
	return fmt.Errorf("session %s died during startup (session destroyed, remain-on-exit may have failed - check tmux version)", sessionID)
}

// Stop terminates a polecat session.
func (m *SessionManager) Stop(polecat string, force bool) error {
	sessionID := m.SessionName(polecat)

	running, err := m.hasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	// Try graceful shutdown first
	if !force {
		_ = m.tmux.SendKeysRaw(sessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	// This prevents orphan bash processes from Claude's Bash tool surviving session termination.
	if err := m.tmux.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if a polecat session is active.
func (m *SessionManager) IsRunning(polecat string) (bool, error) {
	sessionID := m.SessionName(polecat)
	return m.hasSession(sessionID)
}

// Status returns detailed status for a polecat session.
func (m *SessionManager) Status(polecat string) (*SessionInfo, error) {
	sessionID := m.SessionName(polecat)

	running, err := m.hasSession(sessionID)
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

// List returns information about all polecat sessions for this rig.
func (m *SessionManager) List() ([]SessionInfo, error) {
	sessions, err := m.tmux.ListSessions()
	if err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("gt-%s-", m.rig.Name)
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

// Attach attaches to a polecat session.
func (m *SessionManager) Attach(polecat string) error {
	sessionID := m.SessionName(polecat)

	running, err := m.hasSession(sessionID)
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

	running, err := m.hasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	if m.backend != nil {
		return m.backend.CapturePane(sessionID, lines)
	}
	return m.tmux.CapturePane(sessionID, lines)
}

// CaptureSession returns the recent output from a session by raw session ID.
func (m *SessionManager) CaptureSession(sessionID string, lines int) (string, error) {
	running, err := m.hasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	if m.backend != nil {
		return m.backend.CapturePane(sessionID, lines)
	}
	return m.tmux.CapturePane(sessionID, lines)
}

// Inject sends a message to a polecat session.
func (m *SessionManager) Inject(polecat, message string) error {
	sessionID := m.SessionName(polecat)

	running, err := m.hasSession(sessionID)
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
	infos, err := m.List()
	if err != nil {
		return err
	}

	var lastErr error
	for _, info := range infos {
		if err := m.Stop(info.Polecat, force); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// validateIssue checks that an issue exists and is not tombstoned.
// This must be called before starting a session to avoid CPU spin loops
// from agents retrying work on invalid issues.
func (m *SessionManager) validateIssue(issueID, workDir string) error {
	cmd := bdcmd.Command( "show", issueID, "--json") //nolint:gosec
	cmd.Dir = workDir
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

// hookIssue pins an issue to a polecat's hook using bd update.
func (m *SessionManager) hookIssue(issueID, agentID, workDir string) error {
	cmd := bdcmd.Command( "update", issueID, "--status=hooked", "--assignee="+agentID) //nolint:gosec
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd update failed: %w", err)
	}
	fmt.Printf("✓ Hooked issue %s to %s\n", issueID, agentID)
	return nil
}

// polecatMaterializeOrEnsureSettings tries to materialize Claude hooks from config beads.
// If config beads are available, it merges them and writes settings.json.
// If beads are unavailable or no config beads exist, falls back to the
// existing embedded template flow via runtime.EnsureSettingsForRole.
func polecatMaterializeOrEnsureSettings(workDir, role, rigPath, rigName, agent string, rc *config.RuntimeConfig) error {
	townRoot := filepath.Dir(rigPath)
	payloads, err := polecatQueryClaudeHooksPayloads(townRoot, rigName, role, agent)
	if err != nil {
		fmt.Printf("Warning: Config beads unavailable (%v), using embedded templates\n", err)
		return runtime.EnsureSettingsForRole(workDir, role, rc)
	}

	if len(payloads) > 0 {
		if err := claude.MaterializeSettings(workDir, role, payloads); err != nil {
			return err
		}
		// Also ensure MCP config (not stored in config beads yet)
		return claude.EnsureMCPConfig(workDir)
	}

	// No config beads found - fall back to embedded templates
	return runtime.EnsureSettingsForRole(workDir, role, rc)
}

// polecatQueryClaudeHooksPayloads queries config beads for claude-hooks metadata payloads.
// Returns payloads in specificity order (least specific first) for merge.
func polecatQueryClaudeHooksPayloads(townRoot, rigName, role, agent string) ([]string, error) {
	townCfgPath := filepath.Join(townRoot, "mayor", "town.json")
	townCfg, err := config.LoadTownConfig(townCfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading town config: %w", err)
	}

	b := beads.New(townRoot)
	_, fields, err := b.ListConfigBeadsForScope(
		beads.ConfigCategoryClaudeHooks,
		townCfg.Name,
		rigName,
		role,
		agent,
	)
	if err != nil {
		return nil, fmt.Errorf("querying config beads: %w", err)
	}

	var payloads []string
	for _, f := range fields {
		if f.Metadata != "" {
			payloads = append(payloads, f.Metadata)
		}
	}

	return payloads, nil
}
