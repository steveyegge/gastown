package beads

import (
	"errors"
	"fmt"
	"strings"
)

// PersonaBeadInfo describes a persona bead in the database.
type PersonaBeadInfo struct {
	ID     string
	Name   string
	Source string // "rig" or "town"
	Hash   string
}

// PersonaBeadID returns the deterministic bead ID for a persona.
// Follows the existing pattern: <prefix>-<rig>-<role>-<name>.
func PersonaBeadID(prefix, rig, name string) string {
	if rig == "" {
		return prefix + "-persona-" + name
	}
	return prefix + "-" + rig + "-persona-" + name
}

// EnsurePersonaBead creates or updates a persona bead from file content.
// Creates with a deterministic ID if missing; updates description+content if
// hash has changed or force is true. The real content hash is always stored.
// Returns (beadID, updated bool, error).
func EnsurePersonaBead(b *Beads, prefix, rig, name, content, hash string, force bool) (string, bool, error) {
	id := PersonaBeadID(prefix, rig, name)

	source := "rig"
	if rig == "" {
		source = "town"
	}

	// Ensure the persona type is registered in the target database before
	// any create/update. Mirrors what CreateAgentBead does for agent beads.
	targetDir := ResolveRoutingTarget(b.getTownRoot(), id, b.getResolvedBeadsDir())
	if err := EnsureCustomTypes(targetDir); err != nil {
		return "", false, fmt.Errorf("preparing database for persona bead %s: %w", id, err)
	}

	// Always store the real content hash in the bead description so subsequent
	// regular syncs are idempotent.
	desc := formatPersonaDescription(name, hash, source, content)

	existing, err := b.Show(id)
	if err != nil {
		// bd show exits with code 1 when a bead ID is not found, rather than
		// returning a 0-exit empty JSON array that would produce ErrNotFound.
		// Treat any Show error as "not found" — CreateWithID will surface any
		// genuine database failures.
		existing = nil
	}

	if existing == nil {
		// Create new persona bead
		_, createErr := b.CreateWithID(id, CreateOptions{
			Title:       name,
			Type:        "persona",
			Description: desc,
		})
		if createErr != nil {
			return "", false, fmt.Errorf("creating persona bead %s: %w", id, createErr)
		}
		return id, true, nil
	}

	// Check if hash has changed (skip when force=true)
	existingHash := parsePersonaHash(existing.Description)
	if !force && existingHash == hash {
		return id, false, nil
	}

	// Hash changed (or force) — update
	if updateErr := b.Update(id, UpdateOptions{Description: &desc}); updateErr != nil {
		return "", false, fmt.Errorf("updating persona bead %s: %w", id, updateErr)
	}
	return id, true, nil
}

// GetPersonaContent fetches persona content from a persona bead's description.
// Returns empty string if bead not found (not an error).
func GetPersonaContent(b *Beads, beadID string) (string, error) {
	issue, err := b.Show(beadID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("fetching persona bead %s: %w", beadID, err)
	}
	return parsePersonaContent(issue.Description), nil
}

// ListPersonaBeads returns all persona beads in the database.
func ListPersonaBeads(b *Beads) ([]PersonaBeadInfo, error) {
	issues, err := b.List(ListOptions{Label: "gt:persona"})
	if err != nil {
		return nil, fmt.Errorf("listing persona beads: %w", err)
	}

	result := make([]PersonaBeadInfo, 0, len(issues))
	for _, issue := range issues {
		result = append(result, PersonaBeadInfo{
			ID:     issue.ID,
			Name:   issue.Title,
			Source: parsePersonaSource(issue.Description),
			Hash:   parsePersonaHash(issue.Description),
		})
	}
	return result, nil
}

// formatPersonaDescription creates the description string for a persona bead.
// Metadata section (parseable), followed by ---, followed by full content.
func formatPersonaDescription(name, hash, source, content string) string {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("\n\n")
	sb.WriteString("managed_by: gt crew persona sync\n")
	sb.WriteString("hash: sha256:")
	sb.WriteString(hash)
	sb.WriteString("\n")
	sb.WriteString("source: .personas/")
	sb.WriteString(name)
	sb.WriteString(".md\n")
	sb.WriteString("scope: ")
	sb.WriteString(source)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString(content)
	return sb.String()
}

// parsePersonaHash extracts the hash value from a persona bead description.
func parsePersonaHash(description string) string {
	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "hash: sha256:") {
			return strings.TrimPrefix(line, "hash: sha256:")
		}
	}
	return ""
}

// parsePersonaSource extracts the scope value from a persona bead description.
func parsePersonaSource(description string) string {
	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "scope: ") {
			return strings.TrimPrefix(line, "scope: ")
		}
	}
	return ""
}

// parsePersonaContent extracts the persona content section from description.
// Content follows the "---" separator line.
func parsePersonaContent(description string) string {
	idx := strings.Index(description, "\n---\n\n")
	if idx == -1 {
		return ""
	}
	return description[idx+6:] // skip "\n---\n\n"
}
