//go:build !windows

package doltserver

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup puts the command in its own process group so that signals
// sent to the parent process group (e.g. SIGHUP when the caller calls
// syscall.Exec to become tmux) don't reach the spawned process.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// processAlive checks whether a process is still running.
// On Unix, sends signal 0 which doesn't affect the process but returns
// an error if the process doesn't exist.
func processAlive(p *os.Process) bool {
	return p.Signal(syscall.Signal(0)) == nil
}

// processTerminate sends SIGTERM for graceful shutdown.
func processTerminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// processKill sends SIGKILL for forced termination.
func processKill(p *os.Process) error {
	return p.Signal(syscall.SIGKILL)
}
