package crew

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
)

// SyncPersonasFromFiles scans .personas/ directories and creates or updates
// persona beads. Only updates a bead when the file content hash has changed
// unless forceUpdate is true. Returns the names of personas created or updated.
func SyncPersonasFromFiles(
	townRoot, rigPath, prefix, rig string,
	b *beads.Beads,
	forceUpdate bool,
) ([]string, error) {
	var updated []string

	// Scan rig-level .personas/
	rigPersonas, err := readPersonaDir(filepath.Join(rigPath, ".personas"))
	if err != nil {
		return nil, fmt.Errorf("scanning rig .personas: %w", err)
	}
	for _, name := range rigPersonas {
		content, _, readErr := ResolvePersonaFile(townRoot, rigPath, name)
		if readErr != nil {
			return nil, fmt.Errorf("reading rig persona %s: %w", name, readErr)
		}
		hash := ContentHash(content)
		_, changed, syncErr := beads.EnsurePersonaBead(b, prefix, rig, name, content, hash, forceUpdate)
		if syncErr != nil {
			return nil, fmt.Errorf("syncing rig persona %s: %w", name, syncErr)
		}
		if changed {
			updated = append(updated, name)
		}
	}

	// Scan town-level .personas/ (only personas not already in rig)
	rigSet := make(map[string]bool, len(rigPersonas))
	for _, n := range rigPersonas {
		rigSet[n] = true
	}

	townPersonas, err := readPersonaDir(filepath.Join(townRoot, ".personas"))
	if err != nil {
		return nil, fmt.Errorf("scanning town .personas: %w", err)
	}
	for _, name := range townPersonas {
		if rigSet[name] {
			continue // rig-level already handled above
		}
		content, _, readErr := ResolvePersonaFile(townRoot, rigPath, name)
		if readErr != nil {
			return nil, fmt.Errorf("reading town persona %s: %w", name, readErr)
		}
		hash := ContentHash(content)
		// Town-level: rig="" in PersonaBeadID
		_, changed, syncErr := beads.EnsurePersonaBead(b, prefix, "", name, content, hash, forceUpdate)
		if syncErr != nil {
			return nil, fmt.Errorf("syncing town persona %s: %w", name, syncErr)
		}
		if changed {
			updated = append(updated, name)
		}
	}

	return updated, nil
}

// ContentHash returns the hex-encoded SHA-256 digest of content.
func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

// EnsurePersonaBeadExists verifies or creates a persona bead for the given name.
// Resolution order:
//  1. Rig-level bead exists → return its ID
//  2. Town-level bead exists → return its ID
//  3. Rig-level .personas/<name>.md exists → sync it, return new bead ID
//  4. Town-level .personas/<name>.md exists → sync it, return new bead ID
//  5. Neither → return error with actionable message
func EnsurePersonaBeadExists(
	townRoot, rigPath, prefix, rig, name string,
	b *beads.Beads,
) (string, error) {
	rigBeadID := beads.PersonaBeadID(prefix, rig, name)
	townBeadID := beads.PersonaBeadID(prefix, "", name)

	// 1. Check rig-level bead
	if _, err := b.Show(rigBeadID); err == nil {
		return rigBeadID, nil
	}

	// 2. Check town-level bead
	if _, err := b.Show(townBeadID); err == nil {
		return townBeadID, nil
	}

	// 3. Try rig-level .personas/<name>.md
	if content, _, err := ResolvePersonaFile(townRoot, rigPath, name); err == nil && content != "" {
		hash := ContentHash(content)
		id, _, syncErr := beads.EnsurePersonaBead(b, prefix, rig, name, content, hash, false)
		if syncErr != nil {
			return "", fmt.Errorf("bootstrapping persona %q from rig file: %w", name, syncErr)
		}
		return id, nil
	}

	// 4. Try town-level .personas/<name>.md directly (skip rig lookup)
	townFile := filepath.Join(townRoot, ".personas", name+".md")
	if townContent, err := readFileIfExists(townFile); err == nil && townContent != "" {
		hash := ContentHash(townContent)
		id, _, syncErr := beads.EnsurePersonaBead(b, prefix, "", name, townContent, hash, false)
		if syncErr != nil {
			return "", fmt.Errorf("bootstrapping persona %q from town file: %w", name, syncErr)
		}
		return id, nil
	}

	// 5. Nothing found
	return "", fmt.Errorf(
		"persona %q not found: create .personas/%s.md in your rig or town directory, then run `gt crew persona sync`",
		name, name,
	)
}

// readFileIfExists reads a file, returning ("", nil) if it does not exist.
func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
