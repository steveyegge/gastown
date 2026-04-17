//go:build windows

package shellcmd

import "fmt"

// Sleep returns a shell command line that sleeps for the given number of
// seconds (Windows timeout), suitable for tmux when the pane runs cmd.exe.
// The string is not wrapped in powershell.exe; pass it to tmux as the pane command.
func Sleep(seconds int) string {
	return fmt.Sprintf("timeout /T %d > NUL", seconds)
}

// SleepCommand returns the basename of the sleep command for this platform.
func SleepCommand() string {
	return "timeout"
}
