// Package beads provides a wrapper for the bd (beads) CLI.
package beads

import (
	"fmt"
	"strings"
)

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

// BootBeadIDTown returns the Boot agent bead ID for town-level beads.
// Boot is the Deacon's watchdog, stored at hq-boot.
func BootBeadIDTown() string {
	return TownBeadsPrefix + "-boot"
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

// ===== Town-level rig agent bead ID helpers (hq- prefix) =====
//
// These functions generate agent bead IDs for rig-level agents (witness, refinery, crew, polecats)
// but stored in town beads with the hq- prefix. This fixes issue loc-1augh where agent beads
// created with rig prefixes (gt-, loc-) fail when the beads database expects a different prefix.
//
// By storing all agent beads in town beads with hq- prefix:
// - All agent beads use consistent prefix and location
// - No prefix/database mismatch issues
// - Agent beads are coordination artifacts accessible to all rigs
//
// Naming convention: hq-<rig>-<role>[-<name>]
// Examples:
//   - hq-gastown-witness (rig-level singleton)
//   - hq-gastown-refinery (rig-level singleton)
//   - hq-gastown-crew-max (rig-level named agent)
//   - hq-gastown-polecat-Toast (rig-level named agent)

// WitnessBeadIDTown returns a Witness agent bead ID for town-level storage.
// Uses hq- prefix to avoid prefix/database mismatch issues.
func WitnessBeadIDTown(rig string) string {
	return fmt.Sprintf("%s-%s-witness", TownBeadsPrefix, rig)
}

// RefineryBeadIDTown returns a Refinery agent bead ID for town-level storage.
// Uses hq- prefix to avoid prefix/database mismatch issues.
func RefineryBeadIDTown(rig string) string {
	return fmt.Sprintf("%s-%s-refinery", TownBeadsPrefix, rig)
}

// CrewBeadIDTown returns a Crew worker agent bead ID for town-level storage.
// Uses hq- prefix to avoid prefix/database mismatch issues.
func CrewBeadIDTown(rig, name string) string {
	return fmt.Sprintf("%s-%s-crew-%s", TownBeadsPrefix, rig, name)
}

// PolecatBeadIDTown returns a Polecat agent bead ID for town-level storage.
// Uses hq- prefix to avoid prefix/database mismatch issues.
// This is the recommended function for creating polecat agent beads.
func PolecatBeadIDTown(rig, name string) string {
	return fmt.Sprintf("%s-%s-polecat-%s", TownBeadsPrefix, rig, name)
}

// ===== Legacy rig-level agent bead ID helpers (gt- prefix) =====
// NOTE: These functions generate IDs with rig prefixes and are kept for backward compatibility.
// New code should use the *Town() variants above which store agent beads in town beads.

// Agent bead ID naming convention:
//   prefix-rig-role-name
//
// Examples:
//   - gt-mayor (town-level, no rig)
//   - gt-deacon (town-level, no rig)
//   - gt-gastown-witness (rig-level singleton)
//   - gt-gastown-refinery (rig-level singleton)
//   - gt-gastown-crew-max (rig-level named agent)
//   - gt-gastown-polecat-Toast (rig-level named agent)

// AgentBeadIDWithPrefix generates an agent bead ID using the specified prefix.
// The prefix should NOT include the hyphen (e.g., "gt", "bd", not "gt-", "bd-").
// For town-level agents (mayor, deacon), pass empty rig and name.
// For rig-level singletons (witness, refinery), pass empty name.
// For named agents (crew, polecat), pass all three.
// When rig is empty but name is provided (prefix == rig scenario), generates: prefix-role-name
func AgentBeadIDWithPrefix(prefix, rig, role, name string) string {
	if rig == "" {
		// No rig in ID: prefix-role or prefix-role-name
		// This handles town-level agents AND rigs where prefix == rig name
		if name == "" {
			return prefix + "-" + role
		}
		return prefix + "-" + role + "-" + name
	}
	if name == "" {
		// Rig-level singleton: prefix-rig-witness, prefix-rig-refinery
		return prefix + "-" + rig + "-" + role
	}
	// Rig-level named agent: prefix-rig-role-name
	return prefix + "-" + rig + "-" + role + "-" + name
}

// AgentBeadID generates the canonical agent bead ID using "gt" prefix.
// For non-gastown rigs, use AgentBeadIDWithPrefix with the rig's configured prefix.
func AgentBeadID(rig, role, name string) string {
	return AgentBeadIDWithPrefix("gt", rig, role, name)
}

// MayorBeadID returns the Mayor agent bead ID.
//
// Deprecated: Use MayorBeadIDTown() for town-level beads (hq- prefix).
// This function returns "gt-mayor" which is for rig-level storage.
// Town-level agents like Mayor should use the hq- prefix.
func MayorBeadID() string {
	return "gt-mayor"
}

// DeaconBeadID returns the Deacon agent bead ID.
//
// Deprecated: Use DeaconBeadIDTown() for town-level beads (hq- prefix).
// This function returns "gt-deacon" which is for rig-level storage.
// Town-level agents like Deacon should use the hq- prefix.
func DeaconBeadID() string {
	return "gt-deacon"
}

// DogBeadID returns a Dog agent bead ID.
// Dogs are town-level agents, so they follow the pattern: gt-dog-<name>
// Deprecated: Use DogBeadIDTown() for town-level beads with hq- prefix.
// Dogs are town-level agents and should use hq-dog-<name>, not gt-dog-<name>.
func DogBeadID(name string) string {
	return "gt-dog-" + name
}

// WitnessBeadIDWithPrefix returns the Witness agent bead ID for a rig using the specified prefix.
// When prefix equals rig name, the rig is omitted to avoid duplication.
func WitnessBeadIDWithPrefix(prefix, rig string) string {
	// Deduplicate: if prefix == rig, omit rig from ID
	if prefix == rig {
		return AgentBeadIDWithPrefix(prefix, "", "witness", "")
	}
	return AgentBeadIDWithPrefix(prefix, rig, "witness", "")
}

// WitnessBeadID returns the Witness agent bead ID for a rig using "gt" prefix.
func WitnessBeadID(rig string) string {
	return WitnessBeadIDWithPrefix("gt", rig)
}

// RefineryBeadIDWithPrefix returns the Refinery agent bead ID for a rig using the specified prefix.
// When prefix equals rig name, the rig is omitted to avoid duplication.
func RefineryBeadIDWithPrefix(prefix, rig string) string {
	// Deduplicate: if prefix == rig, omit rig from ID
	if prefix == rig {
		return AgentBeadIDWithPrefix(prefix, "", "refinery", "")
	}
	return AgentBeadIDWithPrefix(prefix, rig, "refinery", "")
}

// RefineryBeadID returns the Refinery agent bead ID for a rig using "gt" prefix.
func RefineryBeadID(rig string) string {
	return RefineryBeadIDWithPrefix("gt", rig)
}

// CrewBeadIDWithPrefix returns a Crew worker agent bead ID using the specified prefix.
// When prefix equals rig name, the rig is omitted to avoid duplication.
func CrewBeadIDWithPrefix(prefix, rig, name string) string {
	// Deduplicate: if prefix == rig, omit rig from ID
	if prefix == rig {
		return AgentBeadIDWithPrefix(prefix, "", "crew", name)
	}
	return AgentBeadIDWithPrefix(prefix, rig, "crew", name)
}

// CrewBeadID returns a Crew worker agent bead ID using "gt" prefix.
func CrewBeadID(rig, name string) string {
	return CrewBeadIDWithPrefix("gt", rig, name)
}

// PolecatBeadIDWithPrefix returns a Polecat agent bead ID using the specified prefix.
// When prefix equals rig name (e.g., fhc rig with fhc prefix), the rig is omitted
// to avoid duplication like "fhc-fhc-polecat-name". Instead produces "fhc-polecat-name".
func PolecatBeadIDWithPrefix(prefix, rig, name string) string {
	// Deduplicate: if prefix == rig, omit rig from ID
	if prefix == rig {
		return AgentBeadIDWithPrefix(prefix, "", "polecat", name)
	}
	return AgentBeadIDWithPrefix(prefix, rig, "polecat", name)
}

// PolecatBeadID returns a Polecat agent bead ID using "gt" prefix.
func PolecatBeadID(rig, name string) string {
	return PolecatBeadIDWithPrefix("gt", rig, name)
}

// ParseAgentBeadID parses an agent bead ID into its components.
// Returns rig, role, name, and whether parsing succeeded.
// For town-level agents, rig will be empty.
// For singletons, name will be empty.
// Accepts any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
func ParseAgentBeadID(id string) (rig, role, name string, ok bool) {
	// Find the prefix (everything before the first hyphen)
	// Valid prefixes are 2-3 characters (e.g., "gt", "bd", "hq")
	hyphenIdx := strings.Index(id, "-")
	if hyphenIdx < 2 || hyphenIdx > 3 {
		return "", "", "", false
	}

	rest := id[hyphenIdx+1:]
	parts := strings.Split(rest, "-")

	switch len(parts) {
	case 1:
		// Town-level: gt-mayor, bd-deacon
		return "", parts[0], "", true
	case 2:
		// Could be rig-level singleton (gt-gastown-witness) or
		// town-level named (gt-dog-alpha for dogs)
		if parts[0] == "dog" {
			// Dogs are town-level named agents: gt-dog-<name>
			return "", "dog", parts[1], true
		}
		// Rig-level singleton: gt-gastown-witness
		return parts[0], parts[1], "", true
	case 3:
		// Rig-level named: gt-gastown-crew-max, bd-beads-polecat-pearl
		return parts[0], parts[1], parts[2], true
	default:
		// Handle names with hyphens: gt-gastown-polecat-my-agent-name
		// or gt-dog-my-agent-name
		if len(parts) >= 3 {
			if parts[0] == "dog" {
				// Dog with hyphenated name: gt-dog-my-dog-name
				return "", "dog", strings.Join(parts[1:], "-"), true
			}
			return parts[0], parts[1], strings.Join(parts[2:], "-"), true
		}
		return "", "", "", false
	}
}

// IsAgentSessionBead returns true if the bead ID represents an agent session molecule.
// Agent session beads follow patterns like gt-mayor, bd-beads-witness, gt-gastown-crew-joe.
// Supports any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
// These are used to track agent state and update frequently, which can create noise.
func IsAgentSessionBead(beadID string) bool {
	_, role, _, ok := ParseAgentBeadID(beadID)
	if !ok {
		return false
	}
	// Known agent roles
	switch role {
	case "mayor", "deacon", "witness", "refinery", "crew", "polecat", "dog":
		return true
	default:
		return false
	}
}
