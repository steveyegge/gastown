package beads

import "strings"

// NeedsForceForID returns true when a bead ID uses multiple hyphens.
// Recent bd versions infer the prefix from the last hyphen, which can cause
// prefix-mismatch errors for valid system IDs like "st-stockdrop-polecat-nux"
// and "hq-cv-abc". We pass --force to honor the explicit ID in those cases.
func NeedsForceForID(id string) bool {
	return strings.Count(id, "-") > 1
}
