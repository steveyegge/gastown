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
	// idPattern requires alphanumeric first character, which rejects --flag injection.
	// All gastown IDs (bead IDs like gt-abc12, message IDs like msg.001, rig names)
	// start with [a-zA-Z0-9].
	idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	// rigNamePattern matches valid rig names. Rig names must NOT contain hyphens,
	// dots, or spaces — these are reserved for agent ID parsing (format:
	// <prefix>-<rig>-<role>[-<name>]). Mirrors internal/rig/manager.go:269.
	// Leading underscore is allowed (core manager only rejects "-. ").
	// This regex is intentionally stricter than the manager's ContainsAny("-. ")
	// check — it acts as defense-in-depth by restricting to a safe character
	// class for filesystem paths and shell argument safety.
	rigNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	// repoRefPattern matches GitHub-style owner/repo references.
	repoRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

// isValidID checks if a string is a safe identifier (issue IDs, message IDs, rig names).
func isValidID(s string) bool {
	return len(s) > 0 && len(s) <= 200 && idPattern.MatchString(s)
}

// isValidRigName checks if a string is a valid rig name.
// Rig names allow only alphanumeric + underscore (no hyphens, dots, or spaces),
// matching the constraint in internal/rig/manager.go:AddRig.
func isValidRigName(s string) bool {
	return len(s) > 0 && len(s) <= 200 && rigNamePattern.MatchString(s)
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

// isValidMailAddress checks if a string is a safe mail recipient address.
// Mail addresses may contain '/', ':', '@', '*' (for agent paths, prefixed
// types, @-patterns, and wildcards per internal/mail/resolve.go).
// Rejects empty, leading '-' (flag injection), and control characters.
func isValidMailAddress(s string) bool {
	if len(s) == 0 || len(s) > 200 || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f { // control characters
			return false
		}
	}
	return true
}

// isValidGitURL checks if a string looks like a valid git remote URL.
// Accepts any scheme:// URL (git delegates to git-remote-<scheme> helpers,
// e.g. git-remote-s3 for s3:// URLs), plus SCP-style (user@host:path).
// Rejects local paths, file:// URIs, flag-like strings, and bare names.
func isValidGitURL(s string) bool {
	if len(s) == 0 || strings.HasPrefix(s, "-") {
		return false
	}
	// Reject file:// URIs (local filesystem access)
	if strings.HasPrefix(s, "file://") {
		return false
	}
	// Accept any scheme:// URL where scheme is alphanumeric (plus + - .)
	if idx := strings.Index(s, "://"); idx > 0 {
		scheme := s[:idx]
		for _, c := range scheme {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.') {
				return false
			}
		}
		return true
	}
	// SCP-style: user@host:path (user and host non-empty, path non-empty, host has no slashes)
	atIdx := strings.Index(s, "@")
	colonIdx := strings.Index(s, ":")
	return atIdx > 0 && colonIdx > atIdx+1 && colonIdx < len(s)-1 && !strings.Contains(s[:colonIdx], "/")
}

// expandHomePath safely expands ~ prefix, cleans the result, and ensures
// ~-expanded paths stay within the home directory.
// Returns error if home directory cannot be determined or path escapes home.
func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		// Non-~ paths: clean only. Arbitrary absolute paths are intentional —
		// users may install workspaces anywhere. Callers that need containment
		// checks for ~-relative paths use the ~ prefix form.
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
	// Ensure ~ expansion doesn't escape the home directory.
	// Special-case home=="/" (root user): every absolute path starts with "/",
	// so containment is always true — which matches the intent (root can access anything).
	if home != "/" && cleaned != home && !strings.HasPrefix(cleaned, home+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes home directory")
	}
	return cleaned, nil
}
