//go:build !windows

package shellcmd

import "strconv"

// Sleep returns a shell command line that sleeps for the given number of
// seconds (POSIX sleep(1)), suitable for tmux NewSessionWithCommand and similar.
func Sleep(seconds int) string {
	return "sleep " + strconv.Itoa(seconds)
}

// SleepCommand returns the basename of the sleep command for this platform.
func SleepCommand() string {
	return "sleep"
}
