// Package shellcmd builds portable shell command strings (e.g. sleep) for
// tmux pane commands, hooks, and tests. It is separate from internal/shell,
// which handles interactive shell RC integration.
//
// POSIXSleep is for fragments inside sh/bash scripts and hooks where the text
// must stay POSIX even when Sleep uses cmd.exe’s timeout on Windows.
package shellcmd
