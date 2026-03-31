//go:build !windows

package tmux

import "os/exec"

// hideConsoleWindow is a no-op on Unix (no console window issue).
func hideConsoleWindow(cmd *exec.Cmd) {}
