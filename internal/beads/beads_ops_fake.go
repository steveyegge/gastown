package beads

import (
	"fmt"
	"strings"
)

// FakeBeadsOps implements BeadsOps for testing.
type FakeBeadsOps struct {
	rigName      string
	beads        map[string]*fakeBead  // beadID -> bead
	routes       map[string]routeInfo  // prefix -> route info
	dependencies map[string][]string   // beadID -> list of beads it depends on (blockers)
}

type fakeBead struct {
	id     string
	status string
	labels []string
}

type routeInfo struct {
	rigName string
	ops     BeadsOps // The BeadsOps for this rig (may be self or another FakeBeadsOps)
}

// NewFakeBeadsOpsForRig creates a new FakeBeadsOps for testing.
func NewFakeBeadsOpsForRig(rigName string) *FakeBeadsOps {
	return &FakeBeadsOps{
		rigName:      rigName,
		beads:        make(map[string]*fakeBead),
		routes:       make(map[string]routeInfo),
		dependencies: make(map[string][]string),
	}
}

// AddDependency adds a dependency: beadID is blocked by blockedBy.
// The bead won't be considered "ready" until blockedBy is closed.
func (f *FakeBeadsOps) AddDependency(beadID, blockedBy string) {
	f.dependencies[beadID] = append(f.dependencies[beadID], blockedBy)
}

// AddRouteWithRig adds a route mapping from prefix to rig.
// The ops parameter allows routing to another FakeBeadsOps for cross-rig testing.
func (f *FakeBeadsOps) AddRouteWithRig(prefix, rigName string, ops BeadsOps) {
	f.routes[prefix] = routeInfo{
		rigName: rigName,
		ops:     ops,
	}
}

// AddBead adds a bead to the fake store.
func (f *FakeBeadsOps) AddBead(beadID, status string, labels []string) {
	f.beads[beadID] = &fakeBead{
		id:     beadID,
		status: status,
		labels: labels,
	}
}

// GetLabels returns the labels for a bead (for test assertions).
func (f *FakeBeadsOps) GetLabels(beadID string) []string {
	if bead, ok := f.beads[beadID]; ok {
		return bead.labels
	}
	return nil
}

// IsTownLevelBead returns true if the bead is a town-level bead (hq-* prefix).
func (f *FakeBeadsOps) IsTownLevelBead(beadID string) bool {
	return strings.HasPrefix(beadID, "hq-")
}

// GetRigForBead returns the rig name for a given bead ID based on its prefix.
func (f *FakeBeadsOps) GetRigForBead(beadID string) string {
	prefix := extractPrefix(beadID)
	if prefix == "" {
		return ""
	}

	if route, ok := f.routes[prefix]; ok {
		return route.rigName
	}
	return ""
}

// LabelAdd adds a label to a bead.
// Routes to the correct FakeBeadsOps based on the bead's prefix.
func (f *FakeBeadsOps) LabelAdd(beadID, label string) error {
	// Route to the correct ops based on prefix
	if targetOps := f.getOpsForBead(beadID); targetOps != nil && targetOps != f {
		return targetOps.LabelAdd(beadID, label)
	}

	bead, ok := f.beads[beadID]
	if !ok {
		return fmt.Errorf("bead %s not found", beadID)
	}

	// Check if label already exists
	for _, l := range bead.labels {
		if l == label {
			return nil // Already has label
		}
	}

	bead.labels = append(bead.labels, label)
	return nil
}

// LabelRemove removes a label from a bead.
// Routes to the correct FakeBeadsOps based on the bead's prefix.
func (f *FakeBeadsOps) LabelRemove(beadID, label string) error {
	// Route to the correct ops based on prefix
	if targetOps := f.getOpsForBead(beadID); targetOps != nil && targetOps != f {
		return targetOps.LabelRemove(beadID, label)
	}

	bead, ok := f.beads[beadID]
	if !ok {
		return fmt.Errorf("bead %s not found", beadID)
	}

	newLabels := []string{}
	for _, l := range bead.labels {
		if l != label {
			newLabels = append(newLabels, l)
		}
	}
	bead.labels = newLabels
	return nil
}

// getOpsForBead returns the BeadsOps that handles the given bead based on its prefix.
// Returns nil if no route is configured for the prefix.
func (f *FakeBeadsOps) getOpsForBead(beadID string) BeadsOps {
	prefix := extractPrefix(beadID)
	if prefix == "" {
		return nil
	}
	if route, ok := f.routes[prefix]; ok {
		return route.ops
	}
	return nil
}

// ListReadyByLabel returns all READY beads with the given label across all rigs.
// Ready means: status=open AND no open blockers.
func (f *FakeBeadsOps) ListReadyByLabel(label string) (map[string][]BeadInfo, error) {
	result := make(map[string][]BeadInfo)

	// Collect all unique ops from routes
	seen := make(map[BeadsOps]string) // ops -> rigName
	for _, route := range f.routes {
		if _, ok := seen[route.ops]; !ok {
			seen[route.ops] = route.rigName
		}
	}

	// Query each rig's ops
	for ops, rigName := range seen {
		// Cast to FakeBeadsOps to access internal beads map
		fakeOps, ok := ops.(*FakeBeadsOps)
		if !ok {
			continue
		}

		var beads []BeadInfo
		for _, bead := range fakeOps.beads {
			// Must be open
			if bead.status != "open" {
				continue
			}
			// Must have the label
			hasLabel := false
			for _, l := range bead.labels {
				if l == label {
					hasLabel = true
					break
				}
			}
			if !hasLabel {
				continue
			}
			// Must not be blocked (all blockers must be closed)
			if fakeOps.isBlocked(bead.id) {
				continue
			}
			beads = append(beads, BeadInfo{
				ID:     bead.id,
				Status: bead.status,
				Labels: bead.labels,
			})
		}

		if len(beads) > 0 {
			result[rigName] = beads
		}
	}

	return result, nil
}

// isBlocked returns true if the bead has any open blockers.
func (f *FakeBeadsOps) isBlocked(beadID string) bool {
	blockers, ok := f.dependencies[beadID]
	if !ok {
		return false // No dependencies
	}
	for _, blockerID := range blockers {
		blocker, exists := f.beads[blockerID]
		if !exists {
			// Blocker doesn't exist in this rig - check other rigs via routes
			if f.isBlockerOpenInAnyRig(blockerID) {
				return true
			}
			continue
		}
		if blocker.status != "closed" {
			return true // Has an open blocker
		}
	}
	return false
}

// isBlockerOpenInAnyRig checks if a blocker is open in any routed rig.
func (f *FakeBeadsOps) isBlockerOpenInAnyRig(blockerID string) bool {
	for _, route := range f.routes {
		fakeOps, ok := route.ops.(*FakeBeadsOps)
		if !ok || fakeOps == f {
			continue
		}
		if blocker, exists := fakeOps.beads[blockerID]; exists {
			if blocker.status != "closed" {
				return true
			}
		}
	}
	return false
}

// extractPrefix extracts the prefix from a bead ID (e.g., "gt-abc" -> "gt-").
func extractPrefix(beadID string) string {
	if beadID == "" {
		return ""
	}
	idx := strings.Index(beadID, "-")
	if idx <= 0 {
		return ""
	}
	return beadID[:idx+1]
}
