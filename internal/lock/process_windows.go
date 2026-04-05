//go:build windows

package lock

import (
	"math"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// setProcessGroup detaches the child on Windows, suppressing console window flash.
func setProcessGroup(cmd *exec.Cmd) {
	const CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW,
	}
}

// processExists checks if a process with the given PID exists and is alive.
func processExists(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_ACCESS_DENIED {
			return true
		}
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}
