// Package workspace provides workspace detection and management.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// ErrNotFound indicates no workspace was found.
var ErrNotFound = errors.New("not in a Gas Town workspace")

// Markers used to detect a Gas Town workspace.
const (
	// PrimaryMarker is the main config file that identifies a workspace.
	// The town.json file lives in mayor/ along with other mayor config.
	// This is the REQUIRED marker - a directory with just "mayor/" is NOT
	// enough to be considered a workspace (could be any project with that folder).
	PrimaryMarker = "mayor/town.json"

	// LegacyMarker supports older workspaces that may not have town.json yet.
	// Only used as fallback if mayor/rigs.json exists (gastown-specific file).
	LegacyMarker = "mayor/rigs.json"
)

// Find locates the town root by walking up from the given directory.
// It requires mayor/town.json (or legacy mayor/rigs.json) to identify a workspace.
// A directory with just "mayor/" is NOT considered a workspace.
// When in a worktree path (polecats/ or crew/), continues to outermost workspace.
// Does not resolve symlinks to stay consistent with os.Getwd().
func Find(startDir string) (string, error) {
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	inWorktree := isInWorktreePath(absDir)
	var primaryMatch, legacyMatch string

	current := absDir
	for {
		// Check for primary marker (mayor/town.json)
		if _, err := os.Stat(filepath.Join(current, PrimaryMarker)); err == nil {
			if !inWorktree {
				return current, nil
			}
			primaryMatch = current
		}

		// Check for legacy marker (mayor/rigs.json) - older workspaces
		if legacyMatch == "" {
			if _, err := os.Stat(filepath.Join(current, LegacyMarker)); err == nil {
				legacyMatch = current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			if primaryMatch != "" {
				return primaryMatch, nil
			}
			return legacyMatch, nil
		}
		current = parent
	}
}

func isInWorktreePath(path string) bool {
	sep := string(filepath.Separator)
	return strings.Contains(path, sep+"polecats"+sep) || strings.Contains(path, sep+"crew"+sep)
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

// IsWorkspace checks if the given directory is a Gas Town workspace root.
// A directory is a workspace if it has a primary marker (mayor/town.json)
// or a legacy marker (mayor/rigs.json). Just having a mayor/ directory
// is NOT enough - that could be any project.
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

	// Check for legacy marker (mayor/rigs.json)
	legacyPath := filepath.Join(absDir, LegacyMarker)
	if _, err := os.Stat(legacyPath); err == nil {
		return true, nil
	}

	return false, nil
}

// GetTownName loads the town name from the workspace's town.json config.
// This is used for generating unique tmux session names that avoid collisions
// when running multiple Gas Town instances.
func GetTownName(townRoot string) (string, error) {
	townConfigPath := filepath.Join(townRoot, PrimaryMarker)
	townConfig, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return "", fmt.Errorf("loading town config: %w", err)
	}
	return townConfig.Name, nil
}

// GetTownNameFromCwd locates the town root from the current working directory
// and returns the town name from its configuration.
func GetTownNameFromCwd() (string, error) {
	townRoot, err := FindFromCwdOrError()
	if err != nil {
		return "", err
	}
	return GetTownName(townRoot)
}

// MustGetTownName returns the town name or panics if it cannot be loaded.
// Use sparingly - prefer GetTownName with proper error handling.
func MustGetTownName(townRoot string) string {
	name, err := GetTownName(townRoot)
	if err != nil {
		panic(fmt.Sprintf("failed to get town name: %v", err))
	}
	return name
}
