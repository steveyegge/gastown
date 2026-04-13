//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

func daemonSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	}
}

func isLifecycleSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR1
}

func isReloadRestartSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR2
}

// isNoopSignal returns true for signals that the daemon should ignore.
// SIGHUP is sent when the parent session exits (e.g., sling completes).
// Without handling it, Go's default terminates the process.
func isNoopSignal(sig os.Signal) bool {
	return sig == syscall.SIGHUP
}
