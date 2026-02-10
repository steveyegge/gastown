package web

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Validation patterns for user input.
var (
	// idPattern matches typical IDs: alphanumeric, hyphens, underscores, dots.
	idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	// repoRefPattern matches GitHub-style owner/repo references.
	repoRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

// isValidID checks if a string is a safe identifier (issue IDs, message IDs, rig names).
func isValidID(s string) bool {
	return len(s) > 0 && len(s) <= 200 && idPattern.MatchString(s)
}

// isValidRepoRef checks if a string matches the owner/repo format.
func isValidRepoRef(s string) bool {
	return repoRefPattern.MatchString(s)
}

// isNumeric checks if a string contains only ASCII digits.
func isNumeric(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isValidGitURL checks if a string looks like a valid git remote reference.
// Accepts HTTPS URLs, SSH URLs (git@), and owner/repo shorthand.
func isValidGitURL(s string) bool {
	return strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") ||
		isValidRepoRef(s)
}

// expandHomePath safely expands ~ prefix, cleans the result, and ensures
// ~-expanded paths stay within the home directory.
// Returns error if home directory cannot be determined or path escapes home.
func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		// Non-~ paths: normalize to remove . / .. segments for consistency.
		// Callers that need traversal checks should validate separately.
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	cleaned := filepath.Clean(filepath.Join(home, path[2:]))
	// Ensure ~ expansion doesn't escape the home directory
	if cleaned != home && !strings.HasPrefix(cleaned, home+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes home directory")
	}
	return cleaned, nil
}
