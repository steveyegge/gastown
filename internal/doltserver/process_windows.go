//go:build windows

package doltserver

import (
	"golang.org/x/sys/windows"
)

// isProcessRunning checks if a process with the given PID is still alive on Windows.
// Opens the process with minimal access rights (SYNCHRONIZE) and checks the exit code.
// If GetExitCodeProcess returns STILL_ACTIVE (259), the process is alive.
func isProcessRunning(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	// STILL_ACTIVE (259) means the process hasn't exited.
	return exitCode == 259
}
