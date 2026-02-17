package doctor

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// MalformedSessionNameCheck detects Gas Town tmux sessions whose names don't
// match the canonical format computed from their parsed identity. This catches
// sessions that survived a Gas Town upgrade where the session naming scheme
// changed (e.g., old "gt-whatsapp_automation-witness" vs new "wa-witness").
type MalformedSessionNameCheck struct {
	FixableCheck
	sessionListerForTest SessionLister  // Injectable for testing; nil uses real tmux
	malformed            []sessionRename // Cached during Run for use in Fix
}

type sessionRename struct {
	oldName string
	newName string
}

// NewMalformedSessionNameCheck creates a new malformed session name check.
func NewMalformedSessionNameCheck() *MalformedSessionNameCheck {
	return &MalformedSessionNameCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "session-name-format",
				CheckDescription: "Detect sessions with outdated Gas Town naming format",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run detects sessions whose names don't match the canonical expected name.
func (c *MalformedSessionNameCheck) Run(ctx *CheckContext) *CheckResult {
	lister := c.sessionListerForTest
	if lister == nil {
		lister = &realSessionLister{t: tmux.NewTmux()}
	}

	sessions, err := lister.ListSessions()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not list tmux sessions",
			Details: []string{err.Error()},
		}
	}

	var malformed []sessionRename

	for _, sess := range sessions {
		if sess == "" {
			continue
		}

		identity, err := session.ParseSessionName(sess)
		if err != nil {
			// Not a Gas Town session - skip
			continue
		}

		expected := identity.SessionName()
		if expected != sess {
			malformed = append(malformed, sessionRename{oldName: sess, newName: expected})
		}
	}

	// Cache for Fix
	c.malformed = malformed

	if len(malformed) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All Gas Town sessions use current naming format",
		}
	}

	details := make([]string, len(malformed))
	for i, r := range malformed {
		details[i] = fmt.Sprintf("Malformed: %s → should be %s", r.oldName, r.newName)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d session(s) with outdated naming format", len(malformed)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to rename sessions to current format",
	}
}

// Fix renames malformed sessions to their canonical names.
// Crew sessions are never renamed as they may be currently attached.
func (c *MalformedSessionNameCheck) Fix(ctx *CheckContext) error {
	if len(c.malformed) == 0 {
		return nil
	}

	t := tmux.NewTmux()
	var lastErr error

	for _, r := range c.malformed {
		// SAFEGUARD: Never auto-rename crew sessions.
		// They may be attached and renaming could confuse the human operator.
		if isCrewSession(r.oldName) {
			continue
		}

		// Check the target name isn't already in use
		exists, err := t.HasSession(r.newName)
		if err == nil && exists {
			// Target name already exists - skip to avoid collision
			continue
		}

		if err := t.RenameSession(r.oldName, r.newName); err != nil {
			lastErr = fmt.Errorf("rename %s → %s: %w", r.oldName, r.newName, err)
		}
	}

	return lastErr
}
