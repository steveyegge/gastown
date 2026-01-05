// Package beads provides a wrapper for the bd (beads) CLI.
package beads

import "fmt"

// TownBeadsPrefix is the prefix used for town-level agent beads stored in ~/gt/.beads/.
// This distinguishes them from rig-level beads (which use project prefixes like "gt-").
const TownBeadsPrefix = "hq"

// Town-level agent bead IDs use the "hq-" prefix and are stored in town beads.
// These are global agents that operate at the town level (mayor, deacon, dogs).
//
// The naming convention is:
//   - hq-<role>       for singletons (mayor, deacon)
//   - hq-dog-<name>   for named agents (dogs)
//   - hq-<role>-role  for role definition beads

// MayorBeadIDTown returns the Mayor agent bead ID for town-level beads.
// This uses the "hq-" prefix for town-level storage.
func MayorBeadIDTown() string {
	return TownBeadsPrefix + "-mayor"
}

// DeaconBeadIDTown returns the Deacon agent bead ID for town-level beads.
// This uses the "hq-" prefix for town-level storage.
func DeaconBeadIDTown() string {
	return TownBeadsPrefix + "-deacon"
}

// DogBeadIDTown returns a Dog agent bead ID for town-level beads.
// Dogs are town-level agents, so they follow the pattern: hq-dog-<name>
func DogBeadIDTown(name string) string {
	return fmt.Sprintf("%s-dog-%s", TownBeadsPrefix, name)
}

// RoleBeadIDTown returns the role bead ID for town-level storage.
// Role beads define lifecycle configuration for each agent type.
// Uses "hq-" prefix for town-level storage: hq-<role>-role
func RoleBeadIDTown(role string) string {
	return fmt.Sprintf("%s-%s-role", TownBeadsPrefix, role)
}

// MayorRoleBeadIDTown returns the Mayor role bead ID for town-level storage.
func MayorRoleBeadIDTown() string {
	return RoleBeadIDTown("mayor")
}

// DeaconRoleBeadIDTown returns the Deacon role bead ID for town-level storage.
func DeaconRoleBeadIDTown() string {
	return RoleBeadIDTown("deacon")
}

// DogRoleBeadIDTown returns the Dog role bead ID for town-level storage.
func DogRoleBeadIDTown() string {
	return RoleBeadIDTown("dog")
}

// WitnessRoleBeadIDTown returns the Witness role bead ID for town-level storage.
func WitnessRoleBeadIDTown() string {
	return RoleBeadIDTown("witness")
}

// RefineryRoleBeadIDTown returns the Refinery role bead ID for town-level storage.
func RefineryRoleBeadIDTown() string {
	return RoleBeadIDTown("refinery")
}

// PolecatRoleBeadIDTown returns the Polecat role bead ID for town-level storage.
func PolecatRoleBeadIDTown() string {
	return RoleBeadIDTown("polecat")
}

// CrewRoleBeadIDTown returns the Crew role bead ID for town-level storage.
func CrewRoleBeadIDTown() string {
	return RoleBeadIDTown("crew")
}
