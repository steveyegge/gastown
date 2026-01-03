// Package workspace provides workspace detection and management.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotFound indicates no workspace was found.
var ErrNotFound = errors.New("not in a Gas Town workspace")

// Markers used to detect a Gas Town workspace.
const (
	// PrimaryMarker is the main config file that identifies a workspace.
	// The town.json file lives in mayor/ along with other mayor config.
	PrimaryMarker = "mayor/town.json"

	// SecondaryMarker is an alternative indicator at the town level.
	// Note: This can match rig-level mayors too, so we continue searching
	// upward after finding this to look for primary markers.
	SecondaryMarker = "mayor"
)

// Find locates the town root by walking up from the given directory.
// It looks for mayor/town.json (primary marker) or mayor/ directory (secondary marker).
//
// To avoid matching rig-level mayor directories, we continue searching
// upward after finding a secondary marker, preferring primary matches.
func Find(startDir string) (string, error) {
	// Resolve to absolute path and follow symlinks
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	absDir, err = filepath.EvalSymlinks(absDir)
	if err != nil {
		return "", fmt.Errorf("evaluating symlinks: %w", err)
	}

	// Track the first secondary match in case no primary is found
	var secondaryMatch string

	// Walk up the directory tree
	current := absDir
	for {
		// Check for primary marker (mayor/town.json)
		primaryPath := filepath.Join(current, PrimaryMarker)
		if _, err := os.Stat(primaryPath); err == nil {
			return current, nil
		}

		// Check for secondary marker (mayor/ directory)
		// Don't return immediately - continue searching for primary markers
		if secondaryMatch == "" {
			secondaryPath := filepath.Join(current, SecondaryMarker)
			info, err := os.Stat(secondaryPath)
			if err == nil && info.IsDir() {
				secondaryMatch = current
			}
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root - return secondary match if found
			return secondaryMatch, nil
		}
		current = parent
	}
}

// FindOrError is like Find but returns a user-friendly error if not found.
func FindOrError(startDir string) (string, error) {
	root, err := Find(startDir)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", ErrNotFound
	}
	return root, nil
}

// FindFromCwd locates the town root from the current working directory.
func FindFromCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return Find(cwd)
}

// FindFromCwdOrError is like FindFromCwd but returns an error if not found.
func FindFromCwdOrError() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return FindOrError(cwd)
}

// ResolvePath resolves symlinks in the given path to match the format returned by Find().
// This is necessary because Find() resolves symlinks (e.g., /tmp → /private/tmp on macOS),
// but os.Getwd() may return symlink paths. Using filepath.Rel() with mismatched paths
// produces incorrect results like "../../../tmp/..." instead of "rig/polecats/name".
//
// Use this function on cwd before computing relative paths against a town root:
//
//	townRoot, _ := workspace.FindFromCwd()
//	cwd, _ := workspace.ResolvePath(os.Getwd())
//	relPath, _ := filepath.Rel(townRoot, cwd)  // Now works correctly
func ResolvePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("evaluating symlinks: %w", err)
	}
	return resolved, nil
}

// ResolvedCwd returns the current working directory with symlinks resolved.
// This is a convenience wrapper around ResolvePath(os.Getwd()).
func ResolvedCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return ResolvePath(cwd)
}

// IsWorkspace checks if the given directory is a Gas Town workspace root.
// A directory is a workspace if it has a primary marker (mayor/town.json)
// or a secondary marker (mayor/ directory).
func IsWorkspace(dir string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, fmt.Errorf("resolving path: %w", err)
	}

	// Check for primary marker (mayor/town.json)
	primaryPath := filepath.Join(absDir, PrimaryMarker)
	if _, err := os.Stat(primaryPath); err == nil {
		return true, nil
	}

	// Check for secondary marker (mayor/ directory)
	secondaryPath := filepath.Join(absDir, SecondaryMarker)
	info, err := os.Stat(secondaryPath)
	if err == nil && info.IsDir() {
		return true, nil
	}

	return false, nil
}
