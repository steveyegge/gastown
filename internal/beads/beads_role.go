// Package beads provides role bead management.
package beads

import (
	"errors"
	"fmt"
)

// Role bead ID naming convention:
// Role beads are stored in town beads (~/.beads/) with hq- prefix.
//
// Canonical format: hq-<role>-role
//
// Examples:
//   - hq-mayor-role
//   - hq-deacon-role
//   - hq-witness-role
//   - hq-refinery-role
//   - hq-crew-role
//   - hq-polecat-role
//
// Use RoleBeadIDTown() to get canonical role bead IDs.
// The legacy RoleBeadID() function returns gt-<role>-role for backward compatibility.

// RoleBeadID returns the role bead ID for a given role type.
// Role beads define lifecycle configuration for each agent type.
// Deprecated: Use RoleBeadIDTown() for town-level beads with hq- prefix.
// Role beads are global templates and should use hq-<role>-role, not gt-<role>-role.
func RoleBeadID(roleType string) string {
	return "gt-" + roleType + "-role"
}

// DogRoleBeadID returns the Dog role bead ID.
func DogRoleBeadID() string {
	return RoleBeadID("dog")
}

// MayorRoleBeadID returns the Mayor role bead ID.
func MayorRoleBeadID() string {
	return RoleBeadID("mayor")
}

// DeaconRoleBeadID returns the Deacon role bead ID.
func DeaconRoleBeadID() string {
	return RoleBeadID("deacon")
}

// WitnessRoleBeadID returns the Witness role bead ID.
func WitnessRoleBeadID() string {
	return RoleBeadID("witness")
}

// RefineryRoleBeadID returns the Refinery role bead ID.
func RefineryRoleBeadID() string {
	return RoleBeadID("refinery")
}

// CrewRoleBeadID returns the Crew role bead ID.
func CrewRoleBeadID() string {
	return RoleBeadID("crew")
}

// PolecatRoleBeadID returns the Polecat role bead ID.
func PolecatRoleBeadID() string {
	return RoleBeadID("polecat")
}

// GetRoleConfig looks up a role bead and returns its parsed RoleConfig.
// Returns nil, nil if the role bead doesn't exist or has no config.
func (b *Beads) GetRoleConfig(roleBeadID string) (*RoleConfig, error) {
	issue, err := b.Show(roleBeadID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if !HasLabel(issue, "gt:role") {
		return nil, fmt.Errorf("bead %s is not a role bead (missing gt:role label)", roleBeadID)
	}

	return ParseRoleConfig(issue.Description), nil
}

// HasLabel checks if an issue has a specific label.
func HasLabel(issue *Issue, label string) bool {
	for _, l := range issue.Labels {
		if l == label {
			return true
		}
	}
	return false
}
