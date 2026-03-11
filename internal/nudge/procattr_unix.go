//go:build !windows

package nudge

import (
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM for graceful shutdown.
func terminateProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
