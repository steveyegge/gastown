//go:build !windows

package lock

import (
	"os"
	"syscall"
)

// processExists checks if a process with the given PID exists and is alive.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}

	// On Unix, sending signal 0 checks if process exists without affecting it.
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Try to send signal 0 - this will fail if process doesn't exist.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
