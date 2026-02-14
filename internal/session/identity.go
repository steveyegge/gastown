// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"strings"
)

// Role represents the type of Gas Town agent.
type Role string

const (
	RoleMayor    Role = "mayor"
	RoleDeacon   Role = "deacon"
	RoleWitness  Role = "witness"
	RoleRefinery Role = "refinery"
	RoleCrew     Role = "crew"
	RolePolecat  Role = "polecat"
)

// AgentIdentity represents a parsed Gas Town agent identity.
type AgentIdentity struct {
	Role Role   // mayor, deacon, witness, refinery, crew, polecat
	Rig  string // rig name (empty for mayor/deacon)
	Name string // crew/polecat name (empty for mayor/deacon/witness/refinery)
}

// ParseAddress parses a mail-style address into an AgentIdentity.
func ParseAddress(address string) (*AgentIdentity, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("empty address")
	}

	if address == "mayor" || address == "mayor/" {
		return &AgentIdentity{Role: RoleMayor}, nil
	}
	if address == "deacon" || address == "deacon/" {
		return &AgentIdentity{Role: RoleDeacon}, nil
	}
	if address == "overseer" {
		return nil, fmt.Errorf("overseer has no session")
	}

	address = strings.TrimSuffix(address, "/")
	parts := strings.Split(address, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid address %q", address)
	}

	rig := parts[0]
	switch len(parts) {
	case 2:
		name := parts[1]
		switch name {
		case "witness":
			return &AgentIdentity{Role: RoleWitness, Rig: rig}, nil
		case "refinery":
			return &AgentIdentity{Role: RoleRefinery, Rig: rig}, nil
		case "crew", "polecats":
			return nil, fmt.Errorf("invalid address %q", address)
		default:
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name}, nil
		}
	case 3:
		role := parts[1]
		name := parts[2]
		switch role {
		case "crew":
			return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name}, nil
		case "polecats":
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name}, nil
		default:
			return nil, fmt.Errorf("invalid address %q", address)
		}
	default:
		return nil, fmt.Errorf("invalid address %q", address)
	}
}

// ParseSessionName parses a tmux session name into an AgentIdentity.
//
// Session name formats:
//   - <town>-mayor → Role: mayor (town-level, e.g., "gt-mayor", "redos-mayor")
//   - <town>-deacon → Role: deacon (town-level)
//   - <town>-boot → Role: deacon, Name: "boot" (Boot watchdog)
//   - gt-<rig>-witness → Role: witness, Rig: <rig>
//   - gt-<rig>-refinery → Role: refinery, Rig: <rig>
//   - gt-<rig>-crew-<name> → Role: crew, Rig: <rig>, Name: <name>
//   - gt-<rig>-<name> → Role: polecat, Rig: <rig>, Name: <name>
//
// Town-level sessions use the town name as prefix (from town.json).
// Legacy "hq-mayor"/"hq-deacon" format is still supported (town name "hq").
//
// For polecat sessions without a crew marker, the last segment after the rig
// is assumed to be the polecat name. This works for simple rig names but may
// be ambiguous for rig names containing hyphens.
func ParseSessionName(session string) (*AgentIdentity, error) {
	// Rig-level roles use gt- prefix — check this first since it's unambiguous
	if strings.HasPrefix(session, Prefix) {
		suffix := strings.TrimPrefix(session, Prefix)
		if suffix == "" {
			return nil, fmt.Errorf("invalid session name %q: empty after prefix", session)
		}

		// Legacy: gt-boot was the old Boot watchdog name (before town-scoped naming)
		if suffix == "boot" {
			return &AgentIdentity{Role: RoleDeacon, Name: "boot"}, nil
		}

		// Parse into parts for rig-level roles
		parts := strings.Split(suffix, "-")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid session name %q: expected rig-role format", session)
		}

		// Check for witness/refinery (suffix markers)
		if parts[len(parts)-1] == "witness" {
			rig := strings.Join(parts[:len(parts)-1], "-")
			return &AgentIdentity{Role: RoleWitness, Rig: rig}, nil
		}
		if parts[len(parts)-1] == "refinery" {
			rig := strings.Join(parts[:len(parts)-1], "-")
			return &AgentIdentity{Role: RoleRefinery, Rig: rig}, nil
		}

		// Check for crew (marker in middle)
		for i, p := range parts {
			if p == "crew" && i > 0 && i < len(parts)-1 {
				rig := strings.Join(parts[:i], "-")
				name := strings.Join(parts[i+1:], "-")
				return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name}, nil
			}
		}

		// Default to polecat: rig is everything except the last segment
		rig := strings.Join(parts[:len(parts)-1], "-")
		name := parts[len(parts)-1]
		return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name}, nil
	}

	// Town-level roles: <town>-mayor, <town>-deacon, <town>-boot
	// Matches both new format (redos-mayor) and legacy format (hq-mayor).
	if strings.HasSuffix(session, "-mayor") {
		return &AgentIdentity{Role: RoleMayor}, nil
	}
	if strings.HasSuffix(session, "-deacon") {
		return &AgentIdentity{Role: RoleDeacon}, nil
	}
	if strings.HasSuffix(session, "-boot") {
		return &AgentIdentity{Role: RoleDeacon, Name: "boot"}, nil
	}

	return nil, fmt.Errorf("invalid session name %q: missing %q prefix or unknown town-level role", session, Prefix)
}

// SessionName returns the tmux session name for this identity.
func (a *AgentIdentity) SessionName() string {
	switch a.Role {
	case RoleMayor:
		return MayorSessionName()
	case RoleDeacon:
		return DeaconSessionName()
	case RoleWitness:
		return WitnessSessionName(a.Rig)
	case RoleRefinery:
		return RefinerySessionName(a.Rig)
	case RoleCrew:
		return CrewSessionName(a.Rig, a.Name)
	case RolePolecat:
		return PolecatSessionName(a.Rig, a.Name)
	default:
		return ""
	}
}

// Address returns the mail-style address for this identity.
// Examples:
//   - mayor → "mayor"
//   - deacon → "deacon"
//   - witness → "gastown/witness"
//   - refinery → "gastown/refinery"
//   - crew → "gastown/crew/max"
//   - polecat → "gastown/polecats/Toast"
func (a *AgentIdentity) Address() string {
	switch a.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleWitness:
		return fmt.Sprintf("%s/witness", a.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s/refinery", a.Rig)
	case RoleCrew:
		return fmt.Sprintf("%s/crew/%s", a.Rig, a.Name)
	case RolePolecat:
		return fmt.Sprintf("%s/polecats/%s", a.Rig, a.Name)
	default:
		return ""
	}
}

// GTRole returns the GT_ROLE environment variable format.
// This is the same as Address() for most roles.
func (a *AgentIdentity) GTRole() string {
	return a.Address()
}
