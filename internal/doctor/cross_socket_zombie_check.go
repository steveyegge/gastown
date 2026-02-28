package doctor

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// legacyNamedSockets lists tmux socket names that Gas Town historically used
// before migrating to the "default" socket.
var legacyNamedSockets = []string{"gt", "gas-town"}

// CrossSocketZombieCheck detects agent sessions on other tmux sockets.
// When the town uses "default", checks legacy named sockets (gt, gas-town).
// When the town uses a named socket, checks the default socket.
type CrossSocketZombieCheck struct {
	FixableCheck
	zombieSessions map[string][]string // socket -> sessions, cached for Fix
}

// NewCrossSocketZombieCheck creates a new cross-socket zombie check.
func NewCrossSocketZombieCheck() *CrossSocketZombieCheck {
	return &CrossSocketZombieCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "cross-socket-zombies",
				CheckDescription: "Detect agent sessions on wrong tmux socket",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// crossSocketTargets returns the tmux socket names to check for zombies.
func crossSocketTargets() []string {
	townSocket := tmux.GetDefaultSocket()
	if townSocket == "" {
		return nil
	}
	if townSocket == "default" {
		return legacyNamedSockets
	}
	return []string{"default"}
}

// Run checks for Gas Town agent sessions on other sockets.
func (c *CrossSocketZombieCheck) Run(ctx *CheckContext) *CheckResult {
	targets := crossSocketTargets()
	if len(targets) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No town socket configured",
		}
	}

	c.zombieSessions = make(map[string][]string)
	totalZombies := 0

	for _, socketName := range targets {
		t := tmux.NewTmuxWithSocket(socketName)
		sessions, err := t.ListSessions()
		if err != nil {
			continue // No server on this socket
		}

		for _, sess := range sessions {
			if sess == "" {
				continue
			}
			if session.IsKnownSession(sess) {
				c.zombieSessions[socketName] = append(c.zombieSessions[socketName], sess)
				totalZombies++
			}
		}
	}

	if totalZombies == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No agent sessions on other sockets",
		}
	}

	townSocket := tmux.GetDefaultSocket()
	details := make([]string, 0, totalZombies+1)
	details = append(details, fmt.Sprintf("Town socket: %s (agent sessions should be here)", townSocket))
	for socketName, sessions := range c.zombieSessions {
		for _, sess := range sessions {
			details = append(details, fmt.Sprintf("  Zombie on %s socket: %s", socketName, sess))
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d agent session(s) on other socket(s) (should be on %s)", totalZombies, townSocket),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to kill cross-socket zombie sessions",
	}
}

// Fix kills agent sessions on other sockets, preserving user sessions.
func (c *CrossSocketZombieCheck) Fix(ctx *CheckContext) error {
	if len(c.zombieSessions) == 0 {
		return nil
	}

	var lastErr error

	for socketName, sessions := range c.zombieSessions {
		t := tmux.NewTmuxWithSocket(socketName)
		for _, sess := range sessions {
			_ = events.LogFeed(events.TypeSessionDeath, sess,
				events.SessionDeathPayload(sess, "unknown", "cross-socket zombie cleanup", "gt doctor"))

			if err := t.KillSessionWithProcesses(sess); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}
