package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// SessionEnvReader abstracts tmux session environment access for testing.
type SessionEnvReader interface {
	ListSessions() ([]string, error)
	GetAllEnvironment(session string) (map[string]string, error)
}

// tmuxEnvReader wraps real tmux operations.
type tmuxEnvReader struct {
	t *tmux.Tmux
}

func (r *tmuxEnvReader) ListSessions() ([]string, error) {
	return r.t.ListSessions()
}

func (r *tmuxEnvReader) GetAllEnvironment(session string) (map[string]string, error) {
	return r.t.GetAllEnvironment(session)
}

// EnvVarsCheck verifies that tmux session environment variables match expected values.
type EnvVarsCheck struct {
	BaseCheck
	reader SessionEnvReader // nil means use real tmux
}

// NewEnvVarsCheck creates a new env vars check.
func NewEnvVarsCheck() *EnvVarsCheck {
	return &EnvVarsCheck{
		BaseCheck: BaseCheck{
			CheckName:        "env-vars",
			CheckDescription: "Verify tmux session environment variables match expected values",
			CheckCategory:    CategoryConfig,
		},
	}
}

// NewEnvVarsCheckWithReader creates a check with a custom reader (for testing).
func NewEnvVarsCheckWithReader(reader SessionEnvReader) *EnvVarsCheck {
	c := NewEnvVarsCheck()
	c.reader = reader
	return c
}

// Run checks environment variables for all Gas Town sessions.
func (c *EnvVarsCheck) Run(ctx *CheckContext) *CheckResult {
	reader := c.reader
	if reader == nil {
		reader = &tmuxEnvReader{t: tmux.NewTmux()}
	}

	sessions, err := reader.ListSessions()
	if err != nil {
		// No tmux server - treat as success (valid when Gas Town is down)
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No tmux sessions running",
		}
	}

	// Filter to Gas Town sessions only (gt-* and hq-*)
	var gtSessions []string
	for _, sess := range sessions {
		if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
			gtSessions = append(gtSessions, sess)
		}
	}

	if len(gtSessions) == 0 {
		// No Gas Town sessions - treat as success (valid when Gas Town is down)
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No Gas Town sessions running",
		}
	}

	var mismatches []string
	checkedCount := 0

	for _, sess := range gtSessions {
		identity, err := session.ParseSessionName(sess)
		if err != nil {
			// Skip unparseable sessions
			continue
		}

		// Get expected env vars based on role, emulating real call sites
		// Town-level roles use TownRoot for beads, rig-level roles use rig path
		var beadsDir string
		if identity.Rig != "" {
			rigPath := filepath.Join(ctx.TownRoot, identity.Rig)
			beadsDir = beads.ResolveBeadsDir(rigPath)
		} else {
			beadsDir = beads.ResolveBeadsDir(ctx.TownRoot)
		}
		expected := config.AgentEnv(config.AgentEnvConfig{
			Role:      string(identity.Role),
			Rig:       identity.Rig,
			AgentName: identity.Name,
			TownRoot:  ctx.TownRoot,
			BeadsDir:  beadsDir,
		})

		// Get actual tmux env vars
		actual, err := reader.GetAllEnvironment(sess)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: could not read env vars: %v", sess, err))
			continue
		}

		checkedCount++

		// Compare each expected var
		for key, expectedVal := range expected {
			actualVal, exists := actual[key]
			if !exists {
				mismatches = append(mismatches, fmt.Sprintf("%s: missing %s (expected %q)", sess, key, expectedVal))
			} else if actualVal != expectedVal {
				mismatches = append(mismatches, fmt.Sprintf("%s: %s=%q (expected %q)", sess, key, actualVal, expectedVal))
			}
		}
	}

	if len(mismatches) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d session(s) have correct environment variables", checkedCount),
		}
	}

	// Add explanation about needing restart
	details := append(mismatches,
		"",
		"Note: Mismatched session env vars won't affect running Claude until sessions restart.",
	)

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d env var mismatch(es) across %d session(s)", len(mismatches), checkedCount),
		Details: details,
		FixHint: "Run 'gt shutdown && gt up' to restart sessions with correct env vars",
	}
}
