//go:build windows

package tmux

// killProcessGroup is a no-op on Windows.
// tmux is not available on Windows, so this code path won't be reached.
func killProcessGroup(pgid int) {
	// No-op: tmux is not supported on Windows
}

// killProcessGroupForce is a no-op on Windows.
func killProcessGroupForce(pgid int) {
	// No-op: tmux is not supported on Windows
}
