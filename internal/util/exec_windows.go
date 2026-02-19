//go:build windows

package util

import "os/exec"

// SetProcessGroup is a no-op on Windows.
// Process group management is not supported on Windows.
func SetProcessGroup(cmd *exec.Cmd) {}
