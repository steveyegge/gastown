//go:build windows

package daemon

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

const windowsStillActive = 259

// setSysProcAttr sets platform-specific process attributes.
// On Windows, no special attributes needed for process group detachment.
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Windows - process will run independently
}

// isProcessAlive checks if a process is still running.
// On Windows, signal 0 is not a reliable liveness probe, so query the
// process exit status through the Win32 API.
func isProcessAlive(p *os.Process) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}

// sendTermSignal sends a termination signal.
// On Windows, there's no SIGTERM - we use Kill() directly.
func sendTermSignal(p *os.Process) error {
	return p.Kill()
}

// sendKillSignal sends a kill signal.
// On Windows, Kill() is the only option.
func sendKillSignal(p *os.Process) error {
	return p.Kill()
}
