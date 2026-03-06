//go:build !windows

// daemon_reload_unix.go — signalDaemonReload sends SIGUSR2 to the daemon process to trigger a reload
package cmd

import (
	"os"
	"syscall"
)

// signalDaemonReload sends SIGUSR2 to the daemon process to trigger a reload.
func signalDaemonReload(process *os.Process) error {
	return process.Signal(syscall.SIGUSR2)
}
