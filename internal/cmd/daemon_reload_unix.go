//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// signalDaemonReload sends SIGUSR2 to the daemon process to trigger a reload.
func signalDaemonReload(process *os.Process) error {
	return process.Signal(syscall.SIGUSR2)
}
