package doctor

import (
	"fmt"
	"sort"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// SocketSplitBrainCheck detects tmux sessions that exist on both the town
// socket (e.g., "gt") and the "default" socket. This split-brain causes
// gt nudge and other session-discovery commands to fail because they only
// search the town socket.
type SocketSplitBrainCheck struct {
	FixableCheck
	staleSessions []string // Sessions on "default" that also exist on town socket
}

// NewSocketSplitBrainCheck creates a new socket split-brain check.
func NewSocketSplitBrainCheck() *SocketSplitBrainCheck {
	return &SocketSplitBrainCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "socket-split-brain",
				CheckDescription: "Detect tmux sessions on wrong socket (causes nudge failures)",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks for Gas Town sessions on the "default" socket that duplicate
// sessions on the town socket.
func (c *SocketSplitBrainCheck) Run(ctx *CheckContext) *CheckResult {
	townSocket := tmux.GetDefaultSocket()
	if townSocket == "" || townSocket == "default" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Town socket is default — no split-brain possible",
		}
	}

	// List sessions on both sockets
	townTmux := tmux.NewTmux()
	defaultTmux := tmux.NewTmuxWithSocket("default")

	townSessions, err := townTmux.ListSessions()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Could not list town socket sessions (server may not be running)",
		}
	}

	defaultSessions, err := defaultTmux.ListSessions()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No default socket server running — no split-brain",
		}
	}

	// Build set of town socket sessions
	townSet := make(map[string]bool, len(townSessions))
	for _, s := range townSessions {
		townSet[s] = true
	}

	// Find Gas Town sessions on default that are duplicates or orphans
	var duplicates []string
	var orphans []string

	for _, s := range defaultSessions {
		if !session.IsKnownSession(s) {
			continue // Not a Gas Town session
		}
		if townSet[s] {
			duplicates = append(duplicates, s)
		} else {
			orphans = append(orphans, s)
		}
	}

	c.staleSessions = append(duplicates, orphans...)
	sort.Strings(c.staleSessions)

	if len(c.staleSessions) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("No split-brain: all Gas Town sessions on %q socket", townSocket),
		}
	}

	var details []string
	for _, s := range duplicates {
		details = append(details, fmt.Sprintf("DUPLICATE: %s exists on both %q and \"default\" sockets", s, townSocket))
	}
	for _, s := range orphans {
		details = append(details, fmt.Sprintf("ORPHAN: %s only on \"default\" socket (should be on %q)", s, townSocket))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d Gas Town session(s) on wrong socket — nudge/discovery will fail", len(c.staleSessions)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to kill stale sessions on wrong socket",
	}
}

// Fix kills Gas Town sessions on the "default" socket that shouldn't be there.
func (c *SocketSplitBrainCheck) Fix(ctx *CheckContext) error {
	if len(c.staleSessions) == 0 {
		return nil
	}

	defaultTmux := tmux.NewTmuxWithSocket("default")
	var lastErr error

	for _, s := range c.staleSessions {
		if err := defaultTmux.KillSessionWithProcesses(s); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
