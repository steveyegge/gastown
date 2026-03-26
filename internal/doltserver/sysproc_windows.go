//go:build windows

package doltserver

import (
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on Windows.
// Windows does not support Unix process groups (Setpgid).
func setProcessGroup(cmd *exec.Cmd) {}

// processAlive checks whether a process is still running on Windows.
// Signal(0) is not supported on Windows, so we attempt to open the
// process handle via FindProcess (always succeeds on Windows) and
// then use Process.Signal(os.Kill) check semantics — instead, we
// use a small trick: Process.Signal(nil) is not defined, but we can
// rely on the fact that on Windows, a process handle obtained via
// FindProcess/Start remains valid only while the process is alive.
// The most reliable cross-platform approach is to call Process.Wait
// in a non-blocking way, but Go doesn't expose that. Instead, we
// try to open the process via the Windows API.
func processAlive(p *os.Process) bool {
	// On Windows, os.FindProcess always succeeds. The reliable way to
	// check liveness is to open the process handle with limited access.
	// We use p.Signal(os.Kill) as a probe — but that would kill it.
	// Instead, use the Windows-specific OpenProcess approach via syscall.
	return isProcessRunning(p.Pid)
}

// processTerminate kills the process on Windows.
// Windows has no SIGTERM equivalent; Process.Kill() calls TerminateProcess.
func processTerminate(p *os.Process) error {
	return p.Kill()
}

// processKill forcefully kills the process on Windows.
// Same as processTerminate since Windows only has TerminateProcess.
func processKill(p *os.Process) error {
	return p.Kill()
}
