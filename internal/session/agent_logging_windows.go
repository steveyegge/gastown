//go:build windows

// agent_logging_windows.go provides no-op stubs for agent logging on Windows,
// where the detached subprocess mechanism is not supported.
package session

// ActivateAgentLogging is a no-op on Windows: the detached subprocess relies on
// Unix-specific Setsid / SIGTERM semantics that are not available on Windows.
func ActivateAgentLogging(sessionID, workDir, runID string) error {
	return nil
}

// DeactivateAgentLogging is a no-op on Windows.
func DeactivateAgentLogging(sessionID string) {}
