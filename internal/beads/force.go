package beads

import "strings"

// NeedsForceForID returns true when a bead ID uses multiple hyphens.
// Newer bd versions infer the prefix from the last hyphen, which breaks
// system bead IDs like "st-stockdrop-polecat-nux" and "hq-cv-abc".
// Those IDs are valid in Gas Town, so we pass --force when creating them.
func NeedsForceForID(id string) bool {
	return strings.Count(id, "-") > 1
}
