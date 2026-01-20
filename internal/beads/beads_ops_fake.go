package beads

import (
	"fmt"
	"strings"
)

// FakeBeadsOps implements BeadsOps for testing.
type FakeBeadsOps struct {
	rigName string
	beads   map[string]*fakeBead // beadID -> bead
	routes  map[string]routeInfo // prefix -> route info
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
		rigName: rigName,
		beads:   make(map[string]*fakeBead),
		routes:  make(map[string]routeInfo),
	}
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
func (f *FakeBeadsOps) LabelAdd(beadID, label string) error {
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
func (f *FakeBeadsOps) LabelRemove(beadID, label string) error {
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

// ListByLabelAllRigs returns all beads with the given label across all rigs.
func (f *FakeBeadsOps) ListByLabelAllRigs(label string) (map[string][]BeadInfo, error) {
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
			if bead.status != "open" {
				continue
			}
			for _, l := range bead.labels {
				if l == label {
					beads = append(beads, BeadInfo{
						ID:     bead.id,
						Status: bead.status,
						Labels: bead.labels,
					})
					break
				}
			}
		}

		if len(beads) > 0 {
			result[rigName] = beads
		}
	}

	return result, nil
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
