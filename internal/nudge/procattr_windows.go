//go:build windows

package nudge

import "os"

// terminateProcess kills the process on Windows (no graceful SIGTERM).
func terminateProcess(proc *os.Process) error {
	return proc.Kill()
}
