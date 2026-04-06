//go:build windows

package doltserver

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup detaches the child process on Windows using
// CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW so that it survives
// the parent's exit without flashing a visible console window.
func setProcessGroup(cmd *exec.Cmd) {
	const CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW,
	}
}

// processIsAlive checks whether a process with the given PID is still running.
func processIsAlive(pid int) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}

// gracefulTerminate on Windows has no SIGTERM equivalent — Kill() is the only option.
func gracefulTerminate(p *os.Process) error {
	return p.Kill()
}
