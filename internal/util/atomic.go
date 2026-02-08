// Package util provides common utilities for Gas Town.
package util

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AtomicWriteJSON writes JSON data to a file atomically.
// It first writes to a temporary file, then renames it to the target path.
// This prevents data corruption if the process crashes during write.
// The rename operation is atomic on POSIX systems.
func AtomicWriteJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWriteFile(path, data, 0644)
}

// AtomicWriteJSONWithPerm writes JSON data to a file atomically with custom permissions.
// It first writes to a temporary file, then renames it to the target path.
// This prevents data corruption if the process crashes during write.
// The rename operation is atomic on POSIX systems.
func AtomicWriteJSONWithPerm(path string, v interface{}, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWriteFile(path, data, perm)
}

// EnsureDirAndWriteJSON creates parent directories if needed, then atomically writes JSON.
// This is a convenience function for the common pattern of:
//
//	os.MkdirAll(filepath.Dir(path), 0755)
//	util.AtomicWriteJSON(path, data)
func EnsureDirAndWriteJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return AtomicWriteJSON(path, v)
}

// EnsureDirAndWriteJSONWithPerm creates directories and writes JSON with custom permissions.
func EnsureDirAndWriteJSONWithPerm(path string, v interface{}, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return AtomicWriteJSONWithPerm(path, v, perm)
}

// AtomicWriteFile writes data to a file atomically.
// It first writes to a temporary file, then renames it to the target path.
// This prevents data corruption if the process crashes during write.
// The rename operation is atomic on POSIX systems.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmpFile := path + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return err
	}

	// Atomic rename (on POSIX systems)
	if err := os.Rename(tmpFile, path); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tmpFile)
		return err
	}

	return nil
}
