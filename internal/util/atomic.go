// Package util provides common utilities for Gas Town.
package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
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

// AtomicWriteFile writes data to a file atomically.
// It first writes to a temporary file, then renames it to the target path.
// This prevents data corruption if the process crashes during write.
// The rename operation is atomic on POSIX systems.
//
// On Windows, concurrent writes to the same target are handled with retry logic
// to account for Windows file locking semantics.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create a unique temp file to avoid conflicts with concurrent writers
	tmpFile, err := os.CreateTemp(dir, base+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on any failure path
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data and close the file
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename - with retry logic for Windows file locking
	if err := atomicRename(tmpPath, path); err != nil {
		return err
	}

	// Success - clear tmpPath so defer doesn't remove target
	tmpPath = ""
	return nil
}

// atomicRename renames src to dst atomically.
// On Windows, it includes retry logic for transient file locking errors.
func atomicRename(src, dst string) error {
	const maxRetries = 5
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := os.Rename(src, dst); err == nil {
			return nil
		} else {
			lastErr = err
		}

		// On non-Windows, don't retry - POSIX rename should work or fail definitively
		if runtime.GOOS != "windows" {
			break
		}

		// On Windows, wait briefly and retry for transient locking issues
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}

	return fmt.Errorf("rename %s to %s: %w", src, dst, lastErr)
}
