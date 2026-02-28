package rig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/style"
)

// CopyOverlay copies files from <rigPath>/.runtime/overlay/ to the destination path.
// This allows storing gitignored files (like .env) that services need at their root.
// The overlay is copied non-recursively - only files, not subdirectories.
// File permissions from the source are preserved.
//
// Structure:
//
//	rig/
//	  .runtime/
//	    overlay/
//	      .env          <- Copied to destPath
//	      config.json   <- Copied to destPath
//
// Returns nil if the overlay directory doesn't exist (nothing to copy).
// Individual file copy failures are logged as warnings but don't stop the process.
func CopyOverlay(rigPath, destPath string) error {
	overlayDir := filepath.Join(rigPath, ".runtime", "overlay")

	// Check if overlay directory exists
	entries, err := os.ReadDir(overlayDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No overlay directory - not an error, just nothing to copy
			return nil
		}
		return fmt.Errorf("reading overlay dir: %w", err)
	}

	// Copy each file (not directories) from overlay to destination
	for _, entry := range entries {
		if entry.IsDir() {
			// Skip subdirectories - only copy files at overlay root
			continue
		}

		srcPath := filepath.Join(overlayDir, entry.Name())
		dstPath := filepath.Join(destPath, entry.Name())

		if err := copyFilePreserveMode(srcPath, dstPath); err != nil {
			// Log warning but continue - don't fail spawn for overlay issues
			style.PrintWarning("could not copy overlay file %s: %v", entry.Name(), err)
			continue
		}
	}

	return nil
}

// EnsureGitignorePatterns ensures the .gitignore has required Gas Town patterns.
// This is called after cloning to add patterns that may be missing from the source repo.
func EnsureGitignorePatterns(worktreePath string) error {
	gitignorePath := filepath.Join(worktreePath, ".gitignore")

	// Required patterns for Gas Town worktrees.
	// DO NOT add ".beads/" here. Beads manages its own .beads/.gitignore
	// (created by bd init) which selectively ignores runtime files.
	// Adding .beads/ here overrides that and breaks bd sync.
	// This has regressed twice (PR #753 added it, #891 removed it,
	// #966 re-added it). See overlay_test.go for a regression guard.
	//
	// NOTE: .claude/settings.json is NOT listed here because settings are now
	// installed in gastown-managed parent directories via --settings flag,
	// not in the customer repo worktree.
	requiredPatterns := []string{
		".runtime/",
		".claude/commands/",
		".logs/",
	}

	// Read existing gitignore content
	var existingContent string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existingContent = string(data)
	}

	// Find missing patterns
	var missing []string
	for _, pattern := range requiredPatterns {
		found := false
		for _, line := range strings.Split(existingContent, "\n") {
			line = strings.TrimSpace(line)
			if matchesGitignorePattern(line, pattern) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, pattern)
		}
	}

	if len(missing) == 0 {
		return nil // All patterns present
	}

	// Append missing patterns
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	// Add header if appending to existing file
	if existingContent != "" && !strings.HasSuffix(existingContent, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	if existingContent != "" {
		if _, err := f.WriteString("\n# Gas Town (added by gt)\n"); err != nil {
			return err
		}
	}

	for _, pattern := range missing {
		if _, err := f.WriteString(pattern + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// matchesGitignorePattern checks if a gitignore line covers the required pattern.
// Handles variant forms (with/without trailing slash, leading slash) and recognizes
// that a broader directory pattern (e.g., ".claude/") covers more specific paths
// (e.g., ".claude/commands/").
func matchesGitignorePattern(line, pattern string) bool {
	// Strip leading slash for comparison
	normLine := strings.TrimPrefix(line, "/")
	normPattern := strings.TrimPrefix(pattern, "/")

	// Exact match or trailing-slash variants
	if normLine == normPattern ||
		normLine == strings.TrimSuffix(normPattern, "/") ||
		normLine+"/" == normPattern {
		return true
	}

	// A broader directory pattern covers more specific paths underneath it.
	// e.g., ".claude/" covers ".claude/commands/"
	if strings.HasSuffix(normLine, "/") && strings.HasPrefix(normPattern, normLine) {
		return true
	}
	// Also handle directory pattern without trailing slash
	if !strings.Contains(normLine, "/") && strings.HasPrefix(normPattern, normLine+"/") {
		return true
	}

	return false
}

// copyFilePreserveMode copies a file from src to dst, preserving the source file's permissions.
func copyFilePreserveMode(src, dst string) error {
	// Get source file info for permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	// Create destination file with same permissions
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		return fmt.Errorf("copy contents: %w", err)
	}

	// Explicitly check Close() — on many filesystems, buffered data is flushed
	// at Close() time, so a full-disk error surfaces here, not during Write.
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("closing destination: %w", err)
	}

	return nil
}
