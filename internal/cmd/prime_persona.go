package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/crew"
)

// outputPersonaContext loads and emits the crew member's persona.
// Only runs for crew role. Skips silently if no persona is found.
// Reads from beads (via PersonaBead in agent bead) when available;
// falls back to .personas/ files for sessions before first sync.
func outputPersonaContext(ctx RoleContext) {
	if ctx.Role != RoleCrew {
		return
	}

	rigPath := ""
	if ctx.Rig != "" && ctx.TownRoot != "" {
		rigPath = filepath.Join(ctx.TownRoot, ctx.Rig)
	}
	if rigPath == "" {
		return
	}

	// Try beads-based persona first
	if content := loadPersonaFromBead(ctx, rigPath); content != "" {
		emitPersonaContent(content, "beads")
		return
	}

	// Fall back to file-based persona
	personaName := resolvePersonaName(ctx)
	if personaName == "" {
		return
	}

	content, source, err := crew.ResolvePersonaFile(
		ctx.TownRoot, rigPath, personaName,
	)
	if err != nil {
		explain(true, fmt.Sprintf(
			"Persona: error loading %s: %v", personaName, err,
		))
		return
	}
	if content == "" {
		explain(true, fmt.Sprintf(
			"Persona: no file found for %s", personaName,
		))
		return
	}

	explain(true, fmt.Sprintf(
		"Persona: loaded %s from %s", personaName, source,
	))
	emitPersonaContent(content, source)
}

// loadPersonaFromBead reads persona content from the crew agent bead.
// Returns empty string if no PersonaBead is set or the bead cannot be read.
func loadPersonaFromBead(ctx RoleContext, rigPath string) string {
	if ctx.TownRoot == "" || ctx.Rig == "" || ctx.Polecat == "" {
		return ""
	}

	b := beads.New(beads.ResolveBeadsDir(rigPath))

	prefix := beads.GetPrefixForRig(ctx.TownRoot, ctx.Rig)
	crewBeadID := beads.CrewBeadIDWithPrefix(prefix, ctx.Rig, ctx.Polecat)

	_, fields, err := b.GetAgentBead(crewBeadID)
	if err != nil || fields == nil || fields.PersonaBead == "" {
		return ""
	}

	content, err := beads.GetPersonaContent(b, fields.PersonaBead)
	if err != nil {
		explain(true, fmt.Sprintf(
			"Persona: error reading bead %s: %v", fields.PersonaBead, err,
		))
		return ""
	}
	return content
}

// emitPersonaContent writes the ## Persona section to stdout.
func emitPersonaContent(content, source string) {
	explain(true, fmt.Sprintf("Persona: loaded from %s", source))
	fmt.Println()
	fmt.Println("## Persona")
	fmt.Println()
	fmt.Print(strings.TrimRight(content, "\n"))
	fmt.Println()
}

// outputPersonaBrief emits a short persona reminder for compact/resume.
func outputPersonaBrief(ctx RoleContext) {
	if ctx.Role != RoleCrew {
		return
	}

	personaName := resolvePersonaName(ctx)
	if personaName == "" {
		return
	}

	rigPath := ""
	if ctx.Rig != "" && ctx.TownRoot != "" {
		rigPath = filepath.Join(ctx.TownRoot, ctx.Rig)
	}
	if rigPath == "" {
		return
	}

	content, _, err := crew.ResolvePersonaFile(
		ctx.TownRoot, rigPath, personaName,
	)
	if err != nil || content == "" {
		return
	}

	brief := extractPersonaBrief(content)
	if brief != "" {
		fmt.Printf("> **Persona**: %s\n", brief)
	}
}

// resolvePersonaName determines which persona to load for this crew member.
// Checks state.json for explicit assignment, falls back to crew name.
func resolvePersonaName(ctx RoleContext) string {
	if ctx.Polecat == "" {
		return ""
	}

	// Check state.json for explicit persona assignment.
	// Path derived from known coordinates — stable regardless of cwd.
	stateFile := filepath.Join(ctx.TownRoot, ctx.Rig, "crew", ctx.Polecat, "state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var state struct {
			Persona string `json:"persona"`
		}
		if err := json.Unmarshal(data, &state); err == nil &&
			state.Persona != "" {
			return state.Persona
		}
	}

	// Fall back to crew name
	return ctx.Polecat
}

// extractPersonaBrief returns the first substantive line from persona content
// (skipping headings and blank lines).
func extractPersonaBrief(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
