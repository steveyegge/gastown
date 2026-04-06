//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// setSysProcAttr sets platform-specific process attributes.
// On Windows, detach the child into a new process group and suppress
// console-window creation so background subprocesses don't flash a
// visible window (the daemon itself runs with CREATE_NO_WINDOW).
func setSysProcAttr(cmd *exec.Cmd) {
	const CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW,
	}
}

// isProcessAlive checks if a process is still running.
// On Windows, Signal(0) is not supported, so we open the process handle
// with minimal access to verify it exists.
func isProcessAlive(p *os.Process) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
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
