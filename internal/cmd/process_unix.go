//go:build !windows

package cmd

import "syscall"

// isProcessRunning checks if a process with the given PID exists.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}

	// EPERM means process exists but we don't have permission to signal it.
	return err == syscall.EPERM
}
