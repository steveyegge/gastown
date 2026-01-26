// Package beads provides redirect resolution for beads databases.
package beads

import (
	"encoding/json"
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

	// Check rig-level .beads first, fall back to mayor/rig/.beads (tracked beads architecture)
	usesMayorFallback := false
	if _, err := os.Stat(rigBeadsPath); os.IsNotExist(err) {
		// No rig/.beads - check for mayor/rig/.beads (tracked beads architecture)
		if _, err := os.Stat(mayorBeadsPath); os.IsNotExist(err) {
			return fmt.Errorf("no beads found at %s or %s", rigBeadsPath, mayorBeadsPath)
		}
		// Using mayor fallback - warn user to run bd doctor
		fmt.Fprintf(os.Stderr, "Warning: rig .beads not found at %s, using %s\n", rigBeadsPath, mayorBeadsPath)
		fmt.Fprintf(os.Stderr, "  Run 'bd doctor' to fix rig beads configuration\n")
		usesMayorFallback = true
	}

	// Clean up runtime files in .beads/ but preserve tracked files (formulas/, README.md, etc.)
	worktreeBeadsDir := filepath.Join(worktreePath, ".beads")
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

	// Copy routes.jsonl from town root to enable cross-prefix bead resolution
	townRoutesPath := filepath.Join(townRoot, ".beads", "routes.jsonl")
	if routesData, err := os.ReadFile(townRoutesPath); err == nil {
		worktreeRoutesPath := filepath.Join(worktreeBeadsDir, "routes.jsonl")
		os.WriteFile(worktreeRoutesPath, routesData, 0644)
	}

	// Copy config.yaml from town root to enable custom types
	townConfigPath := filepath.Join(townRoot, ".beads", "config.yaml")
	if configData, err := os.ReadFile(townConfigPath); err == nil {
		worktreeConfigPath := filepath.Join(worktreeBeadsDir, "config.yaml")
		os.WriteFile(worktreeConfigPath, configData, 0644)
	}

	return nil
}

// CopyBeadEntryToWorktree copies a bead entry from the town's issues.jsonl to the worktree's issues.jsonl.
// This allows polecats in worktrees to resolve target beads that exist in the parent town.
//
// Parameters:
//   - townRoot: the town root directory
//   - beadID: the bead ID to copy (e.g., "hq-abc")
//   - worktreePath: the worktree directory
//
// This is safe to call even if the bead doesn't exist in the town beads.
func CopyBeadEntryToWorktree(townRoot, beadID, worktreePath string) error {
	if beadID == "" {
		return nil // Nothing to copy
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	townIssuesPath := filepath.Join(townBeadsDir, "issues.jsonl")

	// Try to read the town's issues.jsonl
	townData, err := os.ReadFile(townIssuesPath)
	if err != nil {
		// Town issues.jsonl doesn't exist or is not readable - not fatal
		return nil
	}

	// Find the line matching the beadID
	lines := strings.Split(strings.TrimSpace(string(townData)), "\n")
	var beadLine string
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Try to parse the line to check if it contains our bead ID
		var issueData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &issueData); err != nil {
			continue
		}
		if id, ok := issueData["id"].(string); ok && id == beadID {
			beadLine = line
			break
		}
	}

	if beadLine == "" {
		// Bead not found in town issues.jsonl - not fatal
		return nil
	}

	// Ensure worktree .beads directory exists
	worktreeBeadsDir := filepath.Join(worktreePath, ".beads")
	if err := os.MkdirAll(worktreeBeadsDir, 0755); err != nil {
		return fmt.Errorf("creating worktree .beads dir: %w", err)
	}

	// Read existing worktree issues.jsonl if it exists
	worktreeIssuesPath := filepath.Join(worktreeBeadsDir, "issues.jsonl")
	var worktreeLines []string
	if existingData, err := os.ReadFile(worktreeIssuesPath); err == nil {
		existingLines := strings.Split(strings.TrimSpace(string(existingData)), "\n")
		for _, line := range existingLines {
			if line == "" {
				continue
			}
			// Check if this line already contains our bead ID
			var issueData map[string]interface{}
			if err := json.Unmarshal([]byte(line), &issueData); err != nil {
				worktreeLines = append(worktreeLines, line)
				continue
			}
			if id, ok := issueData["id"].(string); ok && id == beadID {
				// Already exists - skip (we'll add the new one)
				continue
			}
			worktreeLines = append(worktreeLines, line)
		}
	}

	// Append the bead entry
	worktreeLines = append(worktreeLines, beadLine)

	// Write back to worktree issues.jsonl
	content := strings.Join(worktreeLines, "\n") + "\n"
	if err := os.WriteFile(worktreeIssuesPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing worktree issues.jsonl: %w", err)
	}

	return nil
}
