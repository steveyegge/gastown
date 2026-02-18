package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/crew"
)

// outputIdentityContext loads and emits the crew member's personality file.
// Only runs for crew role. Skips silently if no identity file is found.
func outputIdentityContext(ctx RoleContext) {
	if ctx.Role != RoleCrew {
		return
	}

	identityName := resolveIdentityName(ctx)
	if identityName == "" {
		return
	}

	rigPath := ""
	if ctx.Rig != "" && ctx.TownRoot != "" {
		rigPath = filepath.Join(ctx.TownRoot, ctx.Rig)
	}
	if rigPath == "" {
		return
	}

	content, source, err := crew.ResolveIdentityFile(
		ctx.TownRoot, rigPath, identityName,
	)
	if err != nil {
		explain(true, fmt.Sprintf(
			"Identity: error loading %s: %v", identityName, err,
		))
		return
	}
	if content == "" {
		explain(true, fmt.Sprintf(
			"Identity: no file found for %s", identityName,
		))
		return
	}

	explain(true, fmt.Sprintf(
		"Identity: loaded %s from %s", identityName, source,
	))
	fmt.Println()
	fmt.Println("## Identity")
	fmt.Println()
	fmt.Print(strings.TrimRight(content, "\n"))
	fmt.Println()
}

// outputIdentityBrief emits a short identity reminder for compact/resume.
func outputIdentityBrief(ctx RoleContext) {
	if ctx.Role != RoleCrew {
		return
	}

	identityName := resolveIdentityName(ctx)
	if identityName == "" {
		return
	}

	rigPath := ""
	if ctx.Rig != "" && ctx.TownRoot != "" {
		rigPath = filepath.Join(ctx.TownRoot, ctx.Rig)
	}
	if rigPath == "" {
		return
	}

	content, _, err := crew.ResolveIdentityFile(
		ctx.TownRoot, rigPath, identityName,
	)
	if err != nil || content == "" {
		return
	}

	brief := extractIdentityBrief(content)
	if brief != "" {
		fmt.Printf("> **Identity**: %s\n", brief)
	}
}

// resolveIdentityName determines which identity to load for this crew member.
// Checks state.json for explicit assignment, falls back to crew name.
func resolveIdentityName(ctx RoleContext) string {
	if ctx.Polecat == "" {
		return ""
	}

	// Check state.json for explicit identity assignment
	stateFile := filepath.Join(ctx.WorkDir, "state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var state struct {
			Identity string `json:"identity"`
		}
		if err := json.Unmarshal(data, &state); err == nil &&
			state.Identity != "" {
			return state.Identity
		}
	}

	// Fall back to crew name
	return ctx.Polecat
}

// extractIdentityBrief returns the first substantive line
// from identity content (skipping headings and blank lines).
func extractIdentityBrief(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
