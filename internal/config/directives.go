package config

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadRoleDirective loads role directive content from the directive file layout.
// Resolution order:
//  1. Town-level: <townRoot>/directives/<role>.md
//  2. Rig-level:  <townRoot>/<rigName>/directives/<role>.md
//
// If both exist, they are concatenated (town first, then rig) separated by a
// newline, giving rig-level content the last word. If only one exists, that
// content is returned. Returns empty string if no directive files exist.
//
// Invalid or unreadable paths are treated as absent (no error).
func LoadRoleDirective(role, townRoot, rigName string) string {
	var parts []string

	// Town-level directive
	townPath := filepath.Join(townRoot, "directives", role+".md")
	if content, err := os.ReadFile(townPath); err == nil { //nolint:gosec // G304: path is from trusted config
		if s := strings.TrimSpace(string(content)); s != "" {
			parts = append(parts, s)
		}
	}

	// Rig-level directive (wins by appearing last)
	if rigName != "" {
		rigPath := filepath.Join(townRoot, rigName, "directives", role+".md")
		if content, err := os.ReadFile(rigPath); err == nil { //nolint:gosec // G304: path is from trusted config
			if s := strings.TrimSpace(string(content)); s != "" {
				parts = append(parts, s)
			}
		}
	}

	return strings.Join(parts, "\n")
}
