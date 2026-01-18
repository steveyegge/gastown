// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/ids"
)

// ParseSessionName parses a tmux session name into an AgentID.
//
// Session name formats:
//   - hq-mayor → {Role: "mayor"}
//   - hq-deacon → {Role: "deacon"}
//   - gt-boot → {Role: "boot"}
//   - gt-<rig>-witness → {Role: "witness", Rig: <rig>}
//   - gt-<rig>-refinery → {Role: "refinery", Rig: <rig>}
//   - gt-<rig>-crew-<name> → {Role: "crew", Rig: <rig>, Worker: <name>}
//   - gt-<rig>-<name> → {Role: "polecat", Rig: <rig>, Worker: <name>}
//
// For polecat sessions without a crew marker, the last segment after the rig
// is assumed to be the polecat name. This works for simple rig names but may
// be ambiguous for rig names containing hyphens.
func ParseSessionName(sessionName string) (ids.AgentID, error) {
	// Check for town-level roles (hq- prefix)
	if strings.HasPrefix(sessionName, HQPrefix) {
		suffix := strings.TrimPrefix(sessionName, HQPrefix)
		switch suffix {
		case "mayor":
			return ids.MayorAddress, nil
		case "deacon":
			return ids.DeaconAddress, nil
		default:
			return ids.AgentID{}, fmt.Errorf("invalid session name %q: unknown hq- role", sessionName)
		}
	}

	// Rig-level roles use gt- prefix
	if !strings.HasPrefix(sessionName, Prefix) {
		return ids.AgentID{}, fmt.Errorf("invalid session name %q: missing %q or %q prefix", sessionName, HQPrefix, Prefix)
	}

	suffix := strings.TrimPrefix(sessionName, Prefix)
	if suffix == "" {
		return ids.AgentID{}, fmt.Errorf("invalid session name %q: empty after prefix", sessionName)
	}

	// Special case: boot
	if suffix == "boot" {
		return ids.BootAddress, nil
	}

	// Parse into parts for rig-level roles
	parts := strings.Split(suffix, "-")
	if len(parts) < 2 {
		return ids.AgentID{}, fmt.Errorf("invalid session name %q: expected rig-role format", sessionName)
	}

	// Check for witness/refinery (suffix markers)
	if parts[len(parts)-1] == "witness" {
		rig := strings.Join(parts[:len(parts)-1], "-")
		return ids.WitnessAddress(rig), nil
	}
	if parts[len(parts)-1] == "refinery" {
		rig := strings.Join(parts[:len(parts)-1], "-")
		return ids.RefineryAddress(rig), nil
	}

	// Check for crew (marker in middle)
	for i, p := range parts {
		if p == "crew" && i > 0 && i < len(parts)-1 {
			rig := strings.Join(parts[:i], "-")
			name := strings.Join(parts[i+1:], "-")
			return ids.CrewAddress(rig, name), nil
		}
	}

	// Default to polecat: rig is everything except the last segment
	if len(parts) < 2 {
		return ids.AgentID{}, fmt.Errorf("invalid session name %q: cannot determine rig/name", sessionName)
	}
	rig := strings.Join(parts[:len(parts)-1], "-")
	name := parts[len(parts)-1]
	return ids.PolecatAddress(rig, name), nil
}
