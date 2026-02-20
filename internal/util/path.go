package util

import (
	"os"
	"strings"
	"sync"
)

var (
	homeDir     string
	homeDirOnce sync.Once
)

// cachedHomeDir returns the user's home directory, cached after the first call.
func cachedHomeDir() string {
	homeDirOnce.Do(func() {
		homeDir, _ = os.UserHomeDir()
	})
	return homeDir
}

// ExpandHome expands a leading ~/ to the user's home directory.
// Returns the path unchanged if it doesn't start with ~/ or if
// the home directory cannot be determined.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home := cachedHomeDir()
	if home == "" {
		return path
	}
	return home + path[1:]
}
