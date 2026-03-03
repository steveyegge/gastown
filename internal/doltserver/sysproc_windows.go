//go:build windows

package doltserver

import "os/exec"

// setProcessGroup is a no-op on Windows.
// Windows does not support Unix process groups (Setpgid).
func setProcessGroup(cmd *exec.Cmd) {}
