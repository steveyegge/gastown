//go:build windows

package mayor

import (
	"math"

	"golang.org/x/sys/windows"
)

// processAlive reports whether a process with the given PID exists and is
// still running on Windows.
//
// Opening a handle with PROCESS_QUERY_LIMITED_INFORMATION is sufficient to
// prove the process exists. ERROR_ACCESS_DENIED still indicates the process is
// alive but inaccessible to the current user.
func acpProcessAlive(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err == windows.ERROR_ACCESS_DENIED
	}
	_ = windows.CloseHandle(handle)
	return true
}
