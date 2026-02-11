// Package beads provides role bead management.
//
// DEPRECATED: Role beads are deprecated. Role definitions are now config-based.
// See internal/config/roles/*.toml and config-based-roles.md for the new system.
//
// This file is kept for backward compatibility with existing role beads but
// new code should use config.LoadRoleDefinition() instead of reading role beads.
// The daemon no longer uses role beads as of Phase 2 (config-based roles).
package beads

import (
	"errors"
	"fmt"
)

// DEPRECATED: Role bead ID naming convention is no longer used.
// Role definitions are now config-based (internal/config/roles/*.toml).
//
// Role beads were stored in town beads (~/.beads/) with hq- prefix.
//
// Canonical format was: hq-<role>-role
//
// Examples:
//   - hq-mayor-role
//   - hq-deacon-role
//   - hq-witness-role
//   - hq-refinery-role
//   - hq-crew-role
//   - hq-polecat-role
//
// Use RoleBeadIDTown() from agent_ids.go for role bead IDs (hq- prefix).

// GetRoleConfig looks up a role bead and returns its parsed RoleConfig.
// Returns nil, nil if the role bead doesn't exist or has no config.
//
// Deprecated: Use config.LoadRoleDefinition() instead. Role definitions
// are now config-based, not stored as beads.
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

// RoleBeadDef defines a role bead's metadata.
//
// Deprecated: Role beads are no longer created. Role definitions are
// now config-based (internal/config/roles/*.toml).
type RoleBeadDef struct {
	ID    string // e.g., "hq-witness-role"
	Title string // e.g., "Witness Role"
	Desc  string // Description of the role
}

// AllRoleBeadDefs returns all role bead definitions.
//
// Deprecated: Role beads are no longer created by gt install or gt doctor.
// This function is kept for backward compatibility only.
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
			Desc:  "Role definition for Polecat agents. Persistent identity, ephemeral sessions. Batch work dispatch.",
		},
		{
			ID:    CrewRoleBeadIDTown(),
			Title: "Crew Role",
			Desc:  "Role definition for Crew agents. Persistent user-managed workspaces.",
		},
	}
}
