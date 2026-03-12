//go:build windows

package acp

import (
	"os"
)

// signalsToHandle returns the signals that Forward() should listen for.
// On Windows, only os.Interrupt is available (CTRL+C).
func signalsToHandle() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// setupProcessGroup is a no-op on Windows.
// Windows doesn't have process groups like Unix.
func (p *Proxy) setupProcessGroup() {
	// No-op on Windows - no process group support
}

// isProcessAlive checks if the agent process is still running.
// On Windows, we use os.Signal(nil) to check process liveness.
func (p *Proxy) isProcessAlive() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	// On Windows, checking if a process is alive is often done by checking if wait fails
	// or by trying to get a handle. os.Process.Signal(nil) is partially supported.
	return p.cmd.Process.Signal(os.Signal(nil)) == nil
}

// terminateProcess kills the agent process.
// On Windows, we use Process.Kill() as there's no graceful SIGTERM equivalent.
func (p *Proxy) terminateProcess() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}
