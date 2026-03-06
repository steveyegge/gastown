//go:build windows

// exec_windows.go provides a no-op stub for process group setup on Windows,
// where the Unix Setpgid mechanism is not available.
package util

import "os/exec"

// SetProcessGroup is a no-op on Windows.
// Process group management is not supported on Windows.
func SetProcessGroup(cmd *exec.Cmd) {}
