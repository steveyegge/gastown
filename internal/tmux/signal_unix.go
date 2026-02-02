//go:build !windows

package tmux

import "syscall"

// killProcessGroup sends a signal to a process group.
// On Unix, this uses syscall.Kill with negative PGID to target the group.
func killProcessGroup(pgid int, sig int) error {
	var signal syscall.Signal
	switch sig {
	case sigTERM:
		signal = syscall.SIGTERM
	case sigKILL:
		signal = syscall.SIGKILL
	default:
		signal = syscall.SIGTERM
	}
	return syscall.Kill(-pgid, signal)
}

// Signal constants that work across platforms.
const (
	sigTERM = 15
	sigKILL = 9
)
