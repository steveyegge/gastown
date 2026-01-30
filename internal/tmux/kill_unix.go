//go:build !windows

package tmux

import "syscall"

// killProcessGroup sends SIGTERM and SIGKILL to a process group.
// pgid should be positive; the function will negate it for the syscall.
func killProcessGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
}

// killProcessGroupForce sends SIGKILL to a process group.
func killProcessGroupForce(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
