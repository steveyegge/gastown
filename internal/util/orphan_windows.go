//go:build windows

package util

// OrphanedProcess represents a claude process running without a controlling terminal.
// On Windows, orphan cleanup is not supported, so this is a stub definition.
type OrphanedProcess struct {
	PID int
	Cmd string
	Age int // Age in seconds
}

// CleanupResult describes what happened to an orphaned process.
// On Windows, cleanup is a no-op.
type CleanupResult struct {
	Process OrphanedProcess
	Signal  string // "SIGTERM", "SIGKILL", or "UNKILLABLE"
	Error   error
}

// FindOrphanedClaudeProcesses is a Windows stub.
func FindOrphanedClaudeProcesses() ([]OrphanedProcess, error) {
	return nil, nil
}

// CleanupOrphanedClaudeProcesses is a Windows stub.
func CleanupOrphanedClaudeProcesses() ([]CleanupResult, error) {
	return nil, nil
}
