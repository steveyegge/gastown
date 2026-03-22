//go:build windows

package telegram

import "os"

// InodeChanged detects file rotation on Windows by checking if the new file
// is smaller than the old one (a reliable heuristic since Windows lacks inodes).
func InodeChanged(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return true
	}
	return b.Size() < a.Size()
}
