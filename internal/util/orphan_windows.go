//go:build windows

package util

// OrphanedProcess represents a Claude process that appears to be orphaned.
type OrphanedProcess struct {
	PID int
	Cmd string
	Age int // Age in seconds
}

// CleanupResult represents the result of attempting to clean up an orphaned process.
type CleanupResult struct {
	Process OrphanedProcess
	Signal  string // "SIGTERM", "SIGKILL", or "UNKILLABLE"
	Error   error
}

// FindOrphanedClaudeProcesses is not implemented on Windows.
func FindOrphanedClaudeProcesses() ([]OrphanedProcess, error) {
	return nil, nil
}

// CleanupOrphanedClaudeProcesses is not implemented on Windows.
func CleanupOrphanedClaudeProcesses() ([]CleanupResult, error) {
	return nil, nil
}
