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
	"github.com/steveyegge/gastown/internal/ratelimit"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/terminal"
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
	rig     *rig.Rig
	backend terminal.Backend
}

// NewSessionManager creates a new polecat session manager for a rig.
// Uses CoopBackend by default; use SetBackend to override.
func NewSessionManager(r *rig.Rig) *SessionManager {
	return &SessionManager{
		rig:     r,
		backend: terminal.NewCoopBackend(terminal.CoopConfig{}),
	}
}

// SetBackend overrides the terminal backend used for session liveness checks.
// This enables coop backends for K8s-hosted agents.
func (m *SessionManager) SetBackend(b terminal.Backend) {
	m.backend = b
}

// hasSession checks if a terminal session exists via the configured backend.
func (m *SessionManager) hasSession(sessionID string) (bool, error) {
	return m.backend.HasSession(sessionID)
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

	// SessionID is the session identifier.
	SessionID string `json:"session_id"`

	// Running indicates if the session is currently active.
	Running bool `json:"running"`

	// RigName is the rig this session belongs to.
	RigName string `json:"rig_name"`

	// Attached indicates if someone is attached to the session.
	Attached bool `json:"attached,omitempty"`

	// Created is when the session was created.
	Created time.Time `json:"created,omitempty"`

	// Windows is the number of session windows.
	Windows int `json:"windows,omitempty"`

	// LastActivity is when the session last had activity.
	LastActivity time.Time `json:"last_activity,omitempty"`
}

// SessionName generates the session name for a polecat.
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

	// Send command to coop backend to start the agent session.
	if err := m.backend.SendInput(sessionID, command, true); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Hook the issue to the polecat if provided via --issue flag
	if opts.Issue != "" {
		agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
		if err := m.hookIssue(opts.Issue, agentID, workDir); err != nil {
			fmt.Printf("Warning: could not hook issue %s: %v\n", opts.Issue, err)
		}
	}

	// Wait for runtime to be fully ready at the prompt (not just started)
	runtime.SleepForReadyDelay(runtimeConfig)

	// Handle fallback nudges for non-hook agents.
	// See StartupFallbackInfo in runtime package for the fallback matrix.
	if fallbackInfo.SendBeaconNudge && fallbackInfo.SendStartupNudge && fallbackInfo.StartupNudgeDelayMs == 0 {
		// Hooks + no prompt: Single combined nudge (hook already ran gt prime synchronously)
		combined := beacon + "\n\n" + runtime.StartupNudgeContent()
		debugSession("SendCombinedNudge", m.backend.NudgeSession(sessionID, combined))
	} else {
		if fallbackInfo.SendBeaconNudge {
			// Agent doesn't support CLI prompt - send beacon via nudge
			debugSession("SendBeaconNudge", m.backend.NudgeSession(sessionID, beacon))
		}

		if fallbackInfo.StartupNudgeDelayMs > 0 {
			// Wait for agent to run gt prime before sending work instructions
			time.Sleep(time.Duration(fallbackInfo.StartupNudgeDelayMs) * time.Millisecond)
		}

		if fallbackInfo.SendStartupNudge {
			// Send work instructions via nudge
			debugSession("SendStartupNudge", m.backend.NudgeSession(sessionID, runtime.StartupNudgeContent()))
		}
	}

	// Verify session survived startup
	running, err = m.hasSession(sessionID)
	if err != nil {
		return fmt.Errorf("verifying session: %w", err)
	}

	if running {
		// Check if agent is still running
		agentRunning, agentErr := m.backend.IsAgentRunning(sessionID)
		if agentErr == nil && !agentRunning {
			// Agent exited - capture diagnostic output
			diagnosticOutput, _ := m.backend.CapturePane(sessionID, 50)
			_ = m.backend.KillSession(sessionID)

			// Check for rate limit in diagnostic output
			if ratelimit.DetectRateLimit(diagnosticOutput) {
				tracker := ratelimit.NewTracker(m.rig.Path)
				_ = tracker.Load()
				tracker.RecordRateLimit(fmt.Sprintf("polecat:%s", polecat), opts.Account)
				_ = tracker.Save()
				return fmt.Errorf("%w: session %s died due to rate limiting. Diagnostic output:\n%s", ErrRateLimited, sessionID, diagnosticOutput)
			}

			if diagnosticOutput != "" {
				return fmt.Errorf("session %s died during startup. Diagnostic output:\n%s", sessionID, diagnosticOutput)
			}
			return fmt.Errorf("session %s died during startup (no diagnostic output - check agent binary and credentials)", sessionID)
		}
		return nil
	}

	return fmt.Errorf("session %s died during startup (session destroyed)", sessionID)
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
		_ = m.backend.SendKeys(sessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	if err := m.backend.KillSession(sessionID); err != nil {
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

	// Session details are available through the backend's session info
	// For coop backends, detailed info may be limited; return basic running status.
	return info, nil
}

// List returns information about all polecat sessions for this rig.
// Discovers polecats from filesystem and checks liveness via backend.
func (m *SessionManager) List() ([]SessionInfo, error) {
	polecatsDir := filepath.Join(m.rig.Path, "polecats")
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading polecats dir: %w", err)
	}

	var infos []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecat := entry.Name()
		sessionID := m.SessionName(polecat)
		running, _ := m.hasSession(sessionID)

		infos = append(infos, SessionInfo{
			Polecat:   polecat,
			SessionID: sessionID,
			Running:   running,
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

	return m.backend.AttachSession(sessionID)
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

	return m.backend.CapturePane(sessionID, lines)
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

	return m.backend.CapturePane(sessionID, lines)
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

	return m.backend.NudgeSession(sessionID, message)
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
	cmd := bdcmd.Command( "show", issueID, "--json") //nolint:gosec // args are internal constants, not user input
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
	cmd := bdcmd.Command( "update", issueID, "--status=hooked", "--assignee="+agentID) //nolint:gosec // args are internal constants, not user input
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
