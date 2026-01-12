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

// RoleBeadDef defines a role bead's metadata.
// Used by gt install and gt doctor to create missing role beads.
type RoleBeadDef struct {
	ID    string // e.g., "hq-witness-role"
	Title string // e.g., "Witness Role"
	Desc  string // Description of the role
}

// AllRoleBeadDefs returns all role bead definitions.
// This is the single source of truth for role beads used by both
// gt install (initial creation) and gt doctor --fix (repair).
func AllRoleBeadDefs() []RoleBeadDef {
	return []RoleBeadDef{
		{
			ID:    MayorRoleBeadIDTown(),
			Title: "Mayor Role",
			Desc:  "Role definition for Mayor agents. Global coordinator for cross-rig work.",
		},
		{
			ID:    DeaconRoleBeadIDTown(),
			Title: "Deacon Role",
			Desc:  "Role definition for Deacon agents. Daemon beacon for heartbeats and monitoring.",
		},
		{
			ID:    DogRoleBeadIDTown(),
			Title: "Dog Role",
			Desc:  "Role definition for Dog agents. Town-level workers for cross-rig tasks.",
		},
		{
			ID:    WitnessRoleBeadIDTown(),
			Title: "Witness Role",
			Desc:  "Role definition for Witness agents. Per-rig worker monitor with progressive nudging.",
		},
		{
			ID:    RefineryRoleBeadIDTown(),
			Title: "Refinery Role",
			Desc:  "Role definition for Refinery agents. Merge queue processor with verification gates.",
		},
		{
			ID:    PolecatRoleBeadIDTown(),
			Title: "Polecat Role",
			Desc:  "Role definition for Polecat agents. Ephemeral workers for batch work dispatch.",
		},
		{
			ID:    CrewRoleBeadIDTown(),
			Title: "Crew Role",
			Desc:  "Role definition for Crew agents. Persistent user-managed workspaces.",
		},
	}
}
