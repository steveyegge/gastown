//go:build windows

package session

// ActivateCheckpointWatchdog is a no-op on Windows.
func ActivateCheckpointWatchdog(sessionID, workDir, interval string) error {
	return nil
}

// DeactivateCheckpointWatchdog is a no-op on Windows.
func DeactivateCheckpointWatchdog(sessionID string) {}
