//go:build windows

package tmux

// killProcessGroup is a no-op on Windows.
// tmux is not available on Windows, so process group killing is not needed.
func killProcessGroup(pgid int, sig int) error {
	// No-op on Windows - tmux is not supported
	return nil
}

// Signal constants that work across platforms.
const (
	sigTERM = 15
	sigKILL = 9
)
