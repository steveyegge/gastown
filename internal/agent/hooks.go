// Package agent provides agent runtime abstractions and hooks.
// Different agent runtimes (Claude Code, OpenCode, etc.) may have different
// startup behaviors, UI dialogs, and process signatures. This package
// provides hooks to handle these differences.
package agent

import (
	"errors"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
)

// ReadinessChecker determines if a session is ready based on its output.
type ReadinessChecker interface {
	// IsReady returns true if the output indicates the session is ready.
	IsReady(output string) bool
}

// WaitForReady polls until the checker indicates the session is ready.
// This is a helper function that uses Capture() to check readiness.
func WaitForReady(sess session.Sessions, id session.SessionID, timeout time.Duration, checker ReadinessChecker) error {
	if checker == nil {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := sess.Capture(id, 50)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if checker.IsReady(output) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("timeout waiting for ready")
}

// PromptChecker checks for a specific prompt prefix in the output.
type PromptChecker struct {
	Prefix string
}

// IsReady returns true if any line starts with the prompt prefix.
func (p *PromptChecker) IsReady(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, p.Prefix) {
			return true
		}
	}
	return false
}

// StartupHook is called after an agent session is created.
// It handles agent-specific initialization like dismissing dialogs.
type StartupHook func(sess session.Sessions, id session.SessionID) error

// OnSessionCreated is called immediately after a session is created,
// before waiting for readiness. Use this for synchronous setup like theming.
type OnSessionCreated func(id session.SessionID) error

// Config defines the behavior for a specific agent runtime.
// This is a data-only struct; the Agents type implements the logic.
type Config struct {
	// Name identifies the runtime (e.g., "claude", "opencode").
	Name string

	// ProcessNames are the process names to check for zombie detection.
	// Used internally by Agents.Start() to detect dead sessions.
	ProcessNames []string

	// EnvVars are environment variables to prepend to the start command.
	// These are passed as VAR=value prefix to the command.
	EnvVars map[string]string

	// OnSessionCreated is called immediately after session creation.
	// Use for synchronous setup like theming. Runs before StartupHook.
	OnSessionCreated OnSessionCreated

	// StartupHook is called before checking readiness (e.g., dismiss dialogs).
	StartupHook StartupHook

	// Checker determines readiness by examining session output.
	// If nil, StartupDelay is used instead.
	Checker ReadinessChecker

	// StartupDelay is a fallback wait time when Checker is nil.
	StartupDelay time.Duration

	// Timeout is the maximum time to wait for readiness.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration
}

// WithOnSessionCreated returns a copy of Config with the callback set.
// This allows chaining: agent.FromPreset("claude").WithOnSessionCreated(...)
func (c *Config) WithOnSessionCreated(fn OnSessionCreated) *Config {
	copy := *c
	copy.OnSessionCreated = fn
	return &copy
}

// WithStartupHook returns a copy of Config with the startup hook set.
// This allows chaining: agent.FromPreset("claude").WithStartupHook(...)
func (c *Config) WithStartupHook(fn StartupHook) *Config {
	copy := *c
	copy.StartupHook = fn
	return &copy
}

// WithEnvVars returns a copy of Config with environment variables set.
// These are prepended to the start command as VAR=value prefix.
func (c *Config) WithEnvVars(envVars map[string]string) *Config {
	copy := *c
	copy.EnvVars = envVars
	return &copy
}

// FromPreset creates a Config from an agent preset name.
// Returns a Config with process names from the registry and agent-specific hooks.
func FromPreset(agentName string) *Config {
	// Get process names from the registry
	processNames := config.GetProcessNames(agentName)

	// Create base config with default timeout
	cfg := &Config{
		Name:         agentName,
		ProcessNames: processNames,
		Timeout:      30 * time.Second, // Default timeout
	}

	// Add agent-specific hooks and timeouts
	switch agentName {
	case "claude":
		cfg.StartupHook = ClaudeStartupHook
		cfg.Checker = &PromptChecker{Prefix: ">"}
		cfg.Timeout = 60 * time.Second // Claude can take longer to start
	case "opencode":
		cfg.StartupDelay = 500 * time.Millisecond
		cfg.Timeout = 10 * time.Second
	case "gemini":
		cfg.Checker = &PromptChecker{Prefix: ">"}
		cfg.Timeout = 30 * time.Second
	default:
		// For unknown agents, use a generic startup delay
		cfg.StartupDelay = 1 * time.Second
	}

	return cfg
}

// Claude returns the config for Claude Code.
func Claude() *Config {
	return FromPreset("claude")
}

// OpenCode returns the config for OpenCode.
func OpenCode() *Config {
	return FromPreset("opencode")
}

// ClaudeStartupHook handles Claude Code-specific startup behavior.
// It dismisses the "Bypass Permissions" warning dialog if present.
func ClaudeStartupHook(sess session.Sessions, id session.SessionID) error {
	// Wait for the dialog to potentially render
	time.Sleep(1 * time.Second)

	// Check if the bypass permissions warning is present
	content, err := sess.Capture(id, 30)
	if err != nil {
		return err
	}

	// Look for the characteristic warning text
	if !strings.Contains(content, "Bypass Permissions mode") {
		// Warning not present, nothing to do
		return nil
	}

	// Press Down to select "Yes, I accept" (option 2)
	if err := sess.SendControl(id, "Down"); err != nil {
		return err
	}

	// Small delay to let selection update
	time.Sleep(200 * time.Millisecond)

	// Press Enter to confirm
	return sess.SendControl(id, "Enter")
}

