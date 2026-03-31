//go:build windows

package nudge

import (
	"os"
	"syscall"
)

// detachedProcAttr returns SysProcAttr for Windows.
// CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW detaches the child from the
// parent's console group without flashing a visible console window.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000200 | 0x08000000, // CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW
	}
}

// isProcessAlive checks if a process is running on Windows.
// Uses syscall.OpenProcess directly (no x/sys/windows dependency) for
// maximum CI compatibility. If we can open the process handle, it's alive.
func isProcessAlive(proc *os.Process) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return false // can't open → not running (or access denied)
	}
	syscall.CloseHandle(h)
	return true
}

// terminateProcess kills the process on Windows (no graceful SIGTERM).
func terminateProcess(proc *os.Process) error {
	return proc.Kill()
}
