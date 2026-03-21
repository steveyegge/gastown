//go:build windows

package tmux

import (
	"fmt"
	"time"
)

// acquireFlockLock is not supported on Windows. Tmux is not available on
// Windows, so this function should never be called in practice.
func acquireFlockLock(lockPath string, timeout time.Duration) (func(), error) {
	return nil, fmt.Errorf("flock not supported on windows")
}
