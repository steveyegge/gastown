// Package beads provides beads operations interfaces for testability.
package beads

// BeadInfo represents basic bead information for queue operations.
type BeadInfo struct {
	ID     string
	Title  string
	Status string
	Labels []string
}

// BeadsOps defines the interface for beads operations needed by the queue system.
// This interface abstracts access to beads across multiple rigs in a town,
// enabling both real implementations (using the bd CLI) and fake implementations
// for testing.
type BeadsOps interface {
	// IsTownLevelBead returns true if the bead is a town-level bead (hq-* prefix).
	// Town-level beads cannot be dispatched to polecats since polecats are rig-local.
	IsTownLevelBead(beadID string) bool

	// GetRigForBead returns the rig name for a given bead ID based on its prefix.
	// Returns empty string if the prefix is not routed.
	GetRigForBead(beadID string) string

	// LabelAdd adds a label to a bead.
	LabelAdd(beadID, label string) error

	// LabelRemove removes a label from a bead.
	LabelRemove(beadID, label string) error

	// ListReadyByLabel returns all READY beads with the given label across all rigs.
	// Ready means: status=open AND no open blockers (dependencies are satisfied).
	// This is the correct method for queue dispatch - blocked beads are excluded.
	// The result is a map of rig name to slice of beads.
	ListReadyByLabel(label string) (map[string][]BeadInfo, error)
}
