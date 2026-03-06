// force.go detects bead IDs that require the --force flag when passed to bd.
// IDs with multiple hyphens confuse bd's prefix inference, so callers must
// bypass the check with --force to honor the explicit ID.
package beads

import "strings"

// NeedsForceForID returns true when a bead ID uses multiple hyphens.
// Recent bd versions infer the prefix from the last hyphen, which can cause
// prefix-mismatch errors for valid system IDs like "st-stockdrop-polecat-nux"
// and "hq-cv-abc". We pass --force to honor the explicit ID in those cases.
func NeedsForceForID(id string) bool {
	return strings.Count(id, "-") > 1
}
