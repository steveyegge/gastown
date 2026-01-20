//go:build windows

package lock

import "golang.org/x/sys/windows"

// processExists checks if a process with the given PID exists and is alive.
func processExists(pid int) bool {
	if pid <= 0 {
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
