package shellcmd

import "strconv"

// POSIXSleep returns a POSIX sleep(1) command line ("sleep N"). Use inside
// sh/bash hooks, #!/bin/sh scripts, and JSON argv for "sh -c" where the
// fragment must stay POSIX. For tmux pane commands on Windows (cmd.exe), use
// Sleep instead.
func POSIXSleep(seconds int) string {
	return "sleep " + strconv.Itoa(seconds)
}
