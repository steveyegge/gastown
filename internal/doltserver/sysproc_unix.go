//go:build !windows

package doltserver

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the command in its own process group so that signals
// sent to the parent process group (e.g. SIGHUP when the caller calls
// syscall.Exec to become tmux) don't reach the spawned process.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
