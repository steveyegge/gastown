//go:build windows

// agent_logging_windows.go — ActivateAgentLogging is a no-op on Windows: the detached subprocess relies on
package session

// ActivateAgentLogging is a no-op on Windows: the detached subprocess relies on
// Unix-specific Setsid / SIGTERM semantics that are not available on Windows.
func ActivateAgentLogging(sessionID, workDir, runID string) error {
	return nil
}

// DeactivateAgentLogging is a no-op on Windows.
func DeactivateAgentLogging(sessionID string) {}
