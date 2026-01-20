package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// ZombieSessionCheck detects tmux sessions that are valid Gas Town sessions
// but have no Claude/node process running inside (zombies).
// These occur when Claude exits or crashes but the tmux session remains.
type ZombieSessionCheck struct {
	FixableCheck
	sessionLister  SessionLister
	zombieSessions []string // Cached during Run for use in Fix
}

// NewZombieSessionCheck creates a new zombie session check.
func NewZombieSessionCheck() *ZombieSessionCheck {
	return &ZombieSessionCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "zombie-sessions",
				CheckDescription: "Detect tmux sessions with dead Claude processes",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// NewZombieSessionCheckWithLister creates a check with a custom session lister (for testing).
func NewZombieSessionCheckWithLister(lister SessionLister) *ZombieSessionCheck {
	check := NewZombieSessionCheck()
	check.sessionLister = lister
	return check
}

// Run checks for zombie Gas Town sessions (tmux alive but Claude dead).
func (c *ZombieSessionCheck) Run(ctx *CheckContext) *CheckResult {
	t := tmux.NewTmux()
	lister := c.sessionLister
	if lister == nil {
		lister = &realSessionLister{t: t}
	}

	sessions, err := lister.List()
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  "Could not list tmux sessions",
			Details:  []string{err.Error()},
			Category: c.CheckCategory,
		}
	}

	if len(sessions) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No tmux sessions found",
			Category: c.CheckCategory,
		}
	}

	// Check each Gas Town session for zombie status
	var zombies []string
	var healthyCount int

	// Process names to check for (Claude CLI)
	processNames := []string{"claude", "node"}

	for _, sess := range sessions {
		if sess == "" {
			continue
		}

		// Only check Gas Town sessions (gt-* and hq-*)
		if !strings.HasPrefix(sess, "gt-") && !strings.HasPrefix(sess, "hq-") {
			continue
		}

		// Skip crew sessions - they are human-managed and may intentionally
		// have no Claude running (e.g., between work assignments)
		if isCrewSession(sess) {
			continue
		}

		// Check if Claude is running in this session
		sessionID := session.SessionID(sess)
		if t.IsRunning(sessionID, processNames...) {
			healthyCount++
		} else {
			zombies = append(zombies, sess)
		}
	}

	// Cache zombies for Fix
	c.zombieSessions = zombies

	if len(zombies) == 0 {
		msg := "No zombie sessions found"
		if healthyCount > 0 {
			msg = fmt.Sprintf("All %d Gas Town sessions have running Claude processes", healthyCount)
		}
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  msg,
			Category: c.CheckCategory,
		}
	}

	details := make([]string, len(zombies))
	for i, sess := range zombies {
		details[i] = fmt.Sprintf("Zombie: %s (tmux alive, Claude dead)", sess)
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("Found %d zombie session(s)", len(zombies)),
		Details:  details,
		FixHint:  "Run 'gt doctor --fix' to kill zombie sessions",
		Category: c.CheckCategory,
	}
}

// Fix kills all zombie sessions (tmux sessions with no Claude running).
// Crew sessions are never auto-killed as they are human-managed.
func (c *ZombieSessionCheck) Fix(ctx *CheckContext) error {
	if len(c.zombieSessions) == 0 {
		return nil
	}

	t := tmux.NewTmux()
	var lastErr error

	for _, sess := range c.zombieSessions {
		// SAFEGUARD: Never auto-kill crew sessions (double-check)
		if isCrewSession(sess) {
			continue
		}

		// Log pre-death event for audit trail
		_ = events.LogFeed(events.TypeSessionDeath, sess,
			events.SessionDeathPayload(sess, "unknown", "zombie cleanup", "gt doctor"))

		if err := t.KillSession(sess); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
