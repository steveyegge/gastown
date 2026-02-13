package session

import (
	"fmt"
	"time"
)

// SessionCreatedAt returns the time a session was created.
// In K8s mode, session creation time is not available (no tmux).
// Returns an error so callers fall through to non-stale behavior.
func SessionCreatedAt(sessionName string) (time.Time, error) {
	return time.Time{}, fmt.Errorf("session creation time not available (K8s mode)")
}

// StaleReasonForTimes compares message time to session creation and returns staleness info.
func StaleReasonForTimes(messageTime, sessionCreated time.Time) (bool, string) {
	if messageTime.IsZero() || sessionCreated.IsZero() {
		return false, ""
	}

	if messageTime.Before(sessionCreated) {
		reason := fmt.Sprintf("message=%s session_started=%s",
			messageTime.Format(time.RFC3339),
			sessionCreated.Format(time.RFC3339),
		)
		return true, reason
	}

	return false, ""
}
