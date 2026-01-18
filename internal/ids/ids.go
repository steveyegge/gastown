// Package ids provides shared identity types used across agent and session layers.
package ids

import (
	"fmt"
	"strings"
)

// AgentID represents a parsed Gas Town agent identity.
type AgentID struct {
	Role   string // mayor, deacon, witness, refinery, crew, polecat, boot
	Rig    string // rig name (empty for mayor/deacon/boot)
	Worker string // crew/polecat name (empty for singletons)
}

// String returns the mail-style address for this identity.
// Examples:
//   - {Role: "mayor"} → "mayor"
//   - {Role: "witness", Rig: "myrig"} → "myrig/witness"
//   - {Role: "polecat", Rig: "myrig", Worker: "Toast"} → "myrig/polecat/Toast"
func (id AgentID) String() string {
	if id.Rig == "" {
		// Town-level singleton
		return id.Role
	}
	if id.Worker == "" {
		// Rig-level singleton
		return fmt.Sprintf("%s/%s", id.Rig, id.Role)
	}
	// Named worker
	return fmt.Sprintf("%s/%s/%s", id.Rig, id.Role, id.Worker)
}

// Parse extracts role, rig, and worker from the AgentID.
// This is a convenience method that just returns the struct fields.
//
// Note: Parse() does not validate the AgentID. An empty or malformed AgentID
// will return empty strings without error. Use this method only when the
// AgentID is known to be valid (e.g., from constructor functions or after
// successful parsing). For user input validation, check the Role field
// is non-empty and matches a known role.
func (id AgentID) Parse() (role, rig, worker string) {
	return id.Role, id.Rig, id.Worker
}

// --- Singleton addresses ---

var (
	// MayorAddress is the address for the mayor agent.
	MayorAddress = AgentID{Role: "mayor"}
	// DeaconAddress is the address for the deacon agent.
	DeaconAddress = AgentID{Role: "deacon"}
	// BootAddress is the address for the boot watchdog agent.
	BootAddress = AgentID{Role: "boot"}
)

// WitnessAddress returns the address for a rig's witness.
func WitnessAddress(rig string) AgentID {
	return AgentID{Role: "witness", Rig: rig}
}

// RefineryAddress returns the address for a rig's refinery.
func RefineryAddress(rig string) AgentID {
	return AgentID{Role: "refinery", Rig: rig}
}

// PolecatAddress returns the address for a polecat.
func PolecatAddress(rig, name string) AgentID {
	return AgentID{Role: "polecat", Rig: rig, Worker: name}
}

// CrewAddress returns the address for a crew member.
func CrewAddress(rig, name string) AgentID {
	return AgentID{Role: "crew", Rig: rig, Worker: name}
}

// ParseAddress parses an address string into an AgentID.
// Address formats:
//   - "mayor" → {Role: "mayor"}
//   - "rig/witness" → {Role: "witness", Rig: "rig"}
//   - "rig/polecat/name" → {Role: "polecat", Rig: "rig", Worker: "name"}
//
// Returns an empty AgentID for invalid formats (more than 3 parts).
// Note: This function does not validate that the parsed role is a known role;
// it simply extracts parts from the address string.
func ParseAddress(addr string) AgentID {
	parts := strings.Split(addr, "/")
	switch len(parts) {
	case 1:
		return AgentID{Role: parts[0]}
	case 2:
		return AgentID{Role: parts[1], Rig: parts[0]}
	case 3:
		return AgentID{Role: parts[1], Rig: parts[0], Worker: parts[2]}
	default:
		return AgentID{} // Invalid format
	}
}

// ParseSessionName parses a tmux session name into an AgentID.
// Session name formats:
//   - "gt-mayor" → {Role: "mayor"}
//   - "gt-deacon" → {Role: "deacon"}
//   - "hq-mayor" → {Role: "mayor"} (town-level prefix variant)
//   - "hq-deacon" → {Role: "deacon"} (town-level prefix variant)
//   - "gt-rig-witness" → {Role: "witness", Rig: "rig"}
//   - "gt-rig-refinery" → {Role: "refinery", Rig: "rig"}
//   - "gt-rig-polecat-name" → {Role: "polecat", Rig: "rig", Worker: "name"}
//   - "gt-rig-crew-name" → {Role: "crew", Rig: "rig", Worker: "name"}
//   - "gt-rig-name" → {Role: "polecat", Rig: "rig", Worker: "name"} (legacy polecat)
//
// Returns empty AgentID if format is invalid.
func ParseSessionName(name string) AgentID {
	var rest string

	// Handle both gt- and hq- prefixes
	if strings.HasPrefix(name, "gt-") {
		rest = strings.TrimPrefix(name, "gt-")
	} else if strings.HasPrefix(name, "hq-") {
		rest = strings.TrimPrefix(name, "hq-")
	} else {
		return AgentID{}
	}

	parts := strings.Split(rest, "-")

	if len(parts) < 1 {
		return AgentID{}
	}

	// Check for singletons: gt-mayor, gt-deacon, gt-boot, hq-mayor, hq-deacon
	if len(parts) == 1 {
		switch parts[0] {
		case "mayor", "deacon", "boot":
			return AgentID{Role: parts[0]}
		}
		return AgentID{} // Invalid singleton
	}

	// rig-role or rig-role-worker format
	rig := parts[0]
	role := parts[1]

	switch role {
	case "witness", "refinery":
		// Rig-level singletons: gt-rig-witness, gt-rig-refinery
		if len(parts) != 2 {
			return AgentID{}
		}
		return AgentID{Role: role, Rig: rig}
	case "polecat":
		// Explicit polecat: gt-rig-polecat-name
		if len(parts) < 3 {
			return AgentID{}
		}
		// Worker name may contain dashes, so rejoin
		worker := strings.Join(parts[2:], "-")
		return AgentID{Role: "polecat", Rig: rig, Worker: worker}
	case "crew":
		// Crew: gt-rig-crew-name
		if len(parts) < 3 {
			return AgentID{}
		}
		// Worker name may contain dashes, so rejoin
		worker := strings.Join(parts[2:], "-")
		return AgentID{Role: "crew", Rig: rig, Worker: worker}
	default:
		// Legacy polecat format: gt-rig-name (no explicit "polecat" role)
		// The "role" position is actually the worker name
		// Worker name may contain dashes, so rejoin everything after rig
		worker := strings.Join(parts[1:], "-")
		return AgentID{Role: "polecat", Rig: rig, Worker: worker}
	}
}
