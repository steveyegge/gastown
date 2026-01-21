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
		syscall.SIGUSR1,
	}
}

func isLifecycleSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR1
}
