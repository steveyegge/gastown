// Package beads provides redirect resolution for beads databases.
package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveBeadsDir returns the actual beads directory, following any redirect.
// If workDir/.beads/redirect exists, it reads the redirect path and resolves it
// relative to workDir (not the .beads directory). Otherwise, returns workDir/.beads.
//
// This is essential for crew workers and polecats that use shared beads via redirect.
// The redirect file contains a relative path like "../../mayor/rig/.beads".
//
// Example: if we're at crew/max/ and .beads/redirect contains "../../mayor/rig/.beads",
// the redirect is resolved from crew/max/ (not crew/max/.beads/), giving us
// mayor/rig/.beads at the rig root level.
//
// Circular redirect detection: If the resolved path equals the original beads directory,
// this indicates an errant redirect file that should be removed. The function logs a
// warning and returns the original beads directory.
func ResolveBeadsDir(workDir string) string {
	if filepath.Base(workDir) == ".beads" {
		workDir = filepath.Dir(workDir)
	}
	beadsDir := filepath.Join(workDir, ".beads")
	redirectPath := filepath.Join(beadsDir, "redirect")

	// Check for redirect file
	data, err := os.ReadFile(redirectPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		// No redirect, use local .beads
		return beadsDir
	}

	// Read and clean the redirect path
	redirectTarget := strings.TrimSpace(string(data))
	if redirectTarget == "" {
		return beadsDir
	}

	// Resolve relative to workDir (the redirect is written from the perspective
	// of being inside workDir, not inside workDir/.beads)
	// e.g., redirect contains "../../mayor/rig/.beads"
	// from crew/max/, this resolves to mayor/rig/.beads
	resolved := filepath.Join(workDir, redirectTarget)

	// Clean the path to resolve .. components
	resolved = filepath.Clean(resolved)

	// Detect circular redirects: if resolved path equals original beads dir,
	// this is an errant redirect file (e.g., redirect in mayor/rig/.beads pointing to itself)
	if resolved == beadsDir {
		fmt.Fprintf(os.Stderr, "Warning: circular redirect detected in %s (points to itself), ignoring redirect\n", redirectPath)
		// Remove the errant redirect file to prevent future warnings
		if err := os.Remove(redirectPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove errant redirect file: %v\n", err)
		}
		return beadsDir
	}

	// Follow redirect chains (e.g., crew/.beads -> rig/.beads -> mayor/rig/.beads)
	// This is intentional for the rig-level redirect architecture.
	// Limit depth to prevent infinite loops from misconfigured redirects.
	return resolveBeadsDirWithDepth(resolved, 3)
}

// resolveBeadsDirWithDepth follows redirect chains with a depth limit.
func resolveBeadsDirWithDepth(beadsDir string, maxDepth int) string {
	if maxDepth <= 0 {
		fmt.Fprintf(os.Stderr, "Warning: redirect chain too deep at %s, stopping\n", beadsDir)
		return beadsDir
	}

	redirectPath := filepath.Join(beadsDir, "redirect")
	data, err := os.ReadFile(redirectPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		// No redirect, this is the final destination
		return beadsDir
	}

	redirectTarget := strings.TrimSpace(string(data))
	if redirectTarget == "" {
		return beadsDir
	}

	// Resolve relative to parent of beadsDir (the workDir)
	workDir := filepath.Dir(beadsDir)
	resolved := filepath.Clean(filepath.Join(workDir, redirectTarget))

	// Detect circular redirect
	if resolved == beadsDir {
		fmt.Fprintf(os.Stderr, "Warning: circular redirect detected in %s, stopping\n", redirectPath)
		return beadsDir
	}

	// Recursively follow
	return resolveBeadsDirWithDepth(resolved, maxDepth-1)
}

