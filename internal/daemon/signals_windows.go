//go:build windows

// signals_windows.go provides daemonSignals and related functionality.
package daemon

import (
	"os"
	"syscall"
)

func daemonSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
	}
}

func isLifecycleSignal(sig os.Signal) bool {
	return false
}

func isReloadRestartSignal(sig os.Signal) bool {
	return false
}
