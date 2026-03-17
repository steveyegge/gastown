//go:build !windows

package nudge

import (
	"os"
	"syscall"
)

// detachedProcAttr returns SysProcAttr that detaches the child from
// the parent's process group so it survives the caller's exit.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// isProcessAlive checks if a process is running via signal 0.
func isProcessAlive(proc *os.Process) bool {
	return proc.Signal(syscall.Signal(0)) == nil
}

// terminateProcess sends SIGTERM for graceful shutdown.
func terminateProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