// cleanBeadsRuntimeFiles removes gitignored runtime files from a .beads directory
// while preserving tracked files (formulas/, README.md, config.yaml, .gitignore).
// This is safe to call even if the directory doesn't exist.
func cleanBeadsRuntimeFiles(beadsDir string) error {
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil // Nothing to clean
	}

	// Runtime files/patterns that are gitignored and safe to remove
	runtimePatterns := []string{
		// SQLite databases
		"*.db", "*.db-*", "*.db?*",
		// Daemon runtime
		"daemon.lock", "daemon.log", "daemon.pid", "bd.sock",
		// Sync state
		"sync-state.json", "last-touched", "metadata.json",
		// Version tracking
		".local_version",
		// Redirect file (we're about to recreate it)
		"redirect",
		// Merge artifacts
		"beads.base.*", "beads.left.*", "beads.right.*",
		// JSONL files (tracked but will be redirected, safe to remove in worktrees)
		"issues.jsonl", "interactions.jsonl",
		// Runtime directories
		"mq",
	}

	var firstErr error
	for _, pattern := range runtimePatterns {
		matches, err := filepath.Glob(filepath.Join(beadsDir, pattern))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, match := range matches {
			if err := os.RemoveAll(match); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// SetupRedirect creates a .beads/redirect file for a worktree to point to the rig's shared beads.
// This is used by crew, polecats, and refinery worktrees to share the rig's beads database.
//
// Parameters:
//   - townRoot: the town root directory (e.g., ~/gt)
//   - worktreePath: the worktree directory (e.g., <rig>/crew/<name> or <rig>/refinery/rig)
//
// The function:
//  1. Computes the relative path from worktree to rig-level .beads
//  2. Cleans up runtime files (preserving tracked files like formulas/)
//  3. Creates the redirect file
//
// Safety: This function refuses to create redirects in the canonical beads location
// (mayor/rig) to prevent circular redirect chains.
func SetupRedirect(townRoot, worktreePath string) error {
	// Get rig root from worktree path
	// worktreePath = <town>/<rig>/crew/<name> or <town>/<rig>/refinery/rig etc.
	relPath, err := filepath.Rel(townRoot, worktreePath)
	if err != nil {
		return fmt.Errorf("computing relative path: %w", err)
	}
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid worktree path: must be at least 2 levels deep from town root")
	}

	// Safety check: prevent creating redirect in canonical beads location (mayor/rig)
	// This would create a circular redirect chain since rig/.beads redirects to mayor/rig/.beads
	if len(parts) >= 2 && parts[1] == "mayor" {
		return fmt.Errorf("cannot create redirect in canonical beads location (mayor/rig)")
	}

	rigRoot := filepath.Join(townRoot, parts[0])
	rigBeadsPath := filepath.Join(rigRoot, ".beads")
	mayorBeadsPath := filepath.Join(rigRoot, "mayor", "rig", ".beads")

	// Check rig-level .beads first, fall back to mayor/rig/.beads (tracked beads architecture).
	// For dolt backend, the actual database lives at mayor/rig/.beads/dolt/, not at rig/.beads/.
	// The rig-root .beads/ only has metadata.json (runtime state). If rig/.beads exists but has
	// no database (no dolt/ and no beads.db), redirect to mayor/rig/.beads where the DB is.
	usesMayorFallback := false
	rigBeadsExists := false
	if _, err := os.Stat(rigBeadsPath); err == nil {
		rigBeadsExists = true
	}
	rigHasDB := false
	if rigBeadsExists {
		// Check for actual database: dolt/ directory or beads.db file
		if _, err := os.Stat(filepath.Join(rigBeadsPath, "dolt")); err == nil {
			rigHasDB = true
		} else if _, err := os.Stat(filepath.Join(rigBeadsPath, "beads.db")); err == nil {
			rigHasDB = true
		}
	}

	if !rigBeadsExists || !rigHasDB {
		// Rig .beads doesn't exist or has no database — check mayor/rig/.beads
		if _, err := os.Stat(mayorBeadsPath); os.IsNotExist(err) {
			if !rigBeadsExists {
				return fmt.Errorf("no beads found at %s or %s", rigBeadsPath, mayorBeadsPath)
			}
			// Rig .beads exists but has no DB and mayor path doesn't exist either.
			// Fall through to use rig path (best effort).
		} else {
			usesMayorFallback = true
		}
	}

	// Clean up runtime files in .beads/ but preserve tracked files (formulas/, README.md, etc.)
	worktreeBeadsDir := filepath.Join(worktreePath, ".beads")

	// Handle edge case: if .beads exists as a file (not directory), remove it.
	// This can happen with stale state from previous failed operations or
	// unusual clone state. MkdirAll would fail with "file exists" in this case.
	if info, err := os.Stat(worktreeBeadsDir); err == nil && !info.IsDir() {
		if err := os.Remove(worktreeBeadsDir); err != nil {
			return fmt.Errorf("removing stale .beads file: %w", err)
		}
	}

	if err := cleanBeadsRuntimeFiles(worktreeBeadsDir); err != nil {
		return fmt.Errorf("cleaning runtime files: %w", err)
	}

	// Create .beads directory if it doesn't exist
	if err := os.MkdirAll(worktreeBeadsDir, 0755); err != nil {
		return fmt.Errorf("creating .beads dir: %w", err)
	}

	// Compute relative path from worktree to rig root
	// e.g., crew/<name> (depth 2) -> ../../.beads
	//       refinery/rig (depth 2) -> ../../.beads
	depth := len(parts) - 1 // subtract 1 for rig name itself
	upPath := strings.Repeat("../", depth)

	var redirectPath string
	if usesMayorFallback {
		// Direct redirect to mayor/rig/.beads since rig/.beads doesn't exist
		redirectPath = upPath + "mayor/rig/.beads"
	} else {
		redirectPath = upPath + ".beads"

		// Check if rig-level beads has a redirect (tracked beads case).
		// If so, redirect directly to the final destination to avoid chains.
		// The bd CLI doesn't support redirect chains, so we must skip intermediate hops.
		rigRedirectPath := filepath.Join(rigBeadsPath, "redirect")
		if data, err := os.ReadFile(rigRedirectPath); err == nil {
			rigRedirectTarget := strings.TrimSpace(string(data))
			if rigRedirectTarget != "" {
				// Rig has redirect (e.g., "mayor/rig/.beads" for tracked beads).
				// Redirect worktree directly to the final destination.
				redirectPath = upPath + rigRedirectTarget
			}
		}
	}

	// Create redirect file
	redirectFile := filepath.Join(worktreeBeadsDir, "redirect")
	if err := os.WriteFile(redirectFile, []byte(redirectPath+"\n"), 0644); err != nil {
		return fmt.Errorf("creating redirect file: %w", err)
	}

	return nil
}
