package wisp

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/util"
)

// EnsureDir ensures the .beads directory exists in the given root.
func EnsureDir(root string) (string, error) {
	dir := filepath.Join(root, WispDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create beads dir: %w", err)
	}
	return dir, nil
}

// WispPath returns the full path to a file in the beads directory.
func WispPath(root, filename string) string {
	return filepath.Join(root, WispDir, filename)
}

// writeJSON is a helper to write JSON files atomically.
// Uses the shared util.AtomicWriteJSON for consistent atomic write behavior.
func writeJSON(path string, v interface{}) error {
	if err := util.AtomicWriteJSON(path, v); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}
