//go:build !windows

package tmux

// hasDescendantWithNamesWindows is a stub for non-Windows platforms.
// The real implementation uses the Toolhelp32 API and lives in descendants_windows.go.
func hasDescendantWithNamesWindows(_ string, _ []string, _ int) bool {
	return false
}
